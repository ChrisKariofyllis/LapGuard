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

	"lapguard/internal/api"
	"lapguard/internal/battery"
	"lapguard/internal/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("lapguard exited", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Parse(args)
	if err != nil {
		return err
	}

	var handler slog.Handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	if cfg.LogJSON {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	log := slog.New(handler)
	slog.SetDefault(log)

	log.Info("starting lapguard",
		"version", config.Version,
		"listen", cfg.Listen,
		"provider", cfg.Provider,
		"sysfs_root", cfg.SysfsRoot,
	)
	if !cfg.Loopback() {
		log.Warn("listen address is not loopback; remote access should go through Tailscale, not a public bind")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	provider, err := battery.Open(ctx, battery.OpenOptions{
		Kind:        cfg.Provider,
		SysfsRoot:   cfg.SysfsRoot,
		BatteryName: cfg.BatteryName,
		Logger:      log,
	})
	if err != nil {
		return err
	}

	app := api.New(provider, cfg, log)
	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.Listen)
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		log.Info("shutting down")
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
