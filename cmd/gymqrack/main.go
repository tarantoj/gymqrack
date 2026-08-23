// Command gymqrack is the VivaGym live gym-entry QR proxy server.
//
// Runtime configuration comes from environment variables:
//
//	GYMQRACK_CLIENT_ID, GYMQRACK_CLIENT_SECRET (required)
//	GYMQRACK_LOCALE, PORT, HOST, PUBLIC_URL (optional)
//	COOKIE_MAX_AGE_DAYS, LOGIN_RATE_PER_MIN, TRUST_PROXY (optional)
//	SOCKET, SOCKET_GID (optional; overrides HOST/PORT with a unix socket;
//	SOCKET_GID is a numeric gid or group name owning the socket)
//
// Logging: structured JSON logs go to stderr.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"gymqrack/internal/server"
	"gymqrack/internal/vivagym"
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

// runConfig bundles the process configuration resolved from the environment.
type runConfig struct {
	server   *server.Config
	httpAddr string
	socket   string
	sockGID  string
}

// configFromEnv assembles the runtime configuration from environment variables.
func configFromEnv() (runConfig, error) {
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
	locale := os.Getenv("GYMQRACK_LOCALE")
	if locale == "" {
		locale = "es"
	}

	clientID := os.Getenv("GYMQRACK_CLIENT_ID")
	clientSecret := os.Getenv("GYMQRACK_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return runConfig{}, errors.New("GYMQRACK_CLIENT_ID and GYMQRACK_CLIENT_SECRET must be set")
	}

	return runConfig{
		server: &server.Config{
			PublicURL:       publicURL,
			TrustProxy:      os.Getenv("TRUST_PROXY") == "1",
			CookieMaxAge:    envInt("COOKIE_MAX_AGE_DAYS", 7) * 86_400,
			LoginRatePerMin: envInt("LOGIN_RATE_PER_MIN", 10),
			VivaGymClient:   vivagym.New(vivagym.BaseURL, clientID, clientSecret, locale),
			PublicDir:       publicDir(),
		},
		httpAddr: fmt.Sprintf("%s:%d", host, port),
		socket:   os.Getenv("SOCKET"),
		sockGID:  os.Getenv("SOCKET_GID"),
	}, nil
}

func main() {
	slog.SetDefault(slog.New(newLogHandler()))

	cfg, err := configFromEnv()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	srv := server.New(*cfg.server)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	httpServer := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	var ln net.Listener
	if cfg.socket != "" {
		ln, err = listenUnix(cfg.socket, cfg.sockGID)
		if err != nil {
			slog.Error("cannot listen on unix socket", "socket", cfg.socket, "error", err)
			os.Exit(1)
		}
	} else {
		httpServer.Addr = cfg.httpAddr
		ln, err = net.Listen("tcp", cfg.httpAddr)
		if err != nil {
			slog.Error("cannot listen", "addr", cfg.httpAddr, "error", err)
			os.Exit(1)
		}
	}

	go func() {
		slog.Info("gymqrack live QR running", "url", cfg.server.PublicURL, "addr", ln.Addr())
		if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

// listenUnix binds an HTTP listener on a unix socket. It removes a stale
// socket file, creates the parent directory, and when group (a numeric gid or
// group name, e.g. "nginx") is non-empty chowns the socket to that group with
// mode 0o660 so a same-host reverse proxy sharing the group can connect.
func listenUnix(path, group string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o660); err != nil {
		ln.Close()
		return nil, err
	}
	if group != "" {
		gid, err := lookupGID(group)
		if err != nil {
			ln.Close()
			return nil, err
		}
		if err := os.Chown(path, -1, gid); err != nil {
			ln.Close()
			return nil, err
		}
	}
	return ln, nil
}

// lookupGID resolves a numeric gid string or a group name to a numeric gid.
func lookupGID(group string) (int, error) {
	if gid, err := strconv.Atoi(group); err == nil {
		return gid, nil
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(g.Gid)
}

// publicDir locates the web UI directory next to the binary (nix build) or in
// the source tree (dev).
func publicDir() string {
	dirs := []string{
		filepath.Join("public"),
		filepath.Join(filepath.Dir(os.Args[0]), "public"),
		filepath.Join(filepath.Dir(os.Args[0]), "..", "share", "gymqrack", "public"),
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
