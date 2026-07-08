// Command kubagachi is a terminal UI that renders a Kubernetes cluster as
// a habitat of animated ASCII critters — one critter per pod.
package main

import (
	"fmt"
	"os"

	"github.com/yscale-sh/kubagachi/internal/app"
)

func main() {
	cfg, err := app.ParseConfig(os.Args[1:])
	if err != nil {
		// flag.ContinueOnError already printed usage to stderr.
		os.Exit(2)
	}
	if err := app.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "kubagachi:", err)
		os.Exit(1)
	}
}
