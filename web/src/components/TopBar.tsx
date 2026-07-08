/**
 * TopBar — k9s-style cluster info banner.
 *
 * Two rows of compact, dense info:
 *
 *   row 1: brand · context picker · cluster · version · spacer · search · gear · user
 *   row 2: namespace picker · pods · nodes · ns count · cpu · mem
 *
 * Visual goals (subtle TUI):
 *   - sharp 1px borders, no border-radius on chrome
 *   - bracketed labels like `[ ctx ]`, `[ ns ]`
 *   - colored k9s-style status glyphs leading numeric counters
 *
 * Controls are mouse-driven (Freelens-style); we keep the ⌘K shortcut for
 * search focus because that's a web convention, not a k9s vim binding.
 */

import { useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronDown,
  Menu,
  Pin,
  PinOff,
  Search,
  Settings,
  User,
} from "lucide-react";
import {
  useActiveTab,
  useCluster,
  useContext,
  useContexts,
  useNamespace,
  usePinnedContexts,
  useSearch,
  useTabs,
  workspaceActions,
} from "../store/workspace";
import { formatAge } from "../lib/format";
import type { ClusterContextInfo } from "../lib/cluster-api";

interface PinnedContextItem {
  name: string;
  cluster?: string;
  namespace?: string;
}

