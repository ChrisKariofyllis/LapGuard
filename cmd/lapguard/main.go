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
	"lapguard/internal/discovery"
	"lapguard/internal/notify"
	"lapguard/internal/power"
	"lapguard/internal/storage"
	"lapguard/internal/thresholds"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("lapguard exited", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "discover" {
		return runDiscover(os.Stdout, os.Stderr, args[1:])
	}
	if len(args) > 0 && args[0] == "auth" {
		return runAuth(os.Stdout, os.Stderr, args[1:])
	}
	if len(args) > 0 && args[0] == "tailscale" {
		return runTailscale(os.Stdout, os.Stderr, args[1:])
	}

	cfg, err := config.Load(args)
	if err != nil {
		return err
	}

	var handler slog.Handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	if cfg.LogJSON {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	log := slog.New(config.NewRedactingHandler(handler))
	slog.SetDefault(log)

	log.Info("starting lapguard",
		"version", config.Version,
		"listen", cfg.Listen,
		"provider", cfg.Provider,
		"sysfs_root", cfg.SysfsRoot,
		"threshold_method", cfg.ThresholdMethod,
		"config_source", cfg.ConfigSource(),
		"config", cfg.SafeConfigPath(),
		"events_db", cfg.SafeEventsDBPath(),
		"power_poll", cfg.PowerPoll.String(),
		"power_debounce", cfg.PowerDebounce.String(),
	)
	cfg.LogStartup(log)
	if !cfg.Loopback() {
		log.Warn("listen address is not loopback; remote access should go through Tailscale, not a public bind")
	}

	token, minted, err := cfg.EnsureAuthToken(time.Now().UTC())
	if err != nil {
		return err
	}
	if minted {
		printTokenOnce(os.Stdout, os.Stderr, token, "generated")
		if cfg.ConfigPath != "" {
			if err := cfg.Save(cfg.ConfigPath); err != nil {
				log.Warn("could not persist generated API token hash", "path", cfg.SafeConfigPath(), "err", err)
			}
		}
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

	disc := discovery.NewService(discovery.Options{
		SysfsRoot: cfg.SysfsRoot,
		Hostname:  "",
		Runner:    discovery.ExecRunner(),
	})
	report, err := disc.Refresh(ctx)
	if err != nil {
		return err
	}
	method, warn := config.ResolveThresholdMethod(cfg.ThresholdMethod, report.Features.ChargeThresholds)
	if warn != "" {
		log.Warn("threshold method", "msg", warn)
	}
	ctrl := thresholds.New(report.Thresholds, discovery.ExecRunner())
	log.Info("discovery complete",
		"hostname", report.Hostname,
		"os", report.OS,
		"kernel", report.Kernel,
		"battery", report.Battery.Name,
		"present", report.Battery.Present,
		"naming", report.NamingConvention,
		"power", report.PowerCalculation,
		"charge_thresholds", method,
		"threshold_controller", ctrl.Method(),
		"modules", report.KernelModules,
		"tlp", report.AvailableTools.TLP,
		"tlp_version", report.AvailableTools.TLPVersion,
		"power_loss_detection", report.Features.PowerLossDetection,
	)

	var store *storage.Store
	if path := cfg.EventsDBPath(); path != "" {
		opened, err := storage.Open(path)
		if err != nil {
			log.Warn("event log unavailable", "path", config.SafeDisplayPath(path), "err", err)
		} else {
			store = opened
			defer func() { _ = store.Close() }()
			log.Info("event log ready", "path", config.SafeDisplayPath(path))
		}
	}

	app := api.New(provider, cfg, log, disc)
	notifier := app.Notifier()

	watcher := power.NewWatcher(power.Options{
		SysfsRoot: cfg.SysfsRoot,
		Interval:  cfg.PowerPoll,
		Debounce:  cfg.PowerDebounce,
		Logger:    log,
		OnEvent: func(tr power.Transition) {
			if store != nil {
				if _, err := store.Insert(context.Background(), storage.Event{
					Type:       tr.Type,
					Timestamp:  tr.At,
					Source:     tr.Source,
					DurationMs: tr.DurationMs,
				}); err != nil {
					log.Error("persist power event", "err", err, "type", tr.Type)
				}
			}
			nctx, cancel := context.WithTimeout(context.Background(), notify.DefaultTestTimeout)
			defer cancel()
			if err := notifier.HandlePower(nctx, tr); err != nil {
				log.Error("notification", "err", notify.SanitizeError(err), "type", tr.Type)
			}
		},
	})
	go watcher.Run(ctx)
	app.AttachPower(watcher, store)
	if ctrl := app.Safety(); ctrl != nil {
		go ctrl.Run(ctx)
	}
	if ad := app.AutoDrain(); ad != nil {
		go ad.Run(ctx)
	}

	if cfg.ShouldWrite() {
		if err := cfg.Save(cfg.ConfigPath); err != nil {
			log.Warn("could not write first-run config", "path", cfg.SafeConfigPath(), "err", err)
		} else {
			log.Info("wrote first-run config", "path", cfg.SafeConfigPath())
		}
	}

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
