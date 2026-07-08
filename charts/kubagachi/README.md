# kubagachi Helm chart

Deploy [kubagachi](https://github.com/Yscale-sh/Kubagachi) — a
Kubernetes cockpit where every pod is a pixel-art tamagotchi — into your
cluster. Ships the browser cockpit (`--web`): a live habitat dashboard, an
embedded `kubectl exec` terminal, logs / describe / delete, and first-class
Flux reconcile / suspend.

## Install

The chart is published as an **OCI artifact in GHCR**, right next to the
container image — no `helm repo add`, no external repo:

```sh
helm install kubagachi \
  oci://ghcr.io/yscale-sh/charts/kubagachi \
  --version 0.1.0 \
  --namespace kubagachi --create-namespace
```

Then port-forward and open it:

```sh
kubectl -n kubagachi port-forward svc/kubagachi 8080:80
# http://127.0.0.1:8080
```

> **Naming.** CI derives every published name from the repo, so nothing is
> hardcoded — the artifacts are `ghcr.io/yscale-sh/kubagachi` (image) and
> `ghcr.io/yscale-sh/charts/kubagachi` (chart).

## How it runs

The container entrypoint picks its cluster source automatically; the chart's
`mode` value drives it:

| `mode` | Behavior |
|--------|----------|
| `in-cluster` *(default)* | Watches the cluster it runs in via the pod's ServiceAccount token. Needs the RBAC this chart creates (`rbac.create=true`). |
| `demo` | No cluster access — fake habitat. Great for a public demo. The chart drops the SA token so the entrypoint falls back cleanly. |
| `kubeconfig` | Targets a **different** cluster. Provide `kubeconfig.data` (inlined into a Secret) or `kubeconfig.existingSecret`. |

## RBAC

For `in-cluster` mode the chart creates a `ClusterRole` + `ClusterRoleBinding`
granting the pod:

- **cluster-wide read** (`get`/`list`/`watch` on all resources) — the informers
  stream every kind, plus the Flux toolkit CRDs, and
- **`pods/exec` + `pods/attach`** — the embedded in-pod terminal.

This is broad. It's the right default for a homelab cockpit; for anything shared
or production, set `rbac.create=false` and bind your own least-privilege role,
or turn off `rbac.podExec` to make the cockpit read-only.

For `kubeconfig` mode, hardening, and a recipe to mint a scoped read-only
credential, see the [security guide](../../docs/security.md).

## Common overrides

```sh
# Pin an exact image instead of the chart's appVersion
helm install kubagachi oci://ghcr.io/yscale-sh/charts/kubagachi \
  --version 0.1.0 --set image.tag=latest

# Expose via Ingress (Traefik example)
helm install kubagachi oci://ghcr.io/yscale-sh/charts/kubagachi \
  --version 0.1.0 \
  --set ingress.enabled=true \
  --set ingress.className=traefik \
  --set 'ingress.hosts[0].host=kubagachi.example.com' \
  --set 'ingress.hosts[0].paths[0].path=/' \
  --set 'ingress.hosts[0].paths[0].pathType=Prefix'

# Public demo, no cluster access, no RBAC
helm install demo oci://ghcr.io/yscale-sh/charts/kubagachi \
  --version 0.1.0 --set mode=demo --set rbac.create=false
```

## Values

| Key | Default | Description |
|-----|---------|-------------|
| `replicaCount` | `1` | Number of replicas (informer cache is per-pod). |
| `image.repository` | `ghcr.io/yscale-sh/kubagachi` | Image repository (CI stamps this to `ghcr.io/<owner>/<repo>` at publish). |
| `image.tag` | `""` | Image tag; defaults to `.Chart.AppVersion`. |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy. |
| `mode` | `in-cluster` | `in-cluster` \| `demo` \| `kubeconfig`. |
| `kubeconfig.data` | `""` | Inline kubeconfig (kubeconfig mode). |
| `kubeconfig.existingSecret` | `""` | Existing Secret with a kubeconfig. |
| `kubeconfig.secretKey` | `kubeconfig` | Key in the kubeconfig Secret. |
| `serviceAccount.create` | `true` | Create a dedicated ServiceAccount. |
| `serviceAccount.name` | `""` | Override the ServiceAccount name. |
| `serviceAccount.automount` | `true` | Mount the SA token (forced off in demo/kubeconfig modes). |
| `rbac.create` | `true` | Create ClusterRole + ClusterRoleBinding (in-cluster mode). |
| `rbac.clusterRead` | `true` | Grant cluster-wide read. |
| `rbac.podExec` | `true` | Grant `pods/exec` + `pods/attach` (the terminal). |
| `service.type` | `ClusterIP` | Service type. |
| `service.port` | `80` | Service port. |
| `service.targetPort` | `8080` | Container port. |
| `ingress.enabled` | `false` | Create an Ingress. |
| `ingress.className` | `""` | IngressClass name. |
| `ingress.hosts` | `[kubagachi.local]` | Ingress hosts/paths. |
| `ingress.tls` | `[]` | Ingress TLS config. |
| `resources` | requests 50m/64Mi, limit 256Mi | Container resources. |
| `securityContext.readOnlyRootFilesystem` | `true` | Read-only root (writable `/tmp` emptyDir is mounted). |
| `podSecurityContext` | runAsNonRoot uid 10001 | Pod security context. |
| `extraEnv` | `[]` | Extra environment variables. |
| `nodeSelector` / `tolerations` / `affinity` | `{}` | Scheduling controls. |
