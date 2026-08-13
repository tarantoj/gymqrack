// Command vivagym-access is the VivaGym live gym-entry QR proxy server.
//
// Runtime configuration comes from environment variables:
//
//	VIVAGYM_CLIENT_ID, VIVAGYM_CLIENT_SECRET (required)
//	VIVAGYM_LOCALE, PORT, HOST, PUBLIC_URL (optional)
//	COOKIE_MAX_AGE_DAYS, LOGIN_RATE_PER_MIN, TRUST_PROXY (optional)
//
// Logging: structured JSON logs go to stderr.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"vivagym/internal/server"
	"vivagym/internal/vivagym"
)

func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		slog.Warn("invalid env value, using default", "env", name, "value", v, "default", def)
	}
	return def
}

func main() {
	slog.SetDefault(slog.New(newLogHandler()))

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
		slog.Error("VIVAGYM_CLIENT_ID and VIVAGYM_CLIENT_SECRET must be set")
		os.Exit(1)
	}

	client := vivagym.New(vivagym.BaseURL, clientID, clientSecret, locale)

	srv := server.New(server.Config{
		PublicURL:       publicURL,
		TrustProxy:      os.Getenv("TRUST_PROXY") == "1",
		CookieMaxAge:    envInt("COOKIE_MAX_AGE_DAYS", 7) * 86_400,
		LoginRatePerMin: envInt("LOGIN_RATE_PER_MIN", 10),
		VivaGymClient:   client,
		PublicDir:       publicDir(),
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	addr := fmt.Sprintf("%s:%d", host, port)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info("VivaGym live QR running", "url", publicURL, "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
}

// publicDir locates the web UI directory next to the binary (nix build) or in
// the source tree (dev).
func publicDir() string {
	dirs := []string{
		filepath.Join("public"),
		filepath.Join(filepath.Dir(os.Args[0]), "public"),
		filepath.Join(filepath.Dir(os.Args[0]), "..", "share", "vivagym-access", "public"),
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