export default function TopBar() {
  const cluster = useCluster();
  const ctx = useContext();
  const contexts = useContexts();
  const pinnedContexts = usePinnedContexts();
  const ns = useNamespace();
  const search = useSearch();
  const tabs = useTabs();
  const activeTabId = useActiveTab();
  // On the habitat home the resource Sidebar is an overlay (it isn't docked),
  // so the nav toggle must be reachable on desktop too — not just mobile.
  const habitat = tabs.find((t) => t.id === activeTabId)?.kind === "overview";

  const [searchFocused, setSearchFocused] = useState(false);
  const [ctxOpen, setCtxOpen] = useState(false);
  const [nsOpen, setNsOpen] = useState(false);
  const [userOpen, setUserOpen] = useState(false);
  const [switchingCtx, setSwitchingCtx] = useState<string | null>(null);
  const [ctxError, setCtxError] = useState<string | null>(null);

  const searchRef = useRef<HTMLInputElement | null>(null);

  // ⌘K / Ctrl-K focuses search; Escape clears + blurs.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        searchRef.current?.focus();
        searchRef.current?.select();
      } else if (e.key === "Escape" && document.activeElement === searchRef.current) {
        workspaceActions.setSearch("");
        searchRef.current?.blur();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const stats = useMemo(() => deriveStats(cluster), [cluster]);

  // Honesty: a "live" cluster (real server / in-cluster) vs a mock/demo snapshot.
  const isLive = cluster?.mode === "live" || cluster?.mode === "cluster";
  const currentCtx = cluster?.context || ctx || "unknown";
  const sessionIdentity = isLive ? "—" : cluster?.mode === "demo" ? "demo" : "mock";
  const pinnedContextItems = useMemo((): PinnedContextItem[] => {
    const byName = new Map(contexts.map((c) => [c.name, c]));
    return pinnedContexts.map((name) => {
      const info = byName.get(name);
      return { name, cluster: info?.cluster, namespace: info?.namespace };
    });
  }, [contexts, pinnedContexts]);

  useEffect(() => {
    void workspaceActions.refreshContexts();
  }, []);

  // Real "last refresh": track the wall-clock time the cluster snapshot
  // *reference* last changed, then re-render every second to show "Xs ago".
  const refreshAtRef = useRef<number>(Date.now());
  const lastSnapshotRef = useRef<typeof cluster>(cluster);
  if (lastSnapshotRef.current !== cluster) {
    lastSnapshotRef.current = cluster;
    refreshAtRef.current = Date.now();
  }
  const [, forceTick] = useState(0);
  useEffect(() => {
    const id = window.setInterval(() => forceTick((n) => n + 1), 1000);
    return () => window.clearInterval(id);
  }, []);
  const refreshAgeSec = Math.max(0, Math.floor((Date.now() - refreshAtRef.current) / 1000));

  const switchContext = (name: string, closeDropdown: boolean): void => {
    if (name === currentCtx || switchingCtx) return;
    setSwitchingCtx(name);
    setCtxError(null);
    void workspaceActions.setContext(name)
      .then(() => {
        if (closeDropdown) setCtxOpen(false);
      })
      .catch((err: unknown) => {
        setCtxError(err instanceof Error ? err.message : "context switch failed");
      })
      .finally(() => setSwitchingCtx(null));
  };

  return (
    <div className="sticky top-0 z-30 bg-bg-panel border-b border-border text-text">
      {/* Row 1: brand + context + version + search + utility icons */}
      <div className="h-9 flex items-center gap-2 px-2 sm:px-3 border-b border-border/60">
        {/* Hamburger (mobile only) */}
        <button
          type="button"
          aria-label="Toggle navigation"
          title="Resources (toggle sidebar)"
          onClick={workspaceActions.toggleSidebar}
          className={`${habitat ? "" : "md:hidden"} inline-flex items-center justify-center h-7 w-7 hover:bg-bg-panel2 text-text-muted hover:text-text transition-colors duration-100 k9s-square`}
        >
          <Menu size={15} />
        </button>

        {/* Brand */}
        <div className="hidden md:flex items-baseline gap-2 pr-3 mr-1 border-r border-border/60 h-7 pl-1">
          <span className="font-serif text-accent-bright text-[16px] leading-none tracking-tight self-center drop-shadow-[0_0_6px_rgba(201,184,138,0.25)]">kubagachi</span>
          <span className="text-accent/60 text-[9px] uppercase tracking-[0.24em]">yscale</span>
        </div>

        {/* Cluster context picker */}
        <Field label="ctx">
          <Dropdown
            open={ctxOpen}
            onOpenChange={setCtxOpen}
            trigger={
              <span className="inline-flex items-center gap-1 bg-accent-dim text-accent border border-accent-soft px-1.5 h-[18px] k9s-square hover:border-accent transition-colors">
                <span className="font-medium leading-none">{currentCtx}</span>
                <ChevronDown size={11} className="opacity-70" />
              </span>
            }
          >
            <DropdownHeader>Switch context</DropdownHeader>
            {contexts.length === 0 && (
              <div className="px-3 py-1.5 text-[11px] font-mono text-text-muted">
                contexts unavailable
              </div>
            )}
            {contexts.map((c) => (
              <ContextDropdownRow
                key={c.name}
                context={c}
                active={c.name === currentCtx}
                pinned={pinnedContexts.includes(c.name)}
                switching={switchingCtx === c.name}
                onSwitch={() => switchContext(c.name, true)}
                onTogglePin={() => workspaceActions.togglePinnedContext(c.name)}
              />
            ))}
            {ctxError && (
              <div className="mx-2 mt-1 border-t border-border px-1 pt-1 text-[11px] font-mono text-status-error">
                {ctxError}
              </div>
            )}
          </Dropdown>
        </Field>

        <Sep />

        <Field label="ver">
          <span className="text-text">{cluster?.version ?? "…"}</span>
        </Field>

        <Sep />

        <Field label="api">
          {isLive ? (
            <span className="inline-flex items-center gap-1" title="Connected to a real cluster API">
              <span className="text-status-running k9s-glyph">◉</span>
              <span className="text-status-running text-[11px]">live</span>
            </span>
          ) : (
            <span className="inline-flex items-center gap-1" title="No live cluster — showing mock / demo data">
              <span className="text-status-backoff k9s-glyph">◌</span>
              <span className="text-status-backoff/90 text-[11px]">mock</span>
            </span>
          )}
        </Field>

        {/* Spacer */}
        <div className="flex-1" />

        {/* Global search */}
        <div
          className={
            "relative flex items-center h-7 border border-border bg-bg-base/60 transition-all duration-150 k9s-square " +
            (searchFocused ? "w-80 border-accent/70" : "w-44 sm:w-56")
          }
        >
          <Search size={12} className="absolute left-2 text-text-muted pointer-events-none" />
          <input
            ref={searchRef}
            data-global-search
            type="text"
            value={search}
            placeholder="search resources…"
            onChange={(e) => workspaceActions.setSearch(e.target.value)}
            onFocus={() => setSearchFocused(true)}
            onBlur={() => setSearchFocused(false)}
            className="w-full h-full bg-transparent pl-7 pr-12 text-[12px] text-text placeholder:text-text-muted/60 outline-none font-mono"
          />
          <kbd className="absolute right-2 hidden sm:inline-flex items-center gap-0.5 text-[10px] text-text-muted/80 font-mono select-none">
            <span className="px-1 py-px border border-border/80 bg-bg-panel2 k9s-square">⌘</span>
            <span className="px-1 py-px border border-border/80 bg-bg-panel2 k9s-square">K</span>
          </kbd>
        </div>

        <Clock />

        <button
          type="button"
          aria-label="Keybindings & help"
          title="Keybindings & help"
          onClick={() => workspaceActions.setHelpOpen(true)}
          className="inline-flex items-center justify-center h-7 w-7 hover:bg-bg-panel2 text-text-muted hover:text-accent transition-colors duration-100 k9s-square"
        >
          <Settings size={14} />
        </button>

        {/* Account — small dropdown surfacing the active ctx/ns and user. */}
        <Dropdown
          open={userOpen}
          onOpenChange={setUserOpen}
          align="right"
          triggerLabel="Account"
          trigger={
            <span
              title="Account"
              className="inline-flex items-center justify-center h-7 w-7 hover:bg-bg-panel2 text-text-muted hover:text-accent transition-colors duration-100 k9s-square"
            >
              <User size={14} />
            </span>
          }
        >
          <DropdownHeader>Session</DropdownHeader>
          <div className="px-3 py-1.5 text-[12px] font-mono flex items-center gap-2">
            <span className="k9s-glyph text-accent">▸</span>
            <span className="flex-1 text-text truncate">{sessionIdentity}</span>
          </div>
          <div className="my-1 mx-2 h-px bg-border" />
          <div className="px-3 py-1 text-[11px] font-mono flex items-center justify-between gap-3">
            <span className="text-text-muted uppercase tracking-wider text-[10px]">ctx</span>
            <span className="text-accent truncate">{currentCtx}</span>
          </div>
          <div className="px-3 py-1 text-[11px] font-mono flex items-center justify-between gap-3">
            <span className="text-text-muted uppercase tracking-wider text-[10px]">ns</span>
            <span className="text-accent truncate">{ns === "all" ? "all" : ns}</span>
          </div>
        </Dropdown>
      </div>

      {/* Row 2: namespace picker + live counters (k9s top-strip vibe) */}
      <div className="h-8 flex items-center gap-3 px-2 sm:px-3 text-[11px] tabular-nums overflow-x-auto scrollbar-thin">
        {/* Namespace */}
        <Field label="ns">
          <Dropdown
            open={nsOpen}
            onOpenChange={setNsOpen}
            trigger={
              <span className="inline-flex items-center gap-1 bg-accent-dim text-accent border border-accent-soft px-1.5 h-[18px] k9s-square hover:border-accent transition-colors">
                <span className="font-medium leading-none">{ns === "all" ? "all" : ns}</span>
                <ChevronDown size={11} className="opacity-70" />
              </span>
            }
          >
            <DropdownHeader>Namespaces</DropdownHeader>
            <DropdownItem
              active={ns === "all"}
              onClick={() => {
                workspaceActions.setNamespace("all");
                setNsOpen(false);
              }}
            >
              <span className="k9s-glyph text-accent">▸</span>
              <span className="flex-1">all</span>
            </DropdownItem>
            <div className="my-1 mx-2 h-px bg-border" />
            {(cluster?.namespaces ?? []).map((n) => (
              <DropdownItem
                key={n.uid}
                active={n.name === ns}
                onClick={() => {
                  workspaceActions.setNamespace(n.name);
                  setNsOpen(false);
                }}
              >
                <span className="k9s-glyph text-text-muted">·</span>
                <span className="flex-1">{n.name}</span>
                <span className="text-[10px] text-text-muted">{n.phase}</span>
              </DropdownItem>
            ))}
            </Dropdown>
          </Field>

        {pinnedContextItems.length >= 2 && (
          <>
            <Sep />
            <PinnedContextStrip
              items={pinnedContextItems}
              currentCtx={currentCtx}
              switchingCtx={switchingCtx}
              onSwitch={(name) => switchContext(name, false)}
            />
          </>
        )}

        <Sep />

        <Counter
          label="pods"
          value={`${stats.podsReady}/${stats.podsTotal}`}
          glyph={stats.podsHealthy ? "◉" : "▼"}
          glyphClass={stats.podsHealthy ? "text-status-running" : "text-status-backoff"}
        />
        <Counter
          label="nodes"
          value={`${stats.nodesReady}/${stats.nodesTotal}`}
          glyph={stats.nodesReady === stats.nodesTotal && stats.nodesTotal > 0 ? "◉" : "▼"}
          glyphClass={
            stats.nodesReady === stats.nodesTotal && stats.nodesTotal > 0
              ? "text-status-running"
              : "text-status-backoff"
          }
        />
        <Counter
          label="ns"
          value={`${stats.namespaces}`}
          glyph="◇"
          glyphClass="text-text-muted"
        />
        <Counter
          label="cpu"
          value={stats.cpuPct >= 0 ? `${stats.cpuPct}%` : "—"}
          glyph={stats.cpuPct > 80 ? "▲" : "◉"}
          glyphClass={stats.cpuPct > 80 ? "text-status-backoff" : "text-status-running"}
        />
        <Counter
          label="mem"
          value={stats.memDisplay}
          glyph={stats.memPct > 80 ? "▲" : "◉"}
          glyphClass={stats.memPct > 80 ? "text-status-backoff" : "text-status-running"}
        />

        <div className="ml-auto text-text-muted/70 hidden md:inline" title="Time since the cluster snapshot last changed">
          <span className="opacity-60">─</span> last refresh{" "}
          <span className="text-text tabular-nums">
            {refreshAgeSec < 2 ? "now" : `${formatAge(refreshAgeSec)} ago`}
          </span>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Mock-derived live stats — deterministic from the current cluster snapshot.
// ---------------------------------------------------------------------------

interface Stats {
  podsTotal: number;
  podsReady: number;
  podsHealthy: boolean;
  nodesTotal: number;
  nodesReady: number;
  namespaces: number;
  cpuPct: number;
  memPct: number;
  memDisplay: string;
}

function deriveStats(cluster: ReturnType<typeof useCluster>): Stats {
  if (!cluster) {
    return {
      podsTotal: 0,
      podsReady: 0,
      podsHealthy: true,
      nodesTotal: 0,
      nodesReady: 0,
      namespaces: 0,
      cpuPct: 0,
      memPct: 0,
      memDisplay: "—",
    };
  }
  const podsTotal = cluster.pods.length;
  const podsReady = cluster.pods.filter((p) => p.status === "running" || p.status === "completed").length;
  const nodesTotal = cluster.nodes.length;
  const nodesReady = cluster.nodes.filter((n) => n.status === "ready").length;
  // Real cluster-wide utilisation: average of node percentages from
  // metrics-server. -1 (no metrics) is excluded.
  const cpuPct = avgPct(cluster.nodes.map((n) => n.cpuPct ?? -1));
  const memPct = avgPct(cluster.nodes.map((n) => n.memPct ?? -1));
  return {
    podsTotal,
    podsReady,
    podsHealthy: podsReady === podsTotal,
    nodesTotal,
    nodesReady,
    namespaces: cluster.namespaces.length,
    cpuPct,
    memPct,
    memDisplay: memPct >= 0 ? `${memPct}%` : "—",
  };
}

function avgPct(values: number[]): number {
  const known = values.filter((v) => v >= 0);
  if (known.length === 0) return -1;
  return Math.round(known.reduce((a, b) => a + b, 0) / known.length);
}

// Clock shows a live wall-clock time, matching the mockup's TIME field.
function Clock() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), 1000);
    return () => window.clearInterval(id);
  }, []);
  const hh = String(now.getHours()).padStart(2, "0");
  const mm = String(now.getMinutes()).padStart(2, "0");
  const ss = String(now.getSeconds()).padStart(2, "0");
  return (
    <span className="hidden sm:inline-flex items-center gap-1.5 px-2 text-[11px] whitespace-nowrap border-l border-border/60 h-7 ml-1">
      <span className="text-text-muted/70 uppercase tracking-wider">time</span>
      <span className="text-text tabular-nums">{`${hh}:${mm}:${ss}`}</span>
    </span>
  );
}

