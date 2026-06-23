import { ChevronDown, ChevronRight } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { Cluster } from "../lib/types";
import {
  useActiveTab,
  useCluster,
  usePinnedPodUids,
  useSidebarState,
  useTabs,
  workspaceActions,
  type TabKind,
} from "../store/workspace";
import { iconForKind } from "./TabsBar";

interface LeafEntry {
  kind: TabKind;
  label: string;
  /** Source of a numeric count badge, given the current cluster snapshot. */
  count?: (c: Cluster) => number;
  /**
   * Override that bypasses the `cluster`-derived count — used for things like
   * Kubagachi where the count comes from store state, not cluster state.
   */
  externalCount?: number;
}

interface GroupEntry {
  id: string;
  label: string;
  leaves: LeafEntry[];
}

const len = <T,>(arr: T[]): number => arr.length;

/**
 * Note: the Kubagachi group is injected at render-time inside `Sidebar` so
 * its count can be read from the workspace store (not the cluster snapshot).
 * Everything else is static.
 */
const GROUPS: GroupEntry[] = [
  {
    id: "cluster",
    label: "Cluster",
    leaves: [
      { kind: "overview", label: "Overview" },
      { kind: "Node", label: "Nodes", count: (c) => len(c.nodes) },
      { kind: "Namespace", label: "Namespaces", count: (c) => len(c.namespaces) },
      { kind: "events", label: "Events", count: (c) => len(c.events) },
    ],
  },
  {
    id: "workloads",
    label: "Workloads",
    leaves: [
      { kind: "Pod", label: "Pods", count: (c) => len(c.pods) },
      { kind: "Deployment", label: "Deployments", count: (c) => len(c.deployments) },
      { kind: "StatefulSet", label: "StatefulSets", count: (c) => len(c.statefulSets) },
      { kind: "DaemonSet", label: "DaemonSets", count: (c) => len(c.daemonSets) },
      { kind: "ReplicaSet", label: "ReplicaSets", count: (c) => len(c.replicaSets) },
      { kind: "Job", label: "Jobs", count: (c) => len(c.jobs) },
      { kind: "CronJob", label: "CronJobs", count: (c) => len(c.cronJobs) },
    ],
  },
  {
    id: "config",
    label: "Config",
    leaves: [
      { kind: "ConfigMap", label: "ConfigMaps", count: (c) => len(c.configMaps) },
      { kind: "Secret", label: "Secrets", count: (c) => len(c.secrets) },
      { kind: "ResourceQuota", label: "ResourceQuotas", count: (c) => len(c.resourceQuotas) },
      { kind: "LimitRange", label: "LimitRanges", count: (c) => len(c.limitRanges) },
      {
        kind: "HorizontalPodAutoscaler",
        label: "HPAs",
        count: (c) => len(c.horizontalPodAutoscalers),
      },
      {
        kind: "PodDisruptionBudget",
        label: "PDBs",
        count: (c) => len(c.podDisruptionBudgets),
      },
    ],
  },
  {
    id: "network",
    label: "Network",
    leaves: [
      { kind: "Service", label: "Services", count: (c) => len(c.services) },
      { kind: "Endpoint", label: "Endpoints", count: (c) => len(c.endpoints) },
      { kind: "Ingress", label: "Ingresses", count: (c) => len(c.ingresses) },
      {
        kind: "NetworkPolicy",
        label: "NetworkPolicies",
        count: (c) => len(c.networkPolicies),
      },
    ],
  },
  {
    id: "storage",
    label: "Storage",
    leaves: [
      {
        kind: "PersistentVolume",
        label: "PersistentVolumes",
        count: (c) => len(c.persistentVolumes),
      },
      {
        kind: "PersistentVolumeClaim",
        label: "PersistentVolumeClaims",
        count: (c) => len(c.persistentVolumeClaims),
      },
      { kind: "StorageClass", label: "StorageClasses", count: (c) => len(c.storageClasses) },
    ],
  },
  {
    id: "rbac",
    label: "RBAC",
    leaves: [
      {
        kind: "ServiceAccount",
        label: "ServiceAccounts",
        count: (c) => len(c.serviceAccounts),
      },
      { kind: "Role", label: "Roles", count: (c) => len(c.roles) },
      { kind: "ClusterRole", label: "ClusterRoles", count: (c) => len(c.clusterRoles) },
      { kind: "RoleBinding", label: "RoleBindings", count: (c) => len(c.roleBindings) },
      {
        kind: "ClusterRoleBinding",
        label: "ClusterRoleBindings",
        count: (c) => len(c.clusterRoleBindings),
      },
    ],
  },
  {
    id: "custom",
    label: "Custom",
    leaves: [
      {
        kind: "CustomResourceDefinition",
        label: "CRDs",
        count: (c) => len(c.customResourceDefinitions),
      },
    ],
  },
  {
    id: "helm",
    label: "Helm",
    leaves: [{ kind: "HelmRelease", label: "Releases", count: (c) => len(c.helmReleases) }],
  },
];

