// Package server implements the stateless HTTP proxy: members log in through
// the web UI and get a live, auto-refreshing gym-entry QR code. Tokens live in
// an HttpOnly cookie owned by the browser; the server keeps nothing.
package server

import (
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
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
	cfg     Config
	limiter *rateLimiter
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
		cfg:     cfg,
		limiter: newRateLimiter(cfg.LoginRatePerMin),
	}
}

// Handler returns the fully-routed HTTP handler, wrapped in structured
// request logging.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /auth/login", s.handleLogin)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)
	mux.HandleFunc("GET /qr/fragment", s.handleQRFragment)
	if s.cfg.PublicDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(s.cfg.PublicDir)))
	}
	return s.logRequests(mux)
}

// logRequests logs one structured line per request.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		slog.LogAttrs(r.Context(), slog.LevelInfo, "http request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status()),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	})
}

// statusRecorder captures the response status code written by a handler.
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) status() int {
	if r.code == 0 {
		return http.StatusOK
	}
	return r.code
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handleIndex renders the full page, showing the sign-in form when the visitor
// has no valid session and the live QR view otherwise.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	v := s.sessionView(w, r)
	if v.login != nil {
		renderPage(w, pageData{View: "login", loginData: *v.login})
		return
	}
	renderPage(w, pageData{View: "qr", qrData: *v.qr})
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

	pair, err := s.cfg.VivaGymClient.Login(r.Context(), email, password)
	if err != nil {
		slog.WarnContext(r.Context(), "login failed", "email", email, "error", err)
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
	payload, err := s.cfg.VivaGymClient.FetchQr(r.Context(), pair.AccessToken)
	renderQR(w, qrDataFor(email, payload, err))
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

// qrDataFor builds the QR view state for a payload, falling back to a
// transient error state when payload is unavailable or cannot be rendered.
func qrDataFor(email, payload string, err error) qrData {
	if err == nil {
		if d, verr := qrView(email, payload); verr == nil {
			return d
		}
	}
	return qrData{Email: email, Updated: "Could not refresh QR"}
}

// sessionView is the rendered response for a session-dependent request: either
// the login form or the live QR view. Exactly one field is non-nil.
type sessionView struct {
	login *loginData
	qr    *qrData
}

// sessionView resolves the current session to a view, fetching (and refreshing)
// the QR payload as needed.
func (s *Server) sessionView(w http.ResponseWriter, r *http.Request) sessionView {
	payload, status, err := s.qrResult(w, r)
	if err != nil {
		if status == http.StatusUnauthorized {
			return sessionView{login: &loginData{}}
		}
	}
	email, _ := readTokens(r)
	d := qrDataFor(email.Email, payload, err)
	return sessionView{qr: &d}
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
	fresh, err := s.cfg.VivaGymClient.RefreshTokens(r.Context(), t.RefreshToken)
	if err != nil {
		s.clearTokens(w)
		return Tokens{}, false
	}
	s.rotateTokens(w, &t, fresh)
	return t, true
}

// rotateTokens installs fresh tokens into the session cookie.
func (s *Server) rotateTokens(w http.ResponseWriter, t *Tokens, fresh vivagym.TokenPair) {
	t.AccessToken = fresh.AccessToken
	t.RefreshToken = fresh.RefreshToken
	t.ExpiresIn = fresh.ExpiresIn
	t.IssuedAt = time.Now().UnixMilli()
	s.writeTokens(w, *t)
}

// unauthorized reports whether err is an upstream 401 response.
func unauthorized(err error) bool {
	var vg *vivagym.VivaGymError
	return errors.As(err, &vg) && vg.Status == http.StatusUnauthorized
}

// qrResult fetches the QR payload, refreshing the token once if VivaGym
// rejects it, and clearing the session if the refresh does not help.
func (s *Server) qrResult(w http.ResponseWriter, r *http.Request) (string, int, error) {
	t, ok := s.validTokens(w, r)
	if !ok {
		return "", http.StatusUnauthorized, errors.New("not authenticated")
	}

	payload, err := s.cfg.VivaGymClient.FetchQr(r.Context(), t.AccessToken)
	if err == nil {
		return payload, http.StatusOK, nil
	}
	if t.RefreshToken != "" && unauthorized(err) {
		if fresh, rerr := s.cfg.VivaGymClient.RefreshTokens(r.Context(), t.RefreshToken); rerr == nil {
			s.rotateTokens(w, &t, fresh)
			payload, err = s.cfg.VivaGymClient.FetchQr(r.Context(), t.AccessToken)
			if err == nil {
				return payload, http.StatusOK, nil
			}
		}
	}
	if unauthorized(err) {
		s.clearTokens(w)
		return "", http.StatusUnauthorized, errors.New("session expired, log in again")
	}
	return "", http.StatusBadGateway, err
}

// handleQRFragment serves the auto-refreshing QR view fragment for htmx. When
// the session has expired it renders the login form instead (with a 200) so the
// poller replaces itself and stops.
func (s *Server) handleQRFragment(w http.ResponseWriter, r *http.Request) {
	v := s.sessionView(w, r)
	if v.login != nil {
		v.login.Error = "Session expired, sign in again"
		renderLogin(w, *v.login)
		return
	}
	renderQR(w, *v.qr)
}
