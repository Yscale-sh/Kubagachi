/**
 * PodList — Freelens-inspired pod table.
 *
 * Behavior:
 *   - Unhealthy pods auto-sort to the top.
 *   - Unhealthy rows expand inline with their critter mascot, a human
 *     explanation of the status, and quick-action buttons. Healthy pods stay
 *     one line.
 *   - Click row → toggle manual expand override.
 *   - Double-click row → open the detail drawer.
 *   - A tiny inline mascot accompanies each row's name.
 *   - j/k/g/G/Enter keyboard row-nav (driven by the global KeyboardLayer via
 *     the row-nav registry). Enter opens the detail drawer for the cursor row.
 *   - Expanded-row actions (Shell / Logs / Pin / Describe / Restart / Delete)
 *     are wired to real workspace + cluster-api calls. Destructive actions are
 *     two-step confirmed and, in demo/mock mode, report an honest no-op.
 *
 * Expected workspace hooks:
 *   useCluster()   -> Cluster | null
 *   useNamespace() -> string
 *   useSearch()    -> string
 *   useSelectedRow() -> number   (keyboard cursor index)
 *   workspaceActions.selectResource(uid, drawerTab?)
 *   workspaceActions.openTerminal({ namespace, pod, container })
 *   workspaceActions.toast(message, kind?)
 */

import { useEffect, useMemo, useState } from "react";
import {
  Inbox,
  Pin,
  PinOff,
  RefreshCw,
  ScrollText,
  Search as SearchIcon,
  Terminal as TerminalIcon,
  Trash2,
} from "lucide-react";
import type { Pod, PodStatus } from "../lib/types";
import { formatAge, humanStatus } from "../lib/format";
import {
  useCluster,
  useNamespace,
  usePinnedPodUids,
  useSearch,
  useSelectedRow,
  workspaceActions,
} from "../store/workspace";
import { deletePod } from "../lib/cluster-api";
import { clearRowNav, registerRowNav } from "../lib/row-nav";
import StatusPill from "./StatusPill";
import CritterPlayer from "./CritterPlayer";
import ConfirmButton from "./ConfirmButton";

const UNHEALTHY: ReadonlySet<PodStatus> = new Set<PodStatus>([
  "crashloop",
  "backoff",
  "error",
  "unknown",
  "pending",
  "terminating",
]);

function isUnhealthy(p: Pod): boolean {
  return UNHEALTHY.has(p.status);
}

/** Demo / local-mock clusters have no real backend — gate writes for honesty. */
function isMockMode(mode: string | undefined): boolean {
  return mode === "demo" || mode === "mock";
}

const STATUS_COLOR: Record<PodStatus, string> = {
  running: "border-status-running",
  pending: "border-status-pending",
  completed: "border-status-completed",
  crashloop: "border-status-crashloop",
  backoff: "border-status-backoff",
  terminating: "border-status-terminating",
  unknown: "border-status-unknown",
  error: "border-status-error",
};

function statusExplanation(status: PodStatus): string {
  switch (status) {
    case "running":
      return "All containers ready and reporting healthy.";
    case "pending":
      return "Pod accepted but a container hasn't been created yet — usually waiting for a scheduler decision.";
    case "completed":
      return "All containers exited cleanly. Pod is done.";
    case "error":
      return "At least one container terminated with a non-zero exit code.";
    case "unknown":
      return "The kubelet couldn't be reached to confirm pod state.";
    case "crashloop":
      return "A container keeps crashing — the kubelet is backing off restarts.";
    case "backoff":
      return "The container image couldn't be pulled (auth, network, or not found).";
    case "terminating":
      return "Pod received a delete and is shutting down its containers.";
  }
}

