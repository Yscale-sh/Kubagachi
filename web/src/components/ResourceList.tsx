/**
 * ResourceList — generic, kind-driven dense table for every Kubernetes
 * resource kind except Pod (PodList renders that with extra UX).
 *
 * Reads the cluster from the workspace store, picks the matching array,
 * filters by namespace + search, and renders a column set defined per kind
 * in COLUMNS_BY_KIND. Clicking a row selects the resource for the drawer.
 *
 * Expected workspace hooks:
 *   useCluster()   -> Cluster | null
 *   useNamespace() -> string  (e.g. "all" or "default")
 *   useSearch()    -> string
 *   workspaceActions.selectResource(uid)
 */

import { useEffect, useMemo, useState } from "react";
import { Inbox } from "lucide-react";
import type {
  AnyResource,
  AnyResourceKind,
  ClusterRole,
  ClusterRoleBinding,
  ConfigMap,
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
  PodDisruptionBudget,
  ReplicaSet,
  ResourceQuota,
  Role,
  RoleBinding,
  Secret,
  Service,
  ServiceAccount,
  StatefulSet,
  StorageClass,
} from "../lib/types";
import { formatAge, formatBytes } from "../lib/format";
import {
  useCluster,
  useNamespace,
  useSearch,
  useSelectedRow,
  workspaceActions,
} from "../store/workspace";
import { registerRowNav, clearRowNav, type RowNavRegistration } from "../lib/row-nav";
import StatusPill from "./StatusPill";

interface Column<T> {
  key: string;
  header: string;
  /** Optional class on the <th>/<td>; default left-aligned. */
  className?: string;
  render(item: T): React.ReactNode;
  /**
   * Optional accessor used for client-side column sorting. When present the
   * column header becomes clickable; strings sort lexically and numbers
   * numerically. Columns without a sortValue are not sortable.
   */
  sortValue?(item: T): string | number;
}

interface ResourceListProps {
  kind: Exclude<AnyResourceKind, "Pod">;
}

// ---------------------------------------------------------------------------
// kind -> Cluster<field> resolver
// ---------------------------------------------------------------------------

function resolveCollection(
  cluster: NonNullable<ReturnType<typeof useCluster>>,
  kind: AnyResourceKind,
): readonly AnyResource[] {
  switch (kind) {
    case "Pod": return cluster.pods;
    case "Deployment": return cluster.deployments;
    case "StatefulSet": return cluster.statefulSets;
    case "DaemonSet": return cluster.daemonSets;
    case "ReplicaSet": return cluster.replicaSets;
    case "Job": return cluster.jobs;
    case "CronJob": return cluster.cronJobs;
    case "Service": return cluster.services;
    case "Ingress": return cluster.ingresses;
    case "Endpoint": return cluster.endpoints;
    case "NetworkPolicy": return cluster.networkPolicies;
    case "ConfigMap": return cluster.configMaps;
    case "Secret": return cluster.secrets;
    case "ResourceQuota": return cluster.resourceQuotas;
    case "LimitRange": return cluster.limitRanges;
    case "HorizontalPodAutoscaler": return cluster.horizontalPodAutoscalers;
    case "PodDisruptionBudget": return cluster.podDisruptionBudgets;
    case "PersistentVolume": return cluster.persistentVolumes;
    case "PersistentVolumeClaim": return cluster.persistentVolumeClaims;
    case "StorageClass": return cluster.storageClasses;
    case "ServiceAccount": return cluster.serviceAccounts;
    case "Role": return cluster.roles;
    case "ClusterRole": return cluster.clusterRoles;
    case "RoleBinding": return cluster.roleBindings;
    case "ClusterRoleBinding": return cluster.clusterRoleBindings;
    case "Node": return cluster.nodes;
    case "Namespace": return cluster.namespaces;
    case "Event": return cluster.events;
    case "CustomResourceDefinition": return cluster.customResourceDefinitions;
    case "HelmRelease": return cluster.helmReleases;
  }
}

// ---------------------------------------------------------------------------
// Column definitions per kind. Each takes the strongly-typed resource.
// ---------------------------------------------------------------------------

const ageCol = <T extends { ageSec: number }>(): Column<T> => ({
  key: "age",
  header: "Age",
  className: "w-20 text-right",
  render: (r) => <span className="tabular-nums text-text-muted">{formatAge(r.ageSec)}</span>,
  // Higher ageSec == older; sort on the raw seconds so it's chronologically correct.
  sortValue: (r) => r.ageSec,
});

const nsCol = <T extends { namespace?: string }>(): Column<T> => ({
  key: "namespace",
  header: "Namespace",
  className: "w-40",
  render: (r) => <span className="text-text-muted">{r.namespace ?? "—"}</span>,
  sortValue: (r) => r.namespace ?? "",
});

