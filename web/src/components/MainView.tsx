/**
 * MainView — top-level routing for the active workspace tab.
 *
 * Reads the active tab from the workspace store and dispatches:
 *   overview -> HabitatDashboard (full-bleed home screen)
 *   events   -> EventsView
 *   Pod      -> PodList (special-cased for the mascot UX)
 *   else     -> ResourceList<kind>
 *
 * Header bar inside main shows a breadcrumb plus the namespace pill.
 *
 * Expected workspace hooks:
 *   useTabs()      -> Tab[]
 *   useActiveTab() -> string (active tab id)
 *   useNamespace() -> string
 *   useCluster()   -> Cluster | null
 */

import { useMemo } from "react";
import { Search } from "lucide-react";
import type { AnyResourceKind } from "../lib/types";
import {
  useActiveTab,
  useCluster,
  useNamespace,
  useSearch,
  useTabs,
  workspaceActions,
  titleForTabKind,
  type Tab,
} from "../store/workspace";
import HabitatDashboard from "./HabitatDashboard";
import EventsView from "./EventsView";
import KubagachiTab from "./KubagachiTab";
import PodList from "./PodList";
import ResourceList from "./ResourceList";

const RESOURCE_KINDS = [
  "Pod",
  "Deployment",
  "StatefulSet",
  "DaemonSet",
  "ReplicaSet",
  "Job",
  "CronJob",
  "Service",
  "Ingress",
  "Endpoint",
  "NetworkPolicy",
  "ConfigMap",
  "Secret",
  "ResourceQuota",
  "LimitRange",
  "HorizontalPodAutoscaler",
  "PodDisruptionBudget",
  "PersistentVolume",
  "PersistentVolumeClaim",
  "StorageClass",
  "ServiceAccount",
  "Role",
  "ClusterRole",
  "RoleBinding",
  "ClusterRoleBinding",
  "Node",
  "Namespace",
  "Event",
  "CustomResourceDefinition",
  "HelmRelease",
] as const satisfies readonly AnyResourceKind[];

const RESOURCE_SET = new Set<string>(RESOURCE_KINDS);

export default function MainView() {
  const tabs = useTabs();
  const activeId = useActiveTab();
  const namespace = useNamespace();
  const cluster = useCluster();
  const search = useSearch();

  const active: Tab | undefined = useMemo(
    () => tabs.find((t) => t.id === activeId),
    [tabs, activeId],
  );

  if (!cluster) {
    return (
      <main className="flex-1 min-w-0 overflow-y-auto scrollbar-thin">
        <div className="p-6 text-text-muted text-[12px]">Loading cluster…</div>
      </main>
    );
  }

  if (!active) {
    return (
      <main className="flex-1 min-w-0 overflow-y-auto scrollbar-thin">
        <div className="p-6 text-text-muted text-[12px]">No tab open.</div>
      </main>
    );
  }

  // The home tab is a full-bleed habitat — it owns its own chrome, so no
  // resource header and no outer scroll wrapper.
  if (active.kind === "overview") {
    return (
      <main className="flex-1 min-w-0 min-h-0 flex flex-col">
        <HabitatDashboard />
      </main>
    );
  }

  return (
    <main className="flex-1 min-w-0 overflow-y-auto scrollbar-thin flex flex-col">
      <Header tab={active} namespace={namespace} search={search} count={countForTab(active, cluster)} />
      <div className="flex-1 min-w-0">
        {renderBody(active)}
      </div>
    </main>
  );
}

function countForTab(tab: Tab, c: import("../lib/types").Cluster): number | undefined {
  const map: Record<string, number> = {
    Pod: c.pods.length,
    Deployment: c.deployments.length,
    StatefulSet: c.statefulSets.length,
    DaemonSet: c.daemonSets.length,
    ReplicaSet: c.replicaSets.length,
    Job: c.jobs.length,
    CronJob: c.cronJobs.length,
    Service: c.services.length,
    Ingress: c.ingresses.length,
    Endpoint: c.endpoints.length,
    NetworkPolicy: c.networkPolicies.length,
    ConfigMap: c.configMaps.length,
    Secret: c.secrets.length,
    ResourceQuota: c.resourceQuotas.length,
    LimitRange: c.limitRanges.length,
    HorizontalPodAutoscaler: c.horizontalPodAutoscalers.length,
    PodDisruptionBudget: c.podDisruptionBudgets.length,
    PersistentVolume: c.persistentVolumes.length,
    PersistentVolumeClaim: c.persistentVolumeClaims.length,
    StorageClass: c.storageClasses.length,
    ServiceAccount: c.serviceAccounts.length,
    Role: c.roles.length,
    ClusterRole: c.clusterRoles.length,
    RoleBinding: c.roleBindings.length,
    ClusterRoleBinding: c.clusterRoleBindings.length,
    Node: c.nodes.length,
    Namespace: c.namespaces.length,
    Event: c.events.length,
    CustomResourceDefinition: c.customResourceDefinitions.length,
    HelmRelease: c.helmReleases.length,
  };
  return map[tab.kind];
}

