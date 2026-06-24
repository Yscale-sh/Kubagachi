/**
 * Kubernetes resource types for the Kubagachi dashboard mock.
 *
 * Each resource extends BaseMeta and carries a literal `kind` field so
 * AnyResource is a proper discriminated union and narrowing works without
 * runtime type guards.
 */

// ---------------------------------------------------------------------------
// Shared metadata
// ---------------------------------------------------------------------------

export interface BaseMeta {
  uid: string;
  name: string;
  namespace?: string;
  /** Resource age in seconds (relative to "now" at mock-generation time). */
  ageSec: number;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
}

// ---------------------------------------------------------------------------
// Workloads
// ---------------------------------------------------------------------------

export type PodStatus =
  | "running"
  | "pending"
  | "completed"
  | "crashloop"
  | "backoff"
  | "terminating"
  | "unknown"
  | "error";

export type Phase = "pending" | "running" | "completed" | "error" | "unknown";

export interface ContainerResources {
  cpuRequest?: string;
  cpuLimit?: string;
  memRequest?: string;
  memLimit?: string;
}

export interface ContainerSpec {
  name: string;
  image: string;
  ready: boolean;
  restartCount: number;
  resources?: ContainerResources;
  /** Server-reported container state, e.g. "running" | "waiting" | "terminated". */
  state?: string;
  /** Server-reported wait/termination reason, e.g. "CrashLoopBackOff". */
  reason?: string;
}

export interface Pod extends BaseMeta {
  kind: "Pod";
  status: PodStatus;
  phase: Phase;
  node: string;
  podIP?: string;
  hostIP?: string;
  /** Display alias for the workload identity ("which critter is this?"). */
  critter: string;
  /** Animation deck to play: health state, or a workload animation (e.g. "bursting"). */
  critterState?: string;
  containers: ContainerSpec[];
  restartCount: number;
  ownerKind?: string;
  ownerName?: string;
  qosClass?: "Guaranteed" | "Burstable" | "BestEffort";
  /** Live usage from metrics-server; -1 (or undefined) means unknown. */
  cpuMilli?: number;
  memBytes?: number;
  /** convenience aggregate ready vs total */
  readyContainers: number;
  totalContainers: number;
}

export type WorkloadStatus = "healthy" | "progressing" | "degraded" | "unknown";

export interface Deployment extends BaseMeta {
  kind: "Deployment";
  replicas: number;
  readyReplicas: number;
  updatedReplicas: number;
  availableReplicas: number;
  strategy: "RollingUpdate" | "Recreate";
  status: WorkloadStatus;
  selector: Record<string, string>;
  image: string;
}

export interface StatefulSet extends BaseMeta {
  kind: "StatefulSet";
  replicas: number;
  readyReplicas: number;
  serviceName: string;
  status: WorkloadStatus;
  image: string;
}

export interface DaemonSet extends BaseMeta {
  kind: "DaemonSet";
  desiredNumberScheduled: number;
  numberReady: number;
  numberAvailable: number;
  status: WorkloadStatus;
  image: string;
}

export interface ReplicaSet extends BaseMeta {
  kind: "ReplicaSet";
  replicas: number;
  readyReplicas: number;
  ownerKind?: string;
  ownerName?: string;
  image: string;
}

export type JobStatus = "active" | "completed" | "failed" | "suspended";

export interface Job extends BaseMeta {
  kind: "Job";
  completions: number;
  succeeded: number;
  failed: number;
  active: number;
  status: JobStatus;
  image: string;
  durationSec?: number;
}

export type CronJobStatus = "active" | "suspended";

export interface CronJob extends BaseMeta {
  kind: "CronJob";
  schedule: string;
  suspend: boolean;
  lastScheduleAgeSec?: number;
  activeJobs: number;
  status: CronJobStatus;
  image: string;
}

// ---------------------------------------------------------------------------
// Networking
// ---------------------------------------------------------------------------

export type ServiceType = "ClusterIP" | "NodePort" | "LoadBalancer" | "ExternalName" | "Headless";

export interface ServicePort {
  name?: string;
  port: number;
  targetPort: number;
  nodePort?: number;
  protocol: "TCP" | "UDP";
}

export interface Service extends BaseMeta {
  kind: "Service";
  type: ServiceType;
  clusterIP: string;
  externalIP?: string;
  ports: ServicePort[];
  selector: Record<string, string>;
}

export interface IngressRule {
  host: string;
  path: string;
  serviceName: string;
  servicePort: number;
}

export interface Ingress extends BaseMeta {
  kind: "Ingress";
  className?: string;
  hosts: string[];
  rules: IngressRule[];
  tls: boolean;
  address?: string;
}

export interface EndpointSubset {
  addresses: string[];
  ports: number[];
}

export interface Endpoint extends BaseMeta {
  kind: "Endpoint";
  subsets: EndpointSubset[];
  targetService: string;
}

