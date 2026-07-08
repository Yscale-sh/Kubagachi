<!--
Thanks for contributing to kubagachi! Please fill this out. The "in your own
words" bit below is the part that helps us most — write it yourself, even if an
agent wrote the code. See CONTRIBUTING.md and AGENTS.md.
-->

## What & why (in your own words)

<!-- A few sentences, written by you (a human): what does this change, and what
real problem does it solve? Don't paste an auto-generated summary here. -->

## How I verified

<!-- Which gate commands you actually ran and that they passed:
     go build ./... && go vet ./... && go test ./...
     web touched?   cd web && npm run build && npm run lint  (+ committed web/dist)
     chart touched? helm lint charts/kubagachi && helm template kubagachi charts/kubagachi
-->

## Screenshots (required for any UI change)

<!-- Before/after images or a short GIF. Bring it up with
     `go run ./cmd/kubagachi --demo --web` and capture with agent-browser or
     similar. Delete this section only if the change has no visible UI effect. -->

| Before | After |
|--------|-------|
|        |       |

## Checklist

- [ ] I read my own diff and I **stand behind every line** (including anything an agent wrote).
- [ ] The gate passes locally (`go build`/`go vet`/`go test`; web build+lint if `web/` changed; `helm lint` if `charts/` changed).
- [ ] Any **UI change includes before/after screenshots** above.
- [ ] If `web/` changed, I rebuilt and committed `web/dist`.
- [ ] The diff is **scoped** — no unrelated reformatting, no secrets, no personal paths.
- [ ] If an agent wrote part of this, I followed [`AGENTS.md`](../AGENTS.md).
