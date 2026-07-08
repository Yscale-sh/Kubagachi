# Connecting kubagachi to a cluster — securely

kubagachi reads a cluster the same way `kubectl` does: it needs **credentials**
(a kubeconfig or a ServiceAccount token) and it acts with **exactly the
permissions those credentials grant**. There is no separate kubagachi login and
no privilege escalation — so the whole security story is *which* credentials you
hand it and *how tightly scoped* they are.

There are three ways in. Pick by where kubagachi runs relative to the cluster.

---

## 1. Local — your own kubeconfig

Running the binary on your machine? It uses the standard kubectl rules:

```sh
go run ./cmd/kubagachi --web              # KUBECONFIG if set, else ~/.kube/config
go run ./cmd/kubagachi --web --context staging   # pick a context
go run ./cmd/kubagachi --web -n team-a     # single namespace  (-A = all)
```

- Loading order: `$KUBECONFIG` (colon-separated list, merged) → else `~/.kube/config`.
  Without `--context`, the current-context is used. A bad context fails fast,
  before the UI starts.
- **kubagachi runs as *you*.** Everything the cockpit can do — view, edit-and-apply,
  delete, exec, port-forward, Helm rollback — is bounded by your kubeconfig's
  RBAC. If you don't want the write surface live, point it at a **read-only
  context** (see the scoped-credential recipe in §3, which works locally too:
  set `KUBECONFIG=./kubagachi.kubeconfig`).
- `--web` binds **`127.0.0.1`** by default — local only. Keep it there for solo
  use; see [Hardening](#hardening) before exposing it.

---

## 2. In-cluster — the pod's ServiceAccount (Helm default)

The default `mode: in-cluster` watches the cluster the pod runs in. The
container entrypoint synthesizes a kubeconfig from the auto-mounted
ServiceAccount token + CA, so **no secret is needed** — but the SA needs RBAC,
which the chart provisions:

```sh
helm install kubagachi oci://ghcr.io/yscale-sh/charts/kubagachi \
  -n kubagachi --create-namespace
```

The chart's default RBAC is **broad on purpose** (a homelab cockpit): cluster-wide
`get/list/watch` on every kind, plus `pods/exec` for the terminal. Scope it down
for anything shared:

```sh
# read-only cockpit — no in-pod terminal, no writes
helm install kubagachi oci://ghcr.io/yscale-sh/charts/kubagachi \
  -n kubagachi --create-namespace \
  --set rbac.podExec=false

# or bring your own least-privilege Role instead of the chart's
helm install kubagachi oci://ghcr.io/yscale-sh/charts/kubagachi \
  -n kubagachi --create-namespace \
  --set rbac.create=false --set serviceAccount.name=my-scoped-sa
```

| Value | Default | Effect |
|-------|---------|--------|
| `rbac.create` | `true` | Create the ClusterRole + binding. `false` → bind your own. |
| `rbac.clusterRead` | `true` | Cluster-wide `get/list/watch`. |
| `rbac.podExec` | `true` | `pods/exec` + `pods/attach` (the terminal). Off → read-only. |

---

## 3. A different cluster — kubeconfig mode (Helm)

To point a kubagachi pod at **another** cluster, run `mode: kubeconfig` and give
it a kubeconfig. The entrypoint honors an explicit kubeconfig over the pod's own
SA, so this targets whatever the kubeconfig points at.

**Do not hand it your admin kubeconfig.** Create a dedicated, scoped, read-only
credential *in the target cluster* and give kubagachi only that.

### Step 1 — mint a scoped read-only credential (run against the TARGET cluster)

```sh
# a dedicated identity bound to the built-in read-only ClusterRole ("view"
# omits Secrets — good for a shared cluster; use a custom role for full read)
kubectl create serviceaccount kubagachi -n kube-system
kubectl create clusterrolebinding kubagachi-view \
  --clusterrole=view --serviceaccount=kube-system:kubagachi

# a long-lived-ish token (k8s ≥ 1.24)
TOKEN=$(kubectl -n kube-system create token kubagachi --duration=8760h)

# build a self-contained kubeconfig that embeds the real CA (no skip-verify)
APISERVER=$(kubectl config view --minify --raw -o jsonpath='{.clusters[0].cluster.server}')
kubectl config view --minify --raw \
  -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' | base64 -d > /tmp/ca.crt

KC=./kubagachi.kubeconfig
kubectl config --kubeconfig=$KC set-cluster target \
  --server="$APISERVER" --certificate-authority=/tmp/ca.crt --embed-certs=true
kubectl config --kubeconfig=$KC set-credentials kubagachi --token="$TOKEN"
kubectl config --kubeconfig=$KC set-context target --cluster=target --user=kubagachi
kubectl config --kubeconfig=$KC use-context target
```

### Step 2 — store it as a Secret and install (in the cluster where kubagachi runs)

Prefer an **existing Secret** over inlining the kubeconfig into Helm values —
inline values land in the Helm release history; a Secret you create yourself does
not.

```sh
kubectl create namespace kubagachi
kubectl create secret generic kubagachi-kubeconfig \
  -n kubagachi --from-file=kubeconfig=./kubagachi.kubeconfig

helm install kubagachi oci://ghcr.io/yscale-sh/charts/kubagachi \
  -n kubagachi \
  --set mode=kubeconfig \
  --set kubeconfig.existingSecret=kubagachi-kubeconfig
```

> The chart can also inline the kubeconfig (`--set-file kubeconfig.data=./kubagachi.kubeconfig`),
> which creates the Secret for you — convenient, but the contents are visible in
> `helm get values`. Use `existingSecret` for anything real.

In `kubeconfig` mode the chart automatically **drops the pod's own SA token**, so
kubagachi can't fall back to the local cluster — it only sees what the kubeconfig
grants.

---

## Hardening

The browser cockpit has **no built-in authentication**, and its operate surface
(edit/apply, delete, exec, port-forward, Helm rollback/uninstall) runs with
whatever the credentials above grant. Treat access to the UI as access to that
RBAC. So:

1. **Don't expose it unauthenticated.** `--web` binds `127.0.0.1`; for a pod, use
   `kubectl port-forward` for solo use. Any LAN/public route must sit behind an
   **authenticating proxy** — oauth2-proxy, Cloudflare Access, Tailscale, an
   Ingress with auth. (This project's own dev cluster gates the public route with
   Cloudflare Access.)
2. **Scope the RBAC to read-only for anything shared.** `--set rbac.podExec=false`
   kills the terminal; drop write verbs / bind the built-in `view` role to make
   the cockpit observe-only. Give kubagachi its **own** ServiceAccount, never a
   shared or admin one.
3. **Prefer `existingSecret`** over inline `kubeconfig.data` so credentials stay
   out of Helm release history.
4. **Keep the container locked down.** The chart already runs non-root (uid 10001),
   read-only rootfs, all caps dropped, `RuntimeDefault` seccomp — leave those on.
5. **Rotate tokens.** The `--duration` above is a ceiling; re-mint and update the
   Secret periodically, or use a short duration with a refresh job.

The rule of thumb: **kubagachi is exactly as privileged as the credential you give
it.** Give it the least it needs to show you what you want to see.