const nameCol = <T extends { name: string }>(width = "min-w-[200px]"): Column<T> => ({
  key: "name",
  header: "Name",
  className: width,
  render: (r) => <span className="text-text font-medium truncate">{r.name}</span>,
  sortValue: (r) => r.name,
});

const COLUMNS: { [K in AnyResourceKind]?: Column<never>[] } = {
  Deployment: [
    nameCol<Deployment>(),
    nsCol<Deployment>(),
    {
      key: "pods",
      header: "Pods",
      className: "w-20 text-right",
      render: (d: Deployment) => (
        <span className="tabular-nums">
          {d.readyReplicas}/{d.replicas}
        </span>
      ),
      sortValue: (d: Deployment) => d.replicas,
    },
    {
      key: "updated",
      header: "Updated",
      className: "w-20 text-right",
      render: (d: Deployment) => <span className="tabular-nums">{d.updatedReplicas}</span>,
      sortValue: (d: Deployment) => d.updatedReplicas,
    },
    {
      key: "available",
      header: "Available",
      className: "w-20 text-right",
      render: (d: Deployment) => <span className="tabular-nums">{d.availableReplicas}</span>,
      sortValue: (d: Deployment) => d.availableReplicas,
    },
    {
      key: "strategy",
      header: "Strategy",
      className: "w-32",
      render: (d: Deployment) => <span className="text-text-muted">{d.strategy}</span>,
    },
    {
      key: "status",
      header: "Status",
      className: "w-32",
      render: (d: Deployment) => <StatusPill status={d.status} />,
    },
    ageCol<Deployment>(),
  ] as Column<never>[],

  StatefulSet: [
    nameCol<StatefulSet>(),
    nsCol<StatefulSet>(),
    {
      key: "pods",
      header: "Pods",
      className: "w-20 text-right",
      render: (s: StatefulSet) => (
        <span className="tabular-nums">{s.readyReplicas}/{s.replicas}</span>
      ),
    },
    {
      key: "service",
      header: "Service",
      className: "min-w-[140px]",
      render: (s: StatefulSet) => <span className="text-text-muted">{s.serviceName}</span>,
    },
    {
      key: "status",
      header: "Status",
      className: "w-32",
      render: (s: StatefulSet) => <StatusPill status={s.status} />,
    },
    ageCol<StatefulSet>(),
  ] as Column<never>[],

  DaemonSet: [
    nameCol<DaemonSet>(),
    nsCol<DaemonSet>(),
    {
      key: "desired",
      header: "Desired",
      className: "w-20 text-right",
      render: (d: DaemonSet) => <span className="tabular-nums">{d.desiredNumberScheduled}</span>,
    },
    {
      key: "ready",
      header: "Ready",
      className: "w-20 text-right",
      render: (d: DaemonSet) => <span className="tabular-nums">{d.numberReady}</span>,
    },
    {
      key: "available",
      header: "Available",
      className: "w-20 text-right",
      render: (d: DaemonSet) => <span className="tabular-nums">{d.numberAvailable}</span>,
    },
    {
      key: "status",
      header: "Status",
      className: "w-32",
      render: (d: DaemonSet) => <StatusPill status={d.status} />,
    },
    ageCol<DaemonSet>(),
  ] as Column<never>[],

  ReplicaSet: [
    nameCol<ReplicaSet>(),
    nsCol<ReplicaSet>(),
    {
      key: "desired",
      header: "Desired",
      className: "w-20 text-right",
      render: (r: ReplicaSet) => <span className="tabular-nums">{r.replicas}</span>,
    },
    {
      key: "ready",
      header: "Ready",
      className: "w-20 text-right",
      render: (r: ReplicaSet) => <span className="tabular-nums">{r.readyReplicas}</span>,
    },
    {
      key: "owner",
      header: "Owner",
      className: "min-w-[160px]",
      render: (r: ReplicaSet) => (
        <span className="text-text-muted">
          {r.ownerKind ?? "—"}/{r.ownerName ?? "—"}
        </span>
      ),
    },
    ageCol<ReplicaSet>(),
  ] as Column<never>[],

  Job: [
    nameCol<Job>(),
    nsCol<Job>(),
    {
      key: "completions",
      header: "Completions",
      className: "w-28 text-right",
      render: (j: Job) => (
        <span className="tabular-nums">{j.succeeded}/{j.completions}</span>
      ),
    },
    {
      key: "duration",
      header: "Duration",
      className: "w-20 text-right",
      render: (j: Job) => (
        <span className="tabular-nums text-text-muted">
          {j.durationSec != null ? formatAge(j.durationSec) : "—"}
        </span>
      ),
    },
    {
      key: "status",
      header: "Status",
      className: "w-32",
      render: (j: Job) => <StatusPill status={j.status} />,
    },
    ageCol<Job>(),
  ] as Column<never>[],

  CronJob: [
    nameCol<CronJob>(),
    nsCol<CronJob>(),
    {
      key: "schedule",
      header: "Schedule",
      className: "min-w-[140px]",
      render: (c: CronJob) => <code className="text-accent">{c.schedule}</code>,
    },
    {
      key: "suspend",
      header: "Suspend",
      className: "w-20",
      render: (c: CronJob) => <span className="text-text-muted">{c.suspend ? "true" : "false"}</span>,
    },
    {
      key: "active",
      header: "Active",
      className: "w-16 text-right",
      render: (c: CronJob) => <span className="tabular-nums">{c.activeJobs}</span>,
    },
    {
      key: "lastSchedule",
      header: "Last schedule",
      className: "w-28 text-right",
      render: (c: CronJob) => (
        <span className="tabular-nums text-text-muted">
          {c.lastScheduleAgeSec != null ? formatAge(c.lastScheduleAgeSec) : "—"}
        </span>
      ),
    },
    ageCol<CronJob>(),
  ] as Column<never>[],

  Service: [
    nameCol<Service>(),
    nsCol<Service>(),
    {
      key: "type",
      header: "Type",
      className: "w-32",
      render: (s: Service) => <span className="text-text-muted">{s.type}</span>,
    },
    {
      key: "clusterIP",
      header: "ClusterIP",
      className: "w-32",
      render: (s: Service) => <code className="text-text">{s.clusterIP}</code>,
    },
    {
      key: "externalIP",
      header: "External IP",
      className: "w-32",
      render: (s: Service) => (
        <code className="text-text-muted">{s.externalIP ?? "—"}</code>
      ),
    },
    {
      key: "ports",
      header: "Ports",
      className: "min-w-[140px]",
      render: (s: Service) => (
        <span className="text-text-muted">
          {s.ports.map((p) => `${p.port}/${p.protocol}`).join(", ")}
        </span>
      ),
    },
    ageCol<Service>(),
  ] as Column<never>[],

  Ingress: [
    nameCol<Ingress>(),
    nsCol<Ingress>(),
    {
      key: "class",
      header: "Class",
      className: "w-24",
      render: (i: Ingress) => <span className="text-text-muted">{i.className ?? "—"}</span>,
    },
    {
      key: "hosts",
      header: "Hosts",
      className: "min-w-[200px]",
      render: (i: Ingress) => (
        <span className="text-text truncate">{i.hosts.join(", ")}</span>
      ),
    },
    {
      key: "address",
      header: "Address",
      className: "w-32",
      render: (i: Ingress) => <code className="text-text-muted">{i.address ?? "—"}</code>,
    },
    {
      key: "tls",
      header: "TLS",
      className: "w-16",
      render: (i: Ingress) => <span className="text-text-muted">{i.tls ? "yes" : "no"}</span>,
    },
    ageCol<Ingress>(),
  ] as Column<never>[],

  Endpoint: [
    nameCol<Endpoint>(),
    nsCol<Endpoint>(),
    {
      key: "subsets",
      header: "Endpoints",
      className: "min-w-[200px]",
      render: (e: Endpoint) => (
        <span className="text-text-muted truncate">
          {e.subsets
            .flatMap((s) => s.addresses.map((a) => `${a}:${s.ports.join("/")}`))
            .slice(0, 6)
            .join(", ") || "—"}
        </span>
      ),
    },
    ageCol<Endpoint>(),
  ] as Column<never>[],

  NetworkPolicy: [
    nameCol<NetworkPolicy>(),
    nsCol<NetworkPolicy>(),
    {
      key: "policyTypes",
      header: "Policy types",
      className: "w-32",
      render: (n: NetworkPolicy) => (
        <span className="text-text-muted">{n.policyTypes.join(", ")}</span>
      ),
    },
    {
      key: "rules",
      header: "Rules (ingress/egress)",
      className: "w-40 text-right",
      render: (n: NetworkPolicy) => (
        <span className="tabular-nums">
          {n.ingressRules}/{n.egressRules}
        </span>
      ),
    },
    ageCol<NetworkPolicy>(),
  ] as Column<never>[],

  ConfigMap: [
    nameCol<ConfigMap>(),
    nsCol<ConfigMap>(),
    {
      key: "keys",
      header: "Keys",
      className: "w-20 text-right",
      render: (c: ConfigMap) => <span className="tabular-nums">{c.dataKeys.length}</span>,
    },
    {
      key: "size",
      header: "Size",
      className: "w-24 text-right",
      render: (c: ConfigMap) => (
        <span className="tabular-nums text-text-muted">{formatBytes(c.sizeBytes)}</span>
      ),
    },
    ageCol<ConfigMap>(),
  ] as Column<never>[],

  Secret: [
    nameCol<Secret>(),
    nsCol<Secret>(),
    {
      key: "type",
      header: "Type",
      className: "min-w-[180px]",
      render: (s: Secret) => <span className="text-text-muted">{s.type}</span>,
    },
    {
      key: "keys",
      header: "Keys",
      className: "w-20 text-right",
      render: (s: Secret) => <span className="tabular-nums">{s.dataKeys.length}</span>,
    },
    {
      key: "size",
      header: "Size",
      className: "w-24 text-right",
      render: (s: Secret) => (
        <span className="tabular-nums text-text-muted">{formatBytes(s.sizeBytes)}</span>
      ),
    },
    ageCol<Secret>(),
  ] as Column<never>[],

  ResourceQuota: [
    nameCol<ResourceQuota>(),
    nsCol<ResourceQuota>(),
    {
      key: "hard",
      header: "Hard",
      className: "min-w-[180px]",
      render: (q: ResourceQuota) => (
        <span className="text-text-muted truncate">
          {Object.entries(q.hard).map(([k, v]) => `${k}=${v}`).join(", ")}
        </span>
      ),
    },
    {
      key: "used",
      header: "Used",
      className: "min-w-[180px]",
      render: (q: ResourceQuota) => (
        <span className="text-text-muted truncate">
          {Object.entries(q.used).map(([k, v]) => `${k}=${v}`).join(", ")}
        </span>
      ),
    },
    ageCol<ResourceQuota>(),
  ] as Column<never>[],

  LimitRange: [
    nameCol<LimitRange>(),
    nsCol<LimitRange>(),
    {
      key: "limits",
      header: "Limit items",
      className: "w-32 text-right",
      render: (l: LimitRange) => <span className="tabular-nums">{l.limits.length}</span>,
    },
    ageCol<LimitRange>(),
  ] as Column<never>[],

  HorizontalPodAutoscaler: [
    nameCol<HorizontalPodAutoscaler>(),
    nsCol<HorizontalPodAutoscaler>(),
    {
      key: "target",
      header: "Target",
      className: "min-w-[180px]",
      render: (h: HorizontalPodAutoscaler) => (
        <span className="text-text-muted">{h.targetKind}/{h.targetName}</span>
      ),
    },
    {
      key: "replicas",
      header: "Replicas (cur/min/max)",
      className: "w-40 text-right",
      render: (h: HorizontalPodAutoscaler) => (
        <span className="tabular-nums">
          {h.currentReplicas}/{h.minReplicas}/{h.maxReplicas}
        </span>
      ),
    },
    {
      key: "cpu",
      header: "CPU (cur/target)",
      className: "w-32 text-right",
      render: (h: HorizontalPodAutoscaler) => (
        <span className="tabular-nums">
          {h.currentCPUPercent ?? "—"}%/{h.targetCPUPercent ?? "—"}%
        </span>
      ),
    },
    ageCol<HorizontalPodAutoscaler>(),
  ] as Column<never>[],

  PodDisruptionBudget: [
    nameCol<PodDisruptionBudget>(),
    nsCol<PodDisruptionBudget>(),
    {
      key: "min",
      header: "Min available",
      className: "w-28 text-right",
      render: (p: PodDisruptionBudget) => (
        <span className="tabular-nums">{p.minAvailable ?? "—"}</span>
      ),
    },
    {
      key: "max",
      header: "Max unavailable",
      className: "w-32 text-right",
      render: (p: PodDisruptionBudget) => (
        <span className="tabular-nums">{p.maxUnavailable ?? "—"}</span>
      ),
    },
    {
      key: "current",
      header: "Current healthy",
      className: "w-32 text-right",
      render: (p: PodDisruptionBudget) => (
        <span className="tabular-nums">{p.currentHealthy}/{p.expectedPods}</span>
      ),
    },
    ageCol<PodDisruptionBudget>(),
  ] as Column<never>[],

  PersistentVolume: [
    nameCol<PersistentVolume>(),
    {
      key: "capacity",
      header: "Capacity",
      className: "w-24 text-right",
      render: (p: PersistentVolume) => <span className="tabular-nums">{p.capacity}</span>,
    },
    {
      key: "access",
      header: "Access",
      className: "w-32",
      render: (p: PersistentVolume) => (
        <span className="text-text-muted">{p.accessModes.join(",")}</span>
      ),
    },
    {
      key: "reclaim",
      header: "Reclaim",
      className: "w-24",
      render: (p: PersistentVolume) => <span className="text-text-muted">{p.reclaimPolicy}</span>,
    },
    {
      key: "phase",
      header: "Phase",
      className: "w-28",
      render: (p: PersistentVolume) => <StatusPill status={p.phase} />,
    },
    {
      key: "sc",
      header: "StorageClass",
      className: "w-32",
      render: (p: PersistentVolume) => <span className="text-text-muted">{p.storageClassName}</span>,
    },
    {
      key: "claim",
      header: "Claim",
      className: "min-w-[200px]",
      render: (p: PersistentVolume) => (
        <span className="text-text-muted">
          {p.claimRef ? `${p.claimRef.namespace}/${p.claimRef.name}` : "—"}
        </span>
      ),
    },
    ageCol<PersistentVolume>(),
  ] as Column<never>[],

  PersistentVolumeClaim: [
    nameCol<PersistentVolumeClaim>(),
    nsCol<PersistentVolumeClaim>(),
    {
      key: "capacity",
      header: "Capacity",
      className: "w-24 text-right",
      render: (p: PersistentVolumeClaim) => <span className="tabular-nums">{p.capacity}</span>,
    },
    {
      key: "access",
      header: "Access",
      className: "w-32",
      render: (p: PersistentVolumeClaim) => (
        <span className="text-text-muted">{p.accessModes.join(",")}</span>
      ),
    },
    {
      key: "sc",
      header: "StorageClass",
      className: "w-32",
      render: (p: PersistentVolumeClaim) => <span className="text-text-muted">{p.storageClassName}</span>,
    },
    {
      key: "phase",
      header: "Phase",
      className: "w-28",
      render: (p: PersistentVolumeClaim) => <StatusPill status={p.phase} />,
    },
    {
      key: "volume",
      header: "Volume",
      className: "min-w-[140px]",
      render: (p: PersistentVolumeClaim) => (
        <span className="text-text-muted">{p.volumeName ?? "—"}</span>
      ),
    },
    ageCol<PersistentVolumeClaim>(),
  ] as Column<never>[],

  StorageClass: [
    nameCol<StorageClass>(),
    {
      key: "provisioner",
      header: "Provisioner",
      className: "min-w-[180px]",
      render: (s: StorageClass) => <span className="text-text-muted">{s.provisioner}</span>,
    },
    {
      key: "reclaim",
      header: "Reclaim",
      className: "w-24",
      render: (s: StorageClass) => <span className="text-text-muted">{s.reclaimPolicy}</span>,
    },
    {
      key: "binding",
      header: "Binding",
      className: "w-40",
      render: (s: StorageClass) => <span className="text-text-muted">{s.volumeBindingMode}</span>,
    },
    {
      key: "default",
      header: "Default",
      className: "w-20",
      render: (s: StorageClass) => (
        <span className="text-text-muted">{s.isDefault ? "yes" : "no"}</span>
      ),
    },
    ageCol<StorageClass>(),
  ] as Column<never>[],

  ServiceAccount: [
    nameCol<ServiceAccount>(),
    nsCol<ServiceAccount>(),
    {
      key: "secrets",
      header: "Secrets",
      className: "w-20 text-right",
      render: (s: ServiceAccount) => <span className="tabular-nums">{s.secrets.length}</span>,
    },
    {
      key: "pull",
      header: "Image pull secrets",
      className: "w-40 text-right",
      render: (s: ServiceAccount) => (
        <span className="tabular-nums">{s.imagePullSecrets.length}</span>
      ),
    },
    {
      key: "automount",
      header: "Automount",
      className: "w-24",
      render: (s: ServiceAccount) => (
        <span className="text-text-muted">{s.automountToken ? "yes" : "no"}</span>
      ),
    },
    ageCol<ServiceAccount>(),
  ] as Column<never>[],

  Role: [
    nameCol<Role>(),
    nsCol<Role>(),
    {
      key: "rules",
      header: "Rules",
      className: "w-20 text-right",
      render: (r: Role) => <span className="tabular-nums">{r.rules.length}</span>,
    },
    ageCol<Role>(),
  ] as Column<never>[],

  ClusterRole: [
    nameCol<ClusterRole>(),
    {
      key: "rules",
      header: "Rules",
      className: "w-20 text-right",
      render: (r: ClusterRole) => <span className="tabular-nums">{r.rules.length}</span>,
    },
    ageCol<ClusterRole>(),
  ] as Column<never>[],

  RoleBinding: [
    nameCol<RoleBinding>(),
    nsCol<RoleBinding>(),
    {
      key: "role",
      header: "Role",
      className: "min-w-[180px]",
      render: (r: RoleBinding) => (
        <span className="text-text-muted">{r.roleRef.kind}/{r.roleRef.name}</span>
      ),
    },
    {
      key: "subjects",
      header: "Subjects",
      className: "w-20 text-right",
      render: (r: RoleBinding) => <span className="tabular-nums">{r.subjects.length}</span>,
    },
    ageCol<RoleBinding>(),
  ] as Column<never>[],

  ClusterRoleBinding: [
    nameCol<ClusterRoleBinding>(),
    {
      key: "role",
      header: "Role",
      className: "min-w-[180px]",
      render: (r: ClusterRoleBinding) => (
        <span className="text-text-muted">{r.roleRef.kind}/{r.roleRef.name}</span>
      ),
    },
    {
      key: "subjects",
      header: "Subjects",
      className: "w-20 text-right",
      render: (r: ClusterRoleBinding) => <span className="tabular-nums">{r.subjects.length}</span>,
    },
    ageCol<ClusterRoleBinding>(),
  ] as Column<never>[],

  Node: [
    nameCol<Node>(),
    {
      key: "status",
      header: "Status",
      className: "w-28",
      render: (n: Node) => <StatusPill status={n.status} />,
    },
    {
      key: "roles",
      header: "Roles",
      className: "w-32",
      render: (n: Node) => <span className="text-text-muted">{n.roles.join(",")}</span>,
    },
    {
      key: "version",
      header: "Version",
      className: "w-24",
      render: (n: Node) => <span className="text-text-muted">{n.kubeletVersion}</span>,
    },
    {
      key: "arch",
      header: "Arch",
      className: "w-20",
      render: (n: Node) => <span className="text-text-muted">{n.arch}</span>,
    },
    {
      key: "cpu",
      header: "CPU",
      className: "w-20 text-right",
      render: (n: Node) => <span className="tabular-nums">{n.cpuCapacity}</span>,
    },
    {
      key: "mem",
      header: "Memory",
      className: "w-24 text-right",
      render: (n: Node) => <span className="tabular-nums">{n.memCapacity}</span>,
    },
    {
      key: "pods",
      header: "Pods",
      className: "w-20 text-right",
      render: (n: Node) => (
        <span className="tabular-nums">{n.podCount}/{n.podCapacity}</span>
      ),
    },
    ageCol<Node>(),
  ] as Column<never>[],

  Namespace: [
    nameCol<Namespace>(),
    {
      key: "phase",
      header: "Phase",
      className: "w-28",
      render: (n: Namespace) => <StatusPill status={n.phase} />,
    },
    ageCol<Namespace>(),
  ] as Column<never>[],

  Event: [
    {
      key: "type",
      header: "Type",
      className: "w-24",
      render: (e: Event) => <StatusPill status={e.type} compact />,
    },
    {
      key: "reason",
      header: "Reason",
      className: "w-32",
      render: (e: Event) => <span className="text-text">{e.reason}</span>,
    },
    {
      key: "object",
      header: "Object",
      className: "min-w-[200px]",
      render: (e: Event) => (
        <span className="text-text-muted">
          {e.involvedObject.kind}/{e.involvedObject.name}
        </span>
      ),
    },
    {
      key: "message",
      header: "Message",
      className: "min-w-[260px]",
      render: (e: Event) => <span className="text-text truncate">{e.message}</span>,
    },
    {
      key: "count",
      header: "Count",
      className: "w-16 text-right",
      render: (e: Event) => <span className="tabular-nums">{e.count}</span>,
    },
    {
      key: "lastSeen",
      header: "Last seen",
      className: "w-24 text-right",
      render: (e: Event) => (
        <span className="tabular-nums text-text-muted">{formatAge(e.lastSeenSec)}</span>
      ),
    },
  ] as Column<never>[],

  CustomResourceDefinition: [
    nameCol<CustomResourceDefinition>("min-w-[260px]"),
    {
      key: "group",
      header: "Group",
      className: "min-w-[180px]",
      render: (c: CustomResourceDefinition) => <span className="text-text-muted">{c.group}</span>,
    },
    {
      key: "scope",
      header: "Scope",
      className: "w-28",
      render: (c: CustomResourceDefinition) => <span className="text-text-muted">{c.scope}</span>,
    },
    {
      key: "versions",
      header: "Versions",
      className: "w-32",
      render: (c: CustomResourceDefinition) => (
        <span className="text-text-muted">{c.versions.join(",")}</span>
      ),
    },
    ageCol<CustomResourceDefinition>(),
  ] as Column<never>[],

  HelmRelease: [
    nameCol<HelmRelease>(),
    nsCol<HelmRelease>(),
    {
      key: "chart",
      header: "Chart",
      className: "min-w-[180px]",
      render: (h: HelmRelease) => (
        <span className="text-text-muted">{h.chart}@{h.chartVersion}</span>
      ),
    },
    {
      key: "app",
      header: "App version",
      className: "w-28",
      render: (h: HelmRelease) => <span className="text-text-muted">{h.appVersion}</span>,
    },
    {
      key: "rev",
      header: "Rev",
      className: "w-16 text-right",
      render: (h: HelmRelease) => <span className="tabular-nums">{h.revision}</span>,
    },
    {
      key: "status",
      header: "Status",
      className: "w-32",
      render: (h: HelmRelease) => <StatusPill status={h.status} />,
    },
    {
      key: "updated",
      header: "Updated",
      className: "w-24 text-right",
      render: (h: HelmRelease) => (
        <span className="tabular-nums text-text-muted">{formatAge(h.updatedAgeSec)}</span>
      ),
    },
    ageCol<HelmRelease>(),
  ] as Column<never>[],
};

