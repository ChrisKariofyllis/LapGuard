package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"time"

	"lapguard/internal/config"
)

func runAuth(stdout, stderr io.Writer, args []string) error {
	fs := flag.NewFlagSet("lapguard auth", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lapguard auth <status|generate|rotate|disable> [-config PATH]\n")
		fmt.Fprintf(stderr, "\nBearer token authentication (on by default). Loopback PUT/POST may omit the token.\n")
		fmt.Fprintf(stderr, "Remote clients need Authorization: Bearer. Tokens are shown once on stdout.\n")
		fmt.Fprintf(stderr, "Only a SHA-256 hash is stored in config.json (mode 0600).\n")
		fmt.Fprintf(stderr, "Never put tokens in URLs. This command never starts the HTTP server.\n\n")
		fs.PrintDefaults()
	}
	configPath := fs.String("config", "", "JSON config file (default: ~/.config/lapguard/config.json)")

	if len(args) == 0 {
		fs.Usage()
		return fmt.Errorf("auth requires a subcommand (status, generate, rotate, or disable)")
	}
	sub := args[0]
	rest := args[1:]
	if sub == "-h" || sub == "--help" {
		fs.Usage()
		return nil
	}

	switch sub {
	case "status", "generate", "rotate", "disable":
	default:
		fs.Usage()
		return fmt.Errorf("unknown auth subcommand %q", sub)
	}

	if err := fs.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected extra arguments")
	}

	loadArgs := []string{}
	if *configPath != "" {
		loadArgs = append(loadArgs, "-config", *configPath)
	}
	cfg, err := config.Load(loadArgs)
	if err != nil {
		return err
	}
	if cfg.ConfigPath == "" {
		def, err := config.DefaultPath()
		if err != nil {
			return err
		}
		cfg.ConfigPath = def
	}

	switch sub {
	case "status":
		return writeAuthStatus(stdout, cfg)
	case "generate":
		if cfg.Auth.Enabled && cfg.Auth.TokenHash != "" {
			return fmt.Errorf("authentication is already enabled; use lapguard auth rotate")
		}
		token, err := cfg.GenerateToken(time.Now().UTC())
		if err != nil {
			return err
		}
		if err := cfg.Save(cfg.ConfigPath); err != nil {
			return err
		}
		printTokenOnce(stdout, stderr, token, "generated")
		return nil
	case "rotate":
		token, err := cfg.RotateToken(time.Now().UTC())
		if err != nil {
			return err
		}
		if err := cfg.Save(cfg.ConfigPath); err != nil {
			return err
		}
		printTokenOnce(stdout, stderr, token, "rotated")
		return nil
	case "disable":
		cfg.DisableAuth()
		if err := cfg.Save(cfg.ConfigPath); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "authentication disabled")
		return nil
	default:
		return fmt.Errorf("unknown auth subcommand %q", sub)
	}
}

func writeAuthStatus(stdout io.Writer, cfg config.Config) error {
	view := cfg.Auth.View()
	fmt.Fprintf(stdout, "auth_enabled=%t\n", view.AuthEnabled)
	fmt.Fprintf(stdout, "token_configured=%t\n", view.TokenConfigured)
	fmt.Fprintf(stdout, "allow_loopback_no_token=%t\n", view.AllowLoopbackNoToken)
	if view.TokenCreatedAt != "" {
		fmt.Fprintf(stdout, "token_created_at=%s\n", view.TokenCreatedAt)
	}
	if view.LastRotatedAt != "" {
		fmt.Fprintf(stdout, "last_rotated_at=%s\n", view.LastRotatedAt)
	}
	fmt.Fprintf(stdout, "protect_get=%t\n", view.ProtectGET)
	if warn := cfg.Auth.Warning(); warn != "" {
		fmt.Fprintf(stdout, "warning=%s\n", warn)
	}
	return nil
}

func printTokenOnce(stdout, stderr io.Writer, token, action string) {
	fmt.Fprintf(stderr, "Store this token in a password manager. It will not be shown again (%s).\n", action)
	fmt.Fprintln(stdout, token)
	// Do not attach the token to slog. The redacting handler is extra insurance.
	slog.Info("api token "+action, "auth_enabled", true, "config", "updated")
}
