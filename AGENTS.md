# AGENTS.md — rules for agent-driven contributions

This file is for **coding agents** (Claude Code, Codex, Cursor, Copilot, Gemini,
whatever you drive) working in this repo. If you're a human, read
[`CONTRIBUTING.md`](CONTRIBUTING.md) first — but skim this too, because you are
responsible for what your agent does.

Agent-assisted work is welcome here. Agent-*unattended* work is not. A human
must own, read, and stand behind every change. These rules make that real.

## What this repo is (ground yourself before editing)

**kubagachi** is a Kubernetes cockpit — a browser web cockpit first (`--web`),
with a k9s-style TUI as a companion. One Go binary, both faces. The one seam that
matters: **`app.ClusterSource`** is the only boundary between cluster data and
presentation. The live informer watcher and the demo generator both stream
`state.ClusterState` snapshots over a channel and expose the same `Actions`
surface. **The TUI never imports client-go**; client-go types never leak past
`internal/k8s`. Respect that seam or your change will be sent back.

Layout is in the [README](README.md#project-layout). Read the files you're about
to touch — don't pattern-match from memory.

## Hard rules (non-negotiable)

1. **A human owns this PR.** No fully-autonomous "agent opened a PR nobody read."
   The human submitter must understand every line and be able to explain it.
2. **Write the human section yourself, not with the agent.** Every PR and issue
   needs a short rationale in the submitter's *own words*. Do not generate it.
   Reviewers can tell, and boilerplate gets closed.
3. **The gate must pass — actually run it, don't claim it.**
   ```sh
   go build ./... && go vet ./... && go test ./...
   ```
   - touched `web/`?  →  `cd web && npm install && npm run build && npm run lint`
     and **commit the rebuilt `web/dist`** (it's embedded via `web/embed.go`).
   - touched `charts/`?  →  `helm lint charts/kubagachi` **and**
     `helm template kubagachi charts/kubagachi` (render both `mode=in-cluster`
     and `mode=demo`).
4. **UI change → screenshots, no exceptions.** If anything visible changes,
   attach before/after screenshots (or a GIF) to the PR. Use `agent-browser`,
   Playwright, a headless-browser MCP, or any equivalent. See
   [Verifying UI](#verifying-ui-changes). A UI diff with no image is incomplete.
5. **Respect the seam.** TUI must not import client-go. Don't leak `k8s.io/...`
   types out of `internal/k8s`. Route new data through `state.ClusterState` and
   new operations through the `Actions` surface.
6. **Keep the diff minimal and scoped.** One logical change. No drive-by
   reformats, no renaming things you weren't asked to, no touching unrelated
   files. Match the surrounding style exactly.
7. **Never fabricate.** No invented APIs, made-up benchmark numbers, or
   "tested on a live cluster" claims you didn't actually run. If you didn't
   verify it, say so.
8. **Don't touch secrets or bulk-regenerate art.** Never commit `.env`, tokens,
   kubeconfigs, or personal paths. Don't regenerate the whole critter fleet —
   change the one critter you mean to (`critters.yaml` + its sprites).

## Skill router — match your task, then act

| You're changing… | Do this |
|---|---|
| **Web cockpit** (`web/`, `internal/app/web.go`) | Build + lint the web app; run `--demo --web`; **screenshot with agent-browser**; commit rebuilt `web/dist`. Aim for bold, alive UI — this is the headline product. |
| **Terminal UI** (`internal/tui/`) | `go test ./internal/tui/...`; run `--demo` to eyeball it; keep cards fixed-size (no reflow on tick). |
| **Cluster data / informers** (`internal/k8s/`) | Keep types behind the seam; add to `state.ClusterState`, not the UI. `go test ./internal/k8s/...`. |
| **Actions / operate surface** (`internal/app/`, web + tui) | New ops go on the `Actions` interface so both UIs get them. Guard writes behind RBAC; note the RBAC needed. |
| **Critters / art** (`critters.yaml`, `critters/`, `pkg/critterforge`) | See [`docs/critters.md`](docs/critters.md) + [`docs/mascot-references.md`](docs/mascot-references.md). Add/adjust one critter; don't re-run the whole pipeline. |
| **Helm chart** (`charts/kubagachi/`) | `helm lint` + `helm template` in every `mode`. Don't hardcode the owner/registry — CI derives published names from the GitHub context. |
| **Deploy / IDP** (`deploy.yaml`) | It's a platform contract; IDP deploy is *coming soon*, not wired for external use. Don't invent new fields. |
| **CI** (`.github/workflows/`) | Keep everything derived from `github.repository` so it stays fork-portable. |

## Verifying UI changes

Bring the cockpit up on fake data (no cluster required) and screenshot the view
you changed:

```sh
go run ./cmd/kubagachi --demo --web        # serves http://127.0.0.1:8787
```

Then, with agent-browser (or your equivalent):

```sh
agent-browser open http://127.0.0.1:8787
agent-browser screenshot --full-page -o before.png    # or after.png
```

Attach `before.png` / `after.png` (or a short GIF) to the PR. Show the state
you actually changed — the habitat grid, the ranch view (`v`), a drawer, the
flux view — not just the landing screen.

## If you get stuck

Read the code, not your training data. `internal/app/source.go` and
`internal/app/demo.go` show the seam end to end; `internal/app/web.go` is the
web server + API. When in doubt, prefer the smallest change that makes the gate
pass and looks right on screen — then let a human review it.

BEFORE OPENING A PR OPEN AN ISSUE, MAKE SURE YOU ARE NOT INTRODUCING BREAKING CHANGES.

I recommend using /simplify and instructing your agent to spawn a sub agent with comprehensive prompts, as well as running /code-review
