package vivagym

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T) (*Client, *httptest.Server) {
	t.Helper()
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v2/token":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "temp-token"})
		case "/api/v2.0/es/exerp/newAuth":
			json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "member-access",
				"refresh_token": "member-refresh",
				"expires_in":    600,
			})
		case "/api/email/refresh":
			json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "fresh-access",
				"refresh_token": "fresh-refresh",
				"expires_in":    600,
			})
		case "/api/v2.0/exerp/qr":
			if r.Header.Get("Authorization") != "Bearer member-access" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			payload, _ := json.Marshal("exerp:checkin:123")
			w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	return New(upstream.URL, "cid", "csecret", "es"), upstream
}

func TestLogin(t *testing.T) {
	c, srv := newTestClient(t)
	defer srv.Close()
	pair, err := c.Login(t.Context(), "user@example.com", "hunter2")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if pair.AccessToken != "member-access" || pair.RefreshToken != "member-refresh" {
		t.Fatalf("unexpected pair: %+v", pair)
	}
	if pair.ExpiresIn != 600 {
		t.Fatalf("expected expires_in 600, got %d", pair.ExpiresIn)
	}
}

func TestRefreshTokens(t *testing.T) {
	c, srv := newTestClient(t)
	defer srv.Close()
	pair, err := c.RefreshTokens(t.Context(), "old-refresh")
	if err != nil {
		t.Fatalf("RefreshTokens: %v", err)
	}
	if pair.AccessToken != "fresh-access" || pair.RefreshToken != "fresh-refresh" {
		t.Fatalf("unexpected pair: %+v", pair)
	}
}

func TestFetchQr(t *testing.T) {
	c, srv := newTestClient(t)
	defer srv.Close()
	payload, err := c.FetchQr(t.Context(), "member-access")
	if err != nil {
		t.Fatalf("FetchQr: %v", err)
	}
	if payload != "exerp:checkin:123" {
		t.Fatalf("unexpected payload: %q", payload)
	}
}

func TestFetchQrUnauthorized(t *testing.T) {
	c, srv := newTestClient(t)
	defer srv.Close()
	_, err := c.FetchQr(t.Context(), "bad-token")
	if err == nil {
		t.Fatal("expected error")
	}
	if vg, ok := err.(*VivaGymError); !ok || vg.Status != http.StatusUnauthorized {
		t.Fatalf("expected VivaGymError 401, got %v", err)
	}
}
