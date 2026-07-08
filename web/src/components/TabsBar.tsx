import { useEffect, useRef, useState } from "react";
import {
  Activity,
  BarChart3,
  Box,
  Boxes,
  Calendar,
  CalendarClock,
  ChevronDown,
  Component,
  Cpu,
  Database,
  FileKey,
  FileText,
  GitBranch,
  GitMerge,
  Globe,
  HardDrive,
  Heart,
  HelpCircle,
  Inbox,
  Key,
  KeyRound,
  Layers,
  Network,
  Package,
  Plus,
  Power,
  Search,
  Server,
  Settings,
  Share2,
  Shield,
  Tags,
  User,
  Workflow,
  X,
  Zap,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import {
  titleForTabKind,
  useActiveTab,
  useTabs,
  workspaceActions,
  type TabKind,
} from "../store/workspace";

const ICONS: Record<TabKind, LucideIcon> = {
  // synthetic
  overview: BarChart3,
  events: Activity,
  search: Search,
  kubagachi: Heart,
  yscale: Zap,
  // gitops · flux
  flux: GitMerge,
  "flux-kustomizations": Layers,
  "flux-helmreleases": Package,
  "flux-sources": GitBranch,
  // workloads
  Pod: Box,
  Deployment: Layers,
  StatefulSet: Database,
  DaemonSet: Boxes,
  ReplicaSet: Component,
  Job: Calendar,
  CronJob: CalendarClock,
  // network
  Service: Network,
  Ingress: Globe,
  Endpoint: Share2,
  NetworkPolicy: Shield,
  // config
  ConfigMap: FileText,
  Secret: KeyRound,
  ResourceQuota: Cpu,
  LimitRange: Settings,
  HorizontalPodAutoscaler: GitBranch,
  PodDisruptionBudget: Power,
  // storage
  PersistentVolume: HardDrive,
  PersistentVolumeClaim: HardDrive,
  StorageClass: HardDrive,
  // rbac
  ServiceAccount: User,
  Role: Key,
  ClusterRole: Key,
  RoleBinding: FileKey,
  ClusterRoleBinding: FileKey,
  // cluster
  Node: Server,
  Namespace: Tags,
  Event: Inbox,
  // custom + helm
  CustomResourceDefinition: Workflow,
  HelmRelease: Package,
};

export function iconForKind(kind: TabKind): LucideIcon {
  return ICONS[kind] ?? HelpCircle;
}

// Groups for the "+" menu
const ADD_GROUPS: { label: string; kinds: TabKind[] }[] = [
  { label: "General", kinds: ["overview", "events", "search", "kubagachi", "yscale"] },
  {
    label: "Workloads",
    kinds: ["Pod", "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job", "CronJob"],
  },
  {
    label: "Network",
    kinds: ["Service", "Ingress", "Endpoint", "NetworkPolicy"],
  },
  {
    label: "Config",
    kinds: [
      "ConfigMap",
      "Secret",
      "ResourceQuota",
      "LimitRange",
      "HorizontalPodAutoscaler",
      "PodDisruptionBudget",
    ],
  },
  {
    label: "Storage",
    kinds: ["PersistentVolume", "PersistentVolumeClaim", "StorageClass"],
  },
  {
    label: "RBAC",
    kinds: ["ServiceAccount", "Role", "ClusterRole", "RoleBinding", "ClusterRoleBinding"],
  },
  {
    label: "Cluster",
    kinds: ["Node", "Namespace"],
  },
  {
    label: "Custom · Helm",
    kinds: ["CustomResourceDefinition", "HelmRelease"],
  },
];

export default function TabsBar() {
  const tabs = useTabs();
  const activeTabId = useActiveTab();
  const [menuOpen, setMenuOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const scrollerRef = useRef<HTMLDivElement | null>(null);

  // Auto-scroll active tab into view
  useEffect(() => {
    const scroller = scrollerRef.current;
    if (!scroller) return;
    const el = scroller.querySelector<HTMLElement>(`[data-tab-id="${activeTabId}"]`);
    if (el) el.scrollIntoView({ inline: "nearest", block: "nearest" });
  }, [activeTabId]);

  // Close add-menu on outside click / escape
  useEffect(() => {
    if (!menuOpen) return;
    const onDoc = (e: MouseEvent) => {
      if (!rootRef.current) return;
      if (!rootRef.current.contains(e.target as Node)) setMenuOpen(false);
    };
    const onEsc = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMenuOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onEsc);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onEsc);
    };
  }, [menuOpen]);

  return (
    <div
      ref={rootRef}
      className="sticky top-[68px] z-20 h-9 bg-bg-panel border-b border-border flex items-stretch text-text-muted font-mono"
    >
      <div
        ref={scrollerRef}
        className="relative flex items-stretch overflow-x-auto scrollbar-thin flex-1 min-w-0"
      >
        {tabs.map((tab) => {
          const Icon = iconForKind(tab.kind);
          const active = tab.id === activeTabId;
          const canClose = tabs.length > 1;
          return (
            <div
              key={tab.id}
              data-tab-id={tab.id}
              role="tab"
              aria-selected={active}
              onClick={() => workspaceActions.setActiveTab(tab.id)}
              onMouseDown={(e) => {
                // Middle-click closes
                if (e.button === 1 && canClose) {
                  e.preventDefault();
                  workspaceActions.closeTab(tab.id);
                }
              }}
              className={
                "group relative flex items-center h-full border-r border-border cursor-pointer select-none transition-colors duration-100 px-2 " +
                (active
                  ? "bg-bg-base text-text"
                  : "hover:bg-bg-panel2 hover:text-text")
              }
            >
              {active && (
                <span className="absolute inset-x-0 bottom-0 h-[2px] bg-accent" />
              )}
              {/* bracketed label: [ <icon> <title> ] */}
              <span
                className={
                  "inline-flex items-center gap-1.5 text-[12px] whitespace-nowrap " +
                  (active ? "" : "opacity-90")
                }
              >
                <span
                  aria-hidden="true"
                  className={active ? "text-accent" : "text-text-muted/70"}
                >
                  [
                </span>
                <Icon size={11} className={active ? "text-accent" : "text-text-muted"} />
                <span className="max-w-[200px] truncate">{tab.title}</span>
                <span
                  aria-hidden="true"
                  className={active ? "text-accent" : "text-text-muted/70"}
                >
                  ]
                </span>
              </span>
              {canClose && (
                <button
                  type="button"
                  aria-label={`Close ${tab.title}`}
                  onClick={(e) => {
                    e.stopPropagation();
                    workspaceActions.closeTab(tab.id);
                  }}
                  className={
                    "ml-1 inline-flex items-center justify-center h-4 w-4 transition-colors duration-100 k9s-square hover:bg-status-crashloop/20 hover:text-status-crashloop " +
                    (active ? "opacity-100 text-text" : "text-text-muted opacity-0 group-hover:opacity-100")
                  }
                >
                  <X size={12} />
                </button>
              )}
            </div>
          );
        })}
        <div
          className="pointer-events-none sticky right-0 z-10 -ml-8 h-full w-8 shrink-0 bg-gradient-to-l from-bg-panel via-bg-panel/90 to-transparent"
          aria-hidden="true"
        />
      </div>

      {/* "+" new tab */}
      <div className="relative shrink-0 border-l border-border">
        <button
          type="button"
          aria-label="Open new tab"
          onClick={() => setMenuOpen((v) => !v)}
          className={
            "h-full px-2 flex items-center gap-1 text-text-muted hover:bg-bg-panel2 hover:text-text transition-colors duration-100 " +
            (menuOpen ? "bg-bg-panel2 text-text" : "")
          }
        >
          <Plus size={13} />
          <ChevronDown size={11} className="opacity-70" />
        </button>
        {menuOpen && <AddTabMenu onClose={() => setMenuOpen(false)} />}
      </div>
    </div>
  );
}

