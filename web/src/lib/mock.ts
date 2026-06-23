/**
 * Deterministic mock generator for a fake Kubernetes cluster.
 *
 * All randomness flows through a single `mulberry32` PRNG seeded from a
 * string hash, so calling `generateCluster("foo")` twice yields identical
 * snapshots. Crucially, this module performs NO randomness at load time:
 * no Math.random, no Date.now-derived ids, no top-level seeded generation.
 */

import type {
  AccessMode,
  AnyResourceKind,
  Cluster,
  ClusterRole,
  ClusterRoleBinding,
  ConfigMap,
  ContainerSpec,
  CronJob,
  CustomResourceDefinition,
  DaemonSet,
  Deployment,
  Endpoint,
  Event,
  HelmRelease,
  HorizontalPodAutoscaler,
  Ingress,
  Job,
  LimitRange,
  Namespace,
  NetworkPolicy,
  Node,
  PersistentVolume,
  PersistentVolumeClaim,
  Pod,
  PodDisruptionBudget,
  PodStatus,
  ReplicaSet,
  ResourceQuota,
  Role,
  RoleBinding,
  Secret,
  Service,
  ServiceAccount,
  ServicePort,
  StatefulSet,
  StorageClass,
} from "./types";

// ---------------------------------------------------------------------------
// PRNG
// ---------------------------------------------------------------------------