// ---------------------------------------------------------------------------
// Local UI helpers
// ---------------------------------------------------------------------------

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-[11px] whitespace-nowrap">
      <span className="text-text-muted/70 uppercase tracking-wider">{label}:</span>
      {children}
    </span>
  );
}

function Sep() {
  return <span className="text-border select-none" aria-hidden="true">│</span>;
}

function Counter({
  label,
  value,
  glyph,
  glyphClass,
}: {
  label: string;
  value: string;
  glyph: string;
  glyphClass: string;
}) {
  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
      <span className="text-text-muted/70 uppercase tracking-wider">{label}</span>
      <span className={`k9s-glyph ${glyphClass}`}>{glyph}</span>
      <span className="text-text font-medium">{value}</span>
    </span>
  );
}

function ContextDropdownRow({
  context,
  active,
  pinned,
  switching,
  onSwitch,
  onTogglePin,
}: {
  context: ClusterContextInfo;
  active: boolean;
  pinned: boolean;
  switching: boolean;
  onSwitch: () => void;
  onTogglePin: () => void;
}) {
  const pinLabel = pinned ? `Unpin ${context.name}` : `Pin ${context.name}`;
  return (
    <div
      role="none"
      className={
        "w-full flex items-stretch border-l-2 text-[12px] font-mono transition-colors duration-100 " +
        (active
          ? "bg-bg-panel2 text-text border-accent"
          : "border-transparent text-text-muted hover:bg-bg-panel2 hover:text-text")
      }
    >
      <button
        type="button"
        role="menuitem"
        aria-current={active ? "true" : undefined}
        onClick={onSwitch}
        className="min-w-0 flex flex-1 items-center gap-2 py-1.5 pl-2 pr-1 text-left"
      >
        <span className={`k9s-glyph ${active ? "text-accent" : "text-text-muted"}`}>
          {active ? "▸" : "·"}
        </span>
        <span className="min-w-0 flex-1 truncate">{context.name}</span>
        {context.namespace && (
          <span className="max-w-[90px] truncate text-[10px] text-text-muted">
            {context.namespace}
          </span>
        )}
        {active && <span className="text-[10px] text-text-muted">current</span>}
        {switching && <span className="text-[10px] text-accent">switching</span>}
      </button>
      <button
        type="button"
        aria-label={pinLabel}
        aria-pressed={pinned}
        title={pinLabel}
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          onTogglePin();
        }}
        className={
          "mr-2 my-1 inline-flex h-6 w-6 shrink-0 items-center justify-center border transition-colors duration-100 k9s-square " +
          (pinned
            ? "border-accent/60 bg-accent-dim text-accent hover:border-status-error/70 hover:bg-status-error/10 hover:text-status-error"
            : "border-border/70 text-text-muted hover:border-accent-soft hover:bg-accent/10 hover:text-accent")
        }
      >
        {pinned ? <PinOff size={12} /> : <Pin size={12} />}
      </button>
    </div>
  );
}

