# kubagachi-web

The browser cockpit UI — Vite + React 18 + TypeScript + Tailwind. This is the
live dashboard `kubagachi --web` serves: the pixel-critter habitat, resource
drawers, the embedded terminal, and the Flux/Yscale tabs. The build output is
embedded into the Go binary, so end users never touch this directory.

## Dev

Run the kubagachi server (it provides `/api` + `/critters`) and Vite side by side:

```sh
go run ./cmd/kubagachi --demo --web   # server on http://127.0.0.1:8787
npm install && npm run dev            # UI on http://127.0.0.1:5173
```

Vite serves the UI on `:5173` and proxies `/api` and `/critters` to the server
on `:8787` (see `vite.config.ts`). Use `--demo --web` for fake data, or plain
`--web` against your current kubeconfig.

## Build

```sh
npm run build   # tsc -b && vite build → web/dist/
```

`web/dist/` is embedded into the Go binary via `web/embed.go`
(`//go:embed all:dist`), so `go build ./cmd/kubagachi` ships the UI inside the
single binary. Rebuild here whenever the UI changes.

## Tailwind tokens

Theme colors (`bg.*`, `border.*`, `text.*`, `accent`, `status.*`) and the
`font-mono` stack live in the Tailwind config.
