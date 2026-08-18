package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"lapguard/internal/config"
	"lapguard/internal/discovery"
)

func runDiscover(stdout, stderr io.Writer, args []string) error {
	fs := flag.NewFlagSet("lapguard discover", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lapguard discover --report [--pretty] [--sysfs-root PATH]\n")
		fmt.Fprintf(stderr, "\nPrint a privacy-safe hardware compatibility report as JSON on stdout.\n")
		fmt.Fprintf(stderr, "Omit serial numbers, usernames, home paths, IPs, MACs, UUIDs, and secrets.\n\n")
		fs.PrintDefaults()
	}

	report := fs.Bool("report", false, "write a privacy-safe JSON compatibility report to stdout")
	pretty := fs.Bool("pretty", false, "indent JSON output")
	sysfsRoot := fs.String("sysfs-root", "", "power_supply sysfs root (overridable for tests and fixtures)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if !*report {
		return fmt.Errorf("discover requires --report (try: lapguard discover --report)")
	}

	ctx := context.Background()
	result, err := discovery.Run(ctx, discovery.Options{
		SysfsRoot: *sysfsRoot,
		Runner:    discovery.ExecRunner(),
	})
	if err != nil {
		return err
	}
	export := discovery.CompatibilityFrom(result, config.Version)
	return discovery.WriteCompatibilityReport(stdout, export, *pretty)
}
