package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gymqrack/internal/vivagym"
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

func doForm(t *testing.T, h http.Handler, method, path string, form url.Values, cookies []*http.Cookie) (*httptest.ResponseRecorder, []*http.Cookie) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, rec.Result().Cookies()
}

func seededCookie(email string) *http.Cookie {
	tok := Tokens{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresIn:    600,
		IssuedAt:     time.Now().UnixMilli(),
		Email:        email,
	}
	return &http.Cookie{Name: cookieName, Value: encodeTokens(tok), Path: "/"}
}

func TestIndexRendersLoginWhenUnauthenticated(t *testing.T) {
	s := newTestServer(t)
	rec, _ := doJSON(t, s.Handler(), http.MethodGet, "/", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("index status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"<!doctype html>", `hx-post="/auth/login"`, `id="loginView"`, "/htmx.min.js", `rel="manifest"`, "/app.js", "apple-mobile-web-app-capable"} {
		if !strings.Contains(body, want) {
			t.Fatalf("index missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "id=\"qrView\"") {
		t.Fatalf("index should not show QR view when unauthenticated:\n%s", body)
	}
}

func TestIndexRendersQRWhenAuthenticated(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()
	rec, _ := doJSON(t, h, http.MethodGet, "/", "", []*http.Cookie{seededCookie("user@example.com")})
	if rec.Code != http.StatusOK {
		t.Fatalf("index status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`id="qrView"`, "user@example.com", "exerp:checkin:1", "<svg", `hx-get="/qr/fragment"`, "tabvisible"} {
		if !strings.Contains(body, want) {
			t.Fatalf("index missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `src="/qr.svg`) {
		t.Fatalf("index should inline the SVG, not reference it by URL:\n%s", body)
	}
	if strings.Contains(body, "Updated Updated") {
		t.Fatalf("status should not repeat the Updated label:\n%s", body)
	}
	if svg := extractTag(body, "<svg", "</svg>"); strings.Contains(svg, "exerp:checkin:1") {
		t.Fatalf("payload must not be echoed as SVG text content:\n%s", svg)
	}
}

func TestLoginJSONSuccess(t *testing.T) {
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
	body := rec.Body.String()
	for _, want := range []string{`id="qrView"`, "user@example.com", "exerp:checkin:1", "<svg"} {
		if !strings.Contains(body, want) {
			t.Fatalf("login response missing %q:\n%s", want, body)
		}
	}
}

func TestLoginFormSuccess(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	form := url.Values{"email": {"User@Example.com"}, "password": {"hunter2"}}
	rec, cookies := doForm(t, h, http.MethodPost, "/auth/login", form, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", rec.Code, rec.Body)
	}
	if len(cookies) == 0 {
		t.Fatal("no Set-Cookie on login")
	}
	body := rec.Body.String()
	for _, want := range []string{`id="qrView"`, "user@example.com", "exerp:checkin:1", "<svg"} {
		if !strings.Contains(body, want) {
			t.Fatalf("login response missing %q:\n%s", want, body)
		}
	}
}

func TestLoginValidation(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	// Missing @ → error text, no cookie, 200 (htmx swaps it).
	form := url.Values{"email": {"nope"}, "password": {"x"}}
	rec, cookies := doForm(t, h, http.MethodPost, "/auth/login", form, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Invalid email or password") {
		t.Fatalf("expected validation error, got:\n%s", rec.Body)
	}
	if len(cookies) != 0 {
		t.Fatalf("no cookie should be set on validation failure, got %v", cookies)
	}
}

func TestLoginFailureDoesNotSetCookie(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()
	form := url.Values{"email": {"a@b.com"}, "password": {"wrong"}}
	rec, cookies := doForm(t, h, http.MethodPost, "/auth/login", form, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Invalid credentials") {
		t.Fatalf("expected invalid-credentials message, got:\n%s", rec.Body)
	}
	if len(cookies) != 0 {
		t.Fatalf("no cookie should be set on failure, got %v", cookies)
	}
}

func TestLoginRateLimit(t *testing.T) {
	s := newTestServer(t)
	s.limiter.perMin = 1
	h := s.Handler()
	for i := 0; i < 2; i++ {
		form := url.Values{"email": {"a@b.com"}, "password": {"wrong"}}
		rec, _ := doForm(t, h, http.MethodPost, "/auth/login", form, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("fragment should always be 200, got %d", rec.Code)
		}
		if i == 0 && !strings.Contains(rec.Body.String(), "Invalid credentials") {
			t.Fatalf("first failed login should report credentials, got:\n%s", rec.Body)
		}
		if i == 1 && !strings.Contains(rec.Body.String(), "Too many attempts") {
			t.Fatalf("second failed login should be rate limited, got:\n%s", rec.Body)
		}
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
	rec, cookies := doJSON(t, h, http.MethodGet, "/qr/fragment", "", []*http.Cookie{cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("qr fragment status = %d, body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "exerp:checkin:2") {
		t.Fatalf("expected refreshed payload, got:\n%s", rec.Body)
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

func TestQRFragmentExpiredSessionShowsLogin(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	// Expired access token with no refresh token → session is dead.
	tok := Tokens{
		AccessToken:  "access-1",
		RefreshToken: "",
		ExpiresIn:    600,
		IssuedAt:     time.Now().Add(-20 * time.Minute).UnixMilli(),
		Email:        "u@e.com",
	}
	cookie := &http.Cookie{Name: cookieName, Value: encodeTokens(tok), Path: "/"}
	rec, _ := doJSON(t, h, http.MethodGet, "/qr/fragment", "", []*http.Cookie{cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("expired fragment should be 200 to swap the view, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="loginView"`) || strings.Contains(body, "id=\"qrView\"") {
		t.Fatalf("expected login view fragment, got:\n%s", body)
	}
	if !strings.Contains(body, "Session expired") {
		t.Fatalf("expected expiry message, got:\n%s", body)
	}
}

func TestQRPayloadAuthenticated(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	rec, _ := doJSON(t, h, http.MethodGet, "/qr/payload", "", []*http.Cookie{seededCookie("u@e.com")})
	if rec.Code != http.StatusOK {
		t.Fatalf("payload status = %d, body=%s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/plain", ct)
	}
	if got := rec.Body.String(); got != "exerp:checkin:1" {
		t.Fatalf("payload = %q, want exerp:checkin:1", got)
	}
}

func TestQRPayloadUnauthenticated(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	rec, _ := doJSON(t, h, http.MethodGet, "/qr/payload", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated payload status = %d, want 401", rec.Code)
	}
}

func TestQRPayloadRefreshesToken(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()

	tok := Tokens{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresIn:    600,
		IssuedAt:     time.Now().Add(-20 * time.Minute).UnixMilli(),
		Email:        "u@e.com",
	}
	cookie := &http.Cookie{Name: cookieName, Value: encodeTokens(tok), Path: "/"}
	rec, _ := doJSON(t, h, http.MethodGet, "/qr/payload", "", []*http.Cookie{cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("payload status = %d, body=%s", rec.Code, rec.Body)
	}
	if got := rec.Body.String(); got != "exerp:checkin:2" {
		t.Fatalf("payload = %q, want refreshed exerp:checkin:2", got)
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()
	rec, cookies := doJSON(t, h, http.MethodPost, "/auth/logout", "", []*http.Cookie{seededCookie("u@e.com")})
	if rec.Code != http.StatusOK {
		t.Fatalf("logout status = %d", rec.Code)
	}
	for _, c := range cookies {
		if c.Name == cookieName && c.MaxAge != -1 {
			t.Fatalf("cookie not cleared: %+v", c)
		}
	}
	if !strings.Contains(rec.Body.String(), `id="loginView"`) {
		t.Fatalf("logout should render the login fragment, got:\n%s", rec.Body)
	}
}

func TestHealth(t *testing.T) {
	s := newTestServer(t)
	rec, _ := doJSON(t, s.Handler(), http.MethodGet, "/health", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d", rec.Code)
	}
}

func TestServiceWorkerIsNotCacheable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sw.js"), []byte("const CACHE = \"v2\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log(\"test\");\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(Config{PublicURL: "http://localhost:4567", PublicDir: dir})
	h := s.Handler()

	rec, _ := doJSON(t, h, http.MethodGet, "/sw.js", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("sw.js status = %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("sw.js Cache-Control = %q, want no-cache", got)
	}
	if !strings.Contains(rec.Body.String(), "const CACHE = \"v2\"") {
		t.Fatalf("unexpected sw.js body:\n%s", rec.Body)
	}
	if etag := rec.Header().Get("Etag"); etag == "" {
		t.Fatal("sw.js should carry a content-hash ETag")
	}

	// Revalidation with a matching ETag must 304.
	rec, _ = doJSON(t, h, http.MethodGet, "/sw.js", "", nil)
	etag := rec.Header().Get("Etag")
	req := httptest.NewRequest(http.MethodGet, "/sw.js", nil)
	req.Header.Set("If-None-Match", etag)
	rec304 := httptest.NewRecorder()
	h.ServeHTTP(rec304, req)
	if rec304.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match with current ETag: status = %d, want 304", rec304.Code)
	}

	// Changing the bytes must change the ETag, so revalidation returns 200.
	if err := os.WriteFile(filepath.Join(dir, "sw.js"), []byte("const CACHE = \"v3\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, _ = doJSON(t, h, http.MethodGet, "/sw.js", "", nil)
	newEtag := rec.Header().Get("Etag")
	if newEtag == "" || newEtag == etag {
		t.Fatalf("ETag did not change after content changed: old=%q new=%q", etag, newEtag)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "v3") {
		t.Fatalf("expected updated sw.js body, got status=%d:\n%s", rec.Code, rec.Body)
	}
	req = httptest.NewRequest(http.MethodGet, "/sw.js", nil)
	req.Header.Set("If-None-Match", etag)
	rec200 := httptest.NewRecorder()
	h.ServeHTTP(rec200, req)
	if rec200.Code != http.StatusOK {
		t.Fatalf("stale If-None-Match should revalidate to 200, got %d", rec200.Code)
	}

	// Other static assets should not carry the no-cache header.
	for _, path := range []string{"/app.js", "/nope.js"} {
		rec, _ := doJSON(t, h, http.MethodGet, path, "", nil)
		if got := rec.Header().Get("Cache-Control"); got == "no-cache" {
			t.Fatalf("%s unexpectedly has Cache-Control: no-cache", path)
		}
	}
}

func TestDirectoryListingDisabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log(1);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "icons"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "icons", "icon-192.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(Config{PublicURL: "http://localhost:4567", PublicDir: dir})
	h := s.Handler()

	for _, path := range []string{"/icons", "/icons/"} {
		rec, _ := doJSON(t, h, http.MethodGet, path, "", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404 (no directory listing)", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "index of") || strings.Contains(rec.Body.String(), "<pre>") {
			t.Fatalf("%s: response contains a directory listing:\n%s", path, rec.Body)
		}
	}

	rec, _ := doJSON(t, h, http.MethodGet, "/app.js", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("file should still be served, got %d", rec.Code)
	}
	rec, _ = doJSON(t, h, http.MethodGet, "/icons/icon-192.png", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("nested file should still be served, got %d", rec.Code)
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

// extractTag returns the substring between start and end, or "" if not found.
func extractTag(body, start, end string) string {
	i := strings.Index(body, start)
	if i < 0 {
		return ""
	}
	j := strings.Index(body[i:], end)
	if j < 0 {
		return ""
	}
	return body[i : i+j+len(end)]
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
