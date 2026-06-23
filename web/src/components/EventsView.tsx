/**
 * EventsView — global event stream.
 *
 * Columns: Time (relative + tooltip with absolute), Type pill,
 * Reason, Object (`kind/name`), Message. Toggle to show only Warnings,
 * plus an inline search box that filters by reason / object / message.
 */

import { useMemo, useState } from "react";
import { Inbox } from "lucide-react";
import type { Event } from "../lib/types";
import { formatAge, formatTimestamp } from "../lib/format";
import {
  useCluster,
  useNamespace,
  useSearch,
  workspaceActions,
} from "../store/workspace";
import StatusPill from "./StatusPill";

export default function EventsView() {
  const cluster = useCluster();
  const namespace = useNamespace();
  const search = useSearch();
  const [warningsOnly, setWarningsOnly] = useState(false);
  const [localSearch, setLocalSearch] = useState("");

  const events = useMemo<Event[]>(() => {
    if (!cluster) return [];
    const q = (localSearch || search).trim().toLowerCase();
    return [...cluster.events]
      .filter((e) => {
        if (warningsOnly && e.type !== "warning") return false;
        if (namespace && namespace !== "all" && e.namespace !== namespace) return false;
        if (
          q &&
          !(
            e.reason.toLowerCase().includes(q) ||
            e.message.toLowerCase().includes(q) ||
            e.involvedObject.name.toLowerCase().includes(q) ||
            e.involvedObject.kind.toLowerCase().includes(q)
          )
        ) {
          return false;
        }
        return true;
      })
      .sort((a, b) => a.lastSeenSec - b.lastSeenSec);
  }, [cluster, namespace, search, localSearch, warningsOnly]);

  if (!cluster) {
    return <div className="p-6 text-text-muted text-sm">Loading cluster…</div>;
  }

  return (
    <div className="flex flex-col">
      <div className="flex items-center gap-3 p-3 border-b border-border bg-bg-panel">
        <input
          type="text"
          placeholder="Filter events…"
          value={localSearch}
          onChange={(e) => setLocalSearch(e.target.value)}
          className="flex-1 max-w-xs bg-bg-panel2 border border-border rounded px-2 py-1 text-[12px] text-text placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
        />
        <label className="flex items-center gap-1.5 text-[12px] text-text-muted cursor-pointer select-none">
          <input
            type="checkbox"
            checked={warningsOnly}
            onChange={(e) => setWarningsOnly(e.target.checked)}
            className="accent-status-backoff"
          />
          Warnings only
        </label>
        <span className="ml-auto text-[11px] text-text-muted">
          {events.length} event{events.length === 1 ? "" : "s"}
        </span>
      </div>

      {events.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 text-text-muted gap-2">
          <Inbox className="w-6 h-6 opacity-50" />
          <div className="text-xs">No events match your filters.</div>
        </div>
      ) : (
        <>
          <div className="hidden sm:block">
            <table className="w-full text-[12px] border-separate border-spacing-0">
              <thead className="sticky top-0 z-10 bg-bg-panel">
                <tr>
                  <Th className="w-24 text-right">Time</Th>
                  <Th className="w-24">Type</Th>
                  <Th className="w-32">Reason</Th>
                  <Th className="min-w-[200px]">Object</Th>
                  <Th className="min-w-[260px]">Message</Th>
                  <Th className="w-16 text-right">Count</Th>
                </tr>
              </thead>
              <tbody>
                {events.map((e) => (
                  <tr
                    key={e.uid}
                    onClick={() => workspaceActions.selectResource(e.uid)}
                    className="cursor-pointer hover:bg-bg-panel2 transition-colors duration-100"
                    style={{ height: 32 }}
                  >
                    <td
                      className="px-3 py-1.5 border-b border-border/70 text-right tabular-nums text-text-muted align-middle"
                      title={formatTimestamp(Date.now() - e.lastSeenSec * 1000)}
                    >
                      {formatAge(e.lastSeenSec)}
                    </td>
                    <td className="px-3 py-1.5 border-b border-border/70 align-middle">
                      <StatusPill status={e.type} compact />
                    </td>
                    <td className="px-3 py-1.5 border-b border-border/70 align-middle text-text">
                      {e.reason}
                    </td>
                    <td className="px-3 py-1.5 border-b border-border/70 align-middle text-text-muted truncate">
                      <span className="text-text-muted">{e.involvedObject.kind}/</span>
                      <span className="text-text">{e.involvedObject.name}</span>
                    </td>
                    <td className="px-3 py-1.5 border-b border-border/70 align-middle text-text truncate">
                      {e.message}
                    </td>
                    <td className="px-3 py-1.5 border-b border-border/70 align-middle text-right tabular-nums">
                      {e.count}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile: stacked cards */}
          <div className="sm:hidden flex flex-col gap-2 p-2">
            {events.map((e) => (
              <button
                key={e.uid}
                onClick={() => workspaceActions.selectResource(e.uid)}
                className="text-left bg-bg-panel border border-border rounded p-3"
              >
                <div className="flex items-center gap-2 mb-1">
                  <StatusPill status={e.type} compact />
                  <span className="text-text font-medium text-[12px]">{e.reason}</span>
                  <span className="ml-auto text-[10px] text-text-muted">{formatAge(e.lastSeenSec)}</span>
                </div>
                <div className="text-[11px] text-text-muted mb-1">
                  {e.involvedObject.kind}/{e.involvedObject.name}
                </div>
                <div className="text-[12px] text-text">{e.message}</div>
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function Th({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <th
      className={`text-left font-medium text-text-muted uppercase tracking-wider text-[10px] px-3 py-2 border-b border-border ${className ?? ""}`}
    >
      {children}
    </th>
  );
}
