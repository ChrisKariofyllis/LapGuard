package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"lapguard/internal/tailscale"
)

func runTailscale(stdout, stderr io.Writer, args []string) error {
	return runTailscaleOpts(stdout, stderr, args, tailscale.Options{})
}

func runTailscaleOpts(stdout, stderr io.Writer, args []string, opts tailscale.Options) error {
	fs := flag.NewFlagSet("lapguard tailscale", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lapguard tailscale <instructions|check> [--pretty]\n")
		fmt.Fprintf(stderr, "\nRead-only Tailscale Serve diagnostics.\n")
		fmt.Fprintf(stderr, "LapGuard stays on 127.0.0.1:8585. This CLI never runs sudo,\n")
		fmt.Fprintf(stderr, "never configures Serve or Funnel, and never changes Tailscale state.\n\n")
		fs.PrintDefaults()
	}
	pretty := fs.Bool("pretty", false, "indent JSON output (check only)")

	if len(args) == 0 {
		fs.Usage()
		return fmt.Errorf("tailscale requires a subcommand (instructions or check)")
	}

	sub := args[0]
	rest := args[1:]
	if sub == "-h" || sub == "--help" {
		fs.Usage()
		return nil
	}
	if sub == "-pretty" || sub == "--pretty" {
		fs.Usage()
		return fmt.Errorf("tailscale requires a subcommand (instructions or check)")
	}

	switch sub {
	case "instructions":
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if *pretty {
			return fmt.Errorf("instructions does not take --pretty")
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("instructions does not take extra arguments")
		}
		_, err := io.WriteString(stdout, tailscale.InstructionsText())
		return err
	case "check":
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("check does not take extra arguments")
		}
		report, err := tailscale.Check(context.Background(), opts)
		if err != nil {
			return err
		}
		return tailscale.WriteReport(stdout, report, *pretty)
	default:
		fs.Usage()
		return fmt.Errorf("unknown tailscale subcommand %q (try instructions or check)", sub)
	}
}