// ---------------------------------------------------------------------------
// The actual component
// ---------------------------------------------------------------------------

export default function ResourceList({ kind }: ResourceListProps) {
  const cluster = useCluster();
  const namespace = useNamespace();
  const search = useSearch();

  const cols = COLUMNS[kind] as Column<AnyResource>[] | undefined;

  const filtered = useMemo<AnyResource[]>(() => {
    if (!cluster) return [];
    const items = resolveCollection(cluster, kind);
    const q = search.trim().toLowerCase();
    return items.filter((it) => {
      if (
        namespace &&
        namespace !== "all" &&
        "namespace" in it &&
        it.namespace !== namespace
      ) {
        return false;
      }
      if (q && !it.name.toLowerCase().includes(q)) return false;
      return true;
    });
  }, [cluster, kind, namespace, search]);

  if (!cluster) {
    return (
      <div className="p-6 text-text-muted text-sm">Loading cluster…</div>
    );
  }

  if (filtered.length === 0) {
    // Distinguish "nothing in scope" from "a filter hid everything", and offer
    // a one-click way to clear whichever filter is active.
    const nsActive = !!namespace && namespace !== "all";
    const searchActive = search.trim().length > 0;
    const filterActive = nsActive || searchActive;
    const totalInScope = resolveCollection(cluster, kind).length;
    // If a filter is on AND there is data that the filter is hiding, show the
    // "no match" copy; otherwise the scope is genuinely empty.
    const filteredOut = filterActive && totalInScope > 0;

    return (
      <div className="flex flex-col items-center justify-center py-16 text-text-muted gap-2">
        <Inbox className="w-6 h-6 opacity-50" />
        {filteredOut ? (
          <>
            <div className="text-xs">
              No {kind} match the current filter
              {searchActive ? (
                <>
                  {" "}
                  <code className="text-accent">“{search.trim()}”</code>
                </>
              ) : null}
              {nsActive ? (
                <>
                  {" "}in namespace <code className="text-accent">{namespace}</code>
                </>
              ) : null}
              .
            </div>
            <div className="flex items-center gap-2">
              {searchActive ? (
                <button
                  type="button"
                  onClick={() => workspaceActions.setSearch("")}
                  className="text-[11px] px-2 py-0.5 border border-accent-soft text-accent hover:bg-accent-dim transition-colors k9s-square"
                >
                  clear search
                </button>
              ) : null}
              {nsActive ? (
                <button
                  type="button"
                  onClick={() => workspaceActions.setNamespace("all")}
                  className="text-[11px] px-2 py-0.5 border border-accent-soft text-accent hover:bg-accent-dim transition-colors k9s-square"
                >
                  all namespaces
                </button>
              ) : null}
            </div>
          </>
        ) : (
          <div className="text-xs">No {kind} in this scope.</div>
        )}
      </div>
    );
  }

  if (!cols) {
    return (
      <div className="p-6 text-text-muted text-sm">
        No column definition for kind <code>{kind}</code>.
      </div>
    );
  }

  return (
    <ResourceTable cols={cols} rows={filtered} />
  );
}

