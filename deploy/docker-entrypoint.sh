#!/bin/sh
# kubagachi container entrypoint.
#
# Live-cluster mode, no secret required: when running inside Kubernetes the pod's
# ServiceAccount token + CA are auto-mounted, so we synthesize a kubeconfig from
# them (tokenFile = the rotating projected token) and run against the real
# cluster (-A = all namespaces). This needs cluster-read RBAC on the pod's SA.
#
# Precedence: an explicit KUBECONFIG_DATA secret (to point at a DIFFERENT
# cluster) wins, then in-cluster SA, then --demo.
set -eu

ADDR="0.0.0.0:${PORT:-8080}"
SA=/var/run/secrets/kubernetes.io/serviceaccount

# 1. Explicit kubeconfig override — only if it's actually a kubeconfig (ignore
#    empty / placeholder values so a dev placeholder can't crash-loop the pod).
if [ -n "${KUBECONFIG_DATA:-}" ] && printf '%s' "$KUBECONFIG_DATA" | grep -q "clusters:"; then
  printf '%s' "$KUBECONFIG_DATA" > /tmp/kubeconfig
  export KUBECONFIG=/tmp/kubeconfig
  exec /usr/local/bin/kubagachi --web -A --web-addr "$ADDR" --pixel-critters /critters
fi

# 2. In-cluster ServiceAccount → synthesize a kubeconfig from the mounted token.
if [ -f "$SA/token" ] && [ -n "${KUBERNETES_SERVICE_HOST:-}" ]; then
  cat > /tmp/kubeconfig <<EOF
apiVersion: v1
kind: Config
clusters:
- name: in-cluster
  cluster:
    server: https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT:-443}
    certificate-authority: ${SA}/ca.crt
users:
- name: in-cluster
  user:
    tokenFile: ${SA}/token
contexts:
- name: in-cluster
  context:
    cluster: in-cluster
    user: in-cluster
current-context: in-cluster
EOF
  export KUBECONFIG=/tmp/kubeconfig
  exec /usr/local/bin/kubagachi --web -A --web-addr "$ADDR" --pixel-critters /critters
fi

# 3. Not in a cluster and no kubeconfig → demo.
echo "kubagachi: no cluster access — serving --demo" >&2
exec /usr/local/bin/kubagachi --demo --web --web-addr "$ADDR" --pixel-critters /critters
