/**
 * Workspace store for the Kubagachi web dashboard.
 *
 * A tiny vanilla-React store built on `useSyncExternalStore`. Holds:
 *
 *   - `cluster`              live snapshot fetched + ticked via cluster-api
 *   - `tabs` / `activeTabId` workspace tab strip
 *   - `sidebarOpen`          mobile drawer open flag (transient)
 *   - `sidebarCollapsed`     desktop "icon only" rail flag    (persisted)
 *   - `sidebarGroups`        per-group collapsed state        (persisted)
 *   - `selectedResourceUid`  drives the DetailDrawer          (transient)
 *   - `search`               global search query              (transient)
 *   - `selectedNamespace`    "all" or a namespace name        (persisted)
 *   - `selectedContext`      mock-cluster context name        (persisted)
 *   - `pinnedPodUids`        Kubagachi pinned-pod list       (persisted)
 *
 * The "persisted" subset is mirrored to localStorage under
 * `kubagachi:workspace:v1` and rehydrated on init.
 *
 * Subscriptions use `useSyncExternalStore` per-slice via `makeHook(selector)`,
 * so component re-renders are scoped to the data they actually read.
 */

import { useSyncExternalStore } from "react";
import type { AnyResourceKind, Cluster, Pod } from "../lib/types";
import { loadCluster, subscribeClusterUpdates } from "../lib/cluster-api";

// ---------------------------------------------------------------------------
// Tab kinds
// ---------------------------------------------------------------------------

/** Synthetic tab kinds that aren't a Kubernetes resource type. */
export type SyntheticTabKind =
  | "overview"
  | "events"
  | "search"
  | "kubagachi"
  | "flux"
  | "flux-kustomizations"
  | "flux-helmreleases"
  | "flux-sources";

/** Anything that can drive a tab: every resource kind plus a few synthetic ones. */
export type TabKind = AnyResourceKind | SyntheticTabKind;

export interface Tab {
  id: string;
  kind: TabKind;
  title: string;
  /** Optional namespace scope for the tab. */
  ns?: string;
}

// ---------------------------------------------------------------------------
// Store shape
// ---------------------------------------------------------------------------

/** A live terminal session targeting one pod container. */
export interface TerminalSession {
  namespace: string;
  pod: string;
  container: string;
}

/** A transient toast notification (action feedback). */
export interface Toast {
  id: number;
  kind: "info" | "success" | "error";
  message: string;
}

export interface WorkspaceState {
  cluster: Cluster | null;
  tabs: Tab[];
  activeTabId: string;
  sidebarOpen: boolean;
  sidebarCollapsed: boolean;
  sidebarGroups: Record<string, boolean>;
  selectedResourceUid: string | null;
  /** Optional drawer tab requested alongside the selection ("logs", "shell"…). */
  drawerTab: string | null;
  search: string;
  selectedNamespace: string;
  selectedContext: string;
  /** Pod uids the user has pinned to the Kubagachi hotbar. */
  pinnedPodUids: string[];
  /** Active exec session shown in the bottom TerminalDock (transient). */
  terminalSession: TerminalSession | null;
  /** `:` command palette open flag (transient). */
  paletteOpen: boolean;
  /** `?` keybindings help overlay open flag (transient). */
  helpOpen: boolean;
  /** j/k row cursor for the active table view (transient). */
  selectedRowIndex: number;
  /** Transient action-feedback toasts. */
  toasts: Toast[];
  /** Habitat render mode: dense node-box grid or calm "ranch" pasture (persisted). */
  habitatView: HabitatView;
}

/** How the overview renders the habitat. */
export type HabitatView = "grid" | "ranch";

const STORAGE_KEY = "kubagachi:workspace:v1";
const DEFAULT_CONTEXT = "mock-cluster";

const OVERVIEW_TAB: Tab = {
  id: "tab-overview",
  kind: "overview",
  title: "overview",
};

interface PersistedShape {
  selectedNamespace?: string;
  selectedContext?: string;
  sidebarCollapsed?: boolean;
  sidebarGroups?: Record<string, boolean>;
  pinnedPodUids?: string[];
  habitatView?: HabitatView;
}

