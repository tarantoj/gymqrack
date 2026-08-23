// Package server implements the stateless HTTP proxy: members log in through
// the web UI and get a live, auto-refreshing gym-entry QR code. Tokens live in
// an HttpOnly cookie owned by the browser; the server keeps nothing.
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gymqrack/internal/qr"
	"gymqrack/internal/vivagym"
)

const cookieName = "gymqrack_tokens"

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
	mux.HandleFunc("GET /qr/payload", s.handleQRPayload)
	mux.HandleFunc("GET /qr/png", s.handleQRPNG)
	if s.cfg.PublicDir != "" {
		files := http.FileServer(noListingFS{http.Dir(s.cfg.PublicDir)})
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Service workers must never be long-cached: browsers only replace
			// a SW when /sw.js bytes change, so an edge-cached copy (e.g.
			// Cloudflare's 4h default for .js) would delay updates like the
			// icon cache bump indefinitely. Force revalidation instead.
			//
			// Nix store files share one epoch mtime, so Last-Modified cannot
			// distinguish versions; a content-hash ETag can. ServeContent
			// honors a pre-set ETag for If-None-Match, returning 304 only when
			// the bytes actually match.
			if r.URL.Path == "/sw.js" {
				w.Header().Set("Cache-Control", "no-cache")
				if etag := swETag(s.cfg.PublicDir); etag != "" {
					w.Header().Set("Etag", etag)
				}
			}
			files.ServeHTTP(w, r)
		}))
	}
	return s.logRequests(mux)
}

// noListingFS is an http.FileSystem that 404s directory opens, disabling the
// default directory listing served by http.FileServer. Files (including
// index.html) are served normally.
type noListingFS struct {
	fs http.FileSystem
}

func (n noListingFS) Open(name string) (http.File, error) {
	f, err := n.fs.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if info.IsDir() {
		f.Close()
		return nil, os.ErrNotExist
	}
	return f, nil
}

// swETag returns a strong content-hash ETag for the service worker file, or ""
// if it cannot be read.
func swETag(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "sw.js"))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%q", hex.EncodeToString(sum[:]))
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
	if !validCredentials(email, password) {
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
	renderQR(w, qrView(email))
}

// validCredentials reports whether an email/password pair passes basic checks.
func validCredentials(email, password string) bool {
	return email != "" && password != "" && len(email) <= 254 && len(password) <= 256 && strings.Contains(email, "@")
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearTokens(w)
	renderLogin(w, loginData{})
}

// qrView builds the QR fragment state for email.
func qrView(email string) qrData {
	now := time.Now()
	return qrData{Email: email, Updated: now.Format(time.Kitchen), Nonce: now.UnixNano()}
}

// qrDataFor builds the QR view state, falling back to a transient error state
// when the payload is unavailable.
func qrDataFor(email string, err error) qrData {
	if err == nil {
		return qrView(email)
	}
	return qrData{Email: email, Updated: "Could not refresh QR"}
}

// sessionView is the rendered response for a session-dependent request: either
// the login form or the live QR view. Exactly one field is non-nil.
type sessionView struct {
	login *loginData
	qr    *qrData
}

// sessionView resolves the current session to a view, probing the QR payload
// (fetching and refreshing it) only to decide whether the session is alive.
func (s *Server) sessionView(w http.ResponseWriter, r *http.Request) sessionView {
	_, status, err := s.qrResult(w, r)
	if err != nil {
		if status == http.StatusUnauthorized {
			return sessionView{login: &loginData{}}
		}
	}
	email, _ := readTokens(r)
	d := qrDataFor(email.Email, err)
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

// handleQRPayload serves the raw QR payload as plain text, using the same
// session cookie (with transparent token refresh) as the QR view. When there
// is no valid session it also accepts VivaGym credentials via HTTP Basic auth
// (a scriptable alternative to the web form), prompting with a WWW-Authenticate
// challenge when no credentials are supplied.
func (s *Server) handleQRPayload(w http.ResponseWriter, r *http.Request) {
	payload, status, err := s.qrResult(w, r)
	if err != nil && status == http.StatusUnauthorized {
		if email, password, ok := r.BasicAuth(); ok {
			payload, status, err = s.loginWithBasicAuth(w, r, email, password)
		}
	}
	if err != nil {
		if status == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", `Basic realm="gymqrack", charset="UTF-8"`)
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(payload))
}

// handleQRPNG serves the QR payload rendered as a PNG, using the same session
// cookie (with transparent token refresh) as the QR view. The browser fetches
// it on every fragment poll, so it must never be cached: the payload rotates
// continuously and a stale image would be wrong.
func (s *Server) handleQRPNG(w http.ResponseWriter, r *http.Request) {
	payload, status, err := s.qrResult(w, r)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	png, err := qr.PNG(payload)
	if err != nil {
		http.Error(w, "could not render QR", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	_, _ = w.Write(png)
}

// loginWithBasicAuth validates HTTP Basic credentials against VivaGym,
// installs the resulting session cookie, and returns the freshly fetched QR
// payload.
func (s *Server) loginWithBasicAuth(w http.ResponseWriter, r *http.Request, email, password string) (string, int, error) {
	ip := s.clientIP(r)
	if !s.limiter.allow(ip) {
		return "", http.StatusTooManyRequests, errors.New("too many attempts, try again later")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !validCredentials(email, password) {
		return "", http.StatusUnauthorized, errors.New("invalid credentials")
	}
	pair, err := s.cfg.VivaGymClient.Login(r.Context(), email, password)
	if err != nil {
		slog.WarnContext(r.Context(), "basic login failed", "email", email, "error", err)
		return "", http.StatusUnauthorized, errors.New("invalid credentials")
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
	if err != nil {
		return "", http.StatusBadGateway, err
	}
	return payload, http.StatusOK, nil
}
