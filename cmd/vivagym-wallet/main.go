// Command vivagym-wallet is the VivaGym live gym-entry QR proxy server.
//
// Runtime configuration comes from environment variables:
//
//	VIVAGYM_CLIENT_ID, VIVAGYM_CLIENT_SECRET (required)
//	VIVAGYM_LOCALE, PORT, HOST, PUBLIC_URL (optional)
//	COOKIE_MAX_AGE_DAYS, LOGIN_RATE_PER_MIN, TRUST_PROXY (optional)
//
// OpenTelemetry: traces go to the OTLP HTTP collector at
// OTEL_EXPORTER_OTLP_ENDPOINT when set, otherwise to stdout. Set
// OTEL_SERVICE_NAME (default vivagym-wallet), OTEL_TRACES_EXPORTER
// (none|otlp|console) or OTEL_SDK_DISABLED=1 to control tracing. Standard
// OTEL_EXPORTER_OTLP_* env vars apply.
//
// Logging: structured logs go to the systemd journal via the native protocol
// when running as a systemd service, and fall back to JSON on stderr
// otherwise. Set VIVAGYM_LOG_FORMAT=json to force JSON output.
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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

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

// newTracerProvider sets up the global OpenTelemetry tracer provider. Spans go
// to the OTLP HTTP endpoint from OTEL_EXPORTER_OTLP_ENDPOINT when configured,
// otherwise they are printed to stdout. The standard OTEL_TRACES_EXPORTER env
// var (none|otlp|console) overrides the default choice. Returns a no-op
// provider (and a no-op shutdown) when tracing is disabled.
func newTracerProvider() (*sdktrace.TracerProvider, func(context.Context) error) {
	noop := func(context.Context) error { return nil }
	if os.Getenv("OTEL_SDK_DISABLED") == "1" {
		return sdktrace.NewTracerProvider(), noop
	}

	exporterName := os.Getenv("OTEL_TRACES_EXPORTER")
	endpointSet := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != ""

	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "vivagym-wallet"
	}
	res := resource.NewSchemaless(attribute.String("service.name", serviceName))

	var exporter sdktrace.SpanExporter
	var err error
	switch {
	case exporterName == "none":
		return sdktrace.NewTracerProvider(), noop
	case exporterName == "otlp" || (exporterName == "" && endpointSet):
		exporter, err = otlptracehttp.New(context.Background())
	case exporterName == "console" || exporterName == "stdout" || exporterName == "":
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	default:
		slog.Error("unknown OTEL_TRACES_EXPORTER, tracing disabled", "value", exporterName)
		return sdktrace.NewTracerProvider(), noop
	}
	if err != nil {
		slog.Error("failed to create trace exporter, tracing disabled", "error", err)
		return sdktrace.NewTracerProvider(), noop
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return tp, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return tp.Shutdown(ctx)
	}
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

	_, flush := newTracerProvider()
	defer func() {
		_ = flush(context.Background())
	}()

	client := vivagym.New("", clientID, clientSecret, locale)

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
	httpServer := &http.Server{Addr: addr, Handler: srv.Handler()}

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