function loadPersisted(): PersistedShape {
  if (typeof window === "undefined") return {};
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as unknown;
    if (parsed === null || typeof parsed !== "object") return {};
    const p = parsed as Record<string, unknown>;
    const out: PersistedShape = {};
    if (typeof p.selectedNamespace === "string") out.selectedNamespace = p.selectedNamespace;
    if (typeof p.selectedContext === "string") out.selectedContext = p.selectedContext;
    if (typeof p.sidebarCollapsed === "boolean") out.sidebarCollapsed = p.sidebarCollapsed;
    if (p.sidebarGroups && typeof p.sidebarGroups === "object") {
      const groups: Record<string, boolean> = {};
      for (const [k, v] of Object.entries(p.sidebarGroups as Record<string, unknown>)) {
        if (typeof v === "boolean") groups[k] = v;
      }
      out.sidebarGroups = groups;
    }
    if (Array.isArray(p.pinnedPodUids)) {
      out.pinnedPodUids = p.pinnedPodUids.filter(
        (x): x is string => typeof x === "string",
      );
    }
    if (p.habitatView === "grid" || p.habitatView === "ranch") {
      out.habitatView = p.habitatView;
    }
    return out;
  } catch {
    return {};
  }
}

function persist(state: WorkspaceState): void {
  if (typeof window === "undefined") return;
  try {
    const payload: PersistedShape = {
      selectedNamespace: state.selectedNamespace,
      selectedContext: state.selectedContext,
      sidebarCollapsed: state.sidebarCollapsed,
      sidebarGroups: state.sidebarGroups,
      pinnedPodUids: state.pinnedPodUids,
      habitatView: state.habitatView,
    };
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(payload));
  } catch {
    /* ignore quota / serialization errors */
  }
}

// ---------------------------------------------------------------------------
// Tab title helpers
// ---------------------------------------------------------------------------

const TAB_TITLES: Record<TabKind, string> = {
  // synthetic
  overview: "overview",
  events: "events",
  search: "search",
  kubagachi: "kubagachi",
  flux: "flux",
  "flux-kustomizations": "kustomizations",
  "flux-helmreleases": "helmreleases",
  "flux-sources": "sources",
  // workloads
  Pod: "Pods",
  Deployment: "Deployments",
  StatefulSet: "StatefulSets",
  DaemonSet: "DaemonSets",
  ReplicaSet: "ReplicaSets",
  Job: "Jobs",
  CronJob: "CronJobs",
  // network
  Service: "Services",
  Ingress: "Ingresses",
  Endpoint: "Endpoints",
  NetworkPolicy: "NetworkPolicies",
  // config
  ConfigMap: "ConfigMaps",
  Secret: "Secrets",
  ResourceQuota: "ResourceQuotas",
  LimitRange: "LimitRanges",
  HorizontalPodAutoscaler: "HorizontalPodAutoscalers",
  PodDisruptionBudget: "PodDisruptionBudgets",
  // storage
  PersistentVolume: "PersistentVolumes",
  PersistentVolumeClaim: "PersistentVolumeClaims",
  StorageClass: "StorageClasses",
  // rbac
  ServiceAccount: "ServiceAccounts",
  Role: "Roles",
  ClusterRole: "ClusterRoles",
  RoleBinding: "RoleBindings",
  ClusterRoleBinding: "ClusterRoleBindings",
  // cluster
  Node: "Nodes",
  Namespace: "Namespaces",
  Event: "Events",
  // custom + helm
  CustomResourceDefinition: "CustomResourceDefinitions",
  HelmRelease: "Helm releases",
};

export function titleForTabKind(kind: TabKind): string {
  return TAB_TITLES[kind];
}