export interface NetworkPolicy extends BaseMeta {
  kind: "NetworkPolicy";
  podSelector: Record<string, string>;
  policyTypes: ("Ingress" | "Egress")[];
  ingressRules: number;
  egressRules: number;
}

// ---------------------------------------------------------------------------
// Config & storage of small data
// ---------------------------------------------------------------------------

export interface ConfigMap extends BaseMeta {
  kind: "ConfigMap";
  dataKeys: string[];
  sizeBytes: number;
}

export type SecretType =
  | "Opaque"
  | "kubernetes.io/tls"
  | "kubernetes.io/dockerconfigjson"
  | "kubernetes.io/service-account-token"
  | "helm.sh/release.v1";

export interface Secret extends BaseMeta {
  kind: "Secret";
  type: SecretType;
  dataKeys: string[];
  sizeBytes: number;
}

export interface ResourceQuota extends BaseMeta {
  kind: "ResourceQuota";
  hard: Record<string, string>;
  used: Record<string, string>;
}

export interface LimitRangeItem {
  type: "Container" | "Pod" | "PersistentVolumeClaim";
  defaultRequest?: Record<string, string>;
  defaultLimit?: Record<string, string>;
}

export interface LimitRange extends BaseMeta {
  kind: "LimitRange";
  limits: LimitRangeItem[];
}

// ---------------------------------------------------------------------------
// Autoscaling & disruption
// ---------------------------------------------------------------------------

export interface HorizontalPodAutoscaler extends BaseMeta {
  kind: "HorizontalPodAutoscaler";
  targetKind: "Deployment" | "StatefulSet";
  targetName: string;
  minReplicas: number;
  maxReplicas: number;
  currentReplicas: number;
  targetCPUPercent?: number;
  currentCPUPercent?: number;
}

export interface PodDisruptionBudget extends BaseMeta {
  kind: "PodDisruptionBudget";
  minAvailable?: string;
  maxUnavailable?: string;
  currentHealthy: number;
  desiredHealthy: number;
  expectedPods: number;
  selector: Record<string, string>;
}

// ---------------------------------------------------------------------------
// Storage
// ---------------------------------------------------------------------------

export type PVPhase = "available" | "bound" | "released" | "failed" | "pending";
export type PVCPhase = "pending" | "bound" | "lost";

export type AccessMode = "ReadWriteOnce" | "ReadOnlyMany" | "ReadWriteMany" | "ReadWriteOncePod";

export interface PersistentVolume extends BaseMeta {
  kind: "PersistentVolume";
  capacity: string;
  accessModes: AccessMode[];
  reclaimPolicy: "Retain" | "Delete" | "Recycle";
  phase: PVPhase;
  storageClassName: string;
  claimRef?: { namespace: string; name: string };
}

export interface PersistentVolumeClaim extends BaseMeta {
  kind: "PersistentVolumeClaim";
  capacity: string;
  accessModes: AccessMode[];
  storageClassName: string;
  phase: PVCPhase;
  volumeName?: string;
}

export interface StorageClass extends BaseMeta {
  kind: "StorageClass";
  provisioner: string;
  reclaimPolicy: "Retain" | "Delete";
  volumeBindingMode: "Immediate" | "WaitForFirstConsumer";
  isDefault: boolean;
}

// ---------------------------------------------------------------------------
// RBAC & identity
// ---------------------------------------------------------------------------

export interface ServiceAccount extends BaseMeta {
  kind: "ServiceAccount";
  secrets: string[];
  imagePullSecrets: string[];
  automountToken: boolean;
}

export interface PolicyRule {
  apiGroups: string[];
  resources: string[];
  verbs: string[];
}

export interface Role extends BaseMeta {
  kind: "Role";
  rules: PolicyRule[];
}

export interface ClusterRole extends BaseMeta {
  kind: "ClusterRole";
  rules: PolicyRule[];
  aggregationLabels?: Record<string, string>;
}

export interface Subject {
  kind: "User" | "Group" | "ServiceAccount";
  name: string;
  namespace?: string;
}

export interface RoleBinding extends BaseMeta {
  kind: "RoleBinding";
  roleRef: { kind: "Role" | "ClusterRole"; name: string };
  subjects: Subject[];
}

export interface ClusterRoleBinding extends BaseMeta {
  kind: "ClusterRoleBinding";
  roleRef: { kind: "ClusterRole"; name: string };
  subjects: Subject[];
}

// ---------------------------------------------------------------------------
// Cluster-wide
// ---------------------------------------------------------------------------

export type NodeCondition = "Ready" | "NotReady" | "MemoryPressure" | "DiskPressure" | "PIDPressure";

export interface NodeAddress {
  type: "InternalIP" | "ExternalIP" | "Hostname";
  address: string;
}

export interface Node extends BaseMeta {
  kind: "Node";
  roles: string[];
  status: "ready" | "notready" | "schedulingdisabled";
  conditions: NodeCondition[];
  kubeletVersion: string;
  os: string;
  arch: "amd64" | "arm64";
  cpuCapacity: string;
  memCapacity: string;
  cpuAllocatable: string;
  memAllocatable: string;
  podCount: number;
  podCapacity: number;
  addresses: NodeAddress[];
  containerRuntime: string;
  /** Live utilisation 0..100 from metrics-server; -1 (or undefined) means unknown. */
  cpuPct?: number;
  memPct?: number;
}

