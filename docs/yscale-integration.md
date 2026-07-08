# kubagachi ↔ yscale integration

**What this repo is.** kubagachi (repo dir `kubekritters`, Go module
`github.com/yscale-sh/kubagachi`) is a **Kubernetes cockpit** — "k9s meets
Freelens" where every pod is a pixel-art tamagotchi critter whose mood *is* the
pod's health. One binary (`cmd/kubagachi`), two faces: a k9s-style **TUI** and a
**browser cockpit** served from `web/` (TypeScript + React 18 + Vite +
Tailwind). It's a general cluster viewer first — logs, exec, delete, Flux
reconcile — and does **not** need yscale to run.

**What yscale is (the other repo).** yscale (`../k3shyperscaler`, module
`github.com/yscale-sh/yscale`) is a **burst-compute control plane**: a SaaS
that rents ephemeral cloud VMs into your Kubernetes cluster as real nodes,
per second. `kubectl apply` a `Workload`, yscale boots a VM, joins it to your
cluster over a WireGuard mesh, runs the pod, reaps the node. yscale is the
*engine*; kubagachi is a *dashboard* — different products.

## The seam: the yscale tab

The browser cockpit has an optional **yscale tab** that renders the tenant's
live burst fleet + spend. It is additive: unset the config and the tab shows a
friendly "not wired up" state; the rest of kubagachi is unaffected.

```
 browser (YscaleTab.tsx)          kubagachi server              yscale central
 ────────────────────────         ────────────────────         ──────────────
   fetch('/api/yscale')   ──────►  internal/app/yscale.go  ───► GET /v1/spend
   renders spend + bursts ◄──────  (proxy; token stays     ◄─── GET /v1/bursts
                                    server-side)                 (Bearer auth)
```

- **`GET /api/yscale`** (`internal/app/yscale.go`) is a kubagachi endpoint — a
  read-only proxy. It calls yscale central's `GET /v1/spend` and `GET /v1/bursts`
  server-side and returns a merged JSON envelope:
  `{configured, url, spend, bursts, count}` (or `{configured:false}` when unset,
  or `{...,error}` when central is unreachable). Burst JSON is passed through
  opaquely so new central fields flow through without a schema change here.
- **The tenant bearer token never reaches the browser.** It's held server-side,
  configured via `-yscale-url` / `YSCALE_URL` + `YSCALE_TOKEN`
  (`internal/app/config.go`). No config → tab disabled.
- Rendered by `web/src/components/YscaleTab.tsx`; the tab is registered across
  `web/src/store/workspace.ts`, `TabsBar.tsx`, `Sidebar.tsx`, `CommandPalette.tsx`,
  `MainView.tsx`, and `web/src/lib/cluster-api.ts`.

## Direction of coupling

kubagachi → yscale, **read-only**. yscale has zero knowledge of kubagachi — it
just serves its normal `/v1/*` API to any authenticated tenant. `/api/yscale` is
a kubagachi route that *calls* yscale; it is **not** a yscale route (yscale's own
routes are `/v1/*`).

## Run it wired up

```sh
# point the cockpit at a yscale central (token stays server-side)
kubagachi --web --yscale-url https://api.yscale.sh
export YSCALE_TOKEN=ysk_...   # the tenant bearer token
# then open the "yscale" tab in the browser cockpit
```
