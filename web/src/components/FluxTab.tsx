/**
 * FluxTab — first-class Flux (GitOps) resource table.
 *
 * Columns: KIND / NAME / NAMESPACE / READY / REVISION / AGE.
 * Row click opens a local detail side panel (source + message). Per-row
 * actions: reconcile, suspend/resume — POSTed to /api/flux/action with
 * optimistic toast feedback.
 *
 * `filter` narrows the table to kustomizations / helmreleases / sources.
 */

import { useEffect, useMemo, useState } from "react";
import { GitMerge, Pause, Play, RefreshCw, X } from "lucide-react";
import type { FluxObject } from "../lib/types";
import { fluxAction, type FluxActionKind } from "../lib/cluster-api";
import { registerRowNav, clearRowNav, type RowNavRegistration } from "../lib/row-nav";
import {
  useCluster,
  useNamespace,
  useSearch,
  useSelectedRow,
  workspaceActions,
} from "../store/workspace";
import ConfirmButton from "./ConfirmButton";

export type FluxFilter = "all" | "kustomizations" | "helmreleases" | "sources";

const SOURCE_KINDS: ReadonlySet<string> = new Set([
  "GitRepository",
  "OCIRepository",
  "HelmRepository",
  "Bucket",
]);

export function matchesFluxFilter(f: FluxObject, filter: FluxFilter): boolean {
  switch (filter) {
    case "all":
      return true;
    case "kustomizations":
      return f.kind === "Kustomization";
    case "helmreleases":
      return f.kind === "HelmRelease";
    case "sources":
      return SOURCE_KINDS.has(f.kind);
  }
}

function isTypingTarget(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null;
  if (!el || !el.tagName) return false;
  const tag = el.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true;
  if (el.isContentEditable) return true;
  return false;
}