/** Classic mulberry32 PRNG; returns a stateful () => number in [0,1). */
export function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return function () {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** Cheap deterministic 32-bit string hash. */
function hashString(s: string): number {
  let h = 2166136261 >>> 0;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

interface Rng {
  next: () => number;
  int: (min: number, max: number) => number;
  pick: <T>(arr: readonly T[]) => T;
  bool: (probTrue?: number) => boolean;
  /** Roll a value, biased toward `bias` ([0,1]) via a quadratic curve. */
  biased: (bias: number) => number;
}

function makeRng(seedStr: string): Rng {
  const rand = mulberry32(hashString(seedStr));
  const next = () => rand();
  return {
    next,
    int: (min, max) => Math.floor(next() * (max - min + 1)) + min,
    pick: (arr) => arr[Math.floor(next() * arr.length)] as never,
    bool: (probTrue = 0.5) => next() < probTrue,
    biased: (bias) => {
      const r = next();
      // Lower r -> higher chance of falling under `bias`.
      return r * r < bias ? 1 : 0;
    },
  };
}

// ---------------------------------------------------------------------------
// Canonical fixtures
// ---------------------------------------------------------------------------

const CRITTER_NAMES = [
  "api-gateway",
  "user-service",
  "postgres",
  "redis",
  "claude-code",
  "worker",
  "job-processor",
  "email-sender",
  "payment-service",
  "reporter",
  "frontend",
  "websocket",
  "cache",
  "search",
  "ml-inference",
] as const;

export function getCritterCandidates(): readonly string[] {
  return CRITTER_NAMES;
}

const NAMESPACES = [
  "default",
  "data",
  "ai",
  "jobs",
  "kube-system",
  "monitoring",
  "ingress-nginx",
] as const;

const IMAGES: Record<string, string> = {
  "api-gateway": "ghcr.io/kubagachi/api-gateway:v1.18.3",
  "user-service": "ghcr.io/kubagachi/user-service:v0.42.1",
  postgres: "postgres:16-alpine",
  redis: "redis:7.4-alpine",
  "claude-code": "ghcr.io/anthropic-ai/claude-code:1.0.94",
  worker: "ghcr.io/kubagachi/worker:v2.7.0",
  "job-processor": "ghcr.io/kubagachi/jobproc:v1.3.5",
  "email-sender": "ghcr.io/kubagachi/email-sender:v0.9.4",
  "payment-service": "ghcr.io/kubagachi/payments:v3.1.2",
  reporter: "ghcr.io/kubagachi/reporter:v1.0.7",
  frontend: "nginx:1.27-alpine",
  websocket: "ghcr.io/kubagachi/ws-fan:v0.6.0",
  cache: "memcached:1.6.31-alpine",
  search: "opensearchproject/opensearch:2.15.0",
  "ml-inference": "ghcr.io/kubagachi/ml-inference:v0.4.0-cuda12",
};

const NAMESPACE_FOR_CRITTER: Record<string, string> = {
  "api-gateway": "default",
  "user-service": "default",
  postgres: "data",
  redis: "data",
  "claude-code": "ai",
  worker: "jobs",
  "job-processor": "jobs",
  "email-sender": "jobs",
  "payment-service": "default",
  reporter: "monitoring",
  frontend: "default",
  websocket: "default",
  cache: "data",
  search: "data",
  "ml-inference": "ai",
};

const EVENT_REASONS: { reason: string; type: "normal" | "warning"; message: string }[] = [
  { reason: "Scheduled", type: "normal", message: "Successfully assigned pod to node" },
  { reason: "Pulled", type: "normal", message: "Container image pulled" },
  { reason: "Created", type: "normal", message: "Created container" },
  { reason: "Started", type: "normal", message: "Started container" },
  { reason: "Killing", type: "normal", message: "Stopping container" },
  { reason: "NodeReady", type: "normal", message: "Node status is now: Ready" },
  { reason: "FailedScheduling", type: "warning", message: "0/5 nodes are available: 5 Insufficient cpu" },
  { reason: "BackOff", type: "warning", message: "Back-off restarting failed container" },
  { reason: "FailedMount", type: "warning", message: "Unable to attach or mount volumes" },
  { reason: "Unhealthy", type: "warning", message: "Readiness probe failed: HTTP 503" },
  { reason: "FailedPullImage", type: "warning", message: "Failed to pull image: ImagePullBackOff" },
];

// Pod status distribution weights.
const POD_STATUS_WEIGHTS: { status: PodStatus; weight: number }[] = [
  { status: "running", weight: 78 },
  { status: "pending", weight: 13 },
  { status: "terminating", weight: 3 },
  { status: "crashloop", weight: 3 },
  { status: "backoff", weight: 1 },
  { status: "error", weight: 1 },
  { status: "completed", weight: 1 },
];

function pickWeighted<T>(rng: Rng, items: { weight: number; value: T }[]): T {
  const total = items.reduce((s, it) => s + it.weight, 0);
  let r = rng.next() * total;
  for (const it of items) {
    r -= it.weight;
    if (r <= 0) return it.value;
  }
  return items[items.length - 1].value;
}

function pickPodStatus(rng: Rng): PodStatus {
  return pickWeighted(
    rng,
    POD_STATUS_WEIGHTS.map((s) => ({ weight: s.weight, value: s.status })),
  );
}

function phaseFor(status: PodStatus): Pod["phase"] {
  switch (status) {
    case "running":
      return "running";
    case "completed":
      return "completed";
    case "error":
    case "crashloop":
    case "backoff":
      return "error";
    case "pending":
    case "terminating":
      return "pending";
    case "unknown":
      return "unknown";
  }
}

function restartCountFor(rng: Rng, status: PodStatus): number {
  switch (status) {
    case "crashloop":
      return rng.int(7, 142);
    case "backoff":
      return rng.int(0, 3);
    case "error":
      return rng.int(1, 6);
    case "unknown":
      return rng.int(0, 2);
    case "running":
      // Most are 0; occasional restart.
      return rng.next() < 0.15 ? rng.int(1, 4) : 0;
    default:
      return 0;
  }
}

function readyFor(status: PodStatus): boolean {
  return status === "running" || status === "completed";
}

// ---------------------------------------------------------------------------
// Tiny id helpers (deterministic, NOT crypto)
// ---------------------------------------------------------------------------

function uid(rng: Rng): string {
  const part = () =>
    Math.floor(rng.next() * 0xffffffff)
      .toString(16)
      .padStart(8, "0");
  return `${part()}-${part().slice(0, 4)}-${part().slice(0, 4)}-${part().slice(0, 4)}-${part()}${part().slice(0, 4)}`;
}

function podSuffix(rng: Rng): string {
  const chars = "abcdefghijklmnopqrstuvwxyz0123456789";
  let out = "";
  for (let i = 0; i < 5; i++) out += chars[Math.floor(rng.next() * chars.length)];
  return out;
}

function ipv4(rng: Rng, prefix: string): string {
  return `${prefix}.${rng.int(0, 255)}.${rng.int(0, 255)}`;
}

// ---------------------------------------------------------------------------
// Per-resource generators
// ---------------------------------------------------------------------------

export function generateNamespaces(rng: Rng): Namespace[] {
  return NAMESPACES.map((name) => ({
    kind: "Namespace",
    uid: uid(rng),
    name,
    ageSec: rng.int(60 * 60 * 24 * 3, 60 * 60 * 24 * 90),
    phase: "active",
    labels: { "kubernetes.io/metadata.name": name },
  }));
}

export function generateNodes(rng: Rng): Node[] {
  const nodes: Node[] = [];

  const mk = (
    name: string,
    roles: string[],
    cpu: string,
    mem: string,
    arch: "amd64" | "arm64",
    podCount: number,
  ): Node => ({
    kind: "Node",
    uid: uid(rng),
    name,
    ageSec: rng.int(60 * 60 * 24 * 7, 60 * 60 * 24 * 120),
    status: "ready",
    conditions: ["Ready"],
    kubeletVersion: "v1.30.3",
    os: "linux",
    arch,
    cpuCapacity: cpu,
    memCapacity: mem,
    cpuAllocatable: cpu,
    memAllocatable: mem,
    podCount,
    podCapacity: 110,
    roles,
    addresses: [
      { type: "InternalIP", address: ipv4(rng, "10.0") },
      { type: "Hostname", address: name },
    ],
    containerRuntime: "containerd://1.7.20",
    labels: {
      "kubernetes.io/arch": arch,
      "kubernetes.io/os": "linux",
      [`node-role.kubernetes.io/${roles[0]}`]: "",
    },
  });

  nodes.push(mk("node-control-1", ["control-plane"], "4", "16Gi", "amd64", 18));
  nodes.push(mk("node-worker-1", ["worker"], "8", "32Gi", "amd64", 32));
  nodes.push(mk("node-worker-2", ["worker"], "8", "32Gi", "amd64", 30));
  nodes.push(mk("node-worker-3", ["worker"], "16", "64Gi", "arm64", 28));
  nodes.push(mk("node-ingress-1", ["ingress"], "4", "16Gi", "amd64", 12));

  return nodes;
}

interface BuildContext {
  rng: Rng;
  nodes: Node[];
  namespaces: Namespace[];
}

function generatePodsForCritter(ctx: BuildContext, critter: string, count: number): Pod[] {
  const { rng, nodes } = ctx;
  const ns = NAMESPACE_FOR_CRITTER[critter] ?? "default";
  const image = IMAGES[critter] ?? "busybox:1.36";
  const pods: Pod[] = [];

  for (let i = 0; i < count; i++) {
    const status = pickPodStatus(rng);
    const restarts = restartCountFor(rng, status);
    const ready = readyFor(status);
    const node = rng.pick(nodes.filter((n) => !n.roles.includes("control-plane")));
    const containerName = critter;
    const totalContainers = 1 + (rng.next() < 0.25 ? 1 : 0); // occasional sidecar
    const containers: ContainerSpec[] = [];
    for (let c = 0; c < totalContainers; c++) {
      containers.push({
        name: c === 0 ? containerName : `${containerName}-sidecar`,
        image: c === 0 ? image : "ghcr.io/kubagachi/proxy-sidecar:0.4.2",
        ready,
        restartCount: c === 0 ? restarts : 0,
        resources: {
          cpuRequest: "100m",
          cpuLimit: "1",
          memRequest: "128Mi",
          memLimit: "512Mi",
        },
      });
    }
    const readyContainers = ready ? totalContainers : 0;

    pods.push({
      kind: "Pod",
      uid: uid(rng),
      name: `${critter}-${podSuffix(rng)}`,
      namespace: ns,
      ageSec: rng.int(60, 60 * 60 * 24 * 30),
      labels: {
        app: critter,
        "app.kubernetes.io/name": critter,
        "app.kubernetes.io/instance": critter,
      },
      annotations: {
        "kubernetes.io/psp": "restricted",
      },
      status,
      phase: phaseFor(status),
      node: node.name,
      podIP: ipv4(rng, "10.244"),
      hostIP: ipv4(rng, "10.0"),
      critter,
      containers,
      restartCount: restarts,
      ownerKind: "ReplicaSet",
      ownerName: `${critter}-${podSuffix(rng)}`,
      qosClass: "Burstable",
      readyContainers,
      totalContainers,
    });
  }
  return pods;
}

export function generatePods(ctx: BuildContext, total: number): Pod[] {
  const counts = distributeAcrossCritters(ctx.rng, total);
  const pods: Pod[] = [];
  for (const critter of CRITTER_NAMES) {
    pods.push(...generatePodsForCritter(ctx, critter, counts[critter] ?? 0));
  }
  return pods;
}

function distributeAcrossCritters(rng: Rng, total: number): Record<string, number> {
  const counts: Record<string, number> = {};
  // base 2 per critter
  let remaining = total;
  for (const c of CRITTER_NAMES) {
    counts[c] = 2;
    remaining -= 2;
  }
  // distribute the rest randomly weighted by a "popularity" roll
  while (remaining > 0) {
    const c = rng.pick(CRITTER_NAMES);
    counts[c] += 1;
    remaining -= 1;
  }
  return counts;
}

export function generateDeployments(ctx: BuildContext): Deployment[] {
  const { rng } = ctx;
  // Deployments cover everything except DB-flavored critters that get StatefulSets.
  const dpCritters = CRITTER_NAMES.filter(
    (c) => c !== "postgres" && c !== "redis" && c !== "search",
  );
  return dpCritters.map((critter) => {
    const desired = rng.int(2, 6);
    const ready = Math.max(0, desired - (rng.next() < 0.2 ? rng.int(1, 2) : 0));
    return {
      kind: "Deployment",
      uid: uid(rng),
      name: critter,
      namespace: NAMESPACE_FOR_CRITTER[critter] ?? "default",
      ageSec: rng.int(60 * 60, 60 * 60 * 24 * 60),
      replicas: desired,
      readyReplicas: ready,
      updatedReplicas: ready,
      availableReplicas: ready,
      strategy: "RollingUpdate",
      status: ready === desired ? "healthy" : ready === 0 ? "degraded" : "progressing",
      selector: { app: critter },
      image: IMAGES[critter] ?? "busybox:1.36",
      labels: { app: critter },
    };
  });
}

export function generateStatefulSets(ctx: BuildContext): StatefulSet[] {
  const { rng } = ctx;
  const stsCritters = ["postgres", "redis", "search"] as const;
  return stsCritters.map((critter) => {
    const desired = rng.int(2, 3);
    const ready = desired;
    return {
      kind: "StatefulSet",
      uid: uid(rng),
      name: critter,
      namespace: NAMESPACE_FOR_CRITTER[critter] ?? "data",
      ageSec: rng.int(60 * 60 * 24, 60 * 60 * 24 * 90),
      replicas: desired,
      readyReplicas: ready,
      serviceName: `${critter}-headless`,
      status: "healthy",
      image: IMAGES[critter] ?? "busybox:1.36",
      labels: { app: critter },
    };
  });
}

export function generateDaemonSets(ctx: BuildContext): DaemonSet[] {
  const { rng, nodes } = ctx;
  const dsSpecs = [
    { name: "fluent-bit", ns: "monitoring", image: "fluent/fluent-bit:3.1.6" },
    { name: "node-exporter", ns: "monitoring", image: "prom/node-exporter:v1.8.2" },
    { name: "kube-proxy", ns: "kube-system", image: "registry.k8s.io/kube-proxy:v1.30.3" },
  ];
  return dsSpecs.map((s) => ({
    kind: "DaemonSet",
    uid: uid(rng),
    name: s.name,
    namespace: s.ns,
    ageSec: rng.int(60 * 60 * 24, 60 * 60 * 24 * 120),
    desiredNumberScheduled: nodes.length,
    numberReady: nodes.length,
    numberAvailable: nodes.length,
    status: "healthy",
    image: s.image,
    labels: { "app.kubernetes.io/name": s.name },
  }));
}

export function generateReplicaSets(ctx: BuildContext, deployments: Deployment[]): ReplicaSet[] {
  const { rng } = ctx;
  return deployments.map((d) => ({
    kind: "ReplicaSet",
    uid: uid(rng),
    name: `${d.name}-${podSuffix(rng)}`,
    namespace: d.namespace,
    ageSec: d.ageSec - rng.int(0, 600),
    replicas: d.replicas,
    readyReplicas: d.readyReplicas,
    ownerKind: "Deployment",
    ownerName: d.name,
    image: d.image,
    labels: { app: d.name },
  }));
}

export function generateJobs(ctx: BuildContext): Job[] {
  const { rng } = ctx;
  const jobs: Job[] = [];
  for (let i = 0; i < 4; i++) {
    const completions = rng.int(1, 3);
    const succeeded = rng.next() < 0.8 ? completions : rng.int(0, completions);
    const failed = completions - succeeded;
    jobs.push({
      kind: "Job",
      uid: uid(rng),
      name: `nightly-rollup-${i + 1}`,
      namespace: "jobs",
      ageSec: rng.int(60 * 5, 60 * 60 * 24 * 7),
      completions,
      succeeded,
      failed,
      active: 0,
      status: succeeded === completions ? "completed" : failed > 0 ? "failed" : "active",
      image: IMAGES["job-processor"],
      durationSec: rng.int(20, 600),
      labels: { "app.kubernetes.io/component": "batch" },
    });
  }
  return jobs;
}

export function generateCronJobs(ctx: BuildContext): CronJob[] {
  const { rng } = ctx;
  const specs = [
    { name: "nightly-rollup", schedule: "0 2 * * *" },
    { name: "hourly-cleanup", schedule: "@hourly" },
    { name: "weekly-report", schedule: "0 9 * * 1" },
  ];
  return specs.map((s) => ({
    kind: "CronJob",
    uid: uid(rng),
    name: s.name,
    namespace: "jobs",
    ageSec: rng.int(60 * 60 * 24 * 3, 60 * 60 * 24 * 120),
    schedule: s.schedule,
    suspend: false,
    lastScheduleAgeSec: rng.int(60, 60 * 60 * 24),
    activeJobs: 0,
    status: "active",
    image: IMAGES["job-processor"],
    labels: { "app.kubernetes.io/component": "batch" },
  }));
}

// ----- networking -----

export function generateServices(ctx: BuildContext): Service[] {
  const { rng } = ctx;
  const services: Service[] = [];
  for (const c of CRITTER_NAMES) {
    const ports: ServicePort[] = [
      { name: "http", port: 80, targetPort: 8080, protocol: "TCP" },
    ];
    if (c === "postgres") ports[0] = { name: "psql", port: 5432, targetPort: 5432, protocol: "TCP" };
    if (c === "redis") ports[0] = { name: "redis", port: 6379, targetPort: 6379, protocol: "TCP" };
    if (c === "cache") ports[0] = { name: "memcache", port: 11211, targetPort: 11211, protocol: "TCP" };
    if (c === "search") ports[0] = { name: "http", port: 9200, targetPort: 9200, protocol: "TCP" };
    services.push({
      kind: "Service",
      uid: uid(rng),
      name: c,
      namespace: NAMESPACE_FOR_CRITTER[c] ?? "default",
      ageSec: rng.int(60 * 60, 60 * 60 * 24 * 60),
      type: c === "frontend" ? "LoadBalancer" : "ClusterIP",
      clusterIP: ipv4(rng, "10.96"),
      externalIP: c === "frontend" ? `203.0.113.${rng.int(2, 250)}` : undefined,
      ports,
      selector: { app: c },
      labels: { app: c },
    });
  }
  return services;
}

export function generateIngresses(ctx: BuildContext, services: Service[]): Ingress[] {
  const { rng } = ctx;
  const ingressTargets = ["frontend", "api-gateway"];
  const out: Ingress[] = [];
  for (const target of ingressTargets) {
    const svc = services.find((s) => s.name === target);
    if (!svc) continue;
    const host = `${target}.kubagachi.example.com`;
    const ing: Ingress = {
      kind: "Ingress",
      uid: uid(rng),
      name: `${target}-ingress`,
      namespace: svc.namespace,
      ageSec: rng.int(60 * 60, 60 * 60 * 24 * 30),
      className: "nginx",
      hosts: [host],
      rules: [{ host, path: "/", serviceName: svc.name, servicePort: svc.ports[0].port }],
      tls: true,
      address: ipv4(rng, "203.0"),
      labels: { app: target },
    };
    out.push(ing);
  }
  return out;
}

export function generateEndpoints(ctx: BuildContext, services: Service[], pods: Pod[]): Endpoint[] {
  const { rng } = ctx;
  return services.map((svc) => {
    const matching = pods.filter(
      (p) => p.namespace === svc.namespace && p.labels?.app === svc.selector.app,
    );
    return {
      kind: "Endpoint",
      uid: uid(rng),
      name: svc.name,
      namespace: svc.namespace,
      ageSec: svc.ageSec,
      subsets: [
        {
          addresses: matching.slice(0, 8).map((p) => p.podIP ?? "0.0.0.0"),
          ports: svc.ports.map((p) => p.targetPort),
        },
      ],
      targetService: svc.name,
      labels: { app: svc.selector.app },
    } satisfies Endpoint;
  });
}

export function generateNetworkPolicies(ctx: BuildContext): NetworkPolicy[] {
  const { rng } = ctx;
  return [
    {
      kind: "NetworkPolicy",
      uid: uid(rng),
      name: "default-deny",
      namespace: "default",
      ageSec: rng.int(60 * 60 * 24 * 7, 60 * 60 * 24 * 90),
      podSelector: {},
      policyTypes: ["Ingress", "Egress"],
      ingressRules: 0,
      egressRules: 0,
      labels: { "app.kubernetes.io/component": "network" },
    },
    {
      kind: "NetworkPolicy",
      uid: uid(rng),
      name: "allow-frontend-to-api",
      namespace: "default",
      ageSec: rng.int(60 * 60 * 24 * 3, 60 * 60 * 24 * 30),
      podSelector: { app: "api-gateway" },
      policyTypes: ["Ingress"],
      ingressRules: 1,
      egressRules: 0,
      labels: { "app.kubernetes.io/component": "network" },
    },
  ];
}

// ----- config -----

export function generateConfigMaps(ctx: BuildContext): ConfigMap[] {
  const { rng } = ctx;
  const cms: ConfigMap[] = [];
  for (const c of CRITTER_NAMES) {
    cms.push({
      kind: "ConfigMap",
      uid: uid(rng),
      name: `${c}-config`,
      namespace: NAMESPACE_FOR_CRITTER[c] ?? "default",
      ageSec: rng.int(60 * 60, 60 * 60 * 24 * 60),
      dataKeys: ["config.yaml", "logging.json"],
      sizeBytes: rng.int(256, 8192),
      labels: { app: c },
    });
  }
  cms.push({
    kind: "ConfigMap",
    uid: uid(rng),
    name: "kube-root-ca.crt",
    namespace: "kube-system",
    ageSec: rng.int(60 * 60 * 24 * 30, 60 * 60 * 24 * 120),
    dataKeys: ["ca.crt"],
    sizeBytes: 1281,
  });
  return cms;
}

export function generateSecrets(ctx: BuildContext): Secret[] {
  const { rng } = ctx;
  const secrets: Secret[] = [];
  for (const c of CRITTER_NAMES) {
    secrets.push({
      kind: "Secret",
      uid: uid(rng),
      name: `${c}-credentials`,
      namespace: NAMESPACE_FOR_CRITTER[c] ?? "default",
      ageSec: rng.int(60 * 60, 60 * 60 * 24 * 60),
      type: "Opaque",
      dataKeys: ["api-key", "client-secret"],
      sizeBytes: rng.int(64, 1024),
      labels: { app: c },
    });
  }
  secrets.push({
    kind: "Secret",
    uid: uid(rng),
    name: "wildcard-tls",
    namespace: "ingress-nginx",
    ageSec: rng.int(60 * 60 * 24 * 7, 60 * 60 * 24 * 90),
    type: "kubernetes.io/tls",
    dataKeys: ["tls.crt", "tls.key"],
    sizeBytes: 4096,
  });
  return secrets;
}

export function generateResourceQuotas(ctx: BuildContext): ResourceQuota[] {
  const { rng } = ctx;
  return ["data", "ai", "jobs"].map((ns) => ({
    kind: "ResourceQuota",
    uid: uid(rng),
    name: `${ns}-quota`,
    namespace: ns,
    ageSec: rng.int(60 * 60 * 24, 60 * 60 * 24 * 60),
    hard: { "requests.cpu": "32", "requests.memory": "128Gi", pods: "100" },
    used: { "requests.cpu": `${rng.int(4, 24)}`, "requests.memory": `${rng.int(8, 80)}Gi`, pods: `${rng.int(8, 60)}` },
  }));
}

export function generateLimitRanges(ctx: BuildContext): LimitRange[] {
  const { rng } = ctx;
  return ["default", "data", "ai"].map((ns) => ({
    kind: "LimitRange",
    uid: uid(rng),
    name: `${ns}-limits`,
    namespace: ns,
    ageSec: rng.int(60 * 60 * 24, 60 * 60 * 24 * 30),
    limits: [
      {
        type: "Container",
        defaultRequest: { cpu: "100m", memory: "128Mi" },
        defaultLimit: { cpu: "1", memory: "1Gi" },
      },
    ],
  }));
}

// ----- autoscaling -----

export function generateHPAs(ctx: BuildContext, deployments: Deployment[]): HorizontalPodAutoscaler[] {
  const { rng } = ctx;
  return deployments
    .filter((d) => ["api-gateway", "frontend", "user-service", "ml-inference"].includes(d.name))
    .map((d) => {
      const cur = d.replicas;
      const min = Math.max(2, cur - 1);
      const max = cur + 4;
      return {
        kind: "HorizontalPodAutoscaler",
        uid: uid(rng),
        name: d.name,
        namespace: d.namespace,
        ageSec: rng.int(60 * 60, 60 * 60 * 24 * 30),
        targetKind: "Deployment",
        targetName: d.name,
        minReplicas: min,
        maxReplicas: max,
        currentReplicas: cur,
        targetCPUPercent: 70,
        currentCPUPercent: rng.int(20, 90),
        labels: { app: d.name },
      } satisfies HorizontalPodAutoscaler;
    });
}

export function generatePDBs(ctx: BuildContext, deployments: Deployment[]): PodDisruptionBudget[] {
  const { rng } = ctx;
  return deployments
    .filter((d) => d.replicas >= 3)
    .map((d) => ({
      kind: "PodDisruptionBudget",
      uid: uid(rng),
      name: d.name,
      namespace: d.namespace,
      ageSec: rng.int(60 * 60, 60 * 60 * 24 * 30),
      minAvailable: "1",
      currentHealthy: d.readyReplicas,
      desiredHealthy: 1,
      expectedPods: d.replicas,
      selector: { app: d.name },
      labels: { app: d.name },
    }));
}

// ----- storage -----

export function generateStorageClasses(ctx: BuildContext): StorageClass[] {
  const { rng } = ctx;
  return [
    {
      kind: "StorageClass",
      uid: uid(rng),
      name: "standard",
      ageSec: rng.int(60 * 60 * 24 * 30, 60 * 60 * 24 * 200),
      provisioner: "ebs.csi.aws.com",
      reclaimPolicy: "Delete",
      volumeBindingMode: "WaitForFirstConsumer",
      isDefault: true,
    },
    {
      kind: "StorageClass",
      uid: uid(rng),
      name: "fast-ssd",
      ageSec: rng.int(60 * 60 * 24 * 7, 60 * 60 * 24 * 90),
      provisioner: "ebs.csi.aws.com",
      reclaimPolicy: "Delete",
      volumeBindingMode: "Immediate",
      isDefault: false,
    },
  ];
}

interface PvcSeed {
  ns: string;
  name: string;
  capacity: string;
  sc: string;
}

export function generatePVCsAndPVs(ctx: BuildContext): {
  pvcs: PersistentVolumeClaim[];
  pvs: PersistentVolume[];
} {
  const { rng } = ctx;
  const seeds: PvcSeed[] = [
    { ns: "data", name: "postgres-data-postgres-0", capacity: "20Gi", sc: "fast-ssd" },
    { ns: "data", name: "postgres-data-postgres-1", capacity: "20Gi", sc: "fast-ssd" },
    { ns: "data", name: "redis-data-redis-0", capacity: "5Gi", sc: "standard" },
    { ns: "data", name: "search-data-search-0", capacity: "50Gi", sc: "fast-ssd" },
    { ns: "monitoring", name: "prom-storage-0", capacity: "100Gi", sc: "standard" },
  ];
  const accessModes: AccessMode[] = ["ReadWriteOnce"];

  const pvcs: PersistentVolumeClaim[] = [];
  const pvs: PersistentVolume[] = [];

  for (const s of seeds) {
    const pvName = `pv-${podSuffix(rng)}`;
    pvcs.push({
      kind: "PersistentVolumeClaim",
      uid: uid(rng),
      name: s.name,
      namespace: s.ns,
      ageSec: rng.int(60 * 60, 60 * 60 * 24 * 60),
      capacity: s.capacity,
      accessModes,
      storageClassName: s.sc,
      phase: "bound",
      volumeName: pvName,
    });
    pvs.push({
      kind: "PersistentVolume",
      uid: uid(rng),
      name: pvName,
      ageSec: rng.int(60 * 60, 60 * 60 * 24 * 60),
      capacity: s.capacity,
      accessModes,
      reclaimPolicy: "Delete",
      phase: "bound",
      storageClassName: s.sc,
      claimRef: { namespace: s.ns, name: s.name },
    });
  }
  return { pvcs, pvs };
}

// ----- RBAC -----

export function generateServiceAccounts(ctx: BuildContext): ServiceAccount[] {
  const { rng } = ctx;
  const sas: ServiceAccount[] = [];
  for (const ns of NAMESPACES) {
    sas.push({
      kind: "ServiceAccount",
      uid: uid(rng),
      name: "default",
      namespace: ns,
      ageSec: rng.int(60 * 60 * 24, 60 * 60 * 24 * 90),
      secrets: [],
      imagePullSecrets: [],
      automountToken: true,
    });
  }
  for (const c of CRITTER_NAMES) {
    sas.push({
      kind: "ServiceAccount",
      uid: uid(rng),
      name: c,
      namespace: NAMESPACE_FOR_CRITTER[c] ?? "default",
      ageSec: rng.int(60 * 60, 60 * 60 * 24 * 60),
      secrets: [`${c}-token`],
      imagePullSecrets: [],
      automountToken: true,
      labels: { app: c },
    });
  }
  return sas;
}

export function generateRoles(ctx: BuildContext): Role[] {
  const { rng } = ctx;
  return [
    {
      kind: "Role",
      uid: uid(rng),
      name: "config-reader",
      namespace: "default",
      ageSec: rng.int(60 * 60, 60 * 60 * 24 * 60),
      rules: [
        { apiGroups: [""], resources: ["configmaps", "secrets"], verbs: ["get", "list", "watch"] },
      ],
    },
  ];
}

export function generateClusterRoles(ctx: BuildContext): ClusterRole[] {
  const { rng } = ctx;
  return [
    {
      kind: "ClusterRole",
      uid: uid(rng),
      name: "view",
      ageSec: rng.int(60 * 60 * 24 * 30, 60 * 60 * 24 * 200),
      rules: [
        { apiGroups: [""], resources: ["pods", "services", "configmaps"], verbs: ["get", "list", "watch"] },
      ],
    },
    {
      kind: "ClusterRole",
      uid: uid(rng),
      name: "edit",
      ageSec: rng.int(60 * 60 * 24 * 30, 60 * 60 * 24 * 200),
      rules: [
        { apiGroups: ["*"], resources: ["*"], verbs: ["get", "list", "watch", "create", "update", "patch", "delete"] },
      ],
    },
  ];
}

export function generateRoleBindings(ctx: BuildContext): RoleBinding[] {
  const { rng } = ctx;
  return [
    {
      kind: "RoleBinding",
      uid: uid(rng),
      name: "config-reader-binding",
      namespace: "default",
      ageSec: rng.int(60 * 60, 60 * 60 * 24 * 60),
      roleRef: { kind: "Role", name: "config-reader" },
      subjects: [{ kind: "ServiceAccount", name: "api-gateway", namespace: "default" }],
    },
  ];
}

export function generateClusterRoleBindings(ctx: BuildContext): ClusterRoleBinding[] {
  const { rng } = ctx;
  return [
    {
      kind: "ClusterRoleBinding",
      uid: uid(rng),
      name: "view-all",
      ageSec: rng.int(60 * 60 * 24 * 30, 60 * 60 * 24 * 120),
      roleRef: { kind: "ClusterRole", name: "view" },
      subjects: [{ kind: "Group", name: "system:authenticated" }],
    },
  ];
}

// ----- events / CRDs / helm -----

export function generateEvents(ctx: BuildContext, pods: Pod[]): Event[] {
  const { rng } = ctx;
  const events: Event[] = [];
  const sample = pods.slice(0, Math.min(pods.length, 40));
  for (const p of sample) {
    const reason = rng.pick(EVENT_REASONS);
    const ts = rng.int(5, 60 * 60 * 6);
    events.push({
      kind: "Event",
      uid: uid(rng),
      name: `${p.name}.${reason.reason.toLowerCase()}.${podSuffix(rng)}`,
      namespace: p.namespace,
      ageSec: ts,
      type: reason.type,
      reason: reason.reason,
      message: reason.message,
      involvedObject: { kind: "Pod", name: p.name, namespace: p.namespace },
      source: reason.type === "warning" ? "kubelet" : "kube-scheduler",
      count: rng.int(1, 12),
      firstSeenSec: ts + rng.int(0, 600),
      lastSeenSec: ts,
    });
  }
  return events;
}

export function generateCRDs(ctx: BuildContext): CustomResourceDefinition[] {
  const { rng } = ctx;
  const specs: Omit<CustomResourceDefinition, "kind" | "uid" | "ageSec">[] = [
    {
      name: "certificates.cert-manager.io",
      group: "cert-manager.io",
      scope: "Namespaced",
      versions: ["v1"],
      pluralName: "certificates",
      singularName: "certificate",
      listKind: "CertificateList",
      shortNames: ["cert", "certs"],
    },
    {
      name: "issuers.cert-manager.io",
      group: "cert-manager.io",
      scope: "Namespaced",
      versions: ["v1"],
      pluralName: "issuers",
      singularName: "issuer",
      listKind: "IssuerList",
      shortNames: [],
    },
    {
      name: "prometheuses.monitoring.coreos.com",
      group: "monitoring.coreos.com",
      scope: "Namespaced",
      versions: ["v1"],
      pluralName: "prometheuses",
      singularName: "prometheus",
      listKind: "PrometheusList",
      shortNames: ["prom"],
    },
    {
      name: "servicemonitors.monitoring.coreos.com",
      group: "monitoring.coreos.com",
      scope: "Namespaced",
      versions: ["v1"],
      pluralName: "servicemonitors",
      singularName: "servicemonitor",
      listKind: "ServiceMonitorList",
      shortNames: ["smon"],
    },
    {
      name: "workflows.argoproj.io",
      group: "argoproj.io",
      scope: "Namespaced",
      versions: ["v1alpha1"],
      pluralName: "workflows",
      singularName: "workflow",
      listKind: "WorkflowList",
      shortNames: ["wf"],
    },
  ];
  return specs.map((s) => ({
    kind: "CustomResourceDefinition",
    uid: uid(rng),
    ageSec: rng.int(60 * 60 * 24 * 7, 60 * 60 * 24 * 200),
    ...s,
  }));
}

export function generateHelmReleases(ctx: BuildContext): HelmRelease[] {
  const { rng } = ctx;
  const specs = [
    { name: "ingress-nginx", ns: "ingress-nginx", chart: "ingress-nginx", chartVersion: "4.11.2", appVersion: "1.11.2" },
    { name: "kube-prometheus-stack", ns: "monitoring", chart: "kube-prometheus-stack", chartVersion: "62.3.1", appVersion: "0.76.0" },
    { name: "postgresql", ns: "data", chart: "postgresql", chartVersion: "15.5.20", appVersion: "16.4.0" },
    { name: "redis", ns: "data", chart: "redis", chartVersion: "20.0.3", appVersion: "7.4.0" },
    { name: "claude-code", ns: "ai", chart: "claude-code", chartVersion: "0.3.1", appVersion: "1.0.94" },
  ];
  return specs.map((s) => ({
    kind: "HelmRelease",
    uid: uid(rng),
    name: s.name,
    namespace: s.ns,
    ageSec: rng.int(60 * 60 * 24, 60 * 60 * 24 * 120),
    chart: s.chart,
    chartVersion: s.chartVersion,
    appVersion: s.appVersion,
    revision: rng.int(1, 7),
    status: "deployed",
    updatedAgeSec: rng.int(60 * 60, 60 * 60 * 24 * 14),
  }));
}

// ---------------------------------------------------------------------------
// Top-level cluster builder
// ---------------------------------------------------------------------------

export function generateCluster(seed: string = "mock-cluster"): Cluster {
  const rng = makeRng(seed);

  const namespaces = generateNamespaces(rng);
  const nodes = generateNodes(rng);

  const ctx: BuildContext = { rng, nodes, namespaces };

  const totalPods = rng.int(80, 120);
  const pods = generatePods(ctx, totalPods);
  const deployments = generateDeployments(ctx);
  const statefulSets = generateStatefulSets(ctx);
  const daemonSets = generateDaemonSets(ctx);
  const replicaSets = generateReplicaSets(ctx, deployments);
  const jobs = generateJobs(ctx);
  const cronJobs = generateCronJobs(ctx);

  const services = generateServices(ctx);
  const ingresses = generateIngresses(ctx, services);
  const endpoints = generateEndpoints(ctx, services, pods);
  const networkPolicies = generateNetworkPolicies(ctx);

  const configMaps = generateConfigMaps(ctx);
  const secrets = generateSecrets(ctx);
  const resourceQuotas = generateResourceQuotas(ctx);
  const limitRanges = generateLimitRanges(ctx);

  const hpas = generateHPAs(ctx, deployments);
  const pdbs = generatePDBs(ctx, deployments);

  const storageClasses = generateStorageClasses(ctx);
  const { pvcs, pvs } = generatePVCsAndPVs(ctx);

  const serviceAccounts = generateServiceAccounts(ctx);
  const roles = generateRoles(ctx);
  const clusterRoles = generateClusterRoles(ctx);
  const roleBindings = generateRoleBindings(ctx);
  const clusterRoleBindings = generateClusterRoleBindings(ctx);

  const events = generateEvents(ctx, pods);
  const crds = generateCRDs(ctx);
  const helmReleases = generateHelmReleases(ctx);

  // Refresh node pod counts based on actual placement.
  for (const node of nodes) {
    node.podCount = pods.filter((p) => p.node === node.name).length;
  }

  const cluster: Cluster = {
    context: "kubagachi-dev",
    currentNamespace: "default",
    version: "v1.30.3",
    generatedAtSec: 0,
    mode: "mock",
    fluxInstalled: false,
    flux: [],

    pods,
    deployments,
    statefulSets,
    daemonSets,
    replicaSets,
    jobs,
    cronJobs,

    services,
    ingresses,
    endpoints,
    networkPolicies,

    configMaps,
    secrets,
    resourceQuotas,
    limitRanges,

    horizontalPodAutoscalers: hpas,
    podDisruptionBudgets: pdbs,

    persistentVolumes: pvs,
    persistentVolumeClaims: pvcs,
    storageClasses,

    serviceAccounts,
    roles,
    clusterRoles,
    roleBindings,
    clusterRoleBindings,

    nodes,
    namespaces,
    events,

    customResourceDefinitions: crds,
    helmReleases,
  };

  return cluster;
}

// ---------------------------------------------------------------------------
// Helpers exposed for cluster-api (mutation step needs the same logic).
// ---------------------------------------------------------------------------

/** Re-roll one pod's status; used by the live-tick simulator. */
export function mutatePodStatus(pod: Pod, seed: string): Pod {
  const rng = makeRng(`${seed}:${pod.uid}`);
  // 70% running bias.
  const status: PodStatus = rng.next() < 0.7 ? "running" : pickPodStatus(rng);
  const restarts = restartCountFor(rng, status);
  const ready = readyFor(status);
  const newContainers: ContainerSpec[] = pod.containers.map((c, i) => ({
    ...c,
    ready,
    restartCount: i === 0 ? restarts : c.restartCount,
  }));
  return {
    ...pod,
    status,
    phase: phaseFor(status),
    restartCount: restarts,
    readyContainers: ready ? pod.totalContainers : 0,
    containers: newContainers,
  };
}

/** Exported so tests / external consumers can use the same hashing scheme. */
export function seedHash(s: string): number {
  return hashString(s);
}

/** Tiny no-op to keep the unused-import linter happy if a kind is referenced
 * only in type position. Returning a value of the kind discriminator forces
 * structural use. */
export function asKind<K extends AnyResourceKind>(k: K): K {
  return k;
}