function tabIdFor(kind: TabKind, ns?: string): string {
  return ns ? `tab-${kind}-${ns}` : `tab-${kind}`;
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

type Listener = () => void;

function createStore() {
  const persisted = loadPersisted();

  let state: WorkspaceState = {
    cluster: null,
    tabs: [OVERVIEW_TAB],
    activeTabId: OVERVIEW_TAB.id,
    sidebarOpen: false,
    sidebarCollapsed: persisted.sidebarCollapsed ?? false,
    sidebarGroups: persisted.sidebarGroups ?? {},
    selectedResourceUid: null,
    drawerTab: null,
    search: "",
    selectedNamespace: persisted.selectedNamespace ?? "all",
    selectedContext: persisted.selectedContext ?? DEFAULT_CONTEXT,
    pinnedPodUids: persisted.pinnedPodUids ?? [],
    terminalSession: null,
    paletteOpen: false,
    helpOpen: false,
    selectedRowIndex: 0,
    toasts: [],
    habitatView: persisted.habitatView ?? "grid",
  };

  const listeners = new Set<Listener>();

  const get = (): WorkspaceState => state;

  const set = (patch: Partial<WorkspaceState>): void => {
    state = { ...state, ...patch };
    persist(state);
    for (const l of listeners) l();
  };

  const subscribe = (listener: Listener): (() => void) => {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  };

  // ----- side effects: load cluster + subscribe to live ticks -----
  let liveUnsub: (() => void) | null = null;
  const startLive = (seed: string): void => {
    if (liveUnsub) {
      liveUnsub();
      liveUnsub = null;
    }
    // initial one-shot load gives us something to render immediately
    void loadCluster(seed).then((c) => {
      if (state.selectedContext !== seed) return;
      // First-time auto-adopt: if the user has never pinned anything, pin
      // every pod whose mascot we have a sheet for (which is everything the
      // Go server actually returns — server-side already filters to fresh).
      // Filter out any persisted uids that no longer resolve in the new
      // cluster snapshot. This also drops the `undefined` entries from earlier
      // versions of the auto-pin that didn't see uids from the Go server.
      const valid = new Set(c.pods.map((p) => p.uid).filter((u): u is string => !!u));
      let pinnedPodUids = state.pinnedPodUids.filter((u) => valid.has(u));
      if (pinnedPodUids.length === 0) {
        // First-time auto-adopt: take every pod with a real uid.
        pinnedPodUids = c.pods.map((p) => p.uid).filter((u): u is string => !!u);
      }
      set({ cluster: c, pinnedPodUids });
    });
    liveUnsub = subscribeClusterUpdates(
      (next) => {
        // Guard against stale subscriptions if context changed mid-flight.
        if (state.selectedContext === seed) set({ cluster: next });
      },
      { initialSeed: seed },
    );
  };

  // Kick off the initial load only in the browser.
  if (typeof window !== "undefined") {
    startLive(state.selectedContext);
  }

  // Vite HMR: stop the subscription cleanly on hot reload during development.
  const hot = (import.meta as unknown as {
    hot?: { dispose: (cb: () => void) => void };
  }).hot;
  if (hot) {
    hot.dispose(() => {
      if (liveUnsub) {
        liveUnsub();
        liveUnsub = null;
      }
    });
  }

  // ---------------------------------------------------------------------
  // Actions
  // ---------------------------------------------------------------------

  const openTab = (kind: TabKind, ns?: string): void => {
    const id = tabIdFor(kind, ns);
    const existing = state.tabs.find((t) => t.id === id);
    if (existing) {
      set({ activeTabId: id, selectedRowIndex: 0 });
      return;
    }
    const title = ns ? `${TAB_TITLES[kind]} · ${ns}` : TAB_TITLES[kind];
    const tab: Tab = { id, kind, title, ns };
    set({ tabs: [...state.tabs, tab], activeTabId: id, selectedRowIndex: 0 });
  };

  const closeTab = (id: string): void => {
    if (state.tabs.length <= 1) return; // never close the last tab
    const idx = state.tabs.findIndex((t) => t.id === id);
    if (idx === -1) return;
    const tabs = state.tabs.filter((t) => t.id !== id);
    let activeTabId = state.activeTabId;
    if (state.activeTabId === id) {
      const nextIdx = Math.min(idx, tabs.length - 1);
      activeTabId = tabs[nextIdx].id;
    }
    set({ tabs, activeTabId });
  };

  const setActiveTab = (id: string): void => {
    if (state.tabs.some((t) => t.id === id)) {
      set({ activeTabId: id, selectedRowIndex: 0 });
    }
  };

  const selectResource = (uid: string | null, drawerTab?: string): void => {
    set({ selectedResourceUid: uid, drawerTab: drawerTab ?? null });
  };

  const toggleSidebar = (): void => {
    set({ sidebarOpen: !state.sidebarOpen });
  };

  const collapseSidebar = (): void => {
    set({ sidebarCollapsed: !state.sidebarCollapsed });
  };

  const setSearch = (s: string): void => {
    set({ search: s });
  };

  const setNamespace = (ns: string): void => {
    set({ selectedNamespace: ns });
  };

  const setContext = (ctx: string): void => {
    if (ctx === state.selectedContext) return;
    set({ selectedContext: ctx, cluster: null });
    startLive(ctx);
  };

  const toggleGroup = (groupId: string): void => {
    const current = state.sidebarGroups[groupId] ?? false;
    set({ sidebarGroups: { ...state.sidebarGroups, [groupId]: !current } });
  };

  // ----- Kubagachi pin actions -----

  const pinPod = (uid: string): void => {
    if (state.pinnedPodUids.includes(uid)) return;
    set({ pinnedPodUids: [...state.pinnedPodUids, uid] });
  };

  const unpinPod = (uid: string): void => {
    if (!state.pinnedPodUids.includes(uid)) return;
    set({ pinnedPodUids: state.pinnedPodUids.filter((x) => x !== uid) });
  };

  const togglePinPod = (uid: string): void => {
    if (state.pinnedPodUids.includes(uid)) unpinPod(uid);
    else pinPod(uid);
  };

  // ----- Terminal dock -----

  const openTerminal = (session: TerminalSession): void => {
    set({ terminalSession: session });
  };

  const closeTerminal = (): void => {
    set({ terminalSession: null });
  };

  // ----- Command palette / help overlay -----

  const setPaletteOpen = (open: boolean): void => {
    set({ paletteOpen: open });
  };

  const setHelpOpen = (open: boolean): void => {
    set({ helpOpen: open });
  };

  // ----- Row cursor (j/k navigation) -----

  const setSelectedRow = (index: number): void => {
    set({ selectedRowIndex: Math.max(0, index) });
  };

  const moveSelectedRow = (delta: number, max: number): void => {
    const next = Math.max(0, Math.min(max, state.selectedRowIndex + delta));
    if (next !== state.selectedRowIndex) set({ selectedRowIndex: next });
  };

  // ----- Habitat view (grid / ranch) -----

  const setHabitatView = (v: HabitatView): void => {
    if (v === state.habitatView) return;
    set({ habitatView: v });
  };

  const toggleHabitatView = (): void => {
    set({ habitatView: state.habitatView === "grid" ? "ranch" : "grid" });
  };

  // ----- Toasts (action feedback) -----

  let toastSeq = 0;

  const dismissToast = (id: number): void => {
    if (!state.toasts.some((t) => t.id === id)) return;
    set({ toasts: state.toasts.filter((t) => t.id !== id) });
  };

  /** Show a transient toast; auto-dismisses after ~3.8s. Returns its id. */
  const toast = (message: string, kind: Toast["kind"] = "info"): number => {
    const id = ++toastSeq;
    set({ toasts: [...state.toasts, { id, kind, message }] });
    if (typeof window !== "undefined") {
      window.setTimeout(() => dismissToast(id), 3800);
    }
    return id;
  };

  return {
    get,
    subscribe,
    actions: {
      openTab,
      closeTab,
      setActiveTab,
      selectResource,
      toggleSidebar,
      collapseSidebar,
      setSearch,
      setNamespace,
      setContext,
      toggleGroup,
      pinPod,
      unpinPod,
      togglePinPod,
      openTerminal,
      closeTerminal,
      setPaletteOpen,
      setHelpOpen,
      setSelectedRow,
      moveSelectedRow,
      setHabitatView,
      toggleHabitatView,
      toast,
      dismissToast,
    },
  };
}

const store = createStore();

export const workspaceActions = store.actions;

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

/**
 * Build a memo-stable hook that reads a slice of state via `selector`.
 *
 * We cache the last `(state, selected)` pair so that when an unrelated slice
 * updates we return the same reference and React skips the re-render.
 */
function makeHook<T>(selector: (s: WorkspaceState) => T) {
  let lastState: WorkspaceState | null = null;
  let lastValue: T;
  const getSnapshot = (): T => {
    const s = store.get();
    if (s !== lastState) {
      const next = selector(s);
      // shallow-equal arrays/objects so referentially-equal slices don't churn
      if (!shallowEqual(lastValue as unknown, next as unknown)) {
        lastValue = next;
      }
      lastState = s;
    }
    return lastValue;
  };
  // initialize so the first render has a stable reference
  lastValue = selector(store.get());
  lastState = store.get();
  return () => useSyncExternalStore(store.subscribe, getSnapshot, getSnapshot);
}

function shallowEqual(a: unknown, b: unknown): boolean {
  if (Object.is(a, b)) return true;
  if (a === null || b === null) return false;
  if (typeof a !== "object" || typeof b !== "object") return false;
  if (Array.isArray(a) && Array.isArray(b)) {
    if (a.length !== b.length) return false;
    for (let i = 0; i < a.length; i++) {
      if (!Object.is(a[i], b[i])) return false;
    }
    return true;
  }
  if (Array.isArray(a) !== Array.isArray(b)) return false;
  const ao = a as Record<string, unknown>;
  const bo = b as Record<string, unknown>;
  const ak = Object.keys(ao);
  const bk = Object.keys(bo);
  if (ak.length !== bk.length) return false;
  for (const k of ak) {
    if (!Object.is(ao[k], bo[k])) return false;
  }
  return true;
}

export const useCluster = makeHook((s) => s.cluster);
export const useTabs = makeHook((s) => s.tabs);
export const useActiveTab = makeHook((s) => s.activeTabId);
export const useSidebarState = makeHook((s) => ({
  open: s.sidebarOpen,
  collapsed: s.sidebarCollapsed,
  groups: s.sidebarGroups,
}));
export const useSelection = makeHook((s) => s.selectedResourceUid);
export const useDrawerTab = makeHook((s) => s.drawerTab);
export const useSearch = makeHook((s) => s.search);
export const useNamespace = makeHook((s) => s.selectedNamespace);
export const useContext = makeHook((s) => s.selectedContext);
export const usePinnedPodUids = makeHook((s) => s.pinnedPodUids);
export const useTerminalSession = makeHook((s) => s.terminalSession);
export const usePaletteOpen = makeHook((s) => s.paletteOpen);
export const useHelpOpen = makeHook((s) => s.helpOpen);
export const useSelectedRow = makeHook((s) => s.selectedRowIndex);
export const useToasts = makeHook((s) => s.toasts);
export const useHabitatView = makeHook((s) => s.habitatView);

// ---------------------------------------------------------------------------
// Pinned-pet hook
// ---------------------------------------------------------------------------

export interface PinnedPet {
  pod: Pod;
}

/**
 * Resolve currently-pinned uids against the live cluster snapshot.
 *
 * Pods that no longer exist in the cluster are dropped (no error, no zombie
 * cards).
 */
const usePinnedPetsHook = makeHook((s): PinnedPet[] => {
  const cluster = s.cluster;
  if (!cluster) return [];
  const byUid = new Map<string, Pod>();
  for (const p of cluster.pods) byUid.set(p.uid, p);
  const out: PinnedPet[] = [];
  for (const uid of s.pinnedPodUids) {
    const pod = byUid.get(uid);
    if (!pod) continue;
    out.push({ pod });
  }
  return out;
});

export function usePinnedPods(): PinnedPet[] {
  return usePinnedPetsHook();
}
