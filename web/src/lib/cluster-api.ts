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
  AccessMode,
  Cluster,
  ConfigMap,
  ContainerSpec,
  CronJob,
  CronJobStatus,
  ClusterRole,
  ClusterRoleBinding,
  CustomResourceDefinition,
  DaemonSet,
  Deployment,
  Endpoint,
  Event,
  FluxObject,
  FluxReady,
  HelmRelease,
  HelmReleaseStatus,
  HorizontalPodAutoscaler,
  Ingress,
  Job,
  JobStatus,
  LimitRange,
  LimitRangeItem,
  Namespace,
  Node,
  NetworkPolicy,
  Phase,
  PersistentVolume,
  PersistentVolumeClaim,
  Pod,
  PodDisruptionBudget,
  PodStatus,
  PVCPhase,
  PVPhase,
  PolicyRule,
  ReplicaSet,
  ResourceQuota,
  Role,
  RoleBinding,
  Secret,
  SecretType,
  Service,
  ServiceAccount,
  ServicePort,
  ServiceType,
  StatefulSet,
  StorageClass,
  Subject,
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
  kubeletVersion?: string;
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
  dependsOn?: string[];
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

interface ServerStatefulSet {
  name?: string;
  namespace?: string;
  replicas?: number;
  readyReplicas?: number;
  serviceName?: string;
  image?: string;
  ageSec?: number;
}

interface ServerDaemonSet {
  name?: string;
  namespace?: string;
  desiredNumberScheduled?: number;
  numberReady?: number;
  numberAvailable?: number;
  image?: string;
  ageSec?: number;
}

interface ServerReplicaSet {
  name?: string;
  namespace?: string;
  replicas?: number;
  readyReplicas?: number;
  ownerKind?: string;
  ownerName?: string;
  image?: string;
  ageSec?: number;
}

interface ServerJob {
  name?: string;
  namespace?: string;
  completions?: number;
  succeeded?: number;
  failed?: number;
  active?: number;
  status?: string;
  image?: string;
  durationSec?: number;
  ageSec?: number;
  ownerKind?: string;
  ownerName?: string;
}

