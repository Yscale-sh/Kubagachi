/**
 * cluster-api — bridge between the React UI and the kubagachi Go server.
 *
 * Live path:
 *   GET /api/snapshot   one-shot full snapshot
 *   GET /api/stream     SSE; each `data:` line is a full snapshot JSON
 *
 * Fallback path (pure `npm run dev`, no Go server): the deterministic local
 * mock generator from mock.ts, with the same gentle per-tick pod mutations
 * as before. No client-side random mutations happen when the server is
 * reachable.
 *
 * Also hosts the imperative action endpoints (flux actions, pod delete,
 * log fetch) used by FluxTab / PodList / DetailDrawer.
 */

import { generateCluster, mutatePodStatus } from "./mock";
import type {
  Cluster,
  ConfigMap,
  ContainerSpec,
  Deployment,
  Event,
  FluxObject,
  FluxReady,
  Namespace,
  Node,
  Phase,
  Pod,
  PodStatus,
  Service,
  ServicePort,
  ServiceType,
  WorkloadStatus,
} from "./types";

// ---------------------------------------------------------------------------
// Server wire types (shape of /api/snapshot + /api/stream payloads)
// ---------------------------------------------------------------------------

interface ServerContainer {
  name?: string;
  image?: string;
  ready?: boolean;
  restartCount?: number;
  state?: string;
  reason?: string;
}

interface ServerPod {
  uid?: string;
  name?: string;
  namespace?: string;
  critter?: string;
  status?: string;
  /** Animation deck to play (health state or a workload animation). */
  critterState?: string;
  phase?: string;
  reason?: string;
  node?: string;
  ip?: string;
  /** "1/1" */
  ready?: string;
  restarts?: number;
  ageSec?: number;
  /** "Kind/name" or bare name */
  owner?: string;
  /** usage millicores; -1 == unknown */
  cpuMilli?: number;
  /** usage bytes; -1 == unknown */
  memBytes?: number;
  containers?: ServerContainer[];
}

interface ServerNode {
  name?: string;
  status?: string;
  cpu?: string;
  mem?: string;
  /** utilisation 0..100; -1 == unknown */
  cpuPct?: number;
  memPct?: number;
  podCount?: number;
}

interface ServerEvent {
  type?: string;
  reason?: string;
  /** "Kind/name" */
  object?: string;
  /** Namespace of the involved object ("" / absent for cluster-scoped). */
  namespace?: string;
  message?: string;
  /** Relative age like "12s", "3m", "2h" */
  time?: string;
}

interface ServerFlux {
  kind?: string;
  name?: string;
  namespace?: string;
  ready?: string;
  suspended?: boolean;
  revision?: string;
  source?: string;
  message?: string;
  age?: string;
}

interface ServerDeployment {
  name?: string;
  namespace?: string;
  replicas?: number;
  ready?: number;
  updated?: number;
  available?: number;
  image?: string;
  /** "k=v,k=v" */
  selector?: string;
  ageSec?: number;
}

interface ServerServicePort {
  name?: string;
  port?: number;
  targetPort?: number;
  nodePort?: number;
  protocol?: string;
}

interface ServerService {
  name?: string;
  namespace?: string;
  type?: string;
  clusterIP?: string;
  externalIP?: string;
  ports?: ServerServicePort[];
  /** "k=v,k=v" */
  selector?: string;
  ageSec?: number;
}

interface ServerConfigMap {
  name?: string;
  namespace?: string;
  keys?: string[];
  dataBytes?: number;
  ageSec?: number;
}

interface ServerSnapshot {
  mode?: string;
  context?: string;
  currentNamespace?: string;
  fluxInstalled?: boolean;
  metricsInstalled?: boolean;
  pods?: ServerPod[];
  nodes?: ServerNode[];
  namespaces?: string[];
  events?: ServerEvent[];
  flux?: ServerFlux[];
  deployments?: ServerDeployment[];
  services?: ServerService[];
  configMaps?: ServerConfigMap[];
}

// ---------------------------------------------------------------------------
// Small parsing helpers
// ---------------------------------------------------------------------------

const POD_STATUSES: ReadonlySet<string> = new Set([
  "running",
  "pending",
  "completed",
  "crashloop",
  "backoff",
  "terminating",
  "unknown",
  "error",
]);

function toPodStatus(raw: string | undefined): PodStatus {
  const s = (raw ?? "").toLowerCase();
  return (POD_STATUSES.has(s) ? s : "unknown") as PodStatus;
}

