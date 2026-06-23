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
  ChevronRight,
  FileText,
  Pin,
  PinOff,
  RefreshCw,
  ScrollText,
  Terminal,
  Trash2,
  X,
} from "lucide-react";
import type { AnyResource, Event, Pod, PodStatus } from "../lib/types";
import { formatAge } from "../lib/format";
import { deletePod, fetchLogs } from "../lib/cluster-api";
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

type TabId = "overview" | "yaml" | "events" | "logs" | "shell";

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

  // The set of tab ids this resource actually exposes.
  const tabIds = useMemo<TabId[]>(
    () => (isPod ? ["overview", "yaml", "events", "logs", "shell"] : ["overview", "yaml", "events"]),
    [isPod],
  );

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
          {isPod && <PodDeleteButton pod={resource as Pod} mode={cluster?.mode} />}
          <button
            onClick={close}
            className="p-1 rounded text-text-muted hover:text-text hover:bg-bg-panel2 transition-colors"
            aria-label="Close detail"
          >
            <X className="w-4 h-4" />
          </button>
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
  const yaml = useMemo(() => toFakeYaml(resource), [resource]);
  return (
    <pre className="p-4 text-[11px] leading-relaxed text-text-muted font-mono whitespace-pre-wrap break-words">
      {yaml}
    </pre>
  );
}

/** Render the resource as a hand-wavy YAML-ish blob. */
function toFakeYaml(r: AnyResource): string {
  const raw = JSON.stringify(r, null, 2);
  // Unquote scalar values safely + drop top-level braces.
  return raw
    .replace(/^\{\n/, "")
    .replace(/\n\}$/, "")
    .replace(/^( *)"([^"]+)":/gm, "$1$2:")
    .replace(/: "([^"\\]*)"/g, ": $1")
    .replace(/,\n/g, "\n")
    .replace(/^( *)\[\]/gm, "$1[]")
    .replace(/^( *)\{\}/gm, "$1{}");
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

  const openShell = () => {
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

      <button
        type="button"
        onClick={openShell}
        className="w-full flex items-center justify-center gap-2 px-3 py-2 rounded border border-accent/60 bg-accent/15 text-accent font-medium hover:bg-accent/25 transition-colors"
      >
        <Terminal className="w-3.5 h-3.5" />
        Open shell
      </button>

      <div className="text-[10px] text-text-muted/80 font-mono">
        kubectl exec -it -n {namespace} {pod.name} -c {container} -- sh
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pod delete (destructive, two-step confirm)
// ---------------------------------------------------------------------------

function PodDeleteButton({ pod, mode }: { pod: Pod; mode: string | undefined }) {
  const onConfirm = async () => {
    if (isMockMode(mode)) {
      workspaceActions.toast("demo: pod delete is a no-op", "info");
      return;
    }
    const ns = pod.namespace ?? "default";
    const res = await deletePod(ns, pod.name);
    if (res.ok) {
      workspaceActions.toast(`deleting pod ${pod.name}`, "success");
      workspaceActions.selectResource(null);
    } else {
      workspaceActions.toast(res.error ?? `failed to delete ${pod.name}`, "error");
    }
  };

  return (
    <ConfirmButton
      onConfirm={onConfirm}
      label={<Trash2 className="w-4 h-4" />}
      confirmLabel={<span className="text-[11px] font-medium px-0.5">delete?</span>}
      title="Delete pod"
      aria-label="Delete pod"
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