export default function FluxTab({ filter = "all" }: { filter?: FluxFilter }) {
  const cluster = useCluster();
  const namespace = useNamespace();
  const search = useSearch();
  const selectedRow = useSelectedRow();
  const [detailUid, setDetailUid] = useState<string | null>(null);
  const [pendingUid, setPendingUid] = useState<string | null>(null);

  const objects = useMemo<FluxObject[]>(() => {
    if (!cluster) return [];
    const q = search.trim().toLowerCase();
    return cluster.flux
      .filter((f) => {
        if (!matchesFluxFilter(f, filter)) return false;
        if (namespace && namespace !== "all" && f.namespace !== namespace) return false;
        if (q && !f.name.toLowerCase().includes(q) && !f.kind.toLowerCase().includes(q)) {
          return false;
        }
        return true;
      })
      .sort((a, b) => {
        const k = a.kind.localeCompare(b.kind);
        if (k !== 0) return k;
        return a.name.localeCompare(b.name);
      });
  }, [cluster, filter, namespace, search]);

  // j/k + enter navigation registration.
  useEffect(() => {
    const reg: RowNavRegistration = {
      ids: objects.map((o) => o.uid),
      onEnter: (id) => setDetailUid(id),
    };
    registerRowNav(reg);
    return () => clearRowNav(reg);
  }, [objects]);

  const runAction = async (f: FluxObject, action: FluxActionKind): Promise<void> => {
    setPendingUid(f.uid);
    // Optimistic feedback first.
    workspaceActions.toast(`${action} ${f.kind.toLowerCase()}/${f.name}…`, "info");
    const res = await fluxAction(f.kind, f.namespace, f.name, action);
    setPendingUid(null);
    if (res.ok) {
      workspaceActions.toast(
        `${action} requested for ${f.kind.toLowerCase()}/${f.name}`,
        "success",
      );
    } else {
      workspaceActions.toast(`${action} failed: ${res.error ?? "unknown error"}`, "error");
    }
  };

  // Row-level keybindings (local listener; the global KeyboardLayer drives j/k/Enter).
  //   r  reconcile the selected row
  //   s  suspend/resume toggle the selected row
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.defaultPrevented) return;
      if (e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) return;
      if (isTypingTarget(e.target)) return;
      if (selectedRow < 0 || selectedRow >= objects.length) return;
      const f = objects[selectedRow];
      if (!f || pendingUid !== null) return;

      if (e.key === "r") {
        if (f.suspended) return; // reconcile is a no-op while suspended
        e.preventDefault();
        void runAction(f, "reconcile");
      } else if (e.key === "s") {
        e.preventDefault();
        void runAction(f, f.suspended ? "resume" : "suspend");
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [objects, selectedRow, pendingUid]);

  if (!cluster) {
    return <div className="p-6 text-text-muted text-[12px]">loading cluster…</div>;
  }

  if (!cluster.fluxInstalled && cluster.flux.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-text-muted gap-2">
        <GitMerge className="w-6 h-6 opacity-50" />
        <div className="text-xs">
          {cluster.mode === "mock"
            ? "flux data is only available when connected to the kubagachi server."
            : "flux is not installed in this cluster."}
        </div>
      </div>
    );
  }

  if (objects.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-text-muted gap-2">
        <GitMerge className="w-6 h-6 opacity-50" />
        <div className="text-xs">no flux objects in the selected scope.</div>
      </div>
    );
  }

  const detail = detailUid ? objects.find((o) => o.uid === detailUid) ?? null : null;

  return (
    <div className="relative font-mono">
      <table className="w-full text-[12px] border-separate border-spacing-0">
        <thead className="sticky top-0 z-10 bg-bg-panel">
          <tr>
            <Th className="w-36">kind</Th>
            <Th className="min-w-[200px]">name</Th>
            <Th className="w-36">namespace</Th>
            <Th className="w-24">ready</Th>
            <Th className="min-w-[160px]">revision</Th>
            <Th className="w-16 text-right">age</Th>
            <Th className="w-56 text-right">actions</Th>
          </tr>
        </thead>
        <tbody>
          {objects.map((f, i) => {
            const rowSelected = i === selectedRow;
            return (
              <tr
                key={f.uid}
                data-row-selected={rowSelected || undefined}
                onClick={() => setDetailUid(f.uid)}
                className={
                  "cursor-pointer hover:bg-bg-panel2 transition-colors duration-100 " +
                  (rowSelected ? "bg-accent/5" : "")
                }
                style={{ height: 32 }}
              >
                <td className="pl-2 pr-3 py-1 border-b border-border/70 align-middle text-text-muted whitespace-nowrap">
                  <span
                    aria-hidden="true"
                    className={"mr-1.5 " + (rowSelected ? "text-accent" : "text-transparent")}
                  >
                    ▍
                  </span>
                  {f.kind}
                </td>
                <td className="px-3 py-1 border-b border-border/70 align-middle text-text truncate">
                  {f.name}
                </td>
                <td className="px-3 py-1 border-b border-border/70 align-middle text-text-muted truncate">
                  {f.namespace}
                </td>
                <td className="px-3 py-1 border-b border-border/70 align-middle">
                  <ReadyBadge obj={f} />
                </td>
                <td className="px-3 py-1 border-b border-border/70 align-middle text-text-muted truncate">
                  {f.revision}
                </td>
                <td className="px-3 py-1 border-b border-border/70 align-middle text-right tabular-nums text-text-muted">
                  {f.age}
                </td>
                <td className="px-2 py-1 border-b border-border/70 align-middle text-right whitespace-nowrap">
                  <RowActions
                    obj={f}
                    pending={pendingUid === f.uid}
                    onAction={(a) => void runAction(f, a)}
                  />
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>

      {/* Detail side panel */}
      {detail && (
        <FluxDetailPanel
          obj={detail}
          pending={pendingUid === detail.uid}
          onAction={(a) => void runAction(detail, a)}
          onClose={() => setDetailUid(null)}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pieces
// ---------------------------------------------------------------------------

function Th({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <th
      className={`text-left font-normal text-text-muted/80 uppercase tracking-wider text-[10px] px-3 py-1.5 border-b border-border ${className ?? ""}`}
    >
      {children}
    </th>
  );
}

function ReadyBadge({ obj }: { obj: FluxObject }) {
  if (obj.suspended) {
    return (
      <span className="inline-flex items-center gap-1.5 text-status-terminating text-[11px]">
        <span className="k9s-glyph">‖</span>
        <span className="tracking-wider">SUSP</span>
      </span>
    );
  }
  if (obj.ready === "True") {
    return (
      <span className="inline-flex items-center gap-1.5 text-status-running text-[11px]">
        <span className="k9s-glyph">◉</span>
        <span className="tracking-wider">True</span>
      </span>
    );
  }
  if (obj.ready === "False") {
    return (
      <span className="inline-flex items-center gap-1.5 text-status-crashloop text-[11px]">
        <span className="k9s-glyph">✖</span>
        <span className="tracking-wider">False</span>
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5 text-text-muted text-[11px]">
      <span className="k9s-glyph">·</span>
      <span className="tracking-wider">—</span>
    </span>
  );
}

const CHIP_BASE =
  "inline-flex items-center gap-1 px-1.5 py-0.5 text-[11px] border transition-colors k9s-square ";
const CHIP_IDLE = "border-border text-text-muted hover:text-accent hover:border-accent/50";
const CHIP_DISABLED = "border-border/60 text-text-muted/50 cursor-default";
const CHIP_ARMED = "border-accent text-accent bg-accent-dim";

function RowActions({
  obj,
  pending,
  onAction,
}: {
  obj: FluxObject;
  pending: boolean;
  onAction: (a: FluxActionKind) => void;
}) {
  // suspend/resume is state-mutating → two-step confirm; reconcile stays one-click.
  const toggle: FluxActionKind = obj.suspended ? "resume" : "suspend";
  const toggleLabel = obj.suspended ? "resume" : "suspend";
  return (
    <span className="inline-flex items-center gap-1">
      <ActionChip
        label="reconcile"
        icon={<RefreshCw size={11} className={pending ? "animate-spin" : ""} />}
        disabled={pending || obj.suspended}
        onClick={() => onAction("reconcile")}
      />
      <ConfirmButton
        onConfirm={() => onAction(toggle)}
        title={`${toggleLabel} ${obj.kind.toLowerCase()}/${obj.name}`}
        aria-label={`${toggleLabel} ${obj.name}`}
        className={CHIP_BASE + (pending ? CHIP_DISABLED : CHIP_IDLE)}
        armedClassName={CHIP_ARMED}
        label={
          <span className="inline-flex items-center gap-1">
            {obj.suspended ? <Play size={11} /> : <Pause size={11} />}
            {toggleLabel}
          </span>
        }
        confirmLabel={<span className="inline-flex items-center gap-1">{toggleLabel}?</span>}
      />
    </span>
  );
}

function ActionChip({
  label,
  icon,
  disabled,
  onClick,
}: {
  label: string;
  icon: React.ReactNode;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      className={CHIP_BASE + (disabled ? CHIP_DISABLED : CHIP_IDLE)}
    >
      {icon}
      {label}
    </button>
  );
}

function FluxDetailPanel({
  obj,
  pending,
  onAction,
  onClose,
}: {
  obj: FluxObject;
  pending: boolean;
  onAction: (a: FluxActionKind) => void;
  onClose: () => void;
}) {
  // Esc closes the panel.
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onClose]);

  return (
    <>
      <div
        className="fixed inset-0 z-30 bg-black/40 backdrop-blur-[1px]"
        onClick={onClose}
        aria-hidden="true"
      />
      <aside
        role="dialog"
        aria-label={`${obj.kind} ${obj.name}`}
        className="fixed z-40 bg-bg-panel border-border flex flex-col yscale-overlay-in
                   inset-x-0 bottom-0 h-[70vh] border-t
                   sm:inset-y-0 sm:right-0 sm:left-auto sm:bottom-auto sm:h-auto sm:w-[420px] sm:border-l sm:border-t-0"
      >
        <div className="shrink-0 flex items-start gap-2 p-3 border-b border-border">
          <GitMerge size={15} className="text-accent mt-1 shrink-0" />
          <div className="flex-1 min-w-0">
            <div className="text-[11px] text-text-muted">
              {obj.kind} · {obj.namespace}
            </div>
            <div className="text-[14px] font-medium text-text truncate">{obj.name}</div>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-text-muted hover:text-text hover:bg-bg-panel2 transition-colors k9s-square"
            aria-label="Close detail"
          >
            <X size={15} />
          </button>
        </div>

        <div className="flex-1 min-h-0 overflow-y-auto scrollbar-thin p-4 flex flex-col gap-4">
          <dl className="grid grid-cols-[90px_1fr] gap-x-3 gap-y-1.5 text-[12px]">
            <Kv k="ready" v={<ReadyBadge obj={obj} />} />
            <Kv k="revision" v={<code className="break-all">{obj.revision}</code>} />
            <Kv k="source" v={obj.source || "—"} />
            <Kv k="age" v={obj.age} />
            <Kv k="suspended" v={obj.suspended ? "true" : "false"} />
          </dl>

          <div>
            <div className="text-text-muted uppercase tracking-wider text-[10px] mb-1.5">
              message
            </div>
            <div className="text-[12px] text-text bg-bg-panel2 border border-border p-2 whitespace-pre-wrap break-words">
              {obj.message || "—"}
            </div>
          </div>

          <div className="flex flex-wrap gap-2">
            <PanelButton
              label="reconcile"
              icon={<RefreshCw size={12} className={pending ? "animate-spin" : ""} />}
              disabled={pending || obj.suspended}
              onClick={() => onAction("reconcile")}
            />
            <ConfirmButton
              onConfirm={() => onAction(obj.suspended ? "resume" : "suspend")}
              title={`${obj.suspended ? "resume" : "suspend"} ${obj.kind.toLowerCase()}/${obj.name}`}
              className={PANEL_BASE + (pending ? PANEL_DISABLED : PANEL_IDLE)}
              armedClassName={PANEL_ARMED}
              label={
                <span className="inline-flex items-center gap-1.5">
                  {obj.suspended ? <Play size={12} /> : <Pause size={12} />}
                  {obj.suspended ? "resume" : "suspend"}
                </span>
              }
              confirmLabel={
                <span className="inline-flex items-center gap-1.5">
                  {obj.suspended ? "resume" : "suspend"}?
                </span>
              }
            />
          </div>
        </div>
      </aside>
    </>
  );
}

function Kv({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <div className="contents">
      <dt className="text-text-muted uppercase tracking-wider text-[10px] pt-0.5">{k}</dt>
      <dd className="text-text break-words">{v}</dd>
    </div>
  );
}

const PANEL_BASE =
  "inline-flex items-center gap-1.5 px-2.5 py-1.5 text-[12px] border transition-colors k9s-square ";
const PANEL_IDLE = "border-accent/40 text-accent hover:bg-accent/10 hover:border-accent";
const PANEL_DISABLED = "border-border/60 text-text-muted/50 cursor-default";
const PANEL_ARMED = "border-accent text-accent bg-accent-dim";

function PanelButton({
  label,
  icon,
  disabled,
  onClick,
}: {
  label: string;
  icon: React.ReactNode;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={PANEL_BASE + (disabled ? PANEL_DISABLED : PANEL_IDLE)}
    >
      {icon}
      {label}
    </button>
  );
}
