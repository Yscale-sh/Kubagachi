# kubagachi-native

Run the kubagachi cockpit **natively on your Mac (or Linux) — no Docker, no Go**.
A single Rust binary embeds the web UI (`web/dist`) and serves it, plus a live
**read-only** cluster view built from your local kubeconfig.

## Build & run

```sh
cd native
cargo build --release
# run from the repo root so it finds ./critters (or pass --critters-dir)
./"$(cargo metadata --format-version 1 | jq -r .target_directory)"/release/kubagachi-native --open
```

Or simply, from the repo root:

```sh
cargo run --release --manifest-path native/Cargo.toml -- --critters-dir ./critters --open
```

It serves on `http://127.0.0.1:8080`, reads your current kubeconfig context,
and `--open` launches the browser.

## Flags

| flag | default | meaning |
|---|---|---|
| `--port` | `8080` | listen port (binds `127.0.0.1`) |
| `--context` | current | kube context to use |
| `--namespace` | all | limit to one namespace |
| `--critters-dir` | `critters` | directory of critter sprite sheets |
| `--open` | off | open the browser on start |

## Scope (this cut)

- **Live read-only habitat:** pods → critters, nodes, context switching, served
  over the same `/api/snapshot` + `/api/stream` (SSE) + `/api/critters` contract
  the Go server uses, so the identical web UI runs unchanged.
- Pod → critter assignment (incl. the vendor/phoenix pinning) and pod-status
  detection are ported faithfully from the Go server.
- **Operate actions are not wired yet** (YAML apply, exec, helm, flux,
  port-forward): those endpoints return `501`; the UI degrades gracefully. That
  surface is the natural next pass.

## Notes

- Portable Rust (rustls, no OpenSSL/OS-specific deps) — builds on macOS + Linux.
- Sprites are read from `--critters-dir` on disk (the repo's `critters/`), not
  embedded, to keep the binary small. Bundling a pruned sprite set for a truly
  standalone binary is a follow-up.
