package main

import (
	"fmt"
	"os"

	"github.com/go-postnest/postnest/internal/config"
	postnestMigrate "github.com/go-postnest/postnest/internal/migrate"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: postnest-migrate <command> [args]")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  up        Apply all pending migrations")
		fmt.Fprintln(os.Stderr, "  down [n]  Rollback n migrations (default 1)")
		fmt.Fprintln(os.Stderr, "  version   Show current migration version")
		fmt.Fprintln(os.Stderr, "  force V   Force set version V (use with caution)")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "up":
		if err := postnestMigrate.Up(cfg.PostgresDSN); err != nil {
			fmt.Fprintf(os.Stderr, "migration up failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("migrations applied successfully")

	case "down":
		steps := 1
		if len(os.Args) > 2 {
			if _, err := fmt.Sscanf(os.Args[2], "%d", &steps); err != nil {
				fmt.Fprintf(os.Stderr, "invalid step count: %v\n", err)
				os.Exit(1)
			}
		}
		if err := postnestMigrate.Down(cfg.PostgresDSN, steps); err != nil {
			fmt.Fprintf(os.Stderr, "migration down failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("migrations rolled back successfully")

	case "version":
		v, dirty, err := postnestMigrate.Version(cfg.PostgresDSN)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get version: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("version: %d, dirty: %v\n", v, dirty)

	case "force":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "force requires a version argument")
			os.Exit(1)
		}
		var v int
		if _, err := fmt.Sscanf(os.Args[2], "%d", &v); err != nil {
			fmt.Fprintf(os.Stderr, "invalid version: %v\n", err)
			os.Exit(1)
		}
		if err := postnestMigrate.Force(cfg.PostgresDSN, v); err != nil {
			fmt.Fprintf(os.Stderr, "force failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("forced version to %d\n", v)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(1)
	}
}
