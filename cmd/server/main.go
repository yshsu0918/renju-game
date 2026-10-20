// Command server runs the Gomoku game server.
//
// Settings come from environment variables:
//
//	RENJU_ADDR              listen address (default ":8200")
//	RENJU_ENGINE            AI engine: builtin (default); piskvork and http are planned
//	RENJU_ENGINE_TIMEOUT    think time per AI move (default 3s)
//	RENJU_RECONNECT_GRACE   how long a disconnected player's seat is held (default 60s)
//	RENJU_STATIC_DIR        directory of the built Vue app to serve at / (default: none)
//	RENJU_ALLOWED_ORIGINS   comma-separated WebSocket origin host patterns, e.g. "localhost:5173"
//	                        (default: same origin only)
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"renju-game/internal/engine"
	"renju-game/internal/server"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	addr := env("RENJU_ADDR", ":8200")
	engineTimeout, err := time.ParseDuration(env("RENJU_ENGINE_TIMEOUT", "3s"))
	if err != nil {
		return err
	}
	grace, err := time.ParseDuration(env("RENJU_RECONNECT_GRACE", "60s"))
	if err != nil {
		return err
	}
	factory, err := engine.NewFactory(env("RENJU_ENGINE", "builtin"))
	if err != nil {
		return err
	}
	fallback, _ := engine.NewFactory("builtin")
	var origins []string
	if o := os.Getenv("RENJU_ALLOWED_ORIGINS"); o != "" {
		origins = strings.Split(o, ",")
	}

	hub := server.NewHub(server.Config{
		Engine:         factory,
		Fallback:       fallback,
		EngineTimeout:  engineTimeout,
		MaxRestarts:    1,
		ReconnectGrace: grace,
		Logger:         log,
	})
	srv := &http.Server{
		Addr:    addr,
		Handler: server.NewHandler(hub, os.Getenv("RENJU_STATIC_DIR"), origins),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("listening", "addr", addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
