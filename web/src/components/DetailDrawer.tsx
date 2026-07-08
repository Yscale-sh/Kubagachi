/**
 * DetailDrawer — right-sheet on desktop (440px), bottom-sheet on mobile.
 *
 * Receives the currently selected resource (looked up from `cluster` by uid),
 * renders header + tab strip + tab body. Pods get extra Logs/Shell tabs and
 * a big animated CritterPlayer in the overview.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  Boxes,
  Check,
  ChevronRight,
  Edit2,
  Eye,
  EyeOff,
  FileText,
  Lock,
  LockOpen,
  Network,
  Pin,
  PinOff,
  RefreshCw,
  ScrollText,
  Square,
  Terminal,
  Trash2,
  X,
} from "lucide-react";
import type { AnyResource, CustomResourceDefinition, Event, HelmRelease, Node, Pod, PodStatus } from "../lib/types";
import { formatAge } from "../lib/format";
import {
  applyObjectYaml,
  cordonNode,
  deleteResource,
  fetchCustomResources,
  fetchHelmHistory,
  fetchHelmRelease,
  fetchLogs,
  fetchObjectYaml,
  fetchSecretData,
  helmRollback,
  helmUninstall,
  listPortForwards,
  restartResource,
  scaleResource,
  startPortForward,
  stopPortForward,
} from "../lib/cluster-api";
import type { ActionResult, CustomResourceRef, HelmDetailResult, HelmRevisionInfo, PortForwardInfo, SecretEntry } from "../lib/cluster-api";
import {
  usePinnedPodUids,
  useCluster,
  useDrawerTab,
  useSelection,
  workspaceActions,
} from "../store/workspace";
import StatusPill from "./StatusPill";
import ConfirmButton from "./ConfirmButton";
import CritterPlayer from "./CritterPlayer";

/** Treat demo + the legacy "mock" mode as "not a real cluster" for honesty gating. */
function isMockMode(mode: string | undefined): boolean {
  return mode === "demo" || mode === "mock";
}

type TabId = "overview" | "yaml" | "instances" | "history" | "values" | "events" | "logs" | "shell";

const ALL_RESOURCE_KEYS: ReadonlyArray<keyof ReturnType<typeof useCluster> & string> = [
  "pods",
  "deployments",
  "statefulSets",
  "daemonSets",
  "replicaSets",
  "jobs",
  "cronJobs",
  "services",
  "ingresses",
  "endpoints",
  "networkPolicies",
  "configMaps",
  "secrets",
  "resourceQuotas",
  "limitRanges",
  "horizontalPodAutoscalers",
  "podDisruptionBudgets",
  "persistentVolumes",
  "persistentVolumeClaims",
  "storageClasses",
  "serviceAccounts",
  "roles",
  "clusterRoles",
  "roleBindings",
  "clusterRoleBindings",
  "nodes",
  "namespaces",
  "events",
  "customResourceDefinitions",
  "helmReleases",
] as unknown as ReadonlyArray<keyof ReturnType<typeof useCluster> & string>;

function findResource(
  cluster: NonNullable<ReturnType<typeof useCluster>>,
  uid: string,
): AnyResource | null {
  for (const key of ALL_RESOURCE_KEYS) {
    const arr = (cluster as unknown as Record<string, AnyResource[]>)[key];
    if (!Array.isArray(arr)) continue;
    const found = arr.find((r) => r.uid === uid);
    if (found) return found;
  }
  return null;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function DetailDrawer() {
  const cluster = useCluster();
  const selectedUid = useSelection();
  const requestedTab = useDrawerTab();
  const [tab, setTab] = useState<TabId>("overview");

  const resource = useMemo(() => {
    if (!cluster || !selectedUid) return null;
    return findResource(cluster, selectedUid);
  }, [cluster, selectedUid]);

  const isPod = resource?.kind === "Pod";
  const isCRD = resource?.kind === "CustomResourceDefinition";
  const isHelmRelease = resource?.kind === "HelmRelease";

  // helmCli tracks whether the helm binary is available on the kubagachi host.
  // Fetched once per HelmRelease selection via the history endpoint.
  const [helmCli, setHelmCli] = useState(false);

  useEffect(() => {
    if (!isHelmRelease || !resource) {
      setHelmCli(false);
      return;
    }
    const helm = resource as HelmRelease;
    const cancelled = { v: false };
    void fetchHelmHistory(helm.namespace ?? "", helm.name).then((res) => {
      if (!cancelled.v) setHelmCli(res.helmCli);
    });
    return () => { cancelled.v = true; };
  }, [isHelmRelease, resource]);

  // The set of tab ids this resource actually exposes.
  const tabIds = useMemo<TabId[]>(() => {
    const ids: TabId[] = ["overview", "yaml"];
    if (isCRD) ids.push("instances");
    if (isHelmRelease) ids.push("history", "values");
    ids.push("events");
    if (isPod) ids.push("logs", "shell");
    return ids;
  }, [isCRD, isPod, isHelmRelease]);

  // When a new resource is selected, honor the requested drawerTab if it's one
  // this resource exposes; otherwise fall back to "overview".
  useEffect(() => {
    if (!selectedUid) return;
    const want = requestedTab as TabId | null;
    setTab(want && tabIds.includes(want) ? want : "overview");
    // Keyed on selectedUid + requestedTab so a deep-link re-selecting the same
    // uid with a different tab still switches.
  }, [selectedUid, requestedTab, tabIds]);

  if (!resource) return null;

  const close = () => workspaceActions.selectResource(null);

  const tabs: { id: TabId; label: string; icon: React.ReactNode }[] = [
    { id: "overview", label: "Overview", icon: <Boxes className="w-3.5 h-3.5" /> },
    { id: "yaml", label: "YAML", icon: <FileText className="w-3.5 h-3.5" /> },
    ...(isCRD
      ? [{ id: "instances" as const, label: "Instances", icon: <Boxes className="w-3.5 h-3.5" /> }]
      : []),
    ...(isHelmRelease
      ? [
          { id: "history" as const, label: "History", icon: <Activity className="w-3.5 h-3.5" /> },
          { id: "values" as const, label: "Values", icon: <ScrollText className="w-3.5 h-3.5" /> },
        ]
      : []),
    { id: "events", label: "Events", icon: <Activity className="w-3.5 h-3.5" /> },
    ...(isPod
      ? [
          { id: "logs" as const, label: "Logs", icon: <ScrollText className="w-3.5 h-3.5" /> },
          { id: "shell" as const, label: "Shell", icon: <Terminal className="w-3.5 h-3.5" /> },
        ]
      : []),
  ];

  return (
    <>
      {/* Scrim for mobile (and faint backdrop for desktop) */}
      <div
        className="fixed inset-0 z-30 bg-black/40 backdrop-blur-[1px] sm:bg-black/20"
        onClick={close}
        aria-hidden="true"
      />

      <aside
        role="dialog"
        aria-label={`${resource.kind} ${resource.name}`}
        className="fixed z-40 bg-bg-panel border-border flex flex-col
                   inset-x-0 bottom-0 h-[92vh] border-t rounded-t-lg
                   sm:inset-y-0 sm:right-0 sm:left-auto sm:bottom-auto sm:h-auto sm:w-[440px] sm:border-l sm:border-t-0 sm:rounded-none"
      >
        {/* Header */}
        <div className="shrink-0 flex items-start gap-2 p-3 border-b border-border">
          <KindIcon kind={resource.kind} />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-1.5 text-[11px] text-text-muted">
              <span>{resource.kind}</span>
              {"namespace" in resource && resource.namespace && (
                <>
                  <ChevronRight className="w-3 h-3" />
                  <span>{resource.namespace}</span>
                </>
              )}
            </div>
            <div className="text-[14px] font-medium text-text truncate">{resource.name}</div>
          </div>
          <div className="shrink-0 flex items-center gap-1">
            <ResourceHeaderActions
              resource={resource}
              mode={cluster?.mode}
              currentNamespace={cluster?.currentNamespace}
              helmCli={helmCli}
            />
            <button
              onClick={close}
              className="p-1 rounded text-text-muted hover:text-text hover:bg-bg-panel2 transition-colors"
              aria-label="Close detail"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Tab strip */}
        <div className="shrink-0 flex items-center gap-0.5 border-b border-border bg-bg-panel2/40 px-2 overflow-x-auto scrollbar-thin">
          {tabs.map((t) => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={`flex items-center gap-1.5 px-3 py-2 text-[12px] border-b-2 transition-colors whitespace-nowrap ${
                tab === t.id
                  ? "border-accent text-text"
                  : "border-transparent text-text-muted hover:text-text"
              }`}
            >
              {t.icon}
              {t.label}
            </button>
          ))}
        </div>

        {/* Body */}
        <div className="flex-1 min-h-0 overflow-y-auto scrollbar-thin">
          {tab === "overview" && <OverviewTab resource={resource} />}
          {tab === "yaml" && <YamlTab resource={resource} />}
          {tab === "instances" && resource.kind === "CustomResourceDefinition" && (
            <InstancesTab crd={resource} currentNamespace={cluster?.currentNamespace} />
          )}
          {tab === "history" && resource.kind === "HelmRelease" && (
            <HelmHistoryTab resource={resource} helmCli={helmCli} />
          )}
          {tab === "values" && resource.kind === "HelmRelease" && (
            <HelmValuesTab resource={resource} />
          )}
          {tab === "events" && <EventsTab resource={resource} />}
          {tab === "logs" && isPod && <LogsTab pod={resource as Pod} />}
          {tab === "shell" && isPod && <ShellTab pod={resource as Pod} />}
        </div>
      </aside>
    </>
  );
}

