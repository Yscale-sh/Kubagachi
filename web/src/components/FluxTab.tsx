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

import { useEffect, useMemo, useRef, useState } from "react";
import { GitMerge, Pause, Play, RefreshCw, X } from "lucide-react";
import type { FluxObject } from "../lib/types";
import { fluxAction, type FluxActionKind } from "../lib/cluster-api";
import { registerRowNav, clearRowNav, type RowNavRegistration } from "../lib/row-nav";
import {
  useCluster,
  useNamespace,
  useSearch,
  useSelectedRow,
} from "../store/workspace";

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

interface Toast {
  msg: string;
  ok: boolean;
}

export default function FluxTab({ filter = "all" }: { filter?: FluxFilter }) {
  const cluster = useCluster();
  const namespace = useNamespace();
  const search = useSearch();
  const selectedRow = useSelectedRow();
  const [detailUid, setDetailUid] = useState<string | null>(null);
  const [toast, setToast] = useState<Toast | null>(null);
  const [pendingUid, setPendingUid] = useState<string | null>(null);
  const toastTimer = useRef<number | null>(null);

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

  const showToast = (msg: string, ok: boolean): void => {
    setToast({ msg, ok });
    if (toastTimer.current !== null) window.clearTimeout(toastTimer.current);
    toastTimer.current = window.setTimeout(() => setToast(null), 3200);
  };

  const runAction = async (f: FluxObject, action: FluxActionKind): Promise<void> => {
    setPendingUid(f.uid);
    // Optimistic feedback first.
    showToast(`${action} ${f.kind.toLowerCase()}/${f.name}…`, true);
    const res = await fluxAction(f.kind, f.namespace, f.name, action);
    setPendingUid(null);
    if (res.ok) {
      showToast(`${action} requested for ${f.kind.toLowerCase()}/${f.name}`, true);
    } else {
      showToast(`${action} failed: ${res.error ?? "unknown error"}`, false);
    }
  };

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

      {/* Toast */}
      {toast && (
        <div
          className={
            "fixed bottom-4 left-1/2 -translate-x-1/2 z-50 px-3 py-1.5 text-[12px] border bg-bg-panel shadow-lg shadow-black/50 k9s-square " +
            (toast.ok ? "border-accent/50 text-text" : "border-status-error/60 text-status-error")
          }
        >
          {toast.msg}
        </div>
      )}

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

function RowActions({
  obj,
  pending,
  onAction,
}: {
  obj: FluxObject;
  pending: boolean;
  onAction: (a: FluxActionKind) => void;
}) {
  return (
    <span className="inline-flex items-center gap-1">
      <ActionChip
        label="reconcile"
        icon={<RefreshCw size={11} className={pending ? "animate-spin" : ""} />}
        disabled={pending || obj.suspended}
        onClick={() => onAction("reconcile")}
      />
      {obj.suspended ? (
        <ActionChip
          label="resume"
          icon={<Play size={11} />}
          disabled={pending}
          onClick={() => onAction("resume")}
        />
      ) : (
        <ActionChip
          label="suspend"
          icon={<Pause size={11} />}
          disabled={pending}
          onClick={() => onAction("suspend")}
        />
      )}
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
      className={
        "inline-flex items-center gap-1 px-1.5 py-0.5 text-[11px] border transition-colors k9s-square " +
        (disabled
          ? "border-border/60 text-text-muted/50 cursor-default"
          : "border-border text-text-muted hover:text-accent hover:border-accent/50")
      }
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
            {obj.suspended ? (
              <PanelButton
                label="resume"
                icon={<Play size={12} />}
                disabled={pending}
                onClick={() => onAction("resume")}
              />
            ) : (
              <PanelButton
                label="suspend"
                icon={<Pause size={12} />}
                disabled={pending}
                onClick={() => onAction("suspend")}
              />
            )}
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
      className={
        "inline-flex items-center gap-1.5 px-2.5 py-1.5 text-[12px] border transition-colors k9s-square " +
        (disabled
          ? "border-border/60 text-text-muted/50 cursor-default"
          : "border-accent/40 text-accent hover:bg-accent/10 hover:border-accent")
      }
    >
      {icon}
      {label}
    </button>
  );
}
