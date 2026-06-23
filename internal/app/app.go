package app

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jakenesler/kubagachi/internal/critters"
	"github.com/jakenesler/kubagachi/internal/tui"
)

// Run selects a data source from the config, starts streaming cluster
// snapshots over a channel, and hands control to the Bubble Tea program.
// It blocks until the user quits the TUI.
func Run(cfg Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if cfg.App {
		cfg.Web = true // --app implies --web
	}
	// Auto-load the pixel sprite set (Nori, Cartogopher, …) for BOTH the TUI and
	// the web when a critterforge manifest is present, so plain `kubagachi`
	// renders pixel critters. Use --ascii to force the built-in ASCII animals
	// (e.g. on a terminal without inline-image support).
	if !cfg.ASCII && cfg.PixelCritters == "" {
		if _, err := os.Stat("critters/manifest.json"); err == nil {
			cfg.PixelCritters = "critters"
		}
	}

	// Load pixel sprites BEFORE the source builds pods, because the
	// source's Assign() call needs the pixel critter names to deterministically
	// pick from when pixel mode is on.
	if cfg.PixelCritters != "" {
		if err := critters.LoadPixelSprites(cfg.PixelCritters); err != nil {
			return fmt.Errorf("load pixel critters: %w", err)
		}
	}

	source, err := selectSource(cfg)
	if err != nil {
		return err
	}

	snapshots, err := source.Stream(ctx)
	if err != nil {
		return err
	}

	if cfg.Web {
		return runWeb(ctx, cfg, source, snapshots)
	}

	program := tea.NewProgram(
		tui.New(snapshots, source.Label(), source.Actions()),
		tea.WithAltScreen(),
	)
	_, err = program.Run()
	return err
}