// ---------------------------------------------------------------------------
// Tabs
// ---------------------------------------------------------------------------

function OverviewTab({ resource }: { resource: AnyResource }) {
  const kv = useMemo(() => describeResource(resource), [resource]);
  const pinnedUids = usePinnedPodUids();
  const isPod = resource.kind === "Pod";
  const pinned = isPod && pinnedUids.includes(resource.uid);

  return (
    <div className="p-4 flex flex-col gap-4">
      {isPod && (
        <>
          <div className="bg-bg-panel2 border border-border rounded h-40 flex items-center justify-center">
            <CritterPlayer
              critter={resource.critter}
              status={resource.status}
              fps={7}
            />
          </div>
          <PinHotbarButton pod={resource} pinned={pinned} />
        </>
      )}

      <dl className="grid grid-cols-[110px_1fr] gap-x-3 gap-y-1.5 text-[12px]">
        {kv.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-text-muted uppercase tracking-wider text-[10px] pt-0.5">{k}</dt>
            <dd className="text-text break-words">{v}</dd>
          </div>
        ))}
      </dl>

      {"labels" in resource && resource.labels && Object.keys(resource.labels).length > 0 && (
        <div>
          <div className="text-text-muted uppercase tracking-wider text-[10px] mb-1.5">
            Labels
          </div>
          <div className="flex flex-wrap gap-1">
            {Object.entries(resource.labels).map(([k, v]) => (
              <span
                key={k}
                className="text-[10px] bg-bg-panel2 border border-border rounded px-1.5 py-0.5"
              >
                <span className="text-text-muted">{k}=</span>
                <span className="text-text">{v}</span>
              </span>
            ))}
          </div>
        </div>
      )}

      {isPod && (
        <PortForwardSection pod={resource as Pod} />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Port-forward section (Pod overview tab only)
// ---------------------------------------------------------------------------

function PortForwardSection({ pod }: { pod: Pod }) {
  const namespace = pod.namespace ?? "default";
  const [remotePort, setRemotePort] = useState<string>("");
  const [starting, setStarting] = useState(false);
  const [forwards, setForwards] = useState<PortForwardInfo[]>([]);
  const [listError, setListError] = useState<string | null>(null);

  const refreshList = useCallback(async () => {
    const res = await listPortForwards();
    if (res.ok) {
      // Show only forwards for this pod.
      setForwards(res.forwards.filter((f) => f.namespace === namespace && f.pod === pod.name));
      setListError(null);
    } else {
      setListError(res.error ?? "list failed");
    }
  }, [namespace, pod.name]);

  // Load list when the section mounts or the pod changes.
  useEffect(() => {
    void refreshList();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pod.uid]);

  const handleStart = async () => {
    const port = parseInt(remotePort, 10);
    if (!port || port < 1 || port > 65535) {
      workspaceActions.toast("enter a valid remote port (1–65535)", "error");
      return;
    }
    setStarting(true);
    const res = await startPortForward(namespace, pod.name, port);
    setStarting(false);
    if (res.ok && res.forward) {
      workspaceActions.toast(
        `forwarding ${pod.name}:${port} → localhost:${res.forward.localPort}`,
        "success",
      );
      setRemotePort("");
      void refreshList();
    } else {
      workspaceActions.toast(res.error ?? "port-forward failed", "error");
    }
  };

  const handleStop = async (id: string) => {
    const res = await stopPortForward(id);
    if (res.ok) {
      workspaceActions.toast("port-forward stopped", "success");
      void refreshList();
    } else {
      workspaceActions.toast(res.error ?? "stop failed", "error");
    }
  };

  return (
    <div className="border border-border rounded bg-bg-panel2/40">
      {/* Section header */}
      <div className="flex items-center gap-1.5 px-3 py-2 border-b border-border/60">
        <Network className="w-3.5 h-3.5 text-text-muted" />
        <span className="text-[11px] uppercase tracking-wider text-text-muted">Forward</span>
      </div>

      {/* Start row */}
      <div className="flex items-center gap-2 px-3 py-2">
        <input
          type="number"
          min={1}
          max={65535}
          placeholder="remote port"
          value={remotePort}
          onChange={(e) => setRemotePort(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") void handleStart(); }}
          className="flex-1 bg-bg-base border border-border rounded px-2 py-1 text-[11px] text-text placeholder-text-muted/60 focus:border-accent outline-none tabular-nums"
          aria-label="Remote port to forward"
        />
        <button
          type="button"
          onClick={() => void handleStart()}
          disabled={starting}
          className="flex items-center gap-1 px-2.5 py-1 rounded border border-accent/60 bg-accent/15 text-[11px] text-accent font-medium hover:bg-accent/25 transition-colors disabled:opacity-50 shrink-0"
        >
          {starting ? "starting…" : "Start"}
        </button>
      </div>

      {/* Active forwards for this pod */}
      {listError && (
        <div className="px-3 pb-2 text-[10px] text-status-crashloop">{listError}</div>
      )}
      {forwards.length > 0 && (
        <div className="divide-y divide-border/50">
          {forwards.map((f) => (
            <div key={f.id} className="flex items-center gap-2 px-3 py-2">
              <span className="text-[11px] text-text-muted tabular-nums shrink-0">
                :{f.remotePort}
              </span>
              <span className="text-[10px] text-text-muted">→</span>
              <a
                href={`http://localhost:${f.localPort}`}
                target="_blank"
                rel="noopener noreferrer"
                className="flex-1 text-[11px] text-accent hover:underline tabular-nums truncate"
              >
                http://localhost:{f.localPort}
              </a>
              <button
                type="button"
                onClick={() => void handleStop(f.id)}
                className="shrink-0 p-1 rounded text-text-muted hover:text-status-crashloop transition-colors"
                aria-label={`Stop forward :${f.remotePort}`}
              >
                <Square className="w-3 h-3" />
              </button>
            </div>
          ))}
        </div>
      )}

      {/* Muted note */}
      <div className="px-3 py-1.5 text-[10px] text-text-muted/60 border-t border-border/40">
        forwards bind on the machine running kubagachi
      </div>
    </div>
  );
}

function describeResource(r: AnyResource): Array<[string, React.ReactNode]> {
  const base: Array<[string, React.ReactNode]> = [
    ["uid", <code key="uid" className="text-[10px] break-all">{r.uid}</code>],
    ["kind", r.kind],
  ];
  if ("namespace" in r && r.namespace) base.push(["namespace", r.namespace]);
  base.push(["age", formatAge(r.ageSec)]);

  switch (r.kind) {
    case "Pod":
      base.push(
        ["status", <StatusPill key="s" status={r.status} />],
        ["phase", r.phase],
        ["node", r.node],
        ["pod IP", r.podIP ?? "—"],
        ["host IP", r.hostIP ?? "—"],
        ["containers", `${r.readyContainers}/${r.totalContainers}`],
        ["restarts", String(r.restartCount)],
        ["qos", r.qosClass ?? "—"],
        ["owner", r.ownerKind ? `${r.ownerKind}/${r.ownerName ?? ""}` : "—"],
        ["critter", r.critter],
      );
      break;
    case "Deployment":
      base.push(
        ["replicas", `${r.readyReplicas}/${r.replicas}`],
        ["updated", String(r.updatedReplicas)],
        ["available", String(r.availableReplicas)],
        ["strategy", r.strategy],
        ["status", <StatusPill key="s" status={r.status} />],
        ["image", r.image],
      );
      break;
    case "StatefulSet":
      base.push(
        ["replicas", `${r.readyReplicas}/${r.replicas}`],
        ["service", r.serviceName],
        ["status", <StatusPill key="s" status={r.status} />],
        ["image", r.image],
      );
      break;
    case "DaemonSet":
      base.push(
        ["desired", String(r.desiredNumberScheduled)],
        ["ready", String(r.numberReady)],
        ["available", String(r.numberAvailable)],
        ["status", <StatusPill key="s" status={r.status} />],
        ["image", r.image],
      );
      break;
    case "ReplicaSet":
      base.push(
        ["replicas", `${r.readyReplicas}/${r.replicas}`],
        ["owner", r.ownerKind ? `${r.ownerKind}/${r.ownerName ?? ""}` : "—"],
        ["image", r.image],
      );
      break;
    case "Job":
      base.push(
        ["completions", `${r.succeeded}/${r.completions}`],
        ["failed", String(r.failed)],
        ["active", String(r.active)],
        ["duration", r.durationSec != null ? formatAge(r.durationSec) : "—"],
        ["status", <StatusPill key="s" status={r.status} />],
        ["image", r.image],
      );
      break;
    case "CronJob":
      base.push(
        ["schedule", <code key="s">{r.schedule}</code>],
        ["suspend", r.suspend ? "true" : "false"],
        ["active jobs", String(r.activeJobs)],
        ["last schedule", r.lastScheduleAgeSec != null ? formatAge(r.lastScheduleAgeSec) : "—"],
        ["status", <StatusPill key="ss" status={r.status} />],
        ["image", r.image],
      );
      break;
    case "Service":
      base.push(
        ["type", r.type],
        ["cluster IP", <code key="ip">{r.clusterIP}</code>],
        ["external IP", r.externalIP ?? "—"],
        ["ports", r.ports.map((p) => `${p.port}/${p.protocol}`).join(", ")],
      );
      break;
    case "Ingress":
      base.push(
        ["class", r.className ?? "—"],
        ["hosts", r.hosts.join(", ")],
        ["tls", r.tls ? "yes" : "no"],
        ["address", r.address ?? "—"],
      );
      break;
    case "Endpoint":
      base.push(
        ["target service", r.targetService],
        ["addresses", String(r.subsets.flatMap((s) => s.addresses).length)],
      );
      break;
    case "NetworkPolicy":
      base.push(
        ["policy types", r.policyTypes.join(", ")],
        ["ingress rules", String(r.ingressRules)],
        ["egress rules", String(r.egressRules)],
      );
      break;
    case "ConfigMap":
      base.push(["keys", r.dataKeys.join(", ")]);
      break;
    case "Secret":
      base.push(
        ["type", r.type],
        ["keys", r.dataKeys.join(", ")],
      );
      break;
    case "ResourceQuota":
      base.push(
        ["hard", Object.entries(r.hard).map(([k, v]) => `${k}=${v}`).join(", ")],
        ["used", Object.entries(r.used).map(([k, v]) => `${k}=${v}`).join(", ")],
      );
      break;
    case "LimitRange":
      base.push(["items", String(r.limits.length)]);
      break;
    case "HorizontalPodAutoscaler":
      base.push(
        ["target", `${r.targetKind}/${r.targetName}`],
        ["replicas", `${r.currentReplicas} (min ${r.minReplicas} / max ${r.maxReplicas})`],
        ["cpu", `${r.currentCPUPercent ?? "—"}% / ${r.targetCPUPercent ?? "—"}%`],
      );
      break;
    case "PodDisruptionBudget":
      base.push(
        ["min available", r.minAvailable ?? "—"],
        ["max unavailable", r.maxUnavailable ?? "—"],
        ["current healthy", `${r.currentHealthy}/${r.expectedPods}`],
      );
      break;
    case "PersistentVolume":
      base.push(
        ["capacity", r.capacity],
        ["access modes", r.accessModes.join(", ")],
        ["reclaim", r.reclaimPolicy],
        ["phase", <StatusPill key="p" status={r.phase} />],
        ["storage class", r.storageClassName],
        ["claim", r.claimRef ? `${r.claimRef.namespace}/${r.claimRef.name}` : "—"],
      );
      break;
    case "PersistentVolumeClaim":
      base.push(
        ["capacity", r.capacity],
        ["access modes", r.accessModes.join(", ")],
        ["storage class", r.storageClassName],
        ["phase", <StatusPill key="p" status={r.phase} />],
        ["volume", r.volumeName ?? "—"],
      );
      break;
    case "StorageClass":
      base.push(
        ["provisioner", r.provisioner],
        ["reclaim", r.reclaimPolicy],
        ["binding mode", r.volumeBindingMode],
        ["default", r.isDefault ? "yes" : "no"],
      );
      break;
    case "ServiceAccount":
      base.push(
        ["secrets", String(r.secrets.length)],
        ["image pull secrets", String(r.imagePullSecrets.length)],
        ["automount token", r.automountToken ? "yes" : "no"],
      );
      break;
    case "Role":
    case "ClusterRole":
      base.push(["rules", String(r.rules.length)]);
      break;
    case "RoleBinding":
    case "ClusterRoleBinding":
      base.push(
        ["role ref", `${r.roleRef.kind}/${r.roleRef.name}`],
        ["subjects", String(r.subjects.length)],
      );
      break;
    case "Node":
      base.push(
        ["status", <StatusPill key="s" status={r.status} />],
        ["roles", r.roles.join(", ")],
        ["version", r.kubeletVersion],
        ["arch", r.arch],
        ["os", r.os],
        ["cpu", r.cpuCapacity],
        ["memory", r.memCapacity],
        ["pods", `${r.podCount}/${r.podCapacity}`],
        ["runtime", r.containerRuntime],
      );
      break;
    case "Namespace":
      base.push(["phase", <StatusPill key="p" status={r.phase} />]);
      break;
    case "Event":
      base.push(
        ["type", <StatusPill key="t" status={r.type} compact />],
        ["reason", r.reason],
        ["message", r.message],
        ["object", `${r.involvedObject.kind}/${r.involvedObject.name}`],
        ["source", r.source],
        ["count", String(r.count)],
        ["first seen", formatAge(r.firstSeenSec)],
        ["last seen", formatAge(r.lastSeenSec)],
      );
      break;
    case "CustomResourceDefinition":
      base.push(
        ["group", r.group],
        ["scope", r.scope],
        ["versions", r.versions.join(", ")],
        ["plural", r.pluralName],
        ["singular", r.singularName],
      );
      break;
    case "HelmRelease":
      base.push(
        ["chart", `${r.chart}@${r.chartVersion}`],
        ["app version", r.appVersion],
        ["revision", String(r.revision)],
        ["status", <StatusPill key="s" status={r.status} />],
        ["updated", formatAge(r.updatedAgeSec)],
      );
      break;
  }
  return base;
}

function YamlTab({ resource }: { resource: AnyResource }) {
  const cluster = useCluster();
  const [yaml, setYaml] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Edit mode state
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<string>("");
  const [applying, setApplying] = useState(false);
  const [applyError, setApplyError] = useState<string | null>(null);

  // Secret decode panel state
  const [secretData, setSecretData] = useState<Record<string, SecretEntry> | null>(null);
  const [secretError, setSecretError] = useState<string | null>(null);
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});
  const [revealAll, setRevealAll] = useState(false);

  const isSecret = resource.kind === "Secret";
  const isMock = isMockMode(cluster?.mode);
  const namespace = "namespace" in resource ? resource.namespace : undefined;

  const loadYaml = useCallback((cancelled: { v: boolean }) => {
    setYaml("");
    setLoading(true);
    setError(null);
    void fetchObjectYaml(resource.kind, resource.name, namespace).then((res) => {
      if (cancelled.v) return;
      if (res.ok) {
        setYaml(res.yaml);
      } else {
        setError(res.yaml);
      }
      setLoading(false);
    });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resource.uid, resource.kind, resource.name, namespace]);

  useEffect(() => {
    const cancelled = { v: false };

    // Reset all state on resource change
    setEditing(false);
    setDraft("");
    setApplyError(null);
    setSecretData(null);
    setSecretError(null);
    setRevealed({});
    setRevealAll(false);

    loadYaml(cancelled);

    // Fetch secret data for Secret resources
    if (isSecret && namespace) {
      void fetchSecretData(namespace, resource.name).then((res) => {
        if (cancelled.v) return;
        if (res.ok) {
          setSecretData(res.data);
        } else {
          setSecretError(res.error ?? "failed to fetch secret data");
        }
      });
    }

    return () => {
      cancelled.v = true;
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resource.uid, resource.kind, resource.name, namespace, isSecret]);

  const toggleRevealAll = () => {
    const next = !revealAll;
    setRevealAll(next);
    if (next && secretData) {
      const all: Record<string, boolean> = {};
      for (const k of Object.keys(secretData)) {
        all[k] = true;
      }
      setRevealed(all);
    } else {
      setRevealed({});
    }
  };

  const toggleKey = (key: string) => {
    setRevealed((prev) => ({ ...prev, [key]: !prev[key] }));
  };

  const startEdit = () => {
    setDraft(yaml);
    setApplyError(null);
    setEditing(true);
  };

  const cancelEdit = () => {
    setEditing(false);
    setDraft("");
    setApplyError(null);
  };

  const applyEdit = async () => {
    if (!window.confirm("Apply changes to the cluster? This will mutate the live resource.")) {
      return;
    }
    setApplying(true);
    setApplyError(null);
    const res = await applyObjectYaml(draft);
    setApplying(false);
    if (res.ok) {
      workspaceActions.toast(`applied ${resource.kind}/${resource.name}`, "success");
      setEditing(false);
      setDraft("");
      // Re-fetch to show the server-canonical YAML.
      const cancelled = { v: false };
      loadYaml(cancelled);
    } else {
      setApplyError(res.error ?? "apply failed");
    }
  };

  return (
    <div className="flex flex-col">
      {/* Secret decode panel — shown only for Secret resources */}
      {isSecret && (
        <div className="p-3 border-b border-border bg-bg-panel2/40">
          <div className="flex items-center justify-between mb-2">
            <span className="text-[10px] uppercase tracking-wider text-text-muted">Secret Data</span>
            <button
              type="button"
              onClick={toggleRevealAll}
              className="flex items-center gap-1 text-[10px] text-text-muted hover:text-text transition-colors"
            >
              {revealAll ? <EyeOff className="w-3 h-3" /> : <Eye className="w-3 h-3" />}
              {revealAll ? "hide all" : "reveal all"}
            </button>
          </div>
          {secretError && (
            <div className="text-[11px] text-status-crashloop">{secretError}</div>
          )}
          {!secretData && !secretError && (
            <div className="text-[11px] text-text-muted">loading…</div>
          )}
          {secretData && Object.entries(secretData).map(([k, entry]) => (
            <div
              key={k}
              className="flex items-center gap-2 py-1.5 border-b border-border/50 last:border-0"
            >
              <span className="text-[11px] text-text-muted font-mono w-[120px] shrink-0 truncate">
                {k}
              </span>
              <span className="flex-1 text-[11px] font-mono text-text truncate">
                {revealed[k] ? entry.decoded : "••••••"}
              </span>
              <button
                type="button"
                onClick={() => toggleKey(k)}
                className="shrink-0 p-1 rounded text-text-muted hover:text-text transition-colors"
                aria-label={revealed[k] ? "Hide value" : "Reveal value"}
              >
                {revealed[k] ? <EyeOff className="w-3 h-3" /> : <Eye className="w-3 h-3" />}
              </button>
            </div>
          ))}
        </div>
      )}

      {/* YAML toolbar — edit toggle (hidden while loading or in error) */}
      {!loading && !error && !isMock && (
        <div className="shrink-0 flex items-center justify-end gap-1.5 px-3 py-1.5 border-b border-border bg-bg-panel2/40">
          {editing ? (
            <>
              {applyError && (
                <span className="flex-1 text-[10px] text-status-crashloop truncate mr-1">
                  {applyError}
                </span>
              )}
              <button
                type="button"
                onClick={cancelEdit}
                disabled={applying}
                className="flex items-center gap-1 px-2 py-1 rounded border border-border text-[10px] text-text-muted hover:text-text transition-colors disabled:opacity-50"
              >
                <X className="w-3 h-3" />
                Cancel
              </button>
              <button
                type="button"
                onClick={() => void applyEdit()}
                disabled={applying}
                className="flex items-center gap-1 px-2 py-1 rounded border border-accent/60 bg-accent/15 text-[10px] text-accent font-medium hover:bg-accent/25 transition-colors disabled:opacity-50"
              >
                <Check className="w-3 h-3" />
                {applying ? "applying…" : "Apply"}
              </button>
            </>
          ) : (
            <button
              type="button"
              onClick={startEdit}
              className="flex items-center gap-1 px-2 py-1 rounded border border-border text-[10px] text-text-muted hover:text-text hover:border-accent-soft transition-colors"
            >
              <Edit2 className="w-3 h-3" />
              Edit
            </button>
          )}
        </div>
      )}

      {/* YAML body */}
      {loading && (
        <div className="p-4 text-[11px] text-text-muted">loading yaml…</div>
      )}
      {!loading && error && (
        <div className="p-4 text-[11px] text-status-crashloop">error: {error}</div>
      )}
      {!loading && !error && !editing && (
        <pre className="p-4 text-[11px] leading-relaxed text-text-muted font-mono whitespace-pre-wrap break-words">
          {yaml || "(empty)"}
        </pre>
      )}
      {!loading && !error && editing && (
        <textarea
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          spellCheck={false}
          className="flex-1 min-h-[300px] p-4 text-[11px] leading-relaxed font-mono text-text bg-bg-panel2 border-0 outline-none resize-none whitespace-pre"
          aria-label="Edit YAML"
        />
      )}
    </div>
  );
}

function EventsTab({ resource }: { resource: AnyResource }) {
  const cluster = useCluster();
  const events = useMemo<Event[]>(() => {
    if (!cluster) return [];
    return [...cluster.events]
      .filter((e) => {
        if (e.involvedObject.name !== resource.name) return false;
        if ("namespace" in resource && e.namespace !== resource.namespace) return false;
        return true;
      })
      .sort((a, b) => a.lastSeenSec - b.lastSeenSec);
  }, [cluster, resource]);

  if (events.length === 0) {
    return (
      <div className="p-6 text-text-muted text-[12px]">No events for this resource.</div>
    );
  }

  return (
    <div className="divide-y divide-border/70">
      {events.map((e) => (
        <div key={e.uid} className="p-3 flex gap-2 items-start">
          <StatusPill status={e.type} compact />
          <div className="flex-1 min-w-0">
            <div className="flex items-baseline gap-2">
              <span className="text-[12px] font-medium text-text">{e.reason}</span>
              <span className="ml-auto text-[10px] text-text-muted tabular-nums">
                {formatAge(e.lastSeenSec)}
              </span>
            </div>
            <div className="text-[11px] text-text-muted">{e.message}</div>
            <div className="text-[10px] text-text-muted/80 mt-0.5">{e.source} · {e.count}x</div>
          </div>
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Helm release tabs
// ---------------------------------------------------------------------------

function HelmHistoryTab({ resource, helmCli }: { resource: HelmRelease; helmCli: boolean }) {
  const [revisions, setRevisions] = useState<HelmRevisionInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadHistory = useCallback(() => {
    const cancelled = { v: false };
    setRevisions([]);
    setLoading(true);
    setError(null);

    void fetchHelmHistory(resource.namespace ?? "", resource.name).then((res) => {
      if (cancelled.v) return;
      if (res.ok) {
        setRevisions(res.revisions);
      } else {
        setError(res.error ?? "failed to fetch history");
      }
      setLoading(false);
    });

    return () => { cancelled.v = true; };
  }, [resource.namespace, resource.name]);

  useEffect(() => {
    return loadHistory();
  }, [loadHistory, resource.uid]);

  // The highest revision number is the currently deployed one.
  const currentRevision = revisions.length > 0 ? revisions[0].revision : resource.revision;

  const rollbackDisabled = !helmCli;
  const rollbackDisabledReason = "helm CLI not found on the kubagachi host";

  return (
    <div className="flex flex-col">
      {loading && (
        <div className="p-4 text-[11px] text-text-muted">loading history…</div>
      )}
      {!loading && error && (
        <div className="p-4 text-[11px] text-status-crashloop">error: {error}</div>
      )}
      {!loading && !error && revisions.length === 0 && (
        <div className="p-6 text-text-muted text-[12px]">No history found.</div>
      )}
      {!loading && !error && revisions.length > 0 && (
        <div className="divide-y divide-border/70">
          {revisions.map((rev) => {
            const isCurrent = rev.revision === currentRevision;
            return (
              <div key={rev.revision} className="p-3 flex items-center gap-3">
                <span className="shrink-0 w-8 text-[12px] font-medium tabular-nums text-text text-right">
                  v{rev.revision}
                </span>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <StatusPill status={rev.status} compact />
                    <span className="text-[11px] text-text-muted tabular-nums">{rev.chartVersion}</span>
                    {rev.appVersion && rev.appVersion !== "—" && (
                      <span className="text-[10px] text-text-muted/70">app {rev.appVersion}</span>
                    )}
                  </div>
                  {rev.description && (
                    <div className="text-[10px] text-text-muted/80 mt-0.5 truncate">{rev.description}</div>
                  )}
                </div>
                <span className="shrink-0 text-[10px] tabular-nums text-text-muted">
                  {formatAge(rev.updatedAgeSec)}
                </span>
                {!isCurrent && (
                  rollbackDisabled ? (
                    <button
                      type="button"
                      disabled
                      title={rollbackDisabledReason}
                      className="shrink-0 text-[10px] px-1.5 py-0.5 rounded border border-border/40 text-text-muted/30 cursor-not-allowed"
                    >
                      Rollback
                    </button>
                  ) : (
                    <HelmRollbackButton
                      resource={resource}
                      revision={rev.revision}
                      onDone={loadHistory}
                    />
                  )
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function HelmRollbackButton({
  resource,
  revision,
  onDone,
}: {
  resource: HelmRelease;
  revision: number;
  onDone: () => void;
}) {
  const onConfirm = async () => {
    const res = await helmRollback(resource.namespace ?? "", resource.name, revision);
    if (res.ok) {
      workspaceActions.toast(`rolled back ${resource.name} to v${revision}`, "success");
      onDone();
    } else {
      workspaceActions.toast(res.error ?? `failed to rollback ${resource.name} to v${revision}`, "error");
    }
  };

  return (
    <ConfirmButton
      onConfirm={onConfirm}
      label={<span className="text-[10px]">Rollback</span>}
      confirmLabel={<span className="text-[10px] font-medium">rollback?</span>}
      title={`Rollback ${resource.name} to v${revision}`}
      aria-label={`Rollback ${resource.name} to revision ${revision}`}
      className="shrink-0 text-[10px] px-1.5 py-0.5 rounded border border-border text-text-muted hover:text-text hover:border-accent hover:bg-accent/10 transition-colors"
      armedClassName="border-accent bg-accent/15 text-accent"
    />
  );
}

type ValuesSection = "values" | "manifest" | "notes";

function HelmValuesTab({ resource }: { resource: HelmRelease }) {
  const [detail, setDetail] = useState<HelmDetailResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [section, setSection] = useState<ValuesSection>("values");

  useEffect(() => {
    const cancelled = { v: false };
    setDetail(null);
    setLoading(true);
    setError(null);

    void fetchHelmRelease(resource.namespace ?? "", resource.name, resource.revision).then((res) => {
      if (cancelled.v) return;
      if (res.ok && res.detail) {
        setDetail(res.detail);
      } else {
        setError(res.error ?? "failed to fetch release detail");
      }
      setLoading(false);
    });

    return () => { cancelled.v = true; };
  }, [resource.uid, resource.namespace, resource.name, resource.revision]);

  const sections: { id: ValuesSection; label: string }[] = [
    { id: "values", label: "Values" },
    { id: "manifest", label: "Manifest" },
    { id: "notes", label: "Notes" },
  ];

  const content =
    detail == null
      ? ""
      : section === "values"
        ? detail.values || "(no user-supplied values)"
        : section === "manifest"
          ? detail.manifest || "(empty manifest)"
          : detail.notes || "(no notes)";

  return (
    <div className="flex flex-col">
      {/* Sub-section selector */}
      <div className="shrink-0 flex gap-0.5 px-2 py-1.5 border-b border-border bg-bg-panel2/40">
        {sections.map((s) => (
          <button
            key={s.id}
            type="button"
            onClick={() => setSection(s.id)}
            className={`px-3 py-1 rounded text-[11px] transition-colors ${
              section === s.id
                ? "bg-accent/20 text-accent font-medium"
                : "text-text-muted hover:text-text"
            }`}
          >
            {s.label}
          </button>
        ))}
      </div>

      {loading && (
        <div className="p-4 text-[11px] text-text-muted">loading…</div>
      )}
      {!loading && error && (
        <div className="p-4 text-[11px] text-status-crashloop">error: {error}</div>
      )}
      {!loading && !error && (
        <pre className="p-4 text-[11px] leading-relaxed text-text-muted font-mono whitespace-pre-wrap break-words">
          {content}
        </pre>
      )}
    </div>
  );
}

function crdGroupParam(crd: CustomResourceDefinition): string {
  return crd.group === "—" ? "" : crd.group;
}

function crdApiVersion(crd: CustomResourceDefinition): string {
  const version = crd.versions[0] ?? "";
  const group = crdGroupParam(crd);
  return group ? `${group}/${version}` : version;
}

function crdInstanceKind(crd: CustomResourceDefinition): string {
  return crd.listKind.replace(/List$/, "") || crd.singularName || crd.pluralName;
}

function InstancesTab({
  crd,
  currentNamespace,
}: {
  crd: CustomResourceDefinition;
  currentNamespace?: string;
}) {
  const version = crd.versions[0] ?? "";
  const group = crdGroupParam(crd);
  const namespace = crd.scope === "Namespaced" ? currentNamespace || undefined : undefined;
  const apiVersion = crdApiVersion(crd);
  const instanceKind = crdInstanceKind(crd);

  const [items, setItems] = useState<CustomResourceRef[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<CustomResourceRef | null>(null);
  const [yaml, setYaml] = useState("");
  const [yamlLoading, setYamlLoading] = useState(false);
  const [yamlError, setYamlError] = useState<string | null>(null);

  useEffect(() => {
    const cancelled = { v: false };
    setItems([]);
    setSelected(null);
    setYaml("");
    setYamlError(null);
    setLoading(true);
    setError(null);

    if (!version || !crd.pluralName) {
      setError("CRD version or plural resource is missing.");
      setLoading(false);
      return () => {
        cancelled.v = true;
      };
    }

    void fetchCustomResources(group, version, crd.pluralName, namespace).then((res) => {
      if (cancelled.v) return;
      if (res.ok) {
        setItems(res.items);
      } else {
        setError(res.error ?? "failed to fetch custom resources");
      }
      setLoading(false);
    });

    return () => {
      cancelled.v = true;
    };
  }, [crd.uid, crd.pluralName, group, namespace, version]);

  const loadYaml = useCallback(async (item: CustomResourceRef) => {
    setSelected(item);
    setYaml("");
    setYamlError(null);
    setYamlLoading(true);
    const res = await fetchObjectYaml(instanceKind, item.name, item.namespace || undefined, apiVersion);
    if (res.ok) {
      setYaml(res.yaml);
    } else {
      setYamlError(res.yaml || "failed to fetch yaml");
    }
    setYamlLoading(false);
  }, [apiVersion, instanceKind]);

  return (
    <div className="flex flex-col">
      <div className="divide-y divide-border/70">
        {loading && (
          <div className="p-4 text-[11px] text-text-muted">loading instances...</div>
        )}
        {!loading && error && (
          <div className="p-4 text-[11px] text-status-crashloop">error: {error}</div>
        )}
        {!loading && !error && items.length === 0 && (
          <div className="p-6 text-text-muted text-[12px]">No instances found.</div>
        )}
        {!loading && !error && items.map((item) => {
          const active =
            selected?.name === item.name && (selected.namespace ?? "") === (item.namespace ?? "");
          return (
            <button
              key={`${item.namespace ?? ""}/${item.name}`}
              type="button"
              onClick={() => void loadYaml(item)}
              className={`w-full text-left p-3 flex items-center gap-3 transition-colors ${
                active ? "bg-accent/10" : "hover:bg-bg-panel2/70"
              }`}
            >
              <div className="flex-1 min-w-0">
                <div className="text-[12px] font-medium text-text truncate">{item.name}</div>
                <div className="text-[10px] text-text-muted truncate">
                  {item.namespace || "cluster-scoped"}
                </div>
              </div>
              <span className="shrink-0 text-[10px] tabular-nums text-text-muted">
                {formatAge(item.ageSec)}
              </span>
            </button>
          );
        })}
      </div>

      {selected && (
        <div className="border-t border-border bg-bg-base">
          <div className="px-3 py-2 border-b border-border bg-bg-panel2/40 flex items-center gap-2">
            <FileText className="w-3.5 h-3.5 text-text-muted" />
            <span className="text-[11px] text-text font-medium truncate">
              {selected.namespace ? `${selected.namespace}/${selected.name}` : selected.name}
            </span>
          </div>
          {yamlLoading && (
            <div className="p-4 text-[11px] text-text-muted">loading yaml...</div>
          )}
          {!yamlLoading && yamlError && (
            <div className="p-4 text-[11px] text-status-crashloop">error: {yamlError}</div>
          )}
          {!yamlLoading && !yamlError && (
            <pre className="p-4 text-[11px] leading-relaxed text-text-muted font-mono whitespace-pre-wrap break-words">
              {yaml || "(empty)"}
            </pre>
          )}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pod-only tabs
// ---------------------------------------------------------------------------

const LOG_TEMPLATES: Record<PodStatus, () => string> = {
  running: () => {
    const path = pick(["/healthz", "/api/v1/users", "/api/v1/orders", "/metrics", "/login"]);
    const code = pick(["200", "200", "200", "204", "304", "200"]);
    return `${nowTs()} INFO  http: GET ${path} ${code}`;
  },
  pending: () => `${nowTs()} INFO  scheduler: waiting for image pull (1/2)`,
  completed: () => `${nowTs()} INFO  app: job finished, exiting`,
  terminating: () => `${nowTs()} INFO  app: SIGTERM received, draining…`,
  crashloop: () =>
    `${nowTs()} ERROR app: panic: runtime error: ${pick([
      "invalid memory address",
      "index out of range [5] with length 0",
      "nil pointer dereference",
    ])}`,
  backoff: () =>
    `${nowTs()} WARN  kubelet: Failed to pull image: ${pick([
      "manifest unknown",
      "401 Unauthorized",
      "no such host",
    ])}`,
  error: () => `${nowTs()} ERROR app: exited with status 1`,
  unknown: () => `${nowTs()} WARN  kubelet: lost contact with container runtime`,
};

function pick<T>(arr: readonly T[]): T {
  return arr[Math.floor(Math.random() * arr.length)];
}

function nowTs(): string {
  const d = new Date();
  return `[${d.toISOString().slice(11, 19)}]`;
}

const TAIL_OPTIONS = [100, 500, 1000, 5000] as const;

function lineClass(l: string): string {
  if (/\b(ERROR|FATAL|panic)\b/i.test(l)) return "text-status-crashloop";
  if (/\bWARN(ING)?\b/i.test(l)) return "text-status-backoff";
  return "text-text";
}

function LogsTab({ pod }: { pod: Pod }) {
  const cluster = useCluster();
  const mock = isMockMode(cluster?.mode);
  const namespace = pod.namespace ?? "default";

  const containers = pod.containers.length
    ? pod.containers.map((c) => c.name)
    : ["app"];
  const [container, setContainer] = useState<string>(containers[0]);
  const [tail, setTail] = useState<number>(500);

  const [lines, setLines] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const ref = useRef<HTMLDivElement>(null);
  const stickyRef = useRef(true);

  // Reset the chosen container when the pod changes (and it no longer exists).
  useEffect(() => {
    if (!containers.includes(container)) setContainer(containers[0]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pod.uid]);

  // ---- Real log fetch (live/cluster mode) ----
  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    const res = await fetchLogs(namespace, pod.name, container, tail);
    if (res.ok) {
      const arr = res.text.split("\n");
      // Trim a single trailing empty line from the text payload.
      if (arr.length && arr[arr.length - 1] === "") arr.pop();
      setLines(arr.length ? arr : ["(no log output)"]);
    } else {
      setLines([]);
      setError(res.text || "failed to fetch logs");
    }
    setLoading(false);
    stickyRef.current = true;
  }, [namespace, pod.name, container, tail]);

  // In real mode, (re)fetch whenever pod/container/tail changes.
  useEffect(() => {
    if (mock) return;
    void load();
  }, [mock, load]);

  // ---- Synthetic generator (demo/mock mode only) ----
  useEffect(() => {
    if (!mock) return;
    setError(null);
    setLoading(false);
    setLines([
      `${nowTs()} INFO  app: process started pid=${1000 + Math.floor(Math.random() * 9000)}`,
      `${nowTs()} INFO  app: loading config from /etc/${pod.critter}/config.yaml`,
      `${nowTs()} INFO  app: connected to upstream`,
    ]);
  }, [mock, pod.uid, pod.critter, container]);

  useEffect(() => {
    if (!mock) return;
    const generator = LOG_TEMPLATES[pod.status] ?? LOG_TEMPLATES.running;
    const id = window.setInterval(() => {
      setLines((prev) => {
        const next = [...prev, generator()];
        return next.length > tail ? next.slice(next.length - tail) : next;
      });
    }, 1200);
    return () => window.clearInterval(id);
  }, [mock, pod.status, tail]);

  // Auto-scroll to bottom unless user has scrolled away ("follow").
  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    if (stickyRef.current) {
      node.scrollTop = node.scrollHeight;
    }
  }, [lines]);

  const onScroll = () => {
    const node = ref.current;
    if (!node) return;
    const atBottom = node.scrollHeight - node.scrollTop - node.clientHeight < 20;
    stickyRef.current = atBottom;
  };

  return (
    <div className="h-full flex flex-col">
      {/* Controls */}
      <div className="shrink-0 flex items-center gap-2 px-3 py-2 border-b border-border bg-bg-panel2/40 text-[11px]">
        <label className="flex items-center gap-1.5 text-text-muted">
          <span className="uppercase tracking-wider text-[10px]">ctr</span>
          <select
            value={container}
            onChange={(e) => setContainer(e.target.value)}
            className="bg-bg-base border border-border rounded px-1.5 py-1 text-text focus:border-accent outline-none"
          >
            {containers.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-1.5 text-text-muted">
          <span className="uppercase tracking-wider text-[10px]">tail</span>
          <select
            value={tail}
            onChange={(e) => setTail(Number(e.target.value))}
            className="bg-bg-base border border-border rounded px-1.5 py-1 text-text tabular-nums focus:border-accent outline-none"
          >
            {TAIL_OPTIONS.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
        </label>
        {!mock && (
          <button
            type="button"
            onClick={() => void load()}
            disabled={loading}
            title="Reload logs"
            aria-label="Reload logs"
            className="ml-auto flex items-center gap-1.5 px-2 py-1 rounded border border-border text-text-muted hover:text-text hover:border-accent-soft transition-colors disabled:opacity-50"
          >
            <RefreshCw className={`w-3 h-3 ${loading ? "animate-spin" : ""}`} />
            {loading ? "loading…" : "reload"}
          </button>
        )}
        {mock && <span className="ml-auto text-[10px] text-text-muted">demo · synthetic</span>}
      </div>

      {/* Body */}
      <div
        ref={ref}
        onScroll={onScroll}
        className="flex-1 min-h-0 overflow-y-auto bg-bg-base font-mono text-[11px] leading-relaxed p-3 scrollbar-thin"
      >
        {error && (
          <div className="text-status-crashloop whitespace-pre-wrap">log error: {error}</div>
        )}
        {loading && lines.length === 0 && !error && (
          <div className="text-text-muted">loading logs…</div>
        )}
        {!loading && lines.length === 0 && !error && (
          <div className="text-text-muted">(no logs)</div>
        )}
        {lines.map((l, i) => (
          <div key={i} className={lineClass(l)}>
            {l || " "}
          </div>
        ))}
      </div>
    </div>
  );
}

function ShellTab({ pod }: { pod: Pod }) {
  const namespace = pod.namespace ?? "default";
  const containers = pod.containers.length
    ? pod.containers.map((c) => c.name)
    : ["app"];
  const [container, setContainer] = useState<string>(containers[0]);

  useEffect(() => {
    if (!containers.includes(container)) setContainer(containers[0]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pod.uid]);

  // Exec only works on a running pod; a completed/pending/failed pod has no live
  // container and `kubectl exec` fails with a raw error. Guard it and explain why.
  const canExec = pod.status === "running";

  const openShell = () => {
    if (!canExec) return;
    workspaceActions.openTerminal({ namespace, pod: pod.name, container });
  };

  return (
    <div className="p-4 flex flex-col gap-4 text-[12px]">
      <p className="text-text-muted leading-relaxed">
        Open an interactive shell into a container of{" "}
        <span className="text-text font-medium">{pod.name}</span>. This mounts the
        real terminal session in the dock at the bottom of the screen.
      </p>

      <label className="flex items-center gap-2">
        <span className="text-text-muted uppercase tracking-wider text-[10px] w-[72px]">
          Container
        </span>
        <select
          value={container}
          onChange={(e) => setContainer(e.target.value)}
          className="flex-1 bg-bg-base border border-border rounded px-2 py-1.5 text-text focus:border-accent outline-none"
        >
          {containers.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>
      </label>

      {!canExec && (
        <div className="text-[11px] text-status-backoff bg-status-backoff/10 border border-status-backoff/30 rounded px-2.5 py-2">
          This pod is <span className="font-medium">{pod.status}</span> (phase{" "}
          {pod.phase}). A shell needs a running container — exec is unavailable.
        </div>
      )}

      <button
        type="button"
        onClick={openShell}
        disabled={!canExec}
        className={
          "w-full flex items-center justify-center gap-2 px-3 py-2 rounded border font-medium transition-colors " +
          (canExec
            ? "border-accent/60 bg-accent/15 text-accent hover:bg-accent/25"
            : "border-border bg-bg-panel2 text-text-muted/50 cursor-not-allowed")
        }
      >
        <Terminal className="w-3.5 h-3.5" />
        {canExec ? "Open shell" : "Shell unavailable"}
      </button>

      <div className="text-[10px] text-text-muted/80 font-mono">
        kubectl exec -it -n {namespace} {pod.name} -c {container} -- sh
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Resource actions (destructive actions use two-step confirm)
// ---------------------------------------------------------------------------

type ScalableResource = Extract<
  AnyResource,
  { kind: "Deployment" | "StatefulSet" | "ReplicaSet" }
>;

type RestartableResource = Extract<
  AnyResource,
  { kind: "Deployment" | "StatefulSet" | "DaemonSet" }
>;

const SCALABLE_KINDS = new Set<AnyResource["kind"]>([
  "Deployment",
  "StatefulSet",
  "ReplicaSet",
]);

const RESTARTABLE_KINDS = new Set<AnyResource["kind"]>([
  "Deployment",
  "StatefulSet",
  "DaemonSet",
]);

function isScalableResource(resource: AnyResource): resource is ScalableResource {
  return SCALABLE_KINDS.has(resource.kind);
}

function isRestartableResource(resource: AnyResource): resource is RestartableResource {
  return RESTARTABLE_KINDS.has(resource.kind);
}

function resourceNamespace(resource: AnyResource, currentNamespace?: string): string | undefined {
  if ("namespace" in resource) {
    return resource.namespace || currentNamespace || "default";
  }
  return undefined;
}

function resourceLabel(resource: AnyResource): string {
  return `${resource.kind}/${resource.name}`;
}

function currentReplicas(resource: ScalableResource): number {
  switch (resource.kind) {
    case "Deployment":
    case "StatefulSet":
    case "ReplicaSet":
      return resource.replicas;
  }
}

function ResourceHeaderActions({
  resource,
  mode,
  currentNamespace,
  helmCli,
}: {
  resource: AnyResource;
  mode: string | undefined;
  currentNamespace?: string;
  helmCli?: boolean;
}) {
  return (
    <>
      {isScalableResource(resource) && (
        <ScaleResourceControl
          resource={resource}
          mode={mode}
          currentNamespace={currentNamespace}
        />
      )}
      {isRestartableResource(resource) && (
        <RestartResourceButton
          resource={resource}
          mode={mode}
          currentNamespace={currentNamespace}
        />
      )}
      {resource.kind === "Node" && (
        <CordonNodeButton node={resource as Node} mode={mode} />
      )}
      {resource.kind === "HelmRelease" && (
        <HelmUninstallButton
          resource={resource as HelmRelease}
          mode={mode}
          helmCli={helmCli ?? false}
        />
      )}
      <DeleteResourceButton
        resource={resource}
        mode={mode}
        currentNamespace={currentNamespace}
      />
    </>
  );
}

function ScaleResourceControl({
  resource,
  mode,
  currentNamespace,
}: {
  resource: ScalableResource;
  mode: string | undefined;
  currentNamespace?: string;
}) {
  const replicas = currentReplicas(resource);
  const [draft, setDraft] = useState(String(replicas));
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setDraft(String(replicas));
    setBusy(false);
  }, [resource.uid, replicas]);

  const apply = async () => {
    const next = Number(draft);
    if (!Number.isInteger(next) || next < 0) {
      workspaceActions.toast("replicas must be a non-negative integer", "error");
      return;
    }

    setBusy(true);
    let res: ActionResult = { ok: true };
    if (mode !== "mock") {
      res = await scaleResource(
        resource.kind,
        resource.name,
        resourceNamespace(resource, currentNamespace),
        next,
      );
    }
    setBusy(false);

    if (res.ok) {
      workspaceActions.toast(`scaled ${resourceLabel(resource)} to ${next}`, "success");
    } else {
      workspaceActions.toast(res.error ?? `failed to scale ${resourceLabel(resource)}`, "error");
    }
  };

  return (
    <div className="shrink-0 flex items-center gap-1 rounded border border-border bg-bg-base/70 px-1 py-0.5">
      <input
        type="number"
        min={0}
        step={1}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            void apply();
          }
        }}
        className="w-10 bg-transparent text-[11px] text-text tabular-nums outline-none"
        aria-label={`${resource.kind} replicas`}
      />
      <button
        type="button"
        onClick={() => void apply()}
        disabled={busy}
        className="flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] text-text-muted hover:text-text hover:bg-bg-panel2 transition-colors disabled:opacity-50"
      >
        <Check className="w-3 h-3" />
        {busy ? "..." : "Apply"}
      </button>
    </div>
  );
}

function RestartResourceButton({
  resource,
  mode,
  currentNamespace,
}: {
  resource: RestartableResource;
  mode: string | undefined;
  currentNamespace?: string;
}) {
  const onConfirm = async () => {
    let res: ActionResult = { ok: true };
    if (mode !== "mock") {
      res = await restartResource(
        resource.kind,
        resource.name,
        resourceNamespace(resource, currentNamespace),
      );
    }
    if (res.ok) {
      workspaceActions.toast(`restarted ${resourceLabel(resource)}`, "success");
    } else {
      workspaceActions.toast(res.error ?? `failed to restart ${resourceLabel(resource)}`, "error");
    }
  };

  return (
    <ConfirmButton
      onConfirm={onConfirm}
      label={<RefreshCw className="w-4 h-4" />}
      confirmLabel={<span className="text-[11px] font-medium px-0.5">restart?</span>}
      title={`Restart ${resourceLabel(resource)}`}
      aria-label={`Restart ${resourceLabel(resource)}`}
      className="shrink-0 flex items-center justify-center p-1 rounded text-text-muted hover:text-text hover:bg-bg-panel2 transition-colors"
      armedClassName="bg-accent/15 text-accent ring-1 ring-accent/40"
    />
  );
}

function CordonNodeButton({
  node,
  mode,
}: {
  node: Node;
  mode: string | undefined;
}) {
  const cordoned = node.status === "schedulingdisabled";
  const label = cordoned ? "Uncordon" : "Cordon";

  const onConfirm = async () => {
    let res: ActionResult = { ok: true };
    if (mode !== "mock") {
      res = await cordonNode(node.name, !cordoned);
    }
    if (res.ok) {
      workspaceActions.toast(`${label.toLowerCase()}ed Node/${node.name}`, "success");
    } else {
      workspaceActions.toast(
        res.error ?? `failed to ${label.toLowerCase()} Node/${node.name}`,
        "error",
      );
    }
  };

  return (
    <ConfirmButton
      onConfirm={onConfirm}
      label={cordoned ? <LockOpen className="w-4 h-4" /> : <Lock className="w-4 h-4" />}
      confirmLabel={<span className="text-[11px] font-medium px-0.5">{label.toLowerCase()}?</span>}
      title={`${label} Node/${node.name}`}
      aria-label={`${label} Node/${node.name}`}
      className="shrink-0 flex items-center justify-center p-1 rounded text-text-muted hover:text-text hover:bg-bg-panel2 transition-colors"
      armedClassName="bg-accent/15 text-accent ring-1 ring-accent/40"
    />
  );
}

function HelmUninstallButton({
  resource,
  mode,
  helmCli,
}: {
  resource: HelmRelease;
  mode: string | undefined;
  helmCli: boolean;
}) {
  // Show a disabled button with explanation when helm CLI is unavailable.
  if (!helmCli || isMockMode(mode)) {
    const reason = !helmCli
      ? "helm CLI not found on the kubagachi host"
      : "unavailable in demo mode";
    return (
      <button
        type="button"
        disabled
        title={reason}
        aria-label={`Uninstall HelmRelease/${resource.name} (disabled: ${reason})`}
        className="shrink-0 flex items-center justify-center p-1 rounded text-text-muted/30 cursor-not-allowed"
      >
        <Trash2 className="w-4 h-4" />
      </button>
    );
  }

  const onConfirm = async () => {
    const res = await helmUninstall(resource.namespace ?? "", resource.name);
    if (res.ok) {
      workspaceActions.toast(`uninstalled ${resource.name}`, "success");
      workspaceActions.selectResource(null);
    } else {
      workspaceActions.toast(res.error ?? `failed to uninstall ${resource.name}`, "error");
    }
  };

  return (
    <ConfirmButton
      onConfirm={onConfirm}
      label={<Trash2 className="w-4 h-4" />}
      confirmLabel={<span className="text-[11px] font-medium px-0.5">uninstall?</span>}
      title={`Uninstall ${resource.name}`}
      aria-label={`Uninstall HelmRelease/${resource.name}`}
      className="shrink-0 flex items-center justify-center p-1 rounded text-status-crashloop/80 hover:text-status-crashloop hover:bg-status-crashloop/10 transition-colors"
      armedClassName="bg-status-crashloop/20 text-status-crashloop ring-1 ring-status-crashloop/50"
    />
  );
}

function DeleteResourceButton({
  resource,
  mode,
  currentNamespace,
}: {
  resource: AnyResource;
  mode: string | undefined;
  currentNamespace?: string;
}) {
  const onConfirm = async () => {
    let res: ActionResult = { ok: true };
    if (mode !== "mock") {
      res = await deleteResource(
        resource.kind,
        resource.name,
        resourceNamespace(resource, currentNamespace),
      );
    }
    if (res.ok) {
      workspaceActions.toast(`deleting ${resourceLabel(resource)}`, "success");
      workspaceActions.selectResource(null);
    } else {
      workspaceActions.toast(res.error ?? `failed to delete ${resourceLabel(resource)}`, "error");
    }
  };

  return (
    <ConfirmButton
      onConfirm={onConfirm}
      label={<Trash2 className="w-4 h-4" />}
      confirmLabel={<span className="text-[11px] font-medium px-0.5">delete?</span>}
      title={`Delete ${resourceLabel(resource)}`}
      aria-label={`Delete ${resourceLabel(resource)}`}
      className="shrink-0 flex items-center justify-center p-1 rounded text-status-crashloop/80 hover:text-status-crashloop hover:bg-status-crashloop/10 transition-colors"
      armedClassName="bg-status-crashloop/20 text-status-crashloop ring-1 ring-status-crashloop/50"
    />
  );
}

// ---------------------------------------------------------------------------
// Tiny kind icon (just a colored dot — no need for per-kind imagery)
// ---------------------------------------------------------------------------

const KIND_COLOR: Partial<Record<AnyResource["kind"], string>> = {
  Pod: "bg-accent",
  Deployment: "bg-accent",
  StatefulSet: "bg-accent",
  DaemonSet: "bg-accent",
  Service: "bg-status-running",
  Ingress: "bg-status-running",
  Node: "bg-status-pending",
  Event: "bg-status-backoff",
  Secret: "bg-status-backoff",
};

function KindIcon({ kind }: { kind: AnyResource["kind"] }) {
  const color = KIND_COLOR[kind] ?? "bg-text-muted";
  return (
    <div className="mt-1 w-6 h-6 rounded bg-bg-panel2 border border-border flex items-center justify-center shrink-0">
      <span className={`w-2 h-2 rounded-sm ${color}`} />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Kubagachi pin bits (pod-only)
// ---------------------------------------------------------------------------

function PinHotbarButton({ pod, pinned }: { pod: Pod; pinned: boolean }) {
  const onClick = () => workspaceActions.togglePinPod(pod.uid);
  if (pinned) {
    return (
      <button
        type="button"
        onClick={onClick}
        className="w-full flex items-center justify-center gap-2 px-3 py-2 rounded border border-accent/50 bg-accent/10 text-accent text-[12px] hover:bg-accent/15 transition-colors"
      >
        <PinOff className="w-3.5 h-3.5" />
        Unpin from Hotbar
      </button>
    );
  }
  return (
    <button
      type="button"
      onClick={onClick}
      className="w-full flex items-center justify-center gap-2 px-3 py-2 rounded border border-accent/60 bg-accent/15 text-accent text-[12px] font-medium hover:bg-accent/25 transition-colors"
    >
      <Pin className="w-3.5 h-3.5" />
      Pin to Hotbar
    </button>
  );
}
