// Package server implements the stateless HTTP proxy: members log in through
// the web UI and get a live, auto-refreshing gym-entry QR code. Tokens live in
// an HttpOnly cookie owned by the browser; the server keeps nothing.
package server

import (
	"encoding/json"
	"errors"
	"html/template"
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
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /auth/login", s.handleLogin)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)
	mux.HandleFunc("GET /qr/fragment", s.handleQRFragment)
	mux.HandleFunc("GET /qr.png", s.handleQRPNG)
	mux.HandleFunc("GET /qr.svg", s.handleQRSVG)
	if s.cfg.PublicDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(s.cfg.PublicDir)))
	}
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handleIndex renders the full page, showing the sign-in form when the visitor
// has no valid session and the live QR view otherwise.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	payload, status, err := s.qrResult(w, r)
	email, _ := readTokens(r)
	if err != nil {
		if status == http.StatusUnauthorized {
			renderPage(w, pageData{View: "login", loginData: loginData{}})
			return
		}
		renderPage(w, pageData{View: "qr", qrData: qrData{Email: email.Email, Updated: "Could not refresh QR"}})
		return
	}
	d, err := qrView(email.Email, payload)
	if err != nil {
		renderPage(w, pageData{View: "qr", qrData: qrData{Email: email.Email, Updated: "Could not refresh QR"}})
		return
	}
	renderPage(w, pageData{View: "qr", qrData: d})
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
		renderLogin(w, loginData{Error: "Too many attempts, try again later"})
		return
	}

	var email, password string
	switch ct := r.Header.Get("Content-Type"); {
	case strings.HasPrefix(ct, "application/json"):
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&body); err != nil {
			renderLogin(w, loginData{Error: "Invalid request"})
			return
		}
		email, password = body.Email, body.Password
	case ct == "application/x-www-form-urlencoded" || strings.HasPrefix(ct, "multipart/form-data"):
		if err := r.ParseForm(); err != nil {
			renderLogin(w, loginData{Error: "Invalid request"})
			return
		}
		email, password = r.FormValue("email"), r.FormValue("password")
	default:
		renderLogin(w, loginData{Error: "Invalid request"})
		return
	}

	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" || len(email) > 254 || len(password) > 256 || !strings.Contains(email, "@") {
		renderLogin(w, loginData{Error: "Invalid email or password"})
		return
	}

	pair, err := s.client.Login(r.Context(), email, password)
	if err != nil {
		log.Printf("login failed for %s - %v", email, err)
		renderLogin(w, loginData{Error: "Invalid credentials"})
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
	s.renderQRView(w, r, email, pair.AccessToken)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearTokens(w)
	renderLogin(w, loginData{})
}

// qrView builds the QR fragment state, rendering the payload as an inline SVG
// so the fragment needs no follow-up image request.
func qrView(email, payload string) (qrData, error) {
	svg, err := qr.SVG(payload)
	if err != nil {
		return qrData{}, err
	}
	return qrData{
		Email:   email,
		QR:      template.HTML(svg),
		Updated: time.Now().Format(time.Kitchen),
		Payload: payload,
	}, nil
}

// renderQRView renders the QR fragment, fetching the payload with the given
// fresh access token and falling back to a transient status on failure.
func (s *Server) renderQRView(w http.ResponseWriter, r *http.Request, email, accessToken string) {
	payload, err := s.client.FetchQr(r.Context(), accessToken)
	if err != nil {
		renderQR(w, qrData{Email: email, Updated: "Could not refresh QR"})
		return
	}
	d, err := qrView(email, payload)
	if err != nil {
		renderQR(w, qrData{Email: email, Updated: "Could not refresh QR"})
		return
	}
	renderQR(w, d)
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

// handleQRFragment serves the auto-refreshing QR view fragment for htmx. When
// the session has expired it renders the login form instead (with a 200) so the
// poller replaces itself and stops.
func (s *Server) handleQRFragment(w http.ResponseWriter, r *http.Request) {
	payload, status, err := s.qrResult(w, r)
	if err != nil {
		if status == http.StatusUnauthorized {
			renderLogin(w, loginData{Error: "Session expired, sign in again"})
			return
		}
		email, _ := readTokens(r)
		renderQR(w, qrData{Email: email.Email, Updated: "Could not refresh QR"})
		return
	}
	email, _ := readTokens(r)
	d, err := qrView(email.Email, payload)
	if err != nil {
		renderQR(w, qrData{Email: email.Email, Updated: "Could not refresh QR"})
		return
	}
	renderQR(w, d)
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