function toPhase(raw: string | undefined): Phase {
  const s = (raw ?? "").toLowerCase();
  switch (s) {
    case "running":
      return "running";
    case "pending":
      return "pending";
    case "succeeded":
    case "completed":
      return "completed";
    case "failed":
    case "error":
      return "error";
    default:
      return "unknown";
  }
}

/** Parse "1/2" → [1, 2]; degrade to [0, 1] on garbage. */
function parseReady(s: string | undefined): [number, number] {
  const m = /^(\d+)\s*\/\s*(\d+)$/.exec((s ?? "").trim());
  if (!m) return [0, 1];
  return [Number(m[1]), Math.max(1, Number(m[2]))];
}

/** Parse "Kind/name" → { kind, name }. Bare strings become { name }. */
function parseOwner(s: string | undefined): { kind?: string; name?: string } {
  const raw = (s ?? "").trim();
  if (!raw || raw === "-") return {};
  const idx = raw.indexOf("/");
  if (idx === -1) return { name: raw };
  return { kind: raw.slice(0, idx), name: raw.slice(idx + 1) };
}

/** Parse a compact age string ("12s", "3m", "2h5m", "5d") into seconds. */
export function parseAgeSec(s: string | undefined): number {
  if (!s) return 0;
  const re = /(\d+)\s*([smhd])/g;
  let total = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(s)) !== null) {
    const n = Number(m[1]);
    switch (m[2]) {
      case "s": total += n; break;
      case "m": total += n * 60; break;
      case "h": total += n * 3600; break;
      case "d": total += n * 86400; break;
    }
  }
  return total;
}

