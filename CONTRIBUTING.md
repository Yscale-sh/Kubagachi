# Contributing to kubagachi

We genuinely welcome contributions of **all** kinds — bug fixes, features, new
critters, docs, a sharper README, a wild idea. First-time contributors and
seasoned ones are equally wanted. There's only one real rule.

## The one rule: use your brain

**Whatever you send, make sure it makes sense.** Read your own diff. Run it.
Ask yourself "does this actually solve a real problem, and would I stand behind
it in review?" before you open the PR or issue.

We love that people build with AI assistants now — and you're encouraged to. But
an AI helping you write code is different from an AI posting on your behalf. A
firehose of auto-generated issues and PRs that nobody actually read wastes
everyone's time and gets closed. So: Like me here, use your own brain to insert relevant sentences, and modifications to the ai generated content where applicable. If you use an agent to write a commit, allow them to signoff as well. 

- **You own everything you submit**, including the parts an agent wrote. If you
  can't explain a line, don't ship it.
- **Every issue and PR needs a human-written section** — a few sentences, in
  *your own words*, saying what you hit and why it matters. This is required
  (the issue forms enforce it). Obvious auto-generated boilerplate with no human
  thought behind it will be closed with a pointer back here.

If an agent is doing the typing, read [`AGENTS.md`](AGENTS.md) — it has the hard
rules for agent-driven contributions.

## Before you open a PR

1. **Make the gate pass.** From the repo root:

   ```sh
   go build ./... && go vet ./... && go test ./...
   ```

   If you touched the **web UI** (`web/`):

   ```sh
   cd web && npm install && npm run build && npm run lint
   ```

   `web/dist` is embedded into the Go binary (`web/embed.go`) and is committed —
   so rebuild it and **commit the updated `web/dist`** with your change.

   If you touched the **Helm chart** (`charts/`):

   ```sh
   helm lint charts/kubagachi && helm template kubagachi charts/kubagachi
   ```

2. **UI change? Attach screenshots.** Any visible change to the browser cockpit
   or the TUI must ship with **before/after screenshots** (a short GIF is even
   better) in the PR. Drive the app with `agent-browser` or any equivalent
   (Playwright, a headless-browser MCP, or a manual screenshot) — just show us
   what it looks like. A UI PR with no visual is not reviewable.

   ```sh
   go run ./cmd/kubagachi --demo --web      # http://127.0.0.1:8787, no cluster needed
   # then screenshot the relevant view
   ```

3. **Keep the diff scoped.** One logical change per PR. Don't bundle a feature
   with a repo-wide reformat, and don't touch files you didn't need to.

4. **Fill out the PR template** — it's short and it's there to make review fast.

## Dev setup

```sh
go run ./cmd/kubagachi --demo            # terminal UI, fake cluster
go run ./cmd/kubagachi --demo --web      # browser cockpit, fake cluster
go run ./cmd/kubagachi -A                # live cluster (current kubeconfig), TUI
go test ./...                            # runs without a TTY
```

The architecture, the `app.ClusterSource` seam, and the project layout are
documented in the [README](README.md#project-layout) — skim it before a
non-trivial change. The short version: **the TUI never imports client-go**, and
`app.ClusterSource` is the only seam between cluster data and presentation.

## Adding a critter

Critters live in [`critters.yaml`](critters.yaml) and are generated with
`pkg/critterforge`. See [`docs/critters.md`](docs/critters.md) and
[`docs/mascot-references.md`](docs/mascot-references.md). Don't regenerate the
whole fleet in a PR — add or change the one critter you mean to.

## Reporting bugs & requesting features

Open an issue via the templates (bug report / feature request). Both ask for a
human-written description — that's the part that helps us most. Search first;
your thing might already be tracked.

## Code of conduct

Be kind, be constructive, assume good faith. It's a cozy project about pixel
critters — keep it that way. Harassment or hostility isn't welcome.

Thanks for helping make Kubernetes a little more alive. 🐱
