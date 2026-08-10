// Command vivagym-wallet is the VivaGym live gym-entry QR proxy server.
//
// Runtime configuration comes from environment variables:
//
//	VIVAGYM_CLIENT_ID, VIVAGYM_CLIENT_SECRET (required)
//	VIVAGYM_LOCALE, PORT, HOST, PUBLIC_URL (optional)
//	COOKIE_MAX_AGE_DAYS, LOGIN_RATE_PER_MIN, TRUST_PROXY (optional)
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"vivagym/internal/server"
	"vivagym/internal/vivagym"
)

func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		log.Printf("warning: invalid %s=%q, using %d", name, v, def)
	}
	return def
}

func main() {
	port := envInt("PORT", 4567)
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	publicURL := os.Getenv("PUBLIC_URL")
	if publicURL == "" {
		publicURL = fmt.Sprintf("http://localhost:%d", port)
	}
	publicURL = trimTrailingSlash(publicURL)
	locale := os.Getenv("VIVAGYM_LOCALE")
	if locale == "" {
		locale = "es"
	}

	clientID := os.Getenv("VIVAGYM_CLIENT_ID")
	clientSecret := os.Getenv("VIVAGYM_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		log.Fatal("VIVAGYM_CLIENT_ID and VIVAGYM_CLIENT_SECRET must be set")
	}

	client := vivagym.New("", clientID, clientSecret, locale)

	srv := server.New(server.Config{
		PublicURL:       publicURL,
		TrustProxy:      os.Getenv("TRUST_PROXY") == "1",
		CookieMaxAge:    envInt("COOKIE_MAX_AGE_DAYS", 7) * 86_400,
		LoginRatePerMin: envInt("LOGIN_RATE_PER_MIN", 10),
		VivaGymClient:   client,
		PublicDir:       publicDir(),
	})

	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("VivaGym live QR running: %s", publicURL)
	log.Printf("  live screen : %s/", publicURL)
	log.Printf("  QR PNG      : %s/qr.png", publicURL)
	log.Printf("  QR SVG      : %s/qr.svg", publicURL)
	log.Printf("  JSON payload: %s/qr", publicURL)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

// publicDir locates the web UI directory next to the binary (nix build) or in
// the source tree (dev).
func publicDir() string {
	dirs := []string{
		filepath.Join("public"),
		filepath.Join(filepath.Dir(os.Args[0]), "public"),
		filepath.Join(filepath.Dir(os.Args[0]), "..", "share", "vivagym-wallet", "public"),
	}
	for _, d := range dirs {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			return d
		}
	}
	return "public"
}

func trimTrailingSlash(s string) string {
	for len(s) > 1 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
