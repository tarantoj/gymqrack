package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// Tokens is the client-owned VivaGym token pair, stored in an HttpOnly cookie.
// The server never persists it anywhere.
type Tokens struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
	IssuedAt     int64  `json:"issuedAt"`
	Email        string `json:"email"`
}

func encodeTokens(t Tokens) string {
	data, _ := json.Marshal(t)
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeTokens(raw string) (Tokens, bool) {
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Tokens{}, false
	}
	var t Tokens
	if err := json.Unmarshal(data, &t); err != nil {
		return Tokens{}, false
	}
	if t.AccessToken == "" {
		return Tokens{}, false
	}
	return t, true
}

func readTokens(r *http.Request) (Tokens, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return Tokens{}, false
	}
	return decodeTokens(cookie.Value)
}

// secureCookies reports whether session cookies should carry the Secure flag,
// derived from the public URL scheme.
func (s *Server) secureCookies() bool {
	return strings.HasPrefix(s.cfg.PublicURL, "https://")
}

func (s *Server) writeTokens(w http.ResponseWriter, t Tokens) {
	s.setSessionCookie(w, encodeTokens(t), s.cfg.CookieMaxAge)
}

func (s *Server) clearTokens(w http.ResponseWriter) {
	s.setSessionCookie(w, "", -1)
}

// setSessionCookie writes a session cookie with the standard flags: path "/",
// HttpOnly, SameSite=Lax, and Secure when serving over https.
func (s *Server) setSessionCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
}
