// Command server runs the Family Planner web app (kiosk + admin).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jvmeir/familyplanner/internal/config"
	"github.com/jvmeir/familyplanner/internal/db"
	"github.com/jvmeir/familyplanner/internal/i18n"
	"github.com/jvmeir/familyplanner/internal/server"
	"github.com/jvmeir/familyplanner/internal/widget"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()
	store, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.DB.Close() }()

	reg := widget.NewRegistry()
	widget.RegisterDefaults(reg)

	i18nSvc, err := i18n.New(cfg.DefaultLocale)
	if err != nil {
		return err
	}

	srv, err := server.New(cfg, store, reg, i18nSvc)
	if err != nil {
		return err
	}

	bgCtx, cancelBg := context.WithCancel(ctx)
	defer cancelBg()
	srv.StartBackground(bgCtx) // cache-refresh broker

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("listening", "addr", cfg.Addr, "env", cfg.Env, "locale", cfg.DefaultLocale)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen", "err", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}
