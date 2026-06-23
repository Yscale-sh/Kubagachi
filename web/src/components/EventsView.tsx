/**
 * EventsView — global event stream.
 *
 * Columns: Time (relative + tooltip with absolute), Type pill,
 * Reason, Object (`kind/name`), Message. Toggle to show only Warnings,
 * plus an inline search box that filters by reason / object / message.
 */

import { useCallback, useMemo, useState } from "react";
import { Inbox } from "lucide-react";
import type { BaseMeta, Cluster, Event } from "../lib/types";
import { formatAge, formatTimestamp } from "../lib/format";
import {
  useCluster,
  useNamespace,
  useSearch,
  workspaceActions,
} from "../store/workspace";
import StatusPill from "./StatusPill";

/**
 * Map an event's involvedObject.kind to the cluster collections most likely to
 * hold it, in priority order. We probe these first; if nothing matches we sweep
 * every collection so an exact kind+name(+namespace) hit still resolves.
 */
const KIND_COLLECTIONS: Record<string, (keyof Cluster)[]> = {
  Pod: ["pods"],
  Deployment: ["deployments"],
  StatefulSet: ["statefulSets"],
  DaemonSet: ["daemonSets"],
  ReplicaSet: ["replicaSets"],
  Job: ["jobs"],
  CronJob: ["cronJobs"],
  Service: ["services"],
  Ingress: ["ingresses"],
  Endpoint: ["endpoints"],
  Endpoints: ["endpoints"],
  NetworkPolicy: ["networkPolicies"],
  ConfigMap: ["configMaps"],
  Secret: ["secrets"],
  ResourceQuota: ["resourceQuotas"],
  LimitRange: ["limitRanges"],
  HorizontalPodAutoscaler: ["horizontalPodAutoscalers"],
  PodDisruptionBudget: ["podDisruptionBudgets"],
  PersistentVolume: ["persistentVolumes"],
  PersistentVolumeClaim: ["persistentVolumeClaims"],
  StorageClass: ["storageClasses"],
  ServiceAccount: ["serviceAccounts"],
  Role: ["roles"],
  ClusterRole: ["clusterRoles"],
  RoleBinding: ["roleBindings"],
  ClusterRoleBinding: ["clusterRoleBindings"],
  Node: ["nodes"],
  Namespace: ["namespaces"],
  CustomResourceDefinition: ["customResourceDefinitions"],
  HelmRelease: ["helmReleases"],
};

/** Every collection that holds named, selectable resources (for the fallback sweep). */
const ALL_RESOURCE_COLLECTIONS: (keyof Cluster)[] = [
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
  "customResourceDefinitions",
  "helmReleases",
];

function findInCollections(
  cluster: Cluster,
  collections: (keyof Cluster)[],
  name: string,
  namespace?: string,
): string | null {
  for (const key of collections) {
    const list = cluster[key];
    if (!Array.isArray(list)) continue;
    // Match by name first; if the event carries a namespace, require it to match
    // (when the resource actually has one) so we don't cross-resolve namespaces.
    const hit = (list as BaseMeta[]).find(
      (r) =>
        r.name === name &&
        (!namespace || r.namespace === undefined || r.namespace === namespace),
    );
    if (hit) return hit.uid;
  }
  return null;
}

/**
 * Resolve an event's involvedObject to the uid of the OFFENDING live resource,
 * so opening the drawer shows the thing that's failing — not the Event object.
 * Returns null when nothing in the cluster matches.
 */
function resolveInvolvedUid(cluster: Cluster, e: Event): string | null {
  const { kind, name, namespace } = e.involvedObject;
  const preferred = KIND_COLLECTIONS[kind] ?? [];
  return (
    findInCollections(cluster, preferred, name, namespace) ??
    findInCollections(cluster, ALL_RESOURCE_COLLECTIONS, name, namespace)
  );
}

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
      // Triage order: freshest WARNINGS first, then everything by recency
      // (largest lastSeenSec = oldest, so we sort ascending on age = recent first).
      .sort((a, b) => {
        const aWarn = a.type === "warning" ? 0 : 1;
        const bWarn = b.type === "warning" ? 0 : 1;
        if (aWarn !== bWarn) return aWarn - bWarn;
        return a.lastSeenSec - b.lastSeenSec;
      });
  }, [cluster, namespace, search, localSearch, warningsOnly]);

  // Open the offending resource's drawer; fall back to the Event object itself
  // if nothing in the live cluster resolves.
  const openOffender = useCallback(
    (e: Event) => {
      const uid = cluster ? resolveInvolvedUid(cluster, e) : null;
      workspaceActions.selectResource(uid ?? e.uid);
    },
    [cluster],
  );

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
                {events.map((e) => {
                  const offenderUid = resolveInvolvedUid(cluster, e);
                  return (
                    <tr
                      key={e.uid}
                      onClick={() => openOffender(e)}
                      className="cursor-pointer hover:bg-bg-panel2 transition-colors duration-100 group"
                      style={{ height: 32 }}
                      title={
                        offenderUid
                          ? `Open ${e.involvedObject.kind} ${e.involvedObject.name}`
                          : undefined
                      }
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
                        {offenderUid ? (
                          <span
                            role="link"
                            tabIndex={0}
                            onClick={(ev) => {
                              ev.stopPropagation();
                              workspaceActions.selectResource(offenderUid);
                            }}
                            onKeyDown={(ev) => {
                              if (ev.key === "Enter" || ev.key === " ") {
                                ev.preventDefault();
                                ev.stopPropagation();
                                workspaceActions.selectResource(offenderUid);
                              }
                            }}
                            className="inline-flex items-baseline cursor-pointer text-accent decoration-accent-soft underline-offset-2 group-hover:underline hover:text-accent-bright focus:outline-none focus-visible:underline"
                            title={`Open ${e.involvedObject.kind} ${e.involvedObject.name}`}
                          >
                            <span className="opacity-70">{e.involvedObject.kind}/</span>
                            <span>{e.involvedObject.name}</span>
                          </span>
                        ) : (
                          <span className="inline-flex items-baseline">
                            <span className="text-text-muted">{e.involvedObject.kind}/</span>
                            <span className="text-text">{e.involvedObject.name}</span>
                          </span>
                        )}
                      </td>
                      <td className="px-3 py-1.5 border-b border-border/70 align-middle text-text truncate">
                        {e.message}
                      </td>
                      <td className="px-3 py-1.5 border-b border-border/70 align-middle text-right tabular-nums">
                        {e.count}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* Mobile: stacked cards */}
          <div className="sm:hidden flex flex-col gap-2 p-2">
            {events.map((e) => {
              const offenderUid = resolveInvolvedUid(cluster, e);
              return (
                <button
                  key={e.uid}
                  onClick={() => openOffender(e)}
                  className="text-left bg-bg-panel border border-border rounded p-3"
                >
                  <div className="flex items-center gap-2 mb-1">
                    <StatusPill status={e.type} compact />
                    <span className="text-text font-medium text-[12px]">{e.reason}</span>
                    <span className="ml-auto text-[10px] text-text-muted">{formatAge(e.lastSeenSec)}</span>
                  </div>
                  <div
                    className={`text-[11px] mb-1 ${offenderUid ? "text-accent" : "text-text-muted"}`}
                  >
                    {e.involvedObject.kind}/{e.involvedObject.name}
                  </div>
                  <div className="text-[12px] text-text">{e.message}</div>
                </button>
              );
            })}
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
