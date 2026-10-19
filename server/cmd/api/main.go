// Command api runs the Fireline HTTP server.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/geone39/fireline/server/auth"
	"github.com/geone39/fireline/server/db"
	"github.com/geone39/fireline/server/gateway"
)

// Token lifetimes are fixed by docs/01-architecture.md §8.
const (
	accessTTL  = 15 * time.Minute
	refreshTTL = 30 * 24 * time.Hour
)

func main() {
	log := zerolog.New(os.Stdout).With().Timestamp().Logger()
	if err := run(log); err != nil {
		log.Fatal().Err(err).Msg("fatal")
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return f, nil
}

func run(log zerolog.Logger) error {
	dsn := os.Getenv("DATABASE_URL")
	secret := os.Getenv("JWT_SECRET")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	if len(secret) < auth.MinSecretLen {
		return fmt.Errorf("JWT_SECRET is required (min %d bytes)", auth.MinSecretLen)
	}
	ipRPS, err := envFloat("RATE_IP_RPS", 20)
	if err != nil {
		return err
	}
	userRPS, err := envFloat("RATE_USER_RPS", 10)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := db.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Close()
	if env("MIGRATE_ON_START", "true") == "true" {
		if err := store.Migrate(ctx); err != nil {
			return err
		}
	}
	content, err := gateway.LoadContent(env("CONTENT_DIR", "../content"), os.Getenv("CONTENT_BUNDLE_URL"))
	if err != nil {
		return err
	}
	log.Info().Str("content_version", content.Version()).Msg("content loaded")

	srv := gateway.New(gateway.Deps{
		Config: gateway.Config{
			IPRateRPS: ipRPS, IPRateBurst: int(ipRPS * 2),
			UserRateRPS: userRPS, UserRateBurst: int(userRPS * 2),
		},
		Auth:    &auth.Service{Store: store, Signer: &auth.Signer{Secret: []byte(secret), AccessTTL: accessTTL, RefreshTTL: refreshTTL}},
		Store:   store,
		Idem:    store,
		Content: content,
		Log:     log,
	})
	httpSrv := &http.Server{
		Addr:              env("LISTEN_ADDR", ":8080"),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- httpSrv.ListenAndServe() }()
	log.Info().Str("addr", httpSrv.Addr).Msg("listening")
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}