function PinnedContextStrip({
  items,
  currentCtx,
  switchingCtx,
  onSwitch,
}: {
  items: PinnedContextItem[];
  currentCtx: string;
  switchingCtx: string | null;
  onSwitch: (name: string) => void;
}) {
  return (
    <div aria-label="Pinned contexts" className="inline-flex shrink-0 items-center gap-1.5">
      <span
        title="Pinned contexts"
        className="inline-flex h-5 w-5 shrink-0 items-center justify-center border border-accent-soft bg-accent-dim text-accent k9s-square"
      >
        <Pin size={11} aria-hidden="true" />
      </span>
      <div className="inline-flex items-center gap-1.5">
        {items.map((item) => {
          const active = item.name === currentCtx;
          const switching = switchingCtx === item.name;
          const title = [
            item.name,
            item.cluster && item.cluster !== item.name ? item.cluster : null,
            item.namespace ? `ns ${item.namespace}` : null,
          ].filter(Boolean).join(" • ");
          return (
            <button
              key={item.name}
              type="button"
              title={title}
              aria-current={active ? "true" : undefined}
              aria-busy={switching ? "true" : undefined}
              aria-disabled={active || Boolean(switchingCtx)}
              onClick={() => onSwitch(item.name)}
              className={
                "group relative inline-flex h-6 min-w-[112px] max-w-[220px] items-center gap-1.5 overflow-hidden border px-2 text-[11px] font-mono transition-all duration-150 k9s-square " +
                (active
                  ? "border-accent bg-[linear-gradient(90deg,rgba(201,184,138,0.28),rgba(93,184,232,0.12))] text-text font-semibold shadow-[0_0_18px_-8px_rgba(201,184,138,0.95)]"
                  : "border-border bg-bg-base/45 text-text-muted hover:border-accent-soft hover:bg-bg-panel2 hover:text-text")
              }
            >
              {active && (
                <span className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_16%_50%,rgba(201,184,138,0.24),transparent_36%)] opacity-90" />
              )}
              <span
                className={
                  "relative z-10 k9s-glyph " +
                  (switching
                    ? "text-accent-bright animate-spin"
                    : active
                      ? "text-accent-bright kubagachi-pip"
                      : "text-text-muted")
                }
              >
                {switching ? "↻" : active ? "◉" : "◇"}
              </span>
              <span className="relative z-10 min-w-0 flex-1 truncate text-left">
                {item.name}
              </span>
              {item.namespace && (
                <span
                  className={
                    "relative z-10 max-w-[64px] truncate text-[9px] uppercase tracking-wider " +
                    (active ? "text-accent/90" : "text-text-muted/80")
                  }
                >
                  {item.namespace}
                </span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}

interface DropdownProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  trigger: React.ReactNode;
  children: React.ReactNode;
  /** Which edge the menu aligns to. Defaults to "left". */
  align?: "left" | "right";
  /** Accessible name for icon-only triggers (the visible text otherwise names it). */
  triggerLabel?: string;
}

function Dropdown({ open, onOpenChange, trigger, children, align = "left", triggerLabel }: DropdownProps) {
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (!rootRef.current) return;
      if (!rootRef.current.contains(e.target as Node)) onOpenChange(false);
    };
    const onEsc = (e: KeyboardEvent) => {
      if (e.key === "Escape") onOpenChange(false);
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onEsc);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onEsc);
    };
  }, [open, onOpenChange]);

  return (
    <div ref={rootRef} className="relative inline-flex">
      <button
        type="button"
        onClick={() => onOpenChange(!open)}
        aria-label={triggerLabel}
        aria-haspopup="menu"
        aria-expanded={open}
        className="inline-flex items-center"
      >
        {trigger}
      </button>
      {open && (
        <div
          role="menu"
          className={
            "absolute top-[calc(100%+4px)] z-40 min-w-[220px] border border-border bg-bg-panel shadow-lg shadow-black/40 py-1 max-h-[60vh] overflow-y-auto scrollbar-thin k9s-square " +
            (align === "right" ? "right-0" : "left-0")
          }
        >
          {children}
        </div>
      )}
    </div>
  );
}

function DropdownHeader({ children }: { children: React.ReactNode }) {
  return (
    <div className="px-3 py-1 text-[10px] uppercase tracking-wider text-text-muted">
      {children}
    </div>
  );
}

interface DropdownItemProps {
  active?: boolean;
  onClick: () => void;
  children: React.ReactNode;
}

function DropdownItem({ active, onClick, children }: DropdownItemProps) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className={
        "w-full flex items-center gap-2 px-3 py-1.5 text-[12px] text-left transition-colors duration-100 font-mono " +
        (active
          ? "bg-bg-panel2 text-text border-l-2 border-accent pl-[10px]"
          : "text-text-muted hover:bg-bg-panel2 hover:text-text")
      }
    >
      {children}
    </button>
  );
}