export default function Sidebar({ overlay = false }: { overlay?: boolean }) {
  const { open, collapsed, groups } = useSidebarState();
  const cluster = useCluster();
  const tabs = useTabs();
  const activeTabId = useActiveTab();
  const pinned = usePinnedPodUids();

  // Track which TabKinds are currently open in any tab, and which is "active".
  const openKinds = new Set<TabKind>(tabs.map((t) => t.kind));
  const activeTab = tabs.find((t) => t.id === activeTabId);
  const activeKind: TabKind | undefined = activeTab?.kind;

  const allGroups: GroupEntry[] = [
    {
      id: "kubagachi",
      label: "Kubagachi",
      leaves: [
        {
          kind: "kubagachi",
          label: "My pets",
          externalCount: pinned.length,
        },
      ],
    },
    ...GROUPS,
  ];

  return (
    <>
      {/* Scrim behind the drawer. In overlay mode (habitat home) it covers at
          every width; docked mode only needs it on the mobile drawer. */}
      {open && (
        <div
          aria-hidden
          onClick={() => workspaceActions.toggleSidebar()}
          className={
            (overlay ? "" : "md:hidden ") +
            "fixed inset-0 z-30 bg-black/60 backdrop-blur-sm transition-opacity duration-150"
          }
        />
      )}

      <aside
        className={
          overlay
            ? // Overlay drawer at every width — opened by the TopBar nav toggle
              // on the habitat, where the sidebar isn't docked.
              "shrink-0 border-r border-border bg-bg-panel2 flex flex-col text-text-muted z-40 " +
              "fixed inset-y-0 left-0 w-60 transform transition-transform duration-150 " +
              (open ? "translate-x-0" : "-translate-x-full")
            : // Docked on desktop, off-canvas drawer on mobile.
              "shrink-0 border-r border-border bg-bg-panel2 flex flex-col text-text-muted z-40 " +
              (collapsed ? "md:w-14 " : "md:w-60 ") +
              "fixed inset-y-0 left-0 w-60 transform transition-transform duration-150 md:static md:translate-x-0 " +
              (open ? "translate-x-0 " : "-translate-x-full md:translate-x-0 ")
        }
      >
        {/* Scrollable group list */}
        <div className="flex-1 min-h-0 overflow-y-auto scrollbar-thin py-2">
          {allGroups.map((g) => (
            <SidebarGroup
              key={g.id}
              group={g}
              collapsedSidebar={overlay ? false : collapsed}
              groupCollapsed={!!groups[g.id]}
              cluster={cluster}
              openKinds={openKinds}
              activeKind={activeKind}
              overlay={overlay}
            />
          ))}
        </div>

        {/* Footer */}
        <SidebarFooter collapsed={collapsed} />
      </aside>
    </>
  );
}

interface SidebarGroupProps {
  group: GroupEntry;
  collapsedSidebar: boolean;
  groupCollapsed: boolean;
  cluster: Cluster | null;
  openKinds: Set<TabKind>;
  activeKind: TabKind | undefined;
  overlay: boolean;
}

function SidebarGroup({
  group,
  collapsedSidebar,
  groupCollapsed,
  cluster,
  openKinds,
  activeKind,
  overlay,
}: SidebarGroupProps) {
  // When the sidebar is in icon-only mode we render the leaves directly
  // without a header — icons stack vertically.
  if (collapsedSidebar) {
    return (
      <div className="px-1 pb-2 mb-1 border-b border-border/60 last:border-b-0">
        {group.leaves.map((leaf) => (
          <SidebarLeaf
            key={leaf.kind + leaf.label}
            leaf={leaf}
            collapsedSidebar
            cluster={cluster}
            open={openKinds.has(leaf.kind)}
            active={activeKind === leaf.kind}
            overlay={overlay}
          />
        ))}
      </div>
    );
  }

  const Caret = groupCollapsed ? ChevronRight : ChevronDown;

  return (
    <div className="px-1 mb-1">
      <button
        type="button"
        onClick={() => workspaceActions.toggleGroup(group.id)}
        className="w-full flex items-center gap-1 px-2 py-1 text-[10px] uppercase tracking-wider text-text-muted/80 hover:text-text transition-colors duration-100 font-mono"
      >
        <Caret size={11} className="opacity-70" />
        <span className="flex-1 text-left">─ {group.label} ─</span>
      </button>
      {!groupCollapsed && (
        <div className="mt-0.5">
          {group.leaves.map((leaf) => (
            <SidebarLeaf
              key={leaf.kind + leaf.label}
              leaf={leaf}
              collapsedSidebar={false}
              cluster={cluster}
              open={openKinds.has(leaf.kind)}
              active={activeKind === leaf.kind}
              overlay={overlay}
            />
          ))}
        </div>
      )}
    </div>
  );
}

