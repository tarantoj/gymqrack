package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vivagym/internal/vivagym"
)

// mockUpstream implements the VivaGym API surface the proxy needs.
func mockUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v2/token":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "temp"})
		case "/api/v2.0/es/exerp/newAuth":
			r.ParseForm()
			if r.Form.Get("password") == "wrong" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-1",
				"refresh_token": "refresh-1",
				"expires_in":    600,
			})
		case "/api/email/refresh":
			json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-2",
				"refresh_token": "refresh-2",
				"expires_in":    600,
			})
		case "/api/v2.0/exerp/qr":
			auth := r.Header.Get("Authorization")
			if auth == "Bearer access-1" {
				p, _ := json.Marshal("exerp:checkin:1")
				w.Write(p)
			} else if auth == "Bearer access-2" {
				p, _ := json.Marshal("exerp:checkin:2")
				w.Write(p)
			} else {
				w.WriteHeader(http.StatusUnauthorized)
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	upstream := mockUpstream(t)
	t.Cleanup(upstream.Close)
	client := vivagym.New(upstream.URL, "cid", "secret", "es")
	s := New(Config{
		PublicURL:     "http://localhost:4567",
		VivaGymClient: client,
		PublicDir:     "testdata",
	})
	return s
}

func doJSON(t *testing.T, h http.Handler, method, path, body string, cookies []*http.Cookie) (*httptest.ResponseRecorder, []*http.Cookie) {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, rec.Result().Cookies()
}