export type NamespacePhase = "active" | "terminating";

export interface Namespace extends BaseMeta {
  kind: "Namespace";
  phase: NamespacePhase;
}

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

export type EventType = "normal" | "warning";

export interface Event extends BaseMeta {
  kind: "Event";
  type: EventType;
  reason: string;
  message: string;
  involvedObject: {
    kind: string;
    name: string;
    namespace?: string;
  };
  source: string;
  count: number;
  firstSeenSec: number;
  lastSeenSec: number;
}

// ---------------------------------------------------------------------------
// Custom resources & Helm
// ---------------------------------------------------------------------------

export interface CustomResourceDefinition extends BaseMeta {
  kind: "CustomResourceDefinition";
  group: string;
  scope: "Namespaced" | "Cluster";
  versions: string[];
  pluralName: string;
  singularName: string;
  listKind: string;
  shortNames: string[];
}

export type HelmReleaseStatus =
  | "deployed"
  | "failed"
  | "pending-install"
  | "pending-upgrade"
  | "superseded"
  | "uninstalled";

export interface HelmRelease extends BaseMeta {
  kind: "HelmRelease";
  chart: string;
  chartVersion: string;
  appVersion: string;
  revision: number;
  status: HelmReleaseStatus;
  updatedAgeSec: number;
}

// ---------------------------------------------------------------------------
// Flux (GitOps custom resources, served by the Go backend when installed)
// ---------------------------------------------------------------------------

export type FluxReady = "True" | "False" | "-";

export interface FluxObject {
  uid: string;
  /** "Kustomization" | "HelmRelease" | "GitRepository" | "OCIRepository" | "HelmRepository" | "Bucket" | ... */
  kind: string;
  name: string;
  namespace: string;
  ready: FluxReady;
  suspended: boolean;
  revision: string;
  source: string;
  /** Ordering dependencies as "namespace/name" (from spec.dependsOn). */
  dependsOn: string[];
  message: string;
  age: string;
}

/** Where the snapshot came from. "mock" means the in-browser generator. */
export type ClusterMode = "live" | "demo" | "cluster" | "mock";

// ---------------------------------------------------------------------------
// Union & aggregator
// ---------------------------------------------------------------------------

export type AnyResource =
  | Pod
  | Deployment
  | StatefulSet
  | DaemonSet
  | ReplicaSet
  | Job
  | CronJob
  | Service
  | Ingress
  | Endpoint
  | NetworkPolicy
  | ConfigMap
  | Secret
  | ResourceQuota
  | LimitRange
  | HorizontalPodAutoscaler
  | PodDisruptionBudget
  | PersistentVolume
  | PersistentVolumeClaim
  | StorageClass
  | ServiceAccount
  | Role
  | ClusterRole
  | RoleBinding
  | ClusterRoleBinding
  | Node
  | Namespace
  | Event
  | CustomResourceDefinition
  | HelmRelease;

export type AnyResourceKind = AnyResource["kind"];

export interface Cluster {
  context: string;
  currentNamespace: string;
  version: string;
  /** Seconds since this snapshot was generated; mostly informational. */
  generatedAtSec: number;
  /** Snapshot origin — live server, demo server, real cluster, or local mock. */
  mode: ClusterMode;
  /** Whether the Flux controllers are installed in the cluster. */
  fluxInstalled: boolean;
  flux: FluxObject[];
  /** Whether metrics-server is installed (drives CPU/MEM bars). */
  metricsInstalled?: boolean;

  pods: Pod[];
  deployments: Deployment[];
  statefulSets: StatefulSet[];
  daemonSets: DaemonSet[];
  replicaSets: ReplicaSet[];
  jobs: Job[];
  cronJobs: CronJob[];

  services: Service[];
  ingresses: Ingress[];
  endpoints: Endpoint[];
  networkPolicies: NetworkPolicy[];

  configMaps: ConfigMap[];
  secrets: Secret[];
  resourceQuotas: ResourceQuota[];
  limitRanges: LimitRange[];

  horizontalPodAutoscalers: HorizontalPodAutoscaler[];
  podDisruptionBudgets: PodDisruptionBudget[];

  persistentVolumes: PersistentVolume[];
  persistentVolumeClaims: PersistentVolumeClaim[];
  storageClasses: StorageClass[];

  serviceAccounts: ServiceAccount[];
  roles: Role[];
  clusterRoles: ClusterRole[];
  roleBindings: RoleBinding[];
  clusterRoleBindings: ClusterRoleBinding[];

  nodes: Node[];
  namespaces: Namespace[];
  events: Event[];

  customResourceDefinitions: CustomResourceDefinition[];
  helmReleases: HelmRelease[];
}
