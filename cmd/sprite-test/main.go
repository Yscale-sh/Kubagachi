// Command sprite-test prints a single PNG inline using the iTerm2 image
// protocol. Used to verify that the user's terminal (iTerm2, WezTerm, recent
// VS Code with terminal.integrated.experimentalImageSupport enabled, etc.)
// actually renders inline images before we commit to a full pixel-mode TUI.
//
// Usage:
//
//	go run ./cmd/sprite-test critters/postgres/running.png
//	./sprite-test critters/postgres/running.png --width 24
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	width := flag.Int("width", 16, "width in terminal cells")
	height := flag.Int("height", 0, "height in terminal cells (0 = auto from aspect ratio)")
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: sprite-test [--width N] [--height N] <png-path>")
		os.Exit(2)
	}
	path := args[0]

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}

	nameB64 := base64.StdEncoding.EncodeToString([]byte(filepath.Base(path)))
	dataB64 := base64.StdEncoding.EncodeToString(data)

	// iTerm2 inline image protocol — emit a single OSC 1337 sequence:
	//   ESC ] 1337 ; File = name=B64 ; size=N ; width=Wch ; (height=Hch ;)
	//     preserveAspectRatio=1 ; inline=1 : BASE64_BYTES BEL
	// width/height suffix "ch" = cells. Without "ch" they'd be pixels.
	sizeArgs := fmt.Sprintf("width=%d", *width)
	if *height > 0 {
		sizeArgs += fmt.Sprintf(";height=%d", *height)
	}

	fmt.Printf("== sprite-test: %s (%d bytes) ==\n", path, len(data))
	fmt.Printf("== terminal: TERM_PROGRAM=%s ==\n", os.Getenv("TERM_PROGRAM"))
	fmt.Println("")
	fmt.Println("Below should be an inline pixel-art sprite. If you see raw")
	fmt.Println("escape characters or nothing, your terminal isn't rendering")
	fmt.Println("the iTerm2 image protocol.")
	fmt.Println("")
	fmt.Printf("\x1b]1337;File=name=%s;size=%d;%s;preserveAspectRatio=1;inline=1:%s\x07",
		nameB64, len(data), sizeArgs, dataB64)
	fmt.Println("")
	fmt.Println("")
	fmt.Println("== end ==")
}
