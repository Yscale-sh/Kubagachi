// Command critterforge generates pixel-art critter sprites via an image model
// (Gemini by default, OpenAI optional) and caches results on disk.
//
// The canonical pipeline is two sheet-based stages — no per-state single tiles:
//
//	critterforge sheet     --only nori --provider gemini --quality high   # keyed status sheet
//	go run ./cmd/spriteanim --only nori --provider gemini --quality high   # per-state 8-frame decks
//
// Set provider credentials/config in the environment or a .env file in the
// working directory. The CLI auto-loads .env at startup.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/yscale-sh/kubagachi/pkg/critterforge"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "critterforge:", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	if len(os.Args) < 2 {
		usage()
		return errors.New("missing subcommand")
	}
	switch os.Args[1] {
	case "generate":
		return runGenerate(os.Args[2:])
	case "sheet":
		return runSheet(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown subcommand: %s", os.Args[1])
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  critterforge sheet     [flags]  # keyed-status sprite sheet (the canonical first stage)
  critterforge generate  [flags]  # legacy per-state single tiles (not the canonical flow)

  Canonical pipeline (no single-tile gens):
    critterforge sheet     --only NAME --provider gemini --quality high
    go run ./cmd/spriteanim --only NAME --provider gemini --quality high

shared flags:
  --provider NAME      image provider: gemini (default) | openai
  --model MODEL_ID     image model id (default: the provider's default)
  --quality QUALITY    low | medium | high  (gemini: 1K | 2K | 4K; default medium — we ship at 1024x128, so 4K is wasted; use high for a 4K master)
  --only NAMES         comma-separated critter names (default: all)
  --in PATH            input manifest (default: critters.yaml)
  --out DIR            output directory (default: critters)
  --force              regenerate even when cached / overwrite existing

GEMINI_API_KEY or OPENAI_API_KEY / OPEN_AI_API_KEY can be set in the env or a .env file.
`)
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	input := fs.String("in", "critters.yaml", "input manifest path")
	output := fs.String("out", "critters", "output directory")
	force := fs.Bool("force", false, "regenerate even when cached")
	concurrency := fs.Int("concurrency", 4, "critters in flight at once")
	provider := fs.String("provider", "gemini", "image provider: gemini | openai")
	model := fs.String("model", "", "image model id (default: provider's default)")
	size := fs.String("size", "1024x1024", "image size (openai WxH) / aspect (gemini)")
	quality := fs.String("quality", "medium", "image quality: low | medium | high")
	only := fs.String("only", "", "comma-separated critter names to generate (default: all)")
	styleRef := fs.String("style-ref", "", "path to a global style-reference PNG attached to every call (overrides manifest)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var styleRefPNG []byte
	if *styleRef != "" {
		data, err := os.ReadFile(*styleRef)
		if err != nil {
			return fmt.Errorf("read --style-ref: %w", err)
		}
		styleRefPNG = data
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	imageModel, err := critterforge.BuildImageModel(*provider, *model, *size, *quality)
	if err != nil {
		return err
	}

	forge, err := critterforge.New(critterforge.Options{
		Model:             imageModel,
		OutputDir:         *output,
		StyleReferencePNG: styleRefPNG,
		Force:             *force,
		Concurrency:       *concurrency,
		Logger:            stdLogger{},
	})
	if err != nil {
		return err
	}
	return forge.GenerateOnly(ctx, *input, splitCSV(*only))
}

func runSheet(args []string) error {
	fs := flag.NewFlagSet("sheet", flag.ContinueOnError)
	input := fs.String("in", "critters.yaml", "input manifest path")
	output := fs.String("out", "critters", "output directory")
	force := fs.Bool("force", false, "overwrite existing keyed sheets")
	provider := fs.String("provider", "gemini", "image provider: gemini | openai")
	model := fs.String("model", "", "image model id (default: provider's default)")
	quality := fs.String("quality", "medium", "image quality: low | medium | high")
	only := fs.String("only", "", "comma-separated critter names to generate (default: all)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	imageModel, err := critterforge.BuildImageModel(*provider, *model, critterforge.SheetSize, *quality)
	if err != nil {
		return err
	}

	manifest, err := critterforge.LoadInputManifest(*input)
	if err != nil {
		return err
	}
	specs := critterforge.SheetSpecsFromInput(manifest)
	if len(specs) == 0 {
		return fmt.Errorf("no critters in %s declare a `mascot` field (required for sheet generation)", *input)
	}
	if names := splitCSV(*only); len(names) > 0 {
		wanted := make(map[string]bool, len(names))
		for _, n := range names {
			wanted[n] = true
		}
		filtered := specs[:0]
		for _, s := range specs {
			if wanted[s.Name] {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("no critters in %s matched %v", *input, names)
		}
		specs = filtered
	}

	return critterforge.GenerateSheets(ctx, critterforge.SheetOptions{
		Model:     imageModel,
		OutputDir: *output,
		Force:     *force,
		Logger:    stdLogger{},
	}, specs)
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

type stdLogger struct{}

func (stdLogger) Logf(format string, args ...any) {
	log.Printf(format, args...)
}
