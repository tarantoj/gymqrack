package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
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

func (s *Server) writeTokens(w http.ResponseWriter, t Tokens) {
	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    encodeTokens(t),
		Path:     "/",
		MaxAge:   s.cookieMaxAge,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

func (s *Server) clearTokens(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}