function hash32(s: string): number {
  let h = 2166136261 >>> 0;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

// ---------------------------------------------------------------------------
// Server snapshot → rich typed Cluster
// ---------------------------------------------------------------------------

function toPod(p: ServerPod): Pod {
  const [readyN, totalN] = parseReady(p.ready);
  const owner = parseOwner(p.owner);
  const containers: ContainerSpec[] = (p.containers ?? []).map((c, i) => ({
    name: c.name ?? `container-${i}`,
    image: c.image ?? "—",
    ready: c.ready ?? false,
    restartCount: c.restartCount ?? 0,
    state: c.state,
    reason: c.reason,
  }));
  const name = p.name ?? "pod";
  return {
    kind: "Pod",
    uid: p.uid ?? `pod-${hash32(`${p.namespace}/${name}`)}`,
    name,
    namespace: p.namespace,
    ageSec: p.ageSec ?? 0,
    // Tag the app label with the owner so derived-deployment clamping works.
    labels: owner.name ? { app: owner.name } : undefined,
    status: toPodStatus(p.status),
    phase: toPhase(p.phase),
    node: p.node ?? "—",
    podIP: p.ip || undefined,
    critter: p.critter || name,
    critterState: p.critterState,
    containers,
    restartCount: p.restarts ?? 0,
    ownerKind: owner.kind,
    ownerName: owner.name,
    cpuMilli: p.cpuMilli ?? -1,
    memBytes: p.memBytes ?? -1,
    readyContainers: readyN,
    totalContainers: Math.max(totalN, containers.length || 1),
  };
}

function toNode(n: ServerNode): Node {
  const status = (n.status ?? "").toLowerCase() === "ready" ? "ready" : "notready";
  const name = n.name ?? "node";
  return {
    kind: "Node",
    uid: `node-${name}`,
    name,
    ageSec: 0,
    roles: [],
    status,
    conditions: [status === "ready" ? "Ready" : "NotReady"],
    kubeletVersion: "—",
    os: "linux",
    arch: "amd64",
    cpuCapacity: n.cpu ?? "—",
    memCapacity: n.mem ?? "—",
    cpuAllocatable: n.cpu ?? "—",
    memAllocatable: n.mem ?? "—",
    podCount: n.podCount ?? 0,
    podCapacity: 110,
    addresses: [],
    containerRuntime: "—",
    cpuPct: n.cpuPct ?? -1,
    memPct: n.memPct ?? -1,
  };
}

function toNamespace(name: string): Namespace {
  return {
    kind: "Namespace",
    uid: `ns-${name}`,
    name,
    ageSec: 0,
    phase: "active",
  };
}

function toEvent(e: ServerEvent, i: number): Event {
  const obj = parseOwner(e.object);
  const ageSec = parseAgeSec(e.time);
  const type = (e.type ?? "").toLowerCase() === "warning" ? "warning" : "normal";
  const key = hash32(`${e.reason}|${e.object}|${e.message}`);
  return {
    kind: "Event",
    uid: `ev-${key.toString(16)}-${i}`,
    name: e.reason ?? "Event",
    namespace: e.namespace || undefined,
    ageSec,
    type,
    reason: e.reason ?? "—",
    message: e.message ?? "",
    involvedObject: {
      kind: obj.kind ?? "—",
      name: obj.name ?? e.object ?? "—",
    },
    source: "—",
    count: 1,
    firstSeenSec: ageSec,
    lastSeenSec: ageSec,
  };
}

function toFlux(f: ServerFlux): FluxObject {
  const ready: FluxReady =
    f.ready === "True" ? "True" : f.ready === "False" ? "False" : "-";
  const kind = f.kind ?? "Kustomization";
  const name = f.name ?? "—";
  const namespace = f.namespace ?? "—";
  return {
    uid: `flux-${kind}-${namespace}-${name}`,
    kind,
    name,
    namespace,
    ready,
    suspended: !!f.suspended,
    revision: f.revision ?? "—",
    source: f.source ?? "—",
    message: f.message ?? "",
    age: f.age ?? "—",
  };
}

/**
 * Derive Deployment rows from pod owners: group by owner+namespace,
 * replicas = pod count, readyReplicas = running count.
 */
function deriveDeployments(pods: Pod[]): Deployment[] {
  const groups = new Map<string, Pod[]>();
  for (const p of pods) {
    if (!p.ownerName) continue;
    const key = `${p.namespace ?? ""}|${p.ownerName}`;
    const arr = groups.get(key);
    if (arr) arr.push(p);
    else groups.set(key, [p]);
  }
  const out: Deployment[] = [];
  for (const [key, group] of groups) {
    const [namespace, name] = key.split("|");
    const replicas = group.length;
    const ready = group.filter((p) => p.status === "running").length;
    const bad = group.some(
      (p) => p.status === "crashloop" || p.status === "error" || p.status === "backoff",
    );
    const status: WorkloadStatus =
      ready === replicas ? "healthy" : bad ? "degraded" : "progressing";
    out.push({
      kind: "Deployment",
      uid: `deploy-${namespace}-${name}`,
      name,
      namespace: namespace || undefined,
      ageSec: Math.max(0, ...group.map((p) => p.ageSec)),
      replicas,
      readyReplicas: ready,
      updatedReplicas: replicas,
      availableReplicas: ready,
      strategy: "RollingUpdate",
      status,
      selector: { app: name },
      image: group[0]?.containers[0]?.image ?? "—",
    });
  }
  return out.sort((a, b) =>
    `${a.namespace}/${a.name}`.localeCompare(`${b.namespace}/${b.name}`),
  );
}

/** Parse a "k=v,k=v" selector string into a label map. */
function parseSelector(s: string | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  if (!s) return out;
  for (const pair of s.split(",")) {
    const i = pair.indexOf("=");
    if (i === -1) continue;
    const k = pair.slice(0, i).trim();
    if (k) out[k] = pair.slice(i + 1).trim();
  }
  return out;
}

function toDeployment(d: ServerDeployment): Deployment {
  const replicas = d.replicas ?? 0;
  const ready = d.ready ?? 0;
  const status: WorkloadStatus =
    replicas > 0 && ready >= replicas ? "healthy" : ready === 0 ? "degraded" : "progressing";
  const name = d.name ?? "deployment";
  const namespace = d.namespace || undefined;
  return {
    kind: "Deployment",
    uid: `deploy-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: d.ageSec ?? 0,
    replicas,
    readyReplicas: ready,
    updatedReplicas: d.updated ?? ready,
    availableReplicas: d.available ?? ready,
    strategy: "RollingUpdate",
    status,
    selector: parseSelector(d.selector),
    image: d.image ?? "—",
  };
}

const SERVICE_TYPES: ReadonlySet<string> = new Set([
  "ClusterIP",
  "NodePort",
  "LoadBalancer",
  "ExternalName",
  "Headless",
]);

function toService(s: ServerService): Service {
  const name = s.name ?? "service";
  const namespace = s.namespace || undefined;
  const ports: ServicePort[] = (s.ports ?? []).map((p) => ({
    name: p.name || undefined,
    port: p.port ?? 0,
    targetPort: p.targetPort ?? p.port ?? 0,
    nodePort: p.nodePort || undefined,
    protocol: p.protocol === "UDP" ? "UDP" : "TCP",
  }));
  return {
    kind: "Service",
    uid: `svc-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: s.ageSec ?? 0,
    type: (SERVICE_TYPES.has(s.type ?? "") ? s.type : "ClusterIP") as ServiceType,
    clusterIP: s.clusterIP ?? "—",
    externalIP: s.externalIP || undefined,
    ports,
    selector: parseSelector(s.selector),
  };
}

function toConfigMap(c: ServerConfigMap): ConfigMap {
  const name = c.name ?? "configmap";
  const namespace = c.namespace || undefined;
  return {
    kind: "ConfigMap",
    uid: `cm-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: c.ageSec ?? 0,
    dataKeys: c.keys ?? [],
    sizeBytes: c.dataBytes ?? 0,
  };
}

function snapshotToCluster(s: ServerSnapshot): Cluster {
  const pods = (s.pods ?? []).map(toPod);
  const mode =
    s.mode === "demo" ? "demo" : s.mode === "cluster" ? "cluster" : "live";
  const cluster: Cluster = {
    context: s.context ?? "cluster",
    currentNamespace: s.currentNamespace ?? "default",
    version: "—",
    generatedAtSec: 0,
    mode,
    fluxInstalled: !!s.fluxInstalled,
    metricsInstalled: !!s.metricsInstalled,
    flux: (s.flux ?? []).map(toFlux),

    pods,
    deployments: s.deployments ? s.deployments.map(toDeployment) : deriveDeployments(pods),
    statefulSets: [],
    daemonSets: [],
    replicaSets: [],
    jobs: [],
    cronJobs: [],

    services: (s.services ?? []).map(toService),
    ingresses: [],
    endpoints: [],
    networkPolicies: [],

    configMaps: (s.configMaps ?? []).map(toConfigMap),
    secrets: [],
    resourceQuotas: [],
    limitRanges: [],

    horizontalPodAutoscalers: [],
    podDisruptionBudgets: [],

    persistentVolumes: [],
    persistentVolumeClaims: [],
    storageClasses: [],

    serviceAccounts: [],
    roles: [],
    clusterRoles: [],
    roleBindings: [],
    clusterRoleBindings: [],

    nodes: (s.nodes ?? []).map(toNode),
    namespaces: (s.namespaces ?? []).map(toNamespace),
    events: (s.events ?? []).map(toEvent),

    customResourceDefinitions: [],
    helmReleases: [],
  };
  return recomputeDerived(cluster);
}

/**
 * Recompute fields whose values are derived from another collection.
 * node.podCount is set from pods[].node; deployments[].readyReplicas is
 * clamped against the actual pods matching their app label.
 */
function recomputeDerived(c: Cluster): Cluster {
  const nodes = c.nodes.map((n) => ({
    ...n,
    podCount: c.pods.filter((p) => p.node === n.name).length,
  }));

  const deployments = c.deployments.map((d) => {
    const owned = c.pods.filter(
      (p) => p.namespace === d.namespace && p.labels?.app === d.name,
    );
    const ready = owned.filter((p) => p.status === "running").length;
    return {
      ...d,
      readyReplicas: Math.min(d.replicas, ready),
      availableReplicas: Math.min(d.replicas, ready),
    };
  });

  return { ...c, nodes, deployments };
}

// ---------------------------------------------------------------------------
// Snapshot loading
// ---------------------------------------------------------------------------

async function fetchSnapshot(): Promise<Cluster | null> {
  try {
    const resp = await fetch("/api/snapshot", {
      headers: { Accept: "application/json" },
    });
    if (!resp.ok) return null;
    const snap = (await resp.json()) as ServerSnapshot;
    // Guard against a dev-server HTML fallback masquerading as JSON.
    if (!snap || typeof snap !== "object" || !Array.isArray(snap.pods)) return null;
    return snapshotToCluster(snap);
  } catch {
    return null;
  }
}

/**
 * Fetch the cluster snapshot from the Go server; if unreachable, fall back
 * to the deterministic local mock generator.
 */
export async function loadCluster(seed: string = "mock-cluster"): Promise<Cluster> {
  const live = await fetchSnapshot();
  if (live) return live;
  return generateCluster(seed);
}

// ---------------------------------------------------------------------------
// Live updates
// ---------------------------------------------------------------------------

export type ClusterTickListener = (next: Cluster) => void;

export interface SubscribeOptions {
  /** Mock-mode polling cadence in ms; default 3000. */
  intervalMs?: number;
  /** Seed used to drive the per-tick mock mutation. */
  initialSeed?: string;
}

const STREAM_BACKOFF_MIN_MS = 1000;
const STREAM_BACKOFF_MAX_MS = 30000;

/**
 * Subscribe to cluster updates.
 *
 * If the Go server is reachable, an EventSource on /api/stream supplies
 * full-snapshot frames (no client-side mutations) and reconnects with
 * exponential backoff on error. Otherwise the local mock generator emits a
 * snapshot and mutates one pod per tick, exactly as before.
 *
 * Returns an unsubscribe function.
 */
export function subscribeClusterUpdates(
  onTick: ClusterTickListener,
  options: SubscribeOptions = {},
): () => void {
  const intervalMs = options.intervalMs ?? 3000;
  let cancelled = false;
  let es: EventSource | null = null;
  let retryTimer: number | null = null;
  let retryMs = STREAM_BACKOFF_MIN_MS;
  let mockInterval: number | null = null;

  const openStream = (): void => {
    if (cancelled || typeof EventSource === "undefined") return;
    es = new EventSource("/api/stream");
    es.onmessage = (ev: MessageEvent<string>) => {
      retryMs = STREAM_BACKOFF_MIN_MS;
      try {
        const snap = JSON.parse(ev.data) as ServerSnapshot;
        onTick(snapshotToCluster(snap));
      } catch {
        /* ignore malformed frames */
      }
    };
    es.onerror = () => {
      es?.close();
      es = null;
      if (cancelled) return;
      retryTimer = window.setTimeout(openStream, retryMs);
      retryMs = Math.min(retryMs * 2, STREAM_BACKOFF_MAX_MS);
    };
  };

  const startMock = (): void => {
    const seed = options.initialSeed ?? "mock-cluster";
    let current = generateCluster(seed);
    let tickCounter = 0;
    onTick(current);
    mockInterval = window.setInterval(() => {
      if (cancelled) return;
      tickCounter += 1;
      current = mutateOnePod(current, `tick-${tickCounter}`);
      onTick(current);
    }, intervalMs);
  };

  const init = async (): Promise<void> => {
    const live = await fetchSnapshot();
    if (cancelled) return;
    if (live) {
      onTick(live);
      openStream();
    } else {
      startMock();
    }
  };

  void init();

  return () => {
    cancelled = true;
    es?.close();
    es = null;
    if (retryTimer !== null) window.clearTimeout(retryTimer);
    if (mockInterval !== null) window.clearInterval(mockInterval);
  };
}

function mutateOnePod(cluster: Cluster, tickSeed: string): Cluster {
  if (cluster.pods.length === 0) return cluster;
  const idx = hash32(tickSeed) % cluster.pods.length;
  const target = cluster.pods[idx];
  const next = mutatePodStatus(target, tickSeed);
  const pods = cluster.pods.slice();
  pods[idx] = next;
  return recomputeDerived({ ...cluster, pods });
}

// ---------------------------------------------------------------------------
// Imperative actions (flux, pods, logs)
// ---------------------------------------------------------------------------

export interface ActionResult {
  ok: boolean;
  error?: string;
}

async function postJSON(url: string, body: unknown): Promise<ActionResult> {
  try {
    const resp = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      let msg = `${resp.status} ${resp.statusText}`;
      try {
        const data = (await resp.json()) as { error?: string; message?: string };
        msg = data.error ?? data.message ?? msg;
      } catch {
        /* keep status text */
      }
      return { ok: false, error: msg };
    }
    return { ok: true };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : "request failed" };
  }
}

export type FluxActionKind = "reconcile" | "suspend" | "resume";

export function fluxAction(
  kind: string,
  namespace: string,
  name: string,
  action: FluxActionKind,
): Promise<ActionResult> {
  return postJSON("/api/flux/action", { kind, namespace, name, action });
}

export function deletePod(namespace: string, name: string): Promise<ActionResult> {
  return postJSON("/api/pods/delete", { namespace, name });
}

export async function fetchLogs(
  namespace: string,
  pod: string,
  container: string,
  tail: number,
): Promise<{ ok: boolean; text: string }> {
  try {
    const qs = new URLSearchParams({
      namespace,
      pod,
      container,
      tail: String(tail),
    });
    const resp = await fetch(`/api/logs?${qs.toString()}`, {
      headers: { Accept: "text/plain" },
    });
    const text = await resp.text();
    if (!resp.ok) return { ok: false, text: text || `${resp.status} ${resp.statusText}` };
    return { ok: true, text };
  } catch (e) {
    return { ok: false, text: e instanceof Error ? e.message : "log fetch failed" };
  }
}