export default function PodList() {
  const cluster = useCluster();
  const namespace = useNamespace();
  const search = useSearch();
  const pinnedUids = usePinnedPodUids();
  const selectedRow = useSelectedRow();
  const pinnedSet = useMemo(() => new Set(pinnedUids), [pinnedUids]);
  const [overrides, setOverrides] = useState<Record<string, boolean>>({});

  const mock = isMockMode(cluster?.mode);

  const pods = useMemo<Pod[]>(() => {
    if (!cluster) return [];
    const q = search.trim().toLowerCase();
    const filtered = cluster.pods.filter((p) => {
      if (namespace && namespace !== "all" && p.namespace !== namespace) return false;
      if (q && !p.name.toLowerCase().includes(q)) return false;
      return true;
    });
    // Sort: unhealthy first, then by namespace + name for stability.
    return [...filtered].sort((a, b) => {
      const au = isUnhealthy(a) ? 0 : 1;
      const bu = isUnhealthy(b) ? 0 : 1;
      if (au !== bu) return au - bu;
      const ns = (a.namespace ?? "").localeCompare(b.namespace ?? "");
      if (ns !== 0) return ns;
      return a.name.localeCompare(b.name);
    });
  }, [cluster, namespace, search]);

  // Register the visible rows with the global keyboard layer (j/k/g/G/Enter).
  // Keyed on the joined uid list so it re-registers when the view changes.
  const rowIdsKey = pods.map((p) => p.uid).join("|");
  useEffect(() => {
    const reg = {
      ids: pods.map((p) => p.uid),
      onEnter: (uid: string) => workspaceActions.selectResource(uid),
    };
    registerRowNav(reg);
    return () => clearRowNav(reg);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rowIdsKey]);

  if (!cluster) {
    return <div className="p-6 text-text-muted text-sm">Loading cluster…</div>;
  }
  if (pods.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-text-muted gap-2">
        <Inbox className="w-6 h-6 opacity-50" />
        <div className="text-xs">No pods found in the selected scope.</div>
      </div>
    );
  }

  const toggleOverride = (uid: string) => {
    setOverrides((prev) => ({ ...prev, [uid]: !prev[uid] }));
  };

  // --- Real action handlers shared by row + expanded panel --------------------
  const openShell = (p: Pod) => {
    workspaceActions.openTerminal({
      namespace: p.namespace ?? "",
      pod: p.name,
      container: p.containers[0]?.name ?? "",
    });
  };

  const openLogs = (p: Pod) => {
    workspaceActions.selectResource(p.uid, "logs");
  };

  const openDescribe = (p: Pod) => {
    workspaceActions.selectResource(p.uid);
  };

  const runDelete = async (p: Pod) => {
    if (mock) {
      workspaceActions.toast(
        `demo mode: delete ${p.name} is a no-op (no live cluster)`,
        "info",
      );
      return;
    }
    workspaceActions.toast(`deleting ${p.name}…`, "info");
    const res = await deletePod(p.namespace ?? "", p.name);
    if (res.ok) {
      workspaceActions.toast(`deleted ${p.name}`, "success");
    } else {
      workspaceActions.toast(
        `delete failed: ${res.error ?? "unknown error"}`,
        "error",
      );
    }
  };

  const total = pods.length;
  const unhealthyCount = pods.filter(isUnhealthy).length;

  return (
    <>
      <div className="hidden sm:block font-mono">
        <table className="w-full text-[12px] border-separate border-spacing-0">
          <thead className="sticky top-0 z-10 bg-bg-panel">
            <tr>
              <th className="w-7 px-1 py-1.5 border-b border-border" />
              <th className="w-9 px-1 py-1.5 border-b border-border" />
              <th className="text-left font-normal text-text-muted/80 uppercase tracking-wider text-[10px] px-2 py-1.5 border-b border-border w-20">
                Status
              </th>
              <th className="text-left font-normal text-text-muted/80 uppercase tracking-wider text-[10px] px-3 py-1.5 border-b border-border min-w-[200px]">
                Name
              </th>
              <th className="text-left font-normal text-text-muted/80 uppercase tracking-wider text-[10px] px-3 py-1.5 border-b border-border w-32">
                Namespace
              </th>
              <th className="text-right font-normal text-text-muted/80 uppercase tracking-wider text-[10px] px-2 py-1.5 border-b border-border w-16">
                Rdy
              </th>
              <th className="text-right font-normal text-text-muted/80 uppercase tracking-wider text-[10px] px-2 py-1.5 border-b border-border w-16">
                Rst
              </th>
              <th className="text-right font-normal text-text-muted/80 uppercase tracking-wider text-[10px] px-2 py-1.5 border-b border-border w-16">
                Age
              </th>
              <th className="text-left font-normal text-text-muted/80 uppercase tracking-wider text-[10px] px-3 py-1.5 border-b border-border w-36">
                Node
              </th>
              <th className="text-left font-normal text-text-muted/80 uppercase tracking-wider text-[10px] px-3 py-1.5 border-b border-border w-32">
                IP
              </th>
            </tr>
          </thead>
          <tbody>
            {pods.map((p, i) => {
              const unhealthy = isUnhealthy(p);
              const override = overrides[p.uid];
              // Default-expand unhealthy pods; let clicks invert that.
              const defaultExpanded = unhealthy;
              const expanded = override === undefined ? defaultExpanded : !defaultExpanded;
              return (
                <PodRow
                  key={p.uid}
                  pod={p}
                  expanded={expanded}
                  pinned={pinnedSet.has(p.uid)}
                  selected={i === selectedRow}
                  mock={mock}
                  onToggle={() => toggleOverride(p.uid)}
                  onOpenDrawer={() => workspaceActions.selectResource(p.uid)}
                  onShell={() => openShell(p)}
                  onLogs={() => openLogs(p)}
                  onDescribe={() => openDescribe(p)}
                  onDelete={() => runDelete(p)}
                />
              );
            })}
          </tbody>
        </table>
        <KeyHintBar total={total} unhealthy={unhealthyCount} />
      </div>

      {/* Mobile: card stack */}
      <div className="sm:hidden flex flex-col gap-2 p-2 font-mono">
        {pods.map((p) => {
          const unhealthy = isUnhealthy(p);
          const pinned = pinnedSet.has(p.uid);
          return (
            <div
              key={p.uid}
              className={
                `text-left bg-bg-panel border-l-2 ${STATUS_COLOR[p.status]} border-y border-r border-border p-3 flex gap-3 items-center k9s-square ` +
                (pinned ? "ring-1 ring-accent/40" : "")
              }
            >
              <button
                type="button"
                onClick={() => workspaceActions.selectResource(p.uid)}
                className="flex-1 flex items-center gap-3 text-left"
              >
                <div className="w-12 h-12 shrink-0 bg-bg-panel2 k9s-square">
                  <CritterPlayer critter={p.critter} status={p.critterState ?? p.status} fps={6} />
                </div>
                <div className="flex-1 min-w-0">
                  <div className="text-[13px] font-medium text-text truncate">{p.name}</div>
                  <div className="text-[10px] text-text-muted truncate">{p.namespace} · {p.node}</div>
                  <div className="mt-1 flex items-center gap-2">
                    <StatusPill status={p.status} />
                    {unhealthy && p.restartCount > 0 && (
                      <span className="text-[10px] text-status-crashloop">{p.restartCount}rst</span>
                    )}
                  </div>
                </div>
              </button>
              <PinToggle pinned={pinned} uid={p.uid} />
            </div>
          );
        })}
        <KeyHintBar total={total} unhealthy={unhealthyCount} />
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// k9s-style footer: pod counts + the navigation keys that actually work.
// The global KeyboardLayer owns j/k/g/G/Enter against the row-nav registry,
// so those are the only keycaps we advertise — no inert shortcuts.
// ---------------------------------------------------------------------------

function KeyHintBar({ total, unhealthy }: { total: number; unhealthy: number }) {
  const hints: { keys: string; label: string }[] = [
    { keys: "j / k", label: "move" },
    { keys: "g / G", label: "top / bottom" },
    { keys: "↵", label: "open" },
  ];
  return (
    <div className="sticky bottom-0 z-10 bg-bg-panel border-t border-border px-3 py-1.5 flex items-center gap-3 text-[11px] font-mono text-text-muted overflow-x-auto scrollbar-thin">
      <span className="whitespace-nowrap">
        <span className="text-text-muted/70 uppercase tracking-wider">pods:</span>{" "}
        <span className="text-text tabular-nums">{total}</span>
        {unhealthy > 0 && (
          <>
            <span className="opacity-50 mx-2">·</span>
            <span className="text-status-crashloop k9s-glyph">◆</span>{" "}
            <span className="text-status-crashloop tabular-nums">{unhealthy}</span>{" "}
            <span className="text-text-muted/70">unhealthy</span>
          </>
        )}
      </span>
      <span className="text-border select-none" aria-hidden="true">│</span>
      {hints.map((h) => (
        <span
          key={h.keys}
          className="inline-flex items-center gap-1 px-1.5 py-0.5 whitespace-nowrap"
        >
          <kbd className="px-1 border border-border/80 bg-bg-base text-[10px] text-text k9s-square">
            {h.keys}
          </kbd>
          <span>{h.label}</span>
        </span>
      ))}
      <span className="ml-auto text-text-muted/60 whitespace-nowrap hidden md:inline">
        click row to expand · double-click for drawer
      </span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pin toggle
// ---------------------------------------------------------------------------

function PinToggle({
  pinned,
  uid,
  size = "sm",
}: {
  pinned: boolean;
  uid: string;
  size?: "sm" | "md";
}) {
  const Icon = pinned ? PinOff : Pin;
  const dim = size === "md" ? "h-7 w-7" : "h-6 w-6";
  const iconSize = size === "md" ? "w-3.5 h-3.5" : "w-3 h-3";
  return (
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        workspaceActions.togglePinPod(uid);
      }}
      aria-label={pinned ? "Unpin pod from Kubagachi" : "Pin pod to Kubagachi"}
      title={pinned ? "Unpin" : "Pin"}
      className={
        `inline-flex items-center justify-center ${dim} rounded transition-colors ` +
        (pinned
          ? "text-accent hover:bg-accent/10 hover:text-accent"
          : "text-text-muted hover:text-accent hover:bg-bg-panel2")
      }
    >
      <Icon className={iconSize} />
    </button>
  );
}

// ---------------------------------------------------------------------------
// Single pod row (one line, optional expanded panel below)
// ---------------------------------------------------------------------------

function PodRow({
  pod,
  expanded,
  pinned,
  selected,
  mock,
  onToggle,
  onOpenDrawer,
  onShell,
  onLogs,
  onDescribe,
  onDelete,
}: {
  pod: Pod;
  expanded: boolean;
  pinned: boolean;
  selected: boolean;
  mock: boolean;
  onToggle: () => void;
  onOpenDrawer: () => void;
  onShell: () => void;
  onLogs: () => void;
  onDescribe: () => void;
  onDelete: () => void;
}) {
  const unhealthy = isUnhealthy(pod);
  const colorClass = STATUS_COLOR[pod.status];
  const restartClass =
    pod.restartCount > 5
      ? "text-status-crashloop"
      : pod.restartCount > 0
      ? "text-status-backoff"
      : "text-text-muted";

  // Keyboard cursor wins visually: gold inset bar + accent tint, composed with
  // the unhealthy status edge and the pinned tint.
  const rowTone = selected
    ? "bg-accent-dim shadow-[inset_2px_0_0_0_#c9b88a]"
    : pinned
    ? "bg-accent/5"
    : "";

  return (
    <>
      <tr
        onClick={onToggle}
        onDoubleClick={onOpenDrawer}
        data-row-selected={selected ? "true" : undefined}
        className={`cursor-pointer hover:bg-bg-panel2 transition-colors duration-100 ${
          unhealthy ? `border-l-2 ${colorClass}` : ""
        } ${rowTone}`}
        style={{ height: 32 }}
      >
        <td className="pl-2 pr-1 py-1 border-b border-border/70 align-middle">
          <PinToggle pinned={pinned} uid={pod.uid} />
        </td>
        <td
          className={`pl-2 pr-1 py-1 border-b border-border/70 ${
            unhealthy ? `border-l-2 ${colorClass}` : ""
          }`}
        >
          <div
            className={
              "w-7 h-7 bg-bg-panel2 k9s-square " +
              (pinned ? "ring-1 ring-accent/50" : "")
            }
          >
            <CritterPlayer critter={pod.critter} status={pod.critterState ?? pod.status} fps={6} />
          </div>
        </td>
        <td className="px-2 py-1 border-b border-border/70 align-middle">
          <StatusPill status={pod.status} />
        </td>
        <td className="px-3 py-1 border-b border-border/70 align-middle">
          <span className="text-text truncate">{pod.name}</span>
        </td>
        <td className="px-3 py-1 border-b border-border/70 align-middle text-text-muted truncate">
          {pod.namespace ?? "—"}
        </td>
        <td className="px-2 py-1 border-b border-border/70 align-middle text-right tabular-nums">
          {pod.readyContainers}/{pod.totalContainers}
        </td>
        <td
          className={`px-2 py-1 border-b border-border/70 align-middle text-right tabular-nums ${restartClass}`}
        >
          {pod.restartCount}
        </td>
        <td className="px-2 py-1 border-b border-border/70 align-middle text-right tabular-nums text-text-muted">
          {formatAge(pod.ageSec)}
        </td>
        <td className="px-3 py-1 border-b border-border/70 align-middle text-text-muted truncate">
          {pod.node}
        </td>
        <td className="px-3 py-1 border-b border-border/70 align-middle text-text-muted">
          <code>{pod.podIP ?? "—"}</code>
        </td>
      </tr>
      {expanded && (
        <tr
          className={`bg-bg-panel2/60 ${unhealthy ? `${colorClass} border-l-2` : ""}`}
          onDoubleClick={onOpenDrawer}
        >
          <td colSpan={10} className="px-4 py-4 border-b border-border/70">
            <div className="flex gap-4 items-start">
              <div className="w-24 h-24 shrink-0 bg-bg-panel border border-border k9s-square">
                <CritterPlayer critter={pod.critter} status={pod.critterState ?? pod.status} fps={7} />
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <StatusPill status={pod.status} />
                  <span className="text-text font-medium">{humanStatus(pod.status)}</span>
                </div>
                <div className="text-[12px] text-text-muted mb-3">
                  {statusExplanation(pod.status)}
                </div>
                <div className="flex flex-wrap gap-2">
                  <ActionButton
                    icon={<TerminalIcon className="w-3 h-3" />}
                    label="Shell"
                    title={`Open a shell in ${pod.containers[0]?.name ?? "the pod"}`}
                    onClick={onShell}
                  />
                  <ActionButton
                    icon={<ScrollText className="w-3 h-3" />}
                    label="Logs"
                    title="Open logs in the detail drawer"
                    onClick={onLogs}
                  />
                  <ActionButton
                    icon={<SearchIcon className="w-3 h-3" />}
                    label="Describe"
                    title="Open the detail drawer"
                    onClick={onDescribe}
                  />
                  {!pinned && (
                    <ActionButton
                      icon={<Pin className="w-3 h-3" />}
                      label="Pin"
                      accent
                      title="Pin this pod to Kubagachi"
                      onClick={() => workspaceActions.pinPod(pod.uid)}
                    />
                  )}
                  {/* A pod "restart" is a delete-to-respawn; reuse the same
                      confirmed delete path so there are no inert buttons. */}
                  <ConfirmDeleteButton
                    icon={<RefreshCw className="w-3 h-3" />}
                    label="Restart"
                    confirmLabel="delete & respawn?"
                    title={
                      mock
                        ? "Restart by deleting the pod (no-op in demo mode)"
                        : "Restart by deleting the pod (its controller respawns it)"
                    }
                    onConfirm={onDelete}
                  />
                  <ConfirmDeleteButton
                    icon={<Trash2 className="w-3 h-3" />}
                    label="Delete"
                    confirmLabel="confirm delete?"
                    title={
                      mock
                        ? "Delete the pod (no-op in demo mode)"
                        : "Delete the pod"
                    }
                    onConfirm={onDelete}
                  />
                </div>
              </div>
              <div className="hidden md:block text-[11px] text-text-muted font-mono">
                <Kv k="image" v={pod.containers[0]?.image ?? "—"} />
                <Kv k="restarts" v={String(pod.restartCount)} />
                <Kv k="owner" v={pod.ownerKind ? `${pod.ownerKind}/${pod.ownerName ?? ""}` : "—"} />
                <Kv k="qos" v={pod.qosClass ?? "—"} />
              </div>
            </div>
          </td>
        </tr>
      )}
    </>
  );
}

function ActionButton({
  icon,
  label,
  danger,
  accent,
  title,
  onClick,
}: {
  icon: React.ReactNode;
  label: string;
  danger?: boolean;
  accent?: boolean;
  title?: string;
  onClick?: () => void;
}) {
  return (
    <button
      type="button"
      title={title}
      onClick={(e) => {
        e.stopPropagation();
        onClick?.();
      }}
      className={`flex items-center gap-1.5 px-2 py-1 text-[11px] border transition-colors font-mono k9s-square ${
        danger
          ? "border-status-crashloop/40 text-status-crashloop hover:bg-status-crashloop/10"
          : accent
            ? "border-accent/40 text-accent hover:bg-accent/10 hover:border-accent"
            : "border-border text-text hover:border-border-strong hover:bg-bg-panel"
      }`}
    >
      {icon} {label}
    </button>
  );
}

/**
 * Destructive action styled like ActionButton but gated behind the inline
 * two-step ConfirmButton. The icon + label live in the resting state; the
 * armed state flips to the louder confirm prompt.
 */
function ConfirmDeleteButton({
  icon,
  label,
  confirmLabel,
  title,
  onConfirm,
}: {
  icon: React.ReactNode;
  label: string;
  confirmLabel: string;
  title?: string;
  onConfirm: () => void;
}) {
  return (
    <ConfirmButton
      onConfirm={onConfirm}
      title={title}
      aria-label={label}
      label={
        <span className="flex items-center gap-1.5">
          {icon} {label}
        </span>
      }
      confirmLabel={<span className="flex items-center gap-1.5">{confirmLabel}</span>}
      className="flex items-center gap-1.5 px-2 py-1 text-[11px] border border-status-crashloop/40 text-status-crashloop hover:bg-status-crashloop/10 transition-colors font-mono k9s-square"
      armedClassName="border-status-crashloop bg-status-crashloop/15 text-status-crashloop"
    />
  );
}

function Kv({ k, v }: { k: string; v: string }) {
  return (
    <div className="grid grid-cols-[60px_1fr] gap-2 py-0.5">
      <span className="text-text-muted uppercase text-[9px] tracking-wider">{k}</span>
      <span className="text-text truncate">{v}</span>
    </div>
  );
}
