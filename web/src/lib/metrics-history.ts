/**
 * metrics-history — a tiny module-level ring buffer of recent CPU/MEM samples,
 * keyed by node name and pod uid. The habitat records one sample per cluster
 * snapshot and renders true rolling sparklines from the history, so the bars
 * show a real recent trend rather than a single hashed frame.
 *
 * State is intentionally module-global (not React state): it survives
 * re-renders, is cheap to read, and never needs to trigger a render of its
 * own — the cluster snapshot already drives renders.
 */

const CAP = 16;

interface NodeSeries {
  cpu: number[];
  mem: number[];
}

const nodeSeries = new Map<string, NodeSeries>();
const podSeries = new Map<string, number[]>();

function cap(values: number[], value: number): void {
  values.push(value);
  if (values.length > CAP) values.shift();
}

/** Record node CPU%/MEM% and pod CPU(millicores) for one snapshot. */
export function recordMetrics(
  nodes: { name: string; cpuPct: number; memPct: number }[],
  pods: { uid: string; cpuMilli: number }[],
): void {
  const liveNodes = new Set<string>();
  for (const n of nodes) {
    liveNodes.add(n.name);
    let s = nodeSeries.get(n.name);
    if (!s) {
      s = { cpu: [], mem: [] };
      nodeSeries.set(n.name, s);
    }
    if (n.cpuPct >= 0) cap(s.cpu, n.cpuPct);
    if (n.memPct >= 0) cap(s.mem, n.memPct);
  }
  const livePods = new Set<string>();
  for (const p of pods) {
    livePods.add(p.uid);
    if (p.cpuMilli < 0) continue;
    let s = podSeries.get(p.uid);
    if (!s) {
      s = [];
      podSeries.set(p.uid, s);
    }
    cap(s, p.cpuMilli);
  }
  // Drop series for objects that no longer exist so the maps don't grow
  // unbounded across a long session.
  for (const key of nodeSeries.keys()) if (!liveNodes.has(key)) nodeSeries.delete(key);
  for (const key of podSeries.keys()) if (!livePods.has(key)) podSeries.delete(key);
}

const EMPTY: number[] = [];

export function nodeHistory(name: string): NodeSeries {
  return nodeSeries.get(name) ?? { cpu: EMPTY, mem: EMPTY };
}

export function podHistory(uid: string): number[] {
  return podSeries.get(uid) ?? EMPTY;
}

const BLOCKS = "▁▂▃▄▅▆▇█";

/**
 * Render a fixed-width unicode sparkline from a value series. `max` scales the
 * top of the chart; pass 100 for percentages, or omit to scale to the series'
 * own peak (good for absolute values like millicores).
 */
export function sparkline(values: number[], width = 12, max?: number): string {
  if (values.length === 0) return "─".repeat(width);
  const peak = max ?? Math.max(1, ...values);
  // Right-align the most recent `width` samples; pad the left with the floor.
  const recent = values.slice(-width);
  const pad = width - recent.length;
  let out = BLOCKS[0].repeat(Math.max(0, pad));
  for (const v of recent) {
    const idx = Math.max(0, Math.min(7, Math.floor((v / peak) * 8)));
    out += BLOCKS[idx];
  }
  return out;
}