function renderBody(tab: Tab): React.ReactNode {
  if (tab.kind === "overview") return <HabitatDashboard />;
  if (tab.kind === "events") return <EventsView />;
  if (tab.kind === "search") return <SearchView />;
  if (tab.kind === "kubagachi") return <KubagachiTab />;
  if (tab.kind === "Pod") return <PodList />;
  if (RESOURCE_SET.has(tab.kind)) {
    return <ResourceList kind={tab.kind as Exclude<AnyResourceKind, "Pod">} />;
  }
  return <div className="p-6 text-text-muted text-[12px]">Unknown tab kind: {tab.kind}</div>;
}

// ---------------------------------------------------------------------------
// Header (breadcrumb + namespace pill)
// ---------------------------------------------------------------------------

function Header({
  tab,
  namespace,
  search,
  count,
}: {
  tab: Tab;
  namespace: string;
  search: string;
  count?: number;
}) {
  const kindLabel =
    tab.kind === "overview"
      ? "Cluster overview"
      : tab.kind === "events"
      ? "Events"
      : tab.kind === "search"
      ? "Search"
      : tab.kind === "kubagachi"
      ? "Kubagachi"
      : titleForTabKind(tab.kind);

  const titleText = count !== undefined ? `${kindLabel} (${count})` : kindLabel;

  return (
    <div className="sticky top-0 z-10 bg-bg-panel/95 backdrop-blur border-b border-border px-3 py-1.5 flex items-center gap-3 flex-wrap font-mono">
      <span className="k9s-bracket text-[12px] text-accent tracking-tight">
        <span className="text-text font-semibold">{titleText}</span>
      </span>

      <span className="text-border select-none" aria-hidden="true">│</span>

      <span className="text-[11px] text-text-muted whitespace-nowrap">
        <span className="opacity-70 uppercase tracking-wider">ns:</span>{" "}
        <span className="text-text">{namespace === "all" ? "all" : namespace}</span>
      </span>

      <div className="ml-auto flex items-center gap-2">
        <SearchBox value={search} />
      </div>
    </div>
  );
}

function SearchBox({ value }: { value: string }) {
  return (
    <div className="relative">
      <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-text-muted" />
      <input
        type="text"
        value={value}
        onChange={(e) => workspaceActions.setSearch(e.target.value)}
        placeholder="filter…"
        className="w-32 sm:w-48 pl-7 pr-2 py-1 bg-bg-panel2 border border-border text-[12px] text-text placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors font-mono k9s-square"
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Search view (synthetic tab; renders an aggregate match list)
// ---------------------------------------------------------------------------

function SearchView() {
  const cluster = useCluster();
  const search = useSearch();
  const q = search.trim().toLowerCase();

  const matches = useMemo(() => {
    if (!cluster || !q) return [];
    const all: { kind: string; name: string; namespace?: string; uid: string }[] = [];
    const buckets: Array<[string, { uid: string; name: string; namespace?: string }[]]> = [
      ["Pod", cluster.pods],
      ["Deployment", cluster.deployments],
      ["StatefulSet", cluster.statefulSets],
      ["DaemonSet", cluster.daemonSets],
      ["ReplicaSet", cluster.replicaSets],
      ["Job", cluster.jobs],
      ["CronJob", cluster.cronJobs],
      ["Service", cluster.services],
      ["Ingress", cluster.ingresses],
      ["ConfigMap", cluster.configMaps],
      ["Secret", cluster.secrets],
      ["Node", cluster.nodes],
      ["Namespace", cluster.namespaces],
      ["HelmRelease", cluster.helmReleases],
    ];
    for (const [kind, items] of buckets) {
      for (const it of items) {
        if (it.name.toLowerCase().includes(q)) {
          all.push({ kind, name: it.name, namespace: it.namespace, uid: it.uid });
        }
      }
    }
    return all.slice(0, 100);
  }, [cluster, q]);

  if (!q) {
    return (
      <div className="p-6 text-text-muted text-[12px]">
        Type in the filter box to search across the cluster.
      </div>
    );
  }
  if (matches.length === 0) {
    return (
      <div className="p-6 text-text-muted text-[12px]">No matches for "{q}".</div>
    );
  }
  return (
    <div className="p-4 flex flex-col gap-1">
      {matches.map((m) => (
        <button
          key={m.uid}
          onClick={() => workspaceActions.selectResource(m.uid)}
          className="flex items-center gap-2 text-left p-2 border border-border bg-bg-panel rounded hover:border-border-strong transition-colors"
        >
          <span className="text-[10px] text-text-muted uppercase tracking-wider w-32 shrink-0">
            {m.kind}
          </span>
          <span className="text-[12px] text-text truncate">{m.name}</span>
          {m.namespace && (
            <span className="ml-auto text-[10px] text-text-muted">{m.namespace}</span>
          )}
        </button>
      ))}
    </div>
  );
}