func TestLoginAndSessionFlow(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	rec, cookies := doJSON(t, h, http.MethodPost, "/auth/login",
		`{"email":"User@Example.com","password":"hunter2"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", rec.Code, rec.Body)
	}
	if len(cookies) == 0 {
		t.Fatal("no Set-Cookie on login")
	}
	var loginBody map[string]string
	json.Unmarshal(rec.Body.Bytes(), &loginBody)
	if loginBody["email"] != "user@example.com" {
		t.Fatalf("email not normalized: %q", loginBody["email"])
	}

	// Session reflects the cookie.
	rec, _ = doJSON(t, h, http.MethodGet, "/auth/session", "", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("session status = %d", rec.Code)
	}
	var sess map[string]any
	json.Unmarshal(rec.Body.Bytes(), &sess)
	if sess["authenticated"] != true || sess["email"] != "user@example.com" {
		t.Fatalf("unexpected session: %v", sess)
	}
}

func TestSessionUnauthenticated(t *testing.T) {
	s := newTestServer(t)
	rec, _ := doJSON(t, s.Handler(), http.MethodGet, "/auth/session", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestQRRefreshRotation(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	// Seed a cookie with an expired access token but valid refresh token.
	tok := Tokens{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresIn:    600,
		IssuedAt:     time.Now().Add(-20 * time.Minute).UnixMilli(),
		Email:        "u@e.com",
	}
	cookie := &http.Cookie{Name: cookieName, Value: encodeTokens(tok), Path: "/"}
	rec, cookies := doJSON(t, h, http.MethodGet, "/qr", "", []*http.Cookie{cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("qr status = %d, body=%s", rec.Code, rec.Body)
	}
	var qr map[string]string
	json.Unmarshal(rec.Body.Bytes(), &qr)
	if qr["payload"] != "exerp:checkin:2" {
		t.Fatalf("expected refreshed payload, got %q", qr["payload"])
	}
	if len(cookies) == 0 {
		t.Fatal("expected rotated cookie")
	}
	raw := cookieValue(cookies, cookieName)
	rotated, ok := decodeTokens(raw)
	if !ok || rotated.AccessToken != "access-2" {
		t.Fatalf("cookie not rotated: %+v", rotated)
	}
}

func TestQRPNG(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()
	tok := Tokens{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresIn:    600,
		IssuedAt:     time.Now().UnixMilli(),
		Email:        "u@e.com",
	}
	cookie := &http.Cookie{Name: cookieName, Value: encodeTokens(tok), Path: "/"}
	rec, _ := doJSON(t, h, http.MethodGet, "/qr.png", "", []*http.Cookie{cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("qr.png status = %d, body=%s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type = %q", ct)
	}
	if len(rec.Body.Bytes()) < 100 {
		t.Fatalf("png too small: %d bytes", len(rec.Body.Bytes()))
	}
	// Verify it's a valid PNG signature.
	if !bytes.Equal(rec.Body.Bytes()[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		t.Fatal("not a PNG")
	}
}

func TestQRSVG(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()
	tok := Tokens{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresIn:    600,
		IssuedAt:     time.Now().UnixMilli(),
		Email:        "u@e.com",
	}
	cookie := &http.Cookie{Name: cookieName, Value: encodeTokens(tok), Path: "/"}
	rec, _ := doJSON(t, h, http.MethodGet, "/qr.svg", "", []*http.Cookie{cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("qr.svg status = %d, body=%s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Fatalf("content-type = %q", ct)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("<svg")) {
		t.Fatal("response is not SVG markup")
	}
}

func TestLoginValidation(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()
	// Missing @ → 400
	rec, _ := doJSON(t, h, http.MethodPost, "/auth/login", `{"email":"nope","password":"x"}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	// Malformed JSON → 400
	rec, _ = doJSON(t, h, http.MethodPost, "/auth/login", `{`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()
	tok := Tokens{AccessToken: "a", RefreshToken: "r", ExpiresIn: 600, IssuedAt: time.Now().UnixMilli(), Email: "u@e.com"}
	cookie := &http.Cookie{Name: cookieName, Value: encodeTokens(tok), Path: "/"}
	rec, cookies := doJSON(t, h, http.MethodPost, "/auth/logout", "", []*http.Cookie{cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("logout status = %d", rec.Code)
	}
	for _, c := range cookies {
		if c.Name == cookieName && c.MaxAge != -1 {
			t.Fatalf("cookie not cleared: %+v", c)
		}
	}
}

func TestLoginRateLimit(t *testing.T) {
	s := newTestServer(t)
	// One per minute max.
	s.limiter.perMin = 1
	h := s.Handler()
	for i := 0; i < 2; i++ {
		rec, _ := doJSON(t, h, http.MethodPost, "/auth/login",
			`{"email":"a@b.com","password":"wrong"}`, nil)
		if i == 0 && rec.Code != http.StatusUnauthorized {
			t.Fatalf("first failed login should be rejected by upstream, got %d", rec.Code)
		}
		if i == 1 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("second failed login should be rate limited, got %d", rec.Code)
		}
	}
}

func TestLoginFailureDoesNotSetCookie(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()
	rec, cookies := doJSON(t, h, http.MethodPost, "/auth/login",
		`{"email":"a@b.com","password":"wrong"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if len(cookies) != 0 {
		t.Fatalf("no cookie should be set on failure, got %v", cookies)
	}
}

func cookieValue(cookies []*http.Cookie, name string) string {
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func TestCookieEncodingCompatibility(t *testing.T) {
	// The cookie must be base64url(JSON), readable back and forth.
	tok := Tokens{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 600, IssuedAt: 123, Email: "e@e.com"}
	encoded := encodeTokens(tok)
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("not base64url: %v", err)
	}
	var re Tokens
	if err := json.Unmarshal(decoded, &re); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if re.AccessToken != "at" {
		t.Fatalf("mismatch: %+v", re)
	}
}

func TestCookieDecodeGarbage(t *testing.T) {
	if _, ok := decodeTokens("!!!not-base64!!!"); ok {
		t.Fatal("garbage should fail to decode")
	}
	if _, ok := decodeTokens(base64.RawURLEncoding.EncodeToString([]byte(`{"accessToken":""}`))); ok {
		t.Fatal("empty access token should fail validation")
	}
}
