#!/bin/sh
# kubagachi container entrypoint.
#
# Live-cluster mode is driven entirely by config: if the platform injects a
# kubeconfig (the KUBECONFIG_DATA secret), we materialize it to a file, point
# KUBECONFIG at it, and run against the real cluster (-A = all namespaces).
# Until that secret is set, fall back to --demo so the pod serves immediately
# instead of crash-looping on a missing kubeconfig.
set -eu

ADDR="0.0.0.0:${PORT:-8080}"

if [ -n "${KUBECONFIG_DATA:-}" ]; then
  printf '%s' "$KUBECONFIG_DATA" > /tmp/kubeconfig
  export KUBECONFIG=/tmp/kubeconfig
  exec /usr/local/bin/kubagachi --web -A --web-addr "$ADDR" --pixel-critters /critters "$@"
else
  echo "kubagachi: no KUBECONFIG_DATA secret set — serving --demo" >&2
  exec /usr/local/bin/kubagachi --demo --web --web-addr "$ADDR" --pixel-critters /critters "$@"
fi
