// Package server implements the stateless HTTP proxy: members log in through
// the web UI and get a live, auto-refreshing gym-entry QR code. Tokens live in
// an HttpOnly cookie owned by the browser; the server keeps nothing.
package server

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"vivagym/internal/qr"
	"vivagym/internal/vivagym"
)

const cookieName = "vivagym_tokens"

// Config holds runtime configuration for the server.
type Config struct {
	PublicURL       string
	TrustProxy      bool
	CookieMaxAge    int // seconds
	LoginRatePerMin int
	VivaGymClient   *vivagym.Client
	PublicDir       string // directory of static files to serve at /
}

// Server is the HTTP handler set for the live-QR proxy.
type Server struct {
	cfg          Config
	client       *vivagym.Client
	cookieMaxAge int
	cookieSecure bool
	limiter      *rateLimiter
}

func New(cfg Config) *Server {
	if cfg.PublicURL == "" {
		cfg.PublicURL = "http://localhost:4567"
	}
	if cfg.CookieMaxAge <= 0 {
		cfg.CookieMaxAge = 7 * 86_400
	}
	if cfg.LoginRatePerMin <= 0 {
		cfg.LoginRatePerMin = 10
	}
	return &Server{
		cfg:          cfg,
		client:       cfg.VivaGymClient,
		cookieMaxAge: cfg.CookieMaxAge,
		cookieSecure: strings.HasPrefix(cfg.PublicURL, "https://"),
		limiter:      newRateLimiter(cfg.LoginRatePerMin),
	}
}

// Handler returns the fully-routed HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /auth/session", s.handleSession)
	mux.HandleFunc("POST /auth/login", s.handleLogin)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)
	mux.HandleFunc("GET /qr", s.handleQR)
	mux.HandleFunc("GET /qr.png", s.handleQRPNG)
	mux.HandleFunc("GET /qr.svg", s.handleQRSVG)
	if s.cfg.PublicDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(s.cfg.PublicDir)))
	}
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	t, ok := readTokens(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]bool{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "email": t.Email})
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if forwarded := r.Header.Get("x-forwarded-for"); forwarded != "" {
			parts := strings.Split(forwarded, ",")
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return "unknown"
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	if !s.limiter.allow(ip) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Too many attempts, try again later"})
		return
	}

	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	password := body.Password
	if email == "" || password == "" || len(email) > 254 || len(password) > 256 || !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid email or password"})
		return
	}

	pair, err := s.client.Login(r.Context(), email, password)
	if err != nil {
		log.Printf("login failed for %s - %v", email, err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid credentials"})
		return
	}
	s.limiter.reset(ip)
	s.writeTokens(w, Tokens{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
		IssuedAt:     time.Now().UnixMilli(),
		Email:        email,
	})
	writeJSON(w, http.StatusOK, map[string]string{"email": email})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearTokens(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) isAccessTokenValid(t Tokens) bool {
	if t.AccessToken == "" {
		return false
	}
	// 10s safety margin, mirroring the old TS server.
	return time.Now().UnixMilli() < t.IssuedAt+int64(t.ExpiresIn)*1000-10_000
}

// validTokens returns a usable access token, refreshing (and rotating the
// cookie) if needed.
func (s *Server) validTokens(w http.ResponseWriter, r *http.Request) (Tokens, bool) {
	t, ok := readTokens(r)
	if !ok {
		return Tokens{}, false
	}
	if s.isAccessTokenValid(t) {
		return t, true
	}
	if t.RefreshToken == "" {
		return Tokens{}, false
	}
	fresh, err := s.client.RefreshTokens(r.Context(), t.RefreshToken)
	if err != nil {
		s.clearTokens(w)
		return Tokens{}, false
	}
	t.AccessToken = fresh.AccessToken
	t.RefreshToken = fresh.RefreshToken
	t.ExpiresIn = fresh.ExpiresIn
	t.IssuedAt = time.Now().UnixMilli()
	s.writeTokens(w, t)
	return t, true
}

// qrResult fetches the QR payload, transparently refreshing once if VivaGym
// rejects the token.
func (s *Server) qrResult(w http.ResponseWriter, r *http.Request) (string, int, error) {
	t, ok := s.validTokens(w, r)
	if !ok {
		return "", http.StatusUnauthorized, errors.New("Not authenticated")
	}

	for attempt := 0; ; attempt++ {
		payload, err := s.client.FetchQr(r.Context(), t.AccessToken)
		if err == nil {
			return payload, http.StatusOK, nil
		}
		var vg *vivagym.VivaGymError
		if errors.As(err, &vg) && vg.Status == http.StatusUnauthorized && attempt == 0 && t.RefreshToken != "" {
			fresh, rerr := s.client.RefreshTokens(r.Context(), t.RefreshToken)
			if rerr == nil {
				t.AccessToken = fresh.AccessToken
				t.RefreshToken = fresh.RefreshToken
				t.ExpiresIn = fresh.ExpiresIn
				t.IssuedAt = time.Now().UnixMilli()
				s.writeTokens(w, t)
				continue
			}
		}
		if errors.As(err, &vg) && vg.Status == http.StatusUnauthorized {
			s.clearTokens(w)
			return "", http.StatusUnauthorized, errors.New("Session expired, log in again")
		}
		return "", http.StatusBadGateway, err
	}
}

func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	payload, status, err := s.qrResult(w, r)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"payload":  payload,
		"issuedAt": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleQRPNG(w http.ResponseWriter, r *http.Request) {
	payload, status, err := s.qrResult(w, r)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	png, err := qr.PNG(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(png)
}

func (s *Server) handleQRSVG(w http.ResponseWriter, r *http.Request) {
	payload, status, err := s.qrResult(w, r)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	svg, err := qr.SVG(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.WriteHeader(http.StatusOK)
	w.Write(svg)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
