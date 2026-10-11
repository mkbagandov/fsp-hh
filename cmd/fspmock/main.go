// Command fspmock — эмулятор ФСП ID (OIDC-провайдер с путями Keycloak) и реестра
// достижений ФСП для разработки и демонстрации. См. internal/fspmock и docs/05-fsp-integration.md.
//
// Переменные окружения: FSPMOCK_ADDR (127.0.0.1:8090), FSPMOCK_ISSUER
// (http://127.0.0.1:8090/realms/fsp), FSPMOCK_CLIENT_ID, FSPMOCK_CLIENT_SECRET.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"fsp/internal/fspmock"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()
	addr := envOr("FSPMOCK_ADDR", "127.0.0.1:8090")
	issuer := envOr("FSPMOCK_ISSUER", "http://"+addr+"/realms/fsp")
	srv, err := fspmock.New(fspmock.Config{
		Issuer:       issuer,
		ClientID:     envOr("FSPMOCK_CLIENT_ID", "fsp-platform"),
		ClientSecret: envOr("FSPMOCK_CLIENT_SECRET", "fsp-platform-secret"),
	})
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	httpSrv := &http.Server{Addr: addr, Handler: logging(log, srv.Handler()), ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		log.Info("fspmock listening", "addr", addr, "issuer", issuer)
		for _, u := range srv.Users() {
			log.Info("test user", "username", u.Username, "password", u.Password, "fsp_id", u.FSPID, "achievements", len(u.Achievements))
		}
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shCtx)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Info("http", "method", r.Method, "path", r.URL.Path, "dur_ms", time.Since(start).Milliseconds())
	})
}