// ---------------------------------------------------------------------------
// Reusable dense table renderer (also used by EventsView).
// ---------------------------------------------------------------------------

interface SortState {
  /** Column key currently driving the sort, or null for natural order. */
  key: string | null;
  dir: "asc" | "desc";
}

export function ResourceTable({
  cols,
  rows,
}: {
  cols: Column<AnyResource>[];
  rows: AnyResource[];
}) {
  const selectedRow = useSelectedRow();
  const [sort, setSort] = useState<SortState>({ key: null, dir: "asc" });

  // Apply client-side sorting when a sortable header has been chosen. The
  // original `rows` array is left untouched (we copy before sorting).
  const sortedRows = useMemo<AnyResource[]>(() => {
    if (!sort.key) return rows;
    const col = cols.find((c) => c.key === sort.key);
    if (!col?.sortValue) return rows;
    const accessor = col.sortValue;
    const factor = sort.dir === "asc" ? 1 : -1;
    return [...rows].sort((a, b) => {
      const va = accessor(a);
      const vb = accessor(b);
      if (typeof va === "number" && typeof vb === "number") {
        return (va - vb) * factor;
      }
      return String(va).localeCompare(String(vb)) * factor;
    });
  }, [rows, cols, sort]);

  // Register the *visible, sorted* row ids so j/k/Enter follow what the user
  // sees. The global KeyboardLayer owns the keys; we only publish ids + onEnter.
  useEffect(() => {
    const reg: RowNavRegistration = {
      ids: sortedRows.map((r) => r.uid),
      onEnter: (uid) => workspaceActions.selectResource(uid),
    };
    registerRowNav(reg);
    return () => clearRowNav(reg);
  }, [sortedRows]);

  const toggleSort = (key: string): void => {
    setSort((prev) =>
      prev.key === key
        ? { key, dir: prev.dir === "asc" ? "desc" : "asc" }
        : { key, dir: "asc" },
    );
  };

  return (
    <>
      {/* pb-20 keeps the last rows scrollable clear of the floating HotbarDock. */}
      <div className="hidden sm:block font-mono overflow-auto scrollbar-thin pb-20" style={{ maxHeight: "calc(100vh - 9rem)" }}>
        <table className="w-max min-w-full text-[12px] border-separate border-spacing-0">
          <thead className="sticky top-0 z-10 bg-bg-panel">
            <tr>
              {cols.map((c) => {
                const sortable = !!c.sortValue;
                const active = sort.key === c.key;
                return (
                  <th
                    key={c.key}
                    aria-sort={
                      active ? (sort.dir === "asc" ? "ascending" : "descending") : undefined
                    }
                    onClick={sortable ? () => toggleSort(c.key) : undefined}
                    className={
                      `text-left font-normal uppercase tracking-wider text-[10px] px-3 py-1.5 border-b border-border ${c.className ?? ""} ` +
                      (sortable
                        ? "cursor-pointer select-none hover:text-text "
                        : "") +
                      (active ? "text-accent" : "text-text-muted/80")
                    }
                  >
                    {c.header}
                    {sortable ? (
                      <span
                        aria-hidden="true"
                        className={"ml-1 " + (active ? "text-accent" : "text-text-muted/30")}
                      >
                        {active ? (sort.dir === "asc" ? "▲" : "▼") : "↕"}
                      </span>
                    ) : null}
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {sortedRows.map((row, i) => {
              const rowSelected = i === selectedRow;
              return (
                <tr
                  key={row.uid}
                  data-row-selected={rowSelected || undefined}
                  onClick={() => workspaceActions.selectResource(row.uid)}
                  className={
                    "cursor-pointer hover:bg-bg-panel2 transition-colors duration-100 " +
                    (rowSelected ? "bg-accent-dim shadow-[inset_2px_0_0_0_#c9b88a]" : "")
                  }
                  style={{ height: 30 }}
                >
                  {cols.map((c) => (
                    <td
                      key={c.key}
                      className={`px-3 py-1 border-b border-border/70 align-middle truncate ${c.className ?? ""}`}
                    >
                      {c.render(row)}
                    </td>
                  ))}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <MobileCards cols={cols} rows={sortedRows} selectedRow={selectedRow} />
    </>
  );
}

function MobileCards({
  cols,
  rows,
  selectedRow,
}: {
  cols: Column<AnyResource>[];
  rows: AnyResource[];
  selectedRow: number;
}) {
  return (
    <div className="sm:hidden flex flex-col gap-2 p-2 font-mono">
      {rows.map((row, i) => {
        const rowSelected = i === selectedRow;
        return (
        <button
          key={row.uid}
          data-row-selected={rowSelected || undefined}
          onClick={() => workspaceActions.selectResource(row.uid)}
          className={
            "text-left bg-bg-panel border p-3 hover:border-border-strong transition-colors k9s-square " +
            (rowSelected ? "border-accent bg-accent-dim" : "border-border")
          }
        >
          <div className="text-[13px] font-medium text-text mb-1">{row.name}</div>
          <div className="grid grid-cols-2 gap-x-3 gap-y-1 text-[11px]">
            {cols.slice(1).map((c) => (
              <div key={c.key} className="flex flex-col">
                <span className="text-text-muted uppercase text-[9px] tracking-wider">
                  {c.header}
                </span>
                <div className="truncate">{c.render(row)}</div>
              </div>
            ))}
          </div>
        </button>
        );
      })}
    </div>
  );
}