interface ServerCronJob {
  name?: string;
  namespace?: string;
  schedule?: string;
  suspend?: boolean;
  lastScheduleAgeSec?: number;
  hasLastSchedule?: boolean;
  activeJobs?: number;
  status?: string;
  image?: string;
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

interface ServerIngressRule {
  host?: string;
  path?: string;
  serviceName?: string;
  servicePort?: number;
}

interface ServerIngress {
  name?: string;
  namespace?: string;
  className?: string;
  hosts?: string[];
  rules?: ServerIngressRule[];
  tls?: boolean;
  address?: string;
  ageSec?: number;
}

interface ServerEndpointSubset {
  addresses?: string[];
  ports?: number[];
}

interface ServerEndpoint {
  name?: string;
  namespace?: string;
  subsets?: ServerEndpointSubset[];
  targetService?: string;
  ageSec?: number;
}

interface ServerNetworkPolicy {
  name?: string;
  namespace?: string;
  podSelector?: Record<string, string>;
  policyTypes?: string[];
  ingressRules?: number;
  egressRules?: number;
  ageSec?: number;
}

interface ServerConfigMap {
  name?: string;
  namespace?: string;
  keys?: string[];
  dataBytes?: number;
  ageSec?: number;
}

interface ServerSecret {
  name?: string;
  namespace?: string;
  type?: string;
  keys?: string[];
  dataBytes?: number;
  ageSec?: number;
}

interface ServerResourceQuota {
  name?: string;
  namespace?: string;
  hard?: Record<string, string>;
  used?: Record<string, string>;
  ageSec?: number;
}

interface ServerLimitRangeItem {
  type?: string;
  defaultRequest?: Record<string, string>;
  defaultLimit?: Record<string, string>;
}

interface ServerLimitRange {
  name?: string;
  namespace?: string;
  limits?: ServerLimitRangeItem[];
  ageSec?: number;
}

interface ServerHorizontalPodAutoscaler {
  name?: string;
  namespace?: string;
  targetKind?: string;
  targetName?: string;
  minReplicas?: number;
  maxReplicas?: number;
  currentReplicas?: number;
  targetCPUPercent?: number;
  currentCPUPercent?: number;
  ageSec?: number;
}

interface ServerPodDisruptionBudget {
  name?: string;
  namespace?: string;
  minAvailable?: string;
  maxUnavailable?: string;
  currentHealthy?: number;
  desiredHealthy?: number;
  expectedPods?: number;
  selector?: Record<string, string>;
  ageSec?: number;
}

interface ServerPersistentVolumeClaim {
  name?: string;
  namespace?: string;
  capacity?: string;
  accessModes?: string[];
  storageClassName?: string;
  phase?: string;
  volumeName?: string;
  ageSec?: number;
}

interface ServerPersistentVolume {
  name?: string;
  capacity?: string;
  accessModes?: string[];
  reclaimPolicy?: string;
  phase?: string;
  storageClassName?: string;
  claimNamespace?: string;
  claimName?: string;
  ageSec?: number;
}

interface ServerStorageClass {
  name?: string;
  provisioner?: string;
  reclaimPolicy?: string;
  volumeBindingMode?: string;
  isDefault?: boolean;
  ageSec?: number;
}

interface ServerServiceAccount {
  name?: string;
  namespace?: string;
  secrets?: string[];
  imagePullSecrets?: string[];
  automountToken?: boolean;
  ageSec?: number;
}

interface ServerPolicyRule {
  apiGroups?: string[];
  resources?: string[];
  verbs?: string[];
}

interface ServerRole {
  name?: string;
  namespace?: string;
  rules?: ServerPolicyRule[];
  ageSec?: number;
}

interface ServerClusterRole {
  name?: string;
  rules?: ServerPolicyRule[];
  aggregationLabels?: Record<string, string>;
  ageSec?: number;
}

interface ServerRoleRef {
  kind?: string;
  name?: string;
}

interface ServerSubject {
  kind?: string;
  name?: string;
  namespace?: string;
}

interface ServerRoleBinding {
  name?: string;
  namespace?: string;
  roleRef?: ServerRoleRef;
  subjects?: ServerSubject[];
  ageSec?: number;
}

interface ServerClusterRoleBinding {
  name?: string;
  roleRef?: ServerRoleRef;
  subjects?: ServerSubject[];
  ageSec?: number;
}

interface ServerNamespace {
  name?: string;
  phase?: string;
  labels?: Record<string, string>;
  ageSec?: number;
}

interface ServerCustomResourceDefinition {
  name?: string;
  group?: string;
  scope?: string;
  versions?: string[];
  pluralName?: string;
  singularName?: string;
  listKind?: string;
  shortNames?: string[];
  ageSec?: number;
}

interface ServerHelmRelease {
  name?: string;
  namespace?: string;
  chart?: string;
  chartVersion?: string;
  appVersion?: string;
  revision?: number;
  status?: string;
  updatedAgeSec?: number;
  ageSec?: number;
}

interface ServerSnapshot {
  mode?: string;
  context?: string;
  version?: string;
  currentNamespace?: string;
  fluxInstalled?: boolean;
  metricsInstalled?: boolean;
  pods?: ServerPod[];
  nodes?: ServerNode[];
  namespaces?: Array<ServerNamespace | string>;
  events?: ServerEvent[];
  flux?: ServerFlux[];
  deployments?: ServerDeployment[];
  statefulSets?: ServerStatefulSet[];
  daemonSets?: ServerDaemonSet[];
  replicaSets?: ServerReplicaSet[];
  jobs?: ServerJob[];
  cronJobs?: ServerCronJob[];
  services?: ServerService[];
  ingresses?: ServerIngress[];
  endpoints?: ServerEndpoint[];
  networkPolicies?: ServerNetworkPolicy[];
  configMaps?: ServerConfigMap[];
  secrets?: ServerSecret[];
  resourceQuotas?: ServerResourceQuota[];
  limitRanges?: ServerLimitRange[];
  horizontalPodAutoscalers?: ServerHorizontalPodAutoscaler[];
  podDisruptionBudgets?: ServerPodDisruptionBudget[];
  persistentVolumeClaims?: ServerPersistentVolumeClaim[];
  persistentVolumes?: ServerPersistentVolume[];
  storageClasses?: ServerStorageClass[];
  serviceAccounts?: ServerServiceAccount[];
  roles?: ServerRole[];
  clusterRoles?: ServerClusterRole[];
  roleBindings?: ServerRoleBinding[];
  clusterRoleBindings?: ServerClusterRoleBinding[];
  customResourceDefinitions?: ServerCustomResourceDefinition[];
  helmReleases?: ServerHelmRelease[];
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
  const rawStatus = (n.status ?? "").toLowerCase();
  const status: Node["status"] =
    rawStatus === "ready"
      ? "ready"
      : rawStatus === "schedulingdisabled"
        ? "schedulingdisabled"
        : "notready";
  const name = n.name ?? "node";
  return {
    kind: "Node",
    uid: `node-${name}`,
    name,
    ageSec: 0,
    roles: [],
    status,
    conditions: [status === "ready" ? "Ready" : "NotReady"],
    kubeletVersion: n.kubeletVersion || "—",
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

function toNamespace(n: ServerNamespace | string): Namespace {
  const name = typeof n === "string" ? n : n.name ?? "namespace";
  const phase = typeof n === "string" ? "active" : (n.phase ?? "active").toLowerCase();
  return {
    kind: "Namespace",
    uid: `ns-${name}`,
    name,
    ageSec: typeof n === "string" ? 0 : n.ageSec ?? 0,
    labels: typeof n === "string" ? undefined : n.labels,
    phase: phase === "terminating" ? "terminating" : "active",
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
    dependsOn: f.dependsOn ?? [],
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

function workloadStatus(replicas: number, ready: number): WorkloadStatus {
  return replicas > 0 && ready >= replicas
    ? "healthy"
    : ready === 0
      ? "degraded"
      : "progressing";
}

function toStatefulSet(s: ServerStatefulSet): StatefulSet {
  const name = s.name ?? "statefulset";
  const namespace = s.namespace || undefined;
  const replicas = s.replicas ?? 0;
  const ready = s.readyReplicas ?? 0;
  return {
    kind: "StatefulSet",
    uid: `sts-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: s.ageSec ?? 0,
    replicas,
    readyReplicas: ready,
    serviceName: s.serviceName ?? "—",
    status: workloadStatus(replicas, ready),
    image: s.image ?? "—",
  };
}

function toDaemonSet(d: ServerDaemonSet): DaemonSet {
  const name = d.name ?? "daemonset";
  const namespace = d.namespace || undefined;
  const desired = d.desiredNumberScheduled ?? 0;
  const ready = d.numberReady ?? 0;
  return {
    kind: "DaemonSet",
    uid: `ds-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: d.ageSec ?? 0,
    desiredNumberScheduled: desired,
    numberReady: ready,
    numberAvailable: d.numberAvailable ?? ready,
    status: workloadStatus(desired, ready),
    image: d.image ?? "—",
  };
}

function toReplicaSet(r: ServerReplicaSet): ReplicaSet {
  const name = r.name ?? "replicaset";
  const namespace = r.namespace || undefined;
  return {
    kind: "ReplicaSet",
    uid: `rs-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: r.ageSec ?? 0,
    replicas: r.replicas ?? 0,
    readyReplicas: r.readyReplicas ?? 0,
    ownerKind: r.ownerKind || undefined,
    ownerName: r.ownerName || undefined,
    image: r.image ?? "—",
  };
}

const JOB_STATUSES: ReadonlySet<string> = new Set(["active", "completed", "failed", "suspended"]);

function toJob(j: ServerJob): Job {
  const name = j.name ?? "job";
  const namespace = j.namespace || undefined;
  const status = (JOB_STATUSES.has(j.status ?? "") ? j.status : "active") as JobStatus;
  return {
    kind: "Job",
    uid: `job-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: j.ageSec ?? 0,
    completions: j.completions ?? 1,
    succeeded: j.succeeded ?? 0,
    failed: j.failed ?? 0,
    active: j.active ?? 0,
    status,
    image: j.image ?? "—",
    durationSec: j.durationSec || undefined,
    ownerKind: j.ownerKind || undefined,
    ownerName: j.ownerName || undefined,
  };
}

function toCronJob(c: ServerCronJob): CronJob {
  const name = c.name ?? "cronjob";
  const namespace = c.namespace || undefined;
  const status: CronJobStatus = c.status === "suspended" || c.suspend ? "suspended" : "active";
  return {
    kind: "CronJob",
    uid: `cronjob-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: c.ageSec ?? 0,
    schedule: c.schedule ?? "—",
    suspend: !!c.suspend,
    lastScheduleAgeSec: c.hasLastSchedule ? c.lastScheduleAgeSec ?? 0 : undefined,
    activeJobs: c.activeJobs ?? 0,
    status,
    image: c.image ?? "—",
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

function toIngress(i: ServerIngress): Ingress {
  const name = i.name ?? "ingress";
  const namespace = i.namespace || undefined;
  return {
    kind: "Ingress",
    uid: `ing-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: i.ageSec ?? 0,
    className: i.className || undefined,
    hosts: i.hosts ?? [],
    rules: (i.rules ?? []).map((r) => ({
      host: r.host ?? "",
      path: r.path || "/",
      serviceName: r.serviceName ?? "—",
      servicePort: r.servicePort ?? 0,
    })),
    tls: !!i.tls,
    address: i.address || undefined,
  };
}

function toEndpoint(e: ServerEndpoint): Endpoint {
  const name = e.name ?? "endpoint";
  const namespace = e.namespace || undefined;
  return {
    kind: "Endpoint",
    uid: `endpoint-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: e.ageSec ?? 0,
    subsets: (e.subsets ?? []).map((s) => ({
      addresses: s.addresses ?? [],
      ports: s.ports ?? [],
    })),
    targetService: e.targetService ?? name,
  };
}

function toNetworkPolicy(n: ServerNetworkPolicy): NetworkPolicy {
  const name = n.name ?? "networkpolicy";
  const namespace = n.namespace || undefined;
  return {
    kind: "NetworkPolicy",
    uid: `netpol-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: n.ageSec ?? 0,
    podSelector: n.podSelector ?? {},
    policyTypes: (n.policyTypes ?? []).filter(
      (t): t is "Ingress" | "Egress" => t === "Ingress" || t === "Egress",
    ),
    ingressRules: n.ingressRules ?? 0,
    egressRules: n.egressRules ?? 0,
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

const SECRET_TYPES: ReadonlySet<string> = new Set([
  "Opaque",
  "kubernetes.io/tls",
  "kubernetes.io/dockerconfigjson",
  "kubernetes.io/service-account-token",
  "helm.sh/release.v1",
]);

function toSecret(s: ServerSecret): Secret {
  const name = s.name ?? "secret";
  const namespace = s.namespace || undefined;
  const type = SECRET_TYPES.has(s.type ?? "") ? s.type : "Opaque";
  return {
    kind: "Secret",
    uid: `secret-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: s.ageSec ?? 0,
    type: type as SecretType,
    dataKeys: s.keys ?? [],
    sizeBytes: s.dataBytes ?? 0,
  };
}

function toResourceQuota(r: ServerResourceQuota): ResourceQuota {
  const name = r.name ?? "resourcequota";
  const namespace = r.namespace || undefined;
  return {
    kind: "ResourceQuota",
    uid: `rq-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: r.ageSec ?? 0,
    hard: r.hard ?? {},
    used: r.used ?? {},
  };
}

function toLimitRangeItem(item: ServerLimitRangeItem): LimitRangeItem {
  const type =
    item.type === "Pod" || item.type === "PersistentVolumeClaim"
      ? item.type
      : "Container";
  return {
    type,
    defaultRequest: item.defaultRequest ?? {},
    defaultLimit: item.defaultLimit ?? {},
  };
}

function toLimitRange(l: ServerLimitRange): LimitRange {
  const name = l.name ?? "limitrange";
  const namespace = l.namespace || undefined;
  return {
    kind: "LimitRange",
    uid: `limitrange-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: l.ageSec ?? 0,
    limits: (l.limits ?? []).map(toLimitRangeItem),
  };
}

function toHorizontalPodAutoscaler(h: ServerHorizontalPodAutoscaler): HorizontalPodAutoscaler {
  const name = h.name ?? "hpa";
  const namespace = h.namespace || undefined;
  const targetKind = h.targetKind === "StatefulSet" ? "StatefulSet" : "Deployment";
  return {
    kind: "HorizontalPodAutoscaler",
    uid: `hpa-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: h.ageSec ?? 0,
    targetKind,
    targetName: h.targetName ?? name,
    minReplicas: h.minReplicas ?? 1,
    maxReplicas: h.maxReplicas ?? 1,
    currentReplicas: h.currentReplicas ?? 0,
    targetCPUPercent: typeof h.targetCPUPercent === "number" ? h.targetCPUPercent : undefined,
    currentCPUPercent: typeof h.currentCPUPercent === "number" ? h.currentCPUPercent : undefined,
  };
}

function toPodDisruptionBudget(p: ServerPodDisruptionBudget): PodDisruptionBudget {
  const name = p.name ?? "pdb";
  const namespace = p.namespace || undefined;
  return {
    kind: "PodDisruptionBudget",
    uid: `pdb-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: p.ageSec ?? 0,
    minAvailable: p.minAvailable || undefined,
    maxUnavailable: p.maxUnavailable || undefined,
    currentHealthy: p.currentHealthy ?? 0,
    desiredHealthy: p.desiredHealthy ?? 0,
    expectedPods: p.expectedPods ?? 0,
    selector: p.selector ?? {},
  };
}

const ACCESS_MODES: ReadonlySet<string> = new Set([
  "ReadWriteOnce",
  "ReadOnlyMany",
  "ReadWriteMany",
  "ReadWriteOncePod",
]);

function toAccessModes(modes: string[] | undefined): AccessMode[] {
  return (modes ?? []).filter((m): m is AccessMode => ACCESS_MODES.has(m));
}

function toPVCPhase(raw: string | undefined): PVCPhase {
  switch ((raw ?? "").toLowerCase()) {
    case "bound":
      return "bound";
    case "lost":
      return "lost";
    default:
      return "pending";
  }
}

function toPVPhase(raw: string | undefined): PVPhase {
  switch ((raw ?? "").toLowerCase()) {
    case "bound":
      return "bound";
    case "released":
      return "released";
    case "failed":
      return "failed";
    case "pending":
      return "pending";
    default:
      return "available";
  }
}

function toPersistentVolumeClaim(p: ServerPersistentVolumeClaim): PersistentVolumeClaim {
  const name = p.name ?? "pvc";
  const namespace = p.namespace || undefined;
  return {
    kind: "PersistentVolumeClaim",
    uid: `pvc-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: p.ageSec ?? 0,
    capacity: p.capacity ?? "—",
    accessModes: toAccessModes(p.accessModes),
    storageClassName: p.storageClassName ?? "—",
    phase: toPVCPhase(p.phase),
    volumeName: p.volumeName || undefined,
  };
}

function toPersistentVolume(p: ServerPersistentVolume): PersistentVolume {
  const name = p.name ?? "pv";
  return {
    kind: "PersistentVolume",
    uid: `pv-${name}`,
    name,
    ageSec: p.ageSec ?? 0,
    capacity: p.capacity ?? "—",
    accessModes: toAccessModes(p.accessModes),
    reclaimPolicy: p.reclaimPolicy === "Retain" || p.reclaimPolicy === "Recycle" ? p.reclaimPolicy : "Delete",
    phase: toPVPhase(p.phase),
    storageClassName: p.storageClassName ?? "—",
    claimRef: p.claimName
      ? { namespace: p.claimNamespace ?? "default", name: p.claimName }
      : undefined,
  };
}

function toStorageClass(s: ServerStorageClass): StorageClass {
  const name = s.name ?? "storageclass";
  return {
    kind: "StorageClass",
    uid: `sc-${name}`,
    name,
    ageSec: s.ageSec ?? 0,
    provisioner: s.provisioner ?? "—",
    reclaimPolicy: s.reclaimPolicy === "Retain" ? "Retain" : "Delete",
    volumeBindingMode: s.volumeBindingMode === "WaitForFirstConsumer" ? "WaitForFirstConsumer" : "Immediate",
    isDefault: !!s.isDefault,
  };
}

function toPolicyRule(r: ServerPolicyRule): PolicyRule {
  return {
    apiGroups: r.apiGroups ?? [],
    resources: r.resources ?? [],
    verbs: r.verbs ?? [],
  };
}

function toServiceAccount(s: ServerServiceAccount): ServiceAccount {
  const name = s.name ?? "serviceaccount";
  const namespace = s.namespace || undefined;
  return {
    kind: "ServiceAccount",
    uid: `sa-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: s.ageSec ?? 0,
    secrets: s.secrets ?? [],
    imagePullSecrets: s.imagePullSecrets ?? [],
    automountToken: s.automountToken ?? true,
  };
}

function toRole(r: ServerRole): Role {
  const name = r.name ?? "role";
  const namespace = r.namespace || undefined;
  return {
    kind: "Role",
    uid: `role-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: r.ageSec ?? 0,
    rules: (r.rules ?? []).map(toPolicyRule),
  };
}

function toClusterRole(r: ServerClusterRole): ClusterRole {
  const name = r.name ?? "clusterrole";
  return {
    kind: "ClusterRole",
    uid: `clusterrole-${name}`,
    name,
    ageSec: r.ageSec ?? 0,
    rules: (r.rules ?? []).map(toPolicyRule),
    aggregationLabels: r.aggregationLabels,
  };
}

function toSubject(s: ServerSubject): Subject {
  const kind =
    s.kind === "Group" || s.kind === "ServiceAccount" ? s.kind : "User";
  return {
    kind,
    name: s.name ?? "subject",
    namespace: s.namespace || undefined,
  };
}

function toRoleRef(ref: ServerRoleRef | undefined): { kind: "Role" | "ClusterRole"; name: string } {
  return {
    kind: ref?.kind === "ClusterRole" ? "ClusterRole" : "Role",
    name: ref?.name ?? "role",
  };
}

function toClusterRoleRef(ref: ServerRoleRef | undefined): { kind: "ClusterRole"; name: string } {
  return {
    kind: "ClusterRole",
    name: ref?.name ?? "clusterrole",
  };
}

function toRoleBinding(r: ServerRoleBinding): RoleBinding {
  const name = r.name ?? "rolebinding";
  const namespace = r.namespace || undefined;
  return {
    kind: "RoleBinding",
    uid: `rolebinding-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: r.ageSec ?? 0,
    roleRef: toRoleRef(r.roleRef),
    subjects: (r.subjects ?? []).map(toSubject),
  };
}

function toClusterRoleBinding(r: ServerClusterRoleBinding): ClusterRoleBinding {
  const name = r.name ?? "clusterrolebinding";
  return {
    kind: "ClusterRoleBinding",
    uid: `clusterrolebinding-${name}`,
    name,
    ageSec: r.ageSec ?? 0,
    roleRef: toClusterRoleRef(r.roleRef),
    subjects: (r.subjects ?? []).map(toSubject),
  };
}

function toCustomResourceDefinition(c: ServerCustomResourceDefinition): CustomResourceDefinition {
  const name = c.name ?? "customresourcedefinition";
  return {
    kind: "CustomResourceDefinition",
    uid: `crd-${name}`,
    name,
    ageSec: c.ageSec ?? 0,
    group: c.group ?? "—",
    scope: c.scope === "Cluster" ? "Cluster" : "Namespaced",
    versions: c.versions ?? [],
    pluralName: c.pluralName ?? name,
    singularName: c.singularName ?? "",
    listKind: c.listKind ?? "",
    shortNames: c.shortNames ?? [],
  };
}

const HELM_STATUSES: ReadonlySet<string> = new Set([
  "deployed",
  "failed",
  "pending-install",
  "pending-upgrade",
  "superseded",
  "uninstalled",
]);

function toHelmRelease(h: ServerHelmRelease): HelmRelease {
  const name = h.name ?? "helmrelease";
  const namespace = h.namespace || undefined;
  const rawStatus = h.status ?? "";
  const status = (HELM_STATUSES.has(rawStatus) ? rawStatus : "deployed") as HelmReleaseStatus;
  return {
    kind: "HelmRelease",
    uid: `helm-${namespace ?? ""}-${name}`,
    name,
    namespace,
    ageSec: h.ageSec ?? 0,
    chart: h.chart ?? "—",
    chartVersion: h.chartVersion ?? "—",
    appVersion: h.appVersion ?? "—",
    revision: h.revision ?? 0,
    status,
    updatedAgeSec: h.updatedAgeSec ?? 0,
  };
}

function snapshotToCluster(s: ServerSnapshot): Cluster {
  const pods = (s.pods ?? []).map(toPod);
  const mode =
    s.mode === "demo" ? "demo" : s.mode === "cluster" ? "cluster" : "live";
  const cluster: Cluster = {
    context: s.context || "unknown",
    currentNamespace: s.currentNamespace ?? "default",
    version: s.version || "—",
    generatedAtSec: 0,
    mode,
    fluxInstalled: !!s.fluxInstalled,
    metricsInstalled: !!s.metricsInstalled,
    flux: (s.flux ?? []).map(toFlux),

    pods,
    deployments: s.deployments ? s.deployments.map(toDeployment) : deriveDeployments(pods),
    statefulSets: (s.statefulSets ?? []).map(toStatefulSet),
    daemonSets: (s.daemonSets ?? []).map(toDaemonSet),
    replicaSets: (s.replicaSets ?? []).map(toReplicaSet),
    jobs: (s.jobs ?? []).map(toJob),
    cronJobs: (s.cronJobs ?? []).map(toCronJob),

    services: (s.services ?? []).map(toService),
    ingresses: (s.ingresses ?? []).map(toIngress),
    endpoints: (s.endpoints ?? []).map(toEndpoint),
    networkPolicies: (s.networkPolicies ?? []).map(toNetworkPolicy),

    configMaps: (s.configMaps ?? []).map(toConfigMap),
    secrets: (s.secrets ?? []).map(toSecret),
    resourceQuotas: (s.resourceQuotas ?? []).map(toResourceQuota),
    limitRanges: (s.limitRanges ?? []).map(toLimitRange),

    horizontalPodAutoscalers: (s.horizontalPodAutoscalers ?? []).map(toHorizontalPodAutoscaler),
    podDisruptionBudgets: (s.podDisruptionBudgets ?? []).map(toPodDisruptionBudget),

    persistentVolumes: (s.persistentVolumes ?? []).map(toPersistentVolume),
    persistentVolumeClaims: (s.persistentVolumeClaims ?? []).map(toPersistentVolumeClaim),
    storageClasses: (s.storageClasses ?? []).map(toStorageClass),

    serviceAccounts: (s.serviceAccounts ?? []).map(toServiceAccount),
    roles: (s.roles ?? []).map(toRole),
    clusterRoles: (s.clusterRoles ?? []).map(toClusterRole),
    roleBindings: (s.roleBindings ?? []).map(toRoleBinding),
    clusterRoleBindings: (s.clusterRoleBindings ?? []).map(toClusterRoleBinding),

    nodes: (s.nodes ?? []).map(toNode),
    namespaces: (s.namespaces ?? []).map(toNamespace),
    events: (s.events ?? []).map(toEvent),

    customResourceDefinitions: (s.customResourceDefinitions ?? []).map(toCustomResourceDefinition),
    helmReleases: (s.helmReleases ?? []).map(toHelmRelease),
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
// Context switching
// ---------------------------------------------------------------------------

export interface ClusterContextInfo {
  name: string;
  cluster: string;
  namespace?: string;
}

export interface ClusterContextList {
  current: string;
  contexts: ClusterContextInfo[];
}

export async function fetchClusterContexts(): Promise<ClusterContextList | null> {
  try {
    const resp = await fetch("/api/contexts", {
      headers: { Accept: "application/json" },
    });
    if (!resp.ok) return null;
    const body = (await resp.json()) as ClusterContextList;
    if (!body || !Array.isArray(body.contexts)) return null;
    return body;
  } catch {
    return null;
  }
}

export async function selectClusterContext(name: string): Promise<ClusterContextList> {
  const resp = await fetch("/api/contexts/select", {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ name }),
  });
  if (!resp.ok) {
    const message = (await resp.text()).trim() || `switch context failed (${resp.status})`;
    throw new Error(message);
  }
  return (await resp.json()) as ClusterContextList;
}

/** Plug in a kubeconfig — pasted YAML ("raw") or a server-side file path. */
export type KubeconfigRequest =
  | { mode: "raw"; raw: string }
  | { mode: "path"; path: string };

export async function applyKubeconfig(req: KubeconfigRequest): Promise<ClusterContextList> {
  const resp = await fetch("/api/kubeconfig", {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(req),
  });
  if (!resp.ok) {
    const message = (await resp.text()).trim() || `apply kubeconfig failed (${resp.status})`;
    throw new Error(message);
  }
  return (await resp.json()) as ClusterContextList;
}

// ---------------------------------------------------------------------------
// Yscale burst-fleet API
// ---------------------------------------------------------------------------

export interface YscaleSpend {
  running_bursts: number;
  hourly_usd: number;
  projected_daily_usd: number;
  limits: {
    max_concurrent_bursts: number;
    max_hourly_usd: number;
  };
}

export interface YscaleBurst {
  id: string;
  backend: "flyio" | "linode" | "aws";
  node_name: string;
  ts_hostname: string;
  status: string;
  sku: string;
  hourly_usd: number;
  accrued_usd: number;
  age_seconds: number;
  mesh_provider: "tailscale" | "headscale" | "";
  pod_cidr: string;
  deadline?: string;
  max_usd?: number;
}

export type YscaleResponse =
  | { configured: false }
  | { configured: true; url: string; error: string }
  | { configured: true; url: string; spend: YscaleSpend; bursts: YscaleBurst[]; count: number };

export async function fetchYscale(): Promise<YscaleResponse> {
  const r = await fetch("/api/yscale", { headers: { Accept: "application/json" } });
  if (!r.ok) throw new Error(`yscale fetch failed: ${r.status} ${r.statusText}`);
  return (await r.json()) as YscaleResponse;
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

export function deleteResource(
  kind: string,
  name: string,
  namespace?: string,
  apiVersion?: string,
): Promise<ActionResult> {
  return postJSON("/api/resource/delete", { apiVersion, kind, namespace, name });
}

export function scaleResource(
  kind: string,
  name: string,
  namespace: string | undefined,
  replicas: number,
  apiVersion?: string,
): Promise<ActionResult> {
  return postJSON("/api/resource/scale", { apiVersion, kind, namespace, name, replicas });
}

export function restartResource(
  kind: string,
  name: string,
  namespace?: string,
  apiVersion?: string,
): Promise<ActionResult> {
  return postJSON("/api/resource/restart", { apiVersion, kind, namespace, name });
}

export function cordonNode(name: string, cordon: boolean): Promise<ActionResult> {
  return postJSON("/api/node/cordon", { name, cordon });
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

export async function fetchObjectYaml(
  kind: string,
  name: string,
  namespace?: string,
  apiVersion?: string,
): Promise<{ ok: boolean; yaml: string }> {
  try {
    const params: Record<string, string> = { kind, name };
    if (namespace) params.namespace = namespace;
    if (apiVersion) params.apiVersion = apiVersion;
    const qs = new URLSearchParams(params);
    const resp = await fetch(`/api/object?${qs.toString()}`, {
      headers: { Accept: "application/json" },
    });
    if (!resp.ok) {
      const text = await resp.text();
      return { ok: false, yaml: text || `${resp.status} ${resp.statusText}` };
    }
    const data = (await resp.json()) as { yaml?: string };
    return { ok: true, yaml: data.yaml ?? "" };
  } catch (e) {
    return { ok: false, yaml: e instanceof Error ? e.message : "yaml fetch failed" };
  }
}

export interface CustomResourceRef {
  name: string;
  namespace?: string;
  ageSec: number;
}

export async function fetchCustomResources(
  group: string,
  version: string,
  resource: string,
  namespace?: string,
): Promise<{ ok: boolean; items: CustomResourceRef[]; error?: string }> {
  try {
    const params: Record<string, string> = { version, resource };
    if (group) params.group = group;
    if (namespace) params.namespace = namespace;
    const qs = new URLSearchParams(params);
    const resp = await fetch(`/api/customresources?${qs.toString()}`, {
      headers: { Accept: "application/json" },
    });
    if (!resp.ok) {
      const text = await resp.text();
      return { ok: false, items: [], error: text || `${resp.status} ${resp.statusText}` };
    }
    const data = (await resp.json()) as { items?: CustomResourceRef[] };
    return { ok: true, items: data.items ?? [] };
  } catch (e) {
    return {
      ok: false,
      items: [],
      error: e instanceof Error ? e.message : "custom resource fetch failed",
    };
  }
}

export async function applyObjectYaml(
  yaml: string,
): Promise<{ ok: boolean; yaml: string; error?: string }> {
  try {
    const resp = await fetch("/api/object/apply", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: JSON.stringify({ yaml }),
    });
    if (!resp.ok) {
      const text = await resp.text();
      return { ok: false, yaml: "", error: text || `${resp.status} ${resp.statusText}` };
    }
    const data = (await resp.json()) as { yaml?: string };
    return { ok: true, yaml: data.yaml ?? "" };
  } catch (e) {
    return { ok: false, yaml: "", error: e instanceof Error ? e.message : "apply failed" };
  }
}

// ---------------------------------------------------------------------------
// Port-forward
// ---------------------------------------------------------------------------

export interface PortForwardInfo {
  id: string;
  namespace: string;
  pod: string;
  remotePort: number;
  localPort: number;
  ageSec: number;
}

export async function startPortForward(
  namespace: string,
  pod: string,
  remotePort: number,
  localPort?: number,
): Promise<{ ok: boolean; forward?: PortForwardInfo; error?: string }> {
  try {
    const resp = await fetch("/api/portforward/start", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ namespace, pod, remotePort, localPort: localPort ?? 0 }),
    });
    if (!resp.ok) {
      const text = await resp.text();
      return { ok: false, error: text.trim() || `${resp.status} ${resp.statusText}` };
    }
    const data = (await resp.json()) as PortForwardInfo;
    return { ok: true, forward: data };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : "port-forward start failed" };
  }
}

export async function stopPortForward(
  id: string,
): Promise<{ ok: boolean; error?: string }> {
  try {
    const resp = await fetch("/api/portforward/stop", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ id }),
    });
    if (!resp.ok) {
      const text = await resp.text();
      return { ok: false, error: text.trim() || `${resp.status} ${resp.statusText}` };
    }
    return { ok: true };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : "port-forward stop failed" };
  }
}

export async function listPortForwards(): Promise<{ ok: boolean; forwards: PortForwardInfo[]; error?: string }> {
  try {
    const resp = await fetch("/api/portforward/list", {
      headers: { Accept: "application/json" },
    });
    if (!resp.ok) {
      const text = await resp.text();
      return { ok: false, forwards: [], error: text.trim() || `${resp.status} ${resp.statusText}` };
    }
    const data = (await resp.json()) as { forwards?: PortForwardInfo[] };
    return { ok: true, forwards: data.forwards ?? [] };
  } catch (e) {
    return { ok: false, forwards: [], error: e instanceof Error ? e.message : "port-forward list failed" };
  }
}

export interface SecretEntry {
  b64: string;
  decoded: string;
}

export async function fetchSecretData(
  namespace: string,
  name: string,
): Promise<{ ok: boolean; data: Record<string, SecretEntry>; error?: string }> {
  try {
    const qs = new URLSearchParams({ namespace, name });
    const resp = await fetch(`/api/secret?${qs.toString()}`, {
      headers: { Accept: "application/json" },
    });
    if (!resp.ok) {
      const text = await resp.text();
      return { ok: false, data: {}, error: text || `${resp.status} ${resp.statusText}` };
    }
    const body = (await resp.json()) as { data?: Record<string, SecretEntry> };
    return { ok: true, data: body.data ?? {} };
  } catch (e) {
    return {
      ok: false,
      data: {},
      error: e instanceof Error ? e.message : "secret fetch failed",
    };
  }
}

// ---------------------------------------------------------------------------
// Helm release endpoints
// ---------------------------------------------------------------------------

export interface HelmRevisionInfo {
  revision: number;
  status: string;
  chartVersion: string;
  appVersion: string;
  updatedAgeSec: number;
  description?: string;
}

export async function fetchHelmHistory(
  namespace: string,
  name: string,
): Promise<{ ok: boolean; revisions: HelmRevisionInfo[]; helmCli: boolean; error?: string }> {
  try {
    const qs = new URLSearchParams({ namespace, name });
    const resp = await fetch(`/api/helm/history?${qs.toString()}`, {
      headers: { Accept: "application/json" },
    });
    if (!resp.ok) {
      const text = await resp.text();
      return { ok: false, revisions: [], helmCli: false, error: text || `${resp.status} ${resp.statusText}` };
    }
    const data = (await resp.json()) as { revisions?: HelmRevisionInfo[]; helmCli?: boolean };
    return { ok: true, revisions: data.revisions ?? [], helmCli: data.helmCli ?? false };
  } catch (e) {
    return { ok: false, revisions: [], helmCli: false, error: e instanceof Error ? e.message : "helm history fetch failed" };
  }
}

export async function helmRollback(
  namespace: string,
  name: string,
  revision: number,
): Promise<{ ok: boolean; output?: string; error?: string }> {
  try {
    const resp = await fetch("/api/helm/rollback", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ namespace, name, revision }),
    });
    if (!resp.ok) {
      const text = await resp.text();
      return { ok: false, error: text.trim() || `${resp.status} ${resp.statusText}` };
    }
    const data = (await resp.json()) as { output?: string };
    return { ok: true, output: data.output };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : "helm rollback failed" };
  }
}

export async function helmUninstall(
  namespace: string,
  name: string,
): Promise<{ ok: boolean; output?: string; error?: string }> {
  try {
    const resp = await fetch("/api/helm/uninstall", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ namespace, name }),
    });
    if (!resp.ok) {
      const text = await resp.text();
      return { ok: false, error: text.trim() || `${resp.status} ${resp.statusText}` };
    }
    const data = (await resp.json()) as { output?: string };
    return { ok: true, output: data.output };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : "helm uninstall failed" };
  }
}

export interface HelmDetailResult {
  values: string;
  manifest: string;
  notes: string;
}

export async function fetchHelmRelease(
  namespace: string,
  name: string,
  revision: number,
): Promise<{ ok: boolean; detail?: HelmDetailResult; error?: string }> {
  try {
    const qs = new URLSearchParams({ namespace, name, revision: String(revision) });
    const resp = await fetch(`/api/helm/release?${qs.toString()}`, {
      headers: { Accept: "application/json" },
    });
    if (!resp.ok) {
      const text = await resp.text();
      return { ok: false, error: text || `${resp.status} ${resp.statusText}` };
    }
    const detail = (await resp.json()) as HelmDetailResult;
    return { ok: true, detail };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : "helm release fetch failed" };
  }
}
