// Package app wires the CLI flags, the data source and the Bubble Tea program
// together. It is the only package that knows about both Kubernetes and the
// TUI; everything below it stays decoupled behind the ClusterSource interface.
package app

import (
	"flag"
	"os"
)

// Config holds the parsed command-line options.
type Config struct {
	Namespace     string // single namespace filter ("" == use context default)
	AllNamespaces bool   // watch every namespace
	Demo          bool   // use fake data instead of a real cluster
	Context       string // kubeconfig context override ("" == current-context)

	// KubeconfigPath is an explicit kubeconfig file ("" == default loading
	// rules). KubeconfigRaw is inline kubeconfig YAML and, when set, wins over
	// the path and default rules — it is populated at runtime by the web UI's
	// settings panel, never from a flag.
	KubeconfigPath string
	KubeconfigRaw  string
	PixelCritters string // path to a critterforge-generated critters dir; empty = auto-load if present
	ASCII         bool   // force the built-in ASCII critters (skip pixel auto-load)
	Web           bool   // serve the browser UI instead of the terminal UI
	WebAddr       string // host:port for the browser UI
	App           bool   // open the browser UI in a chromeless app window

	// YscaleURL is the base URL of a yscale central; when set, the browser
	// cockpit shows a "yscale" tab with the tenant's live burst fleet + spend.
	YscaleURL string
	// YscaleToken is the tenant bearer token for YscaleURL. Env-only
	// (YSCALE_TOKEN) — never a flag, so it can't leak via ps/argv.
	YscaleToken string
}

// ParseConfig parses CLI arguments into a Config. On a flag error the standard
// library prints usage to stderr and the returned error is non-nil.
func ParseConfig(args []string) (Config, error) {
	var c Config
	fs := flag.NewFlagSet("kubagachi", flag.ContinueOnError)

	fs.StringVar(&c.Namespace, "namespace", "", "filter to a single namespace")
	fs.StringVar(&c.Namespace, "n", "", "filter to a single namespace (shorthand)")
	fs.BoolVar(&c.AllNamespaces, "all-namespaces", false, "show pods across all namespaces")
	fs.BoolVar(&c.AllNamespaces, "A", false, "show pods across all namespaces (shorthand)")
	fs.BoolVar(&c.Demo, "demo", false, "run with fake cluster data (no Kubernetes needed)")
	fs.StringVar(&c.Context, "context", "", "kubeconfig context to use (defaults to current-context)")
	fs.StringVar(&c.KubeconfigPath, "kubeconfig", "", "explicit kubeconfig file (defaults to KUBECONFIG / ~/.kube/config)")
	fs.StringVar(&c.PixelCritters, "pixel-critters", "", "directory of critterforge-generated PNGs (auto-loaded from ./critters if present)")
	fs.BoolVar(&c.ASCII, "ascii", false, "force built-in ASCII critters instead of pixel sprites")
	fs.BoolVar(&c.Web, "web", false, "serve a browser UI instead of the terminal UI")
	fs.StringVar(&c.WebAddr, "web-addr", "127.0.0.1:8787", "address for --web mode")
	fs.BoolVar(&c.App, "app", false, "with --web: open the UI in a native-feeling app window")
	fs.StringVar(&c.YscaleURL, "yscale-url", "", "base URL of a yscale central (enables the yscale tab in --web)")

	if err := fs.Parse(args); err != nil {
		return c, err
	}

	// Env fallbacks. The token is env-ONLY (a flag would leak via argv); the URL
	// accepts an env default too so a deploy can wire both without argv.
	if c.YscaleURL == "" {
		c.YscaleURL = os.Getenv("YSCALE_URL")
	}
	c.YscaleToken = os.Getenv("YSCALE_TOKEN")

	return c, nil
}