interface SidebarLeafProps {
  leaf: LeafEntry;
  collapsedSidebar: boolean;
  cluster: Cluster | null;
  open: boolean;
  active: boolean;
  overlay: boolean;
}

function SidebarLeaf({ leaf, collapsedSidebar, cluster, open, active, overlay }: SidebarLeafProps) {
  const Icon: LucideIcon = iconForKind(leaf.kind);
  const count =
    leaf.externalCount !== undefined
      ? leaf.externalCount
      : cluster && leaf.count
        ? leaf.count(cluster)
        : undefined;
  const mutedBadge = leaf.externalCount === 0;

  const onClick = () => {
    workspaceActions.openTab(leaf.kind);
    // Close the drawer on selection: always in overlay mode (habitat), and on
    // the mobile off-canvas drawer.
    if (overlay || (typeof window !== "undefined" && window.innerWidth < 768)) {
      workspaceActions.toggleSidebar();
    }
  };

  if (collapsedSidebar) {
    return (
      <button
        type="button"
        onClick={onClick}
        className={
          "group relative w-full flex items-center justify-center h-9 my-0.5 transition-colors duration-100 k9s-square " +
          (active
            ? "bg-bg-panel text-text shadow-[inset_2px_0_0_0_#c9b88a]"
            : open
              ? "text-text hover:bg-bg-panel"
              : "text-text-muted hover:bg-bg-panel hover:text-text")
        }
        aria-label={leaf.label}
      >
        <Icon size={15} className={active ? "text-accent" : ""} />
        {/* Tooltip */}
        <span className="pointer-events-none absolute left-full ml-2 z-50 hidden group-hover:flex items-center gap-2 whitespace-nowrap rounded border border-border bg-bg-panel px-2 py-1 text-[11px] text-text shadow-lg shadow-black/40">
          {leaf.label}
          {count !== undefined && (
            <span className="text-text-muted">{count}</span>
          )}
        </span>
      </button>
    );
  }

  return (
    <button
      type="button"
      onClick={onClick}
      className={
        "group w-full flex items-center gap-2 h-7 px-2 my-px text-[12px] transition-colors duration-100 font-mono k9s-square " +
        (active
          ? "bg-bg-panel text-text border-l-2 border-accent pl-[6px]"
          : open
            ? "text-text hover:bg-bg-panel"
            : "text-text-muted hover:bg-bg-panel hover:text-text")
      }
    >
      <Icon
        size={13}
        className={active ? "text-accent" : "text-text-muted group-hover:text-text"}
      />
      <span className="flex-1 text-left truncate">{leaf.label}</span>
      {count !== undefined && (
        <span
          className={
            "text-[10px] tabular-nums px-1 py-0.5 k9s-square " +
            (mutedBadge
              ? "text-text-muted/70"
              : active
                ? "text-text"
                : "text-text-muted group-hover:text-text")
          }
        >
          [{count}]
        </span>
      )}
    </button>
  );
}

function SidebarFooter({ collapsed }: { collapsed: boolean }) {
  return (
    <div className="shrink-0 border-t border-border px-2 py-2 flex items-center gap-2 font-mono">
      <button
        type="button"
        onClick={() => workspaceActions.collapseSidebar()}
        aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
        className="hidden md:inline-flex h-6 w-6 items-center justify-center k9s-square hover:bg-bg-panel text-text-muted hover:text-text transition-colors duration-100"
      >
        {collapsed ? <ChevronRight size={12} /> : <ChevronDown size={12} className="rotate-90" />}
      </button>
      {!collapsed && (
        <div className="flex flex-col leading-tight min-w-0">
          <span className="text-[11px] text-text font-semibold tracking-tight truncate">
            <span className="text-accent">▮</span> kubagachi
          </span>
          <span className="text-[10px] text-text-muted">v0.1 · mock cluster</span>
        </div>
      )}
    </div>
  );
}