function AddTabMenu({ onClose }: { onClose: () => void }) {
  return (
    <div
      role="menu"
      className="absolute right-0 top-[calc(100%+4px)] z-40 w-[280px] border border-border bg-bg-panel shadow-lg shadow-black/40 py-1 max-h-[70vh] overflow-y-auto scrollbar-thin k9s-square"
    >
      {ADD_GROUPS.map((g) => (
        <div key={g.label} className="py-1">
          <div className="px-3 py-1 text-[10px] uppercase tracking-wider text-text-muted">
            {g.label}
          </div>
          {g.kinds.map((kind) => {
            const Icon = iconForKind(kind);
            return (
              <button
                key={kind}
                type="button"
                role="menuitem"
                onClick={() => {
                  workspaceActions.openTab(kind);
                  onClose();
                }}
                className="w-full flex items-center gap-2 px-3 py-1.5 text-[12px] text-left text-text-muted hover:bg-bg-panel2 hover:text-text transition-colors duration-100"
              >
                <Icon size={12} className="text-text-muted" />
                <span className="flex-1 truncate">{titleForTabKind(kind)}</span>
              </button>
            );
          })}
        </div>
      ))}
    </div>
  );
}

/**
 * Re-export for other components (Sidebar) so they can reuse the same icon
 * mapping without duplicating it. The export is intentional and the import
 * sites use it; do not remove during dead-code passes.
 */
export type { TabKind };
