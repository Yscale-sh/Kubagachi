/**
 * CritterPlayer — animates a pod's pixel-art mascot.
 *
 * Pre-slices the sprite sheet for the given (critter, status) into N <img>
 * data-URL frames, mounts all N stacked absolute, and toggles display
 * between ticks. No <img src> reassignment per tick — discrete swap, no
 * flicker.
 *
 * Falls back to the keyed/composite sheet at low opacity when the
 * status-specific anim sheet is unavailable.
 */

import { useEffect, useRef, useState } from "react";
import { subscribeAnim, animTick } from "../lib/anim-clock";

interface CritterPlayerProps {
  critter: string;
  /** Deck to play: a health state ("running") or a workload animation ("bursting"). */
  status: string;
  fps?: number;
  className?: string;
}

interface FrameBounds {
  x0: number;
  y0: number;
  x1: number;
  y1: number;
}

interface AnimSrc {
  url: string;
  w: number;
  h: number;
  frames: number;
  has_alpha: boolean;
  bounds?: FrameBounds[];
}

interface CritterInfo {
  name: string;
  keyed_url?: string;
  keyed_dim?: { w: number; h: number };
  keyed_has_alpha?: boolean;
  anim?: Record<string, AnimSrc>;
}

interface CrittersApiResponse {
  states: string[];
  critters: CritterInfo[];
}

interface CrittersIndex {
  byName: Record<string, CritterInfo>;
}

// ---------------------------------------------------------------------------
// Module-level cache for the /api/critters response (one-time fetch).
// ---------------------------------------------------------------------------

let indexPromise: Promise<CrittersIndex> | null = null;

function getCrittersIndex(): Promise<CrittersIndex> {
  if (indexPromise) return indexPromise;
  indexPromise = (async () => {
    try {
      const r = await fetch("/api/critters", { headers: { Accept: "application/json" } });
      if (!r.ok) throw new Error(`critters ${r.status}`);
      const data = (await r.json()) as CrittersApiResponse;
      const byName: Record<string, CritterInfo> = {};
      for (const c of data.critters ?? []) byName[c.name] = c;
      if (Object.keys(byName).length === 0) throw new Error("empty critters index");
      return { byName };
    } catch {
      // Don't cache a transient failure (e.g. a server restart) — clear the
      // singleton so the next mount retries instead of being stuck spriteless.
      indexPromise = null;
      return { byName: {} };
    }
  })();
  return indexPromise;
}

// ---------------------------------------------------------------------------
// Sheet slicing — also cached per (url, frames, bounds-shape)
// ---------------------------------------------------------------------------

interface SlicedFrame {
  dataUrl: string;
  w: number;
  h: number;
}

const sliceCache = new Map<string, Promise<SlicedFrame[]>>();

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.crossOrigin = "anonymous";
    img.onload = () => resolve(img);
    img.onerror = (e) => reject(e);
    img.src = src;
  });
}

async function sliceSheet(
  url: string,
  bounds: FrameBounds[] | undefined,
  frames: number,
): Promise<SlicedFrame[]> {
  const cacheKey = `${url}|boundsv5`;
  const cached = sliceCache.get(cacheKey);
  if (cached) return cached;

  const promise = (async () => {
    const img = await loadImage(url);
    const W = img.naturalWidth;
    const H = img.naturalHeight;

    // The server's column-scan detection reliably finds the *actual* frames
    // (sheets aren't always 8 — many are 7 — and aren't perfectly even). Trust
    // those bounds; only fall back to an even grid when they're missing.
    let fr: FrameBounds[];
    if (bounds && bounds.length >= 2) {
      fr = bounds;
    } else {
      const step = W / Math.max(1, frames);
      fr = [];
      for (let i = 0; i < frames; i++) {
        fr.push({ x0: Math.round(i * step), x1: Math.round((i + 1) * step), y0: 0, y1: H });
      }
    }

    // Common crop: the widest frame's width + the shared content band, so the
    // critter fills the card (most of these sheets are ~60% empty vertically)
    // and every frame is the exact same size — no wobble, no size pulsing.
    let yTop = H;
    let yBot = 0;
    let maxW = 1;
    for (const b of fr) {
      yTop = Math.min(yTop, b.y0);
      yBot = Math.max(yBot, b.y1);
      maxW = Math.max(maxW, b.x1 - b.x0);
    }
    // Frame pitch = the even spacing between frame centroids. Capping the crop
    // window to just under the pitch guarantees a neighbouring frame can never
    // bleed in, even when one detected box is an outlier (which would otherwise
    // inflate maxW and pull a sliver of the next critter into every frame).
    const centroids = fr.map((b) => (b.x0 + b.x1) / 2);
    const n = centroids.length;
    const pitch = n >= 2 ? (centroids[n - 1] - centroids[0]) / (n - 1) : maxW + 16;

    const pad = 6;
    const vTop = Math.max(0, yTop - pad);
    const cropH = Math.min(H - vTop, yBot - yTop + pad * 2);
    const cropW = Math.max(8, Math.min(maxW + pad * 2, Math.round(pitch) - 2));

    const out: SlicedFrame[] = [];
    for (let i = 0; i < n; i++) {
      const c = document.createElement("canvas");
      c.width = cropW;
      c.height = cropH;
      const ctx = c.getContext("2d");
      if (!ctx) continue;
      ctx.imageSmoothingEnabled = false;
      // Center on THIS frame's own content centroid so a critter that's drawn
      // walking across its frames animates in place (no left-right drift). The
      // pitch-capped window already prevents a neighbour from bleeding in.
      const cx = centroids[i];
      let sx = Math.round(cx - cropW / 2);
      let dx = 0;
      if (sx < 0) {
        dx = -sx;
        sx = 0;
      }
      const sw = Math.min(cropW - dx, W - sx);
      if (sw > 0) {
        ctx.drawImage(img, sx, vTop, sw, cropH, dx, 0, sw, cropH);
      }
      out.push({ dataUrl: c.toDataURL("image/png"), w: cropW, h: cropH });
    }
    return out;
  })();

  sliceCache.set(cacheKey, promise);
  // Drop cache on failure so a retry can succeed.
  promise.catch(() => sliceCache.delete(cacheKey));
  return promise;
}

// ---------------------------------------------------------------------------
// Status → anim-key mapping.
// ---------------------------------------------------------------------------

/**
 * Map a (lowercase) pod status onto an animation key. The /api/critters
 * endpoint serves anim sheets keyed by exactly these strings.
 */
function statusToAnimKey(status: string): string {
  // Lowercase canonical: the server already emits these and our mock generator
  // does too — so this is the identity mapping. Keeping the function shape so
  // future status -> anim aliasing has a single landing point.
  return status;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function CritterPlayer({
  critter,
  status,
  className,
}: CritterPlayerProps) {
  const [info, setInfo] = useState<CritterInfo | null>(null);
  const [frames, setFrames] = useState<SlicedFrame[] | null>(null);
  const idxRef = useRef(0);
  // Stable per-instance phase so critters don't stutter in lockstep.
  const phaseRef = useRef(Math.floor(Math.random() * 1009));
  const nodesRef = useRef<(HTMLImageElement | null)[]>([]);

  // One-time global index fetch.
  useEffect(() => {
    let cancelled = false;
    void getCrittersIndex().then((ix) => {
      if (cancelled) return;
      setInfo(ix.byName[critter] ?? null);
    });
    return () => {
      cancelled = true;
    };
  }, [critter]);

  const animKey = statusToAnimKey(status);
  // For `error` fall back to `failed` (some critters ship sheets under either
  // key). For any other key, miss → fall back to `running` so a pinned mascot
  // never renders empty when the cluster surfaces a fresh status the critter
  // hasn't been baked with yet.
  const anim =
    info?.anim?.[animKey] ??
    (animKey === "error" ? info?.anim?.["failed"] : undefined) ??
    info?.anim?.["running"];
  const animUrl = anim?.url;

  // Slice once per (url, frame-count).
  useEffect(() => {
    if (!anim || !animUrl) {
      setFrames(null);
      return;
    }
    let cancelled = false;
    void sliceSheet(animUrl, anim.bounds, anim.frames || 8).then((out) => {
      if (!cancelled) setFrames(out);
    });
    return () => {
      cancelled = true;
    };
    // anim.bounds is stable per url; re-running on url alone is enough.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [animUrl, anim?.frames]);

  // Animation: a single global clock drives every critter in lockstep (nori
  // style) — show `frame = tick % count`, swapping display on the pre-mounted
  // img array. No per-component timer, no random phase.
  useEffect(() => {
    if (!frames || frames.length === 0) return;
    const swap = () => {
      const nodes = nodesRef.current;
      if (nodes.length === 0) return;
      const next = (animTick() + phaseRef.current) % frames.length;
      const prev = idxRef.current;
      if (prev === next) return;
      if (nodes[prev]) nodes[prev]!.style.display = "none";
      if (nodes[next]) nodes[next]!.style.display = "block";
      idxRef.current = next;
    };
    swap();
    return subscribeAnim(swap);
  }, [frames]);

  const container = `relative w-full h-full flex items-center justify-center overflow-hidden ${className ?? ""}`;

  // No anim sheet — fall back to keyed/composite at low opacity.
  if (!frames) {
    const keyed = info?.keyed_url;
    return (
      <div className={container}>
        {keyed ? (
          <img
            src={keyed}
            alt={`${critter} (fallback)`}
            className="max-w-full max-h-full object-contain opacity-40 pixelated"
          />
        ) : (
          <div className="text-text-muted text-[10px] uppercase tracking-wider opacity-60">
            {critter}
          </div>
        )}
      </div>
    );
  }

  const initialIdx = idxRef.current;
  nodesRef.current = new Array(frames.length);

  return (
    <div className={container}>
      {frames.map((f, i) => (
        <img
          key={`${animUrl}-${i}`}
          ref={(el) => {
            nodesRef.current[i] = el;
          }}
          src={f.dataUrl}
          alt={`${critter} ${animKey} frame ${i}`}
          className="absolute max-w-[96%] max-h-[96%] object-contain pixelated"
          style={{
            top: "50%",
            left: "50%",
            transform: "translate(-50%, -50%)",
            display: i === initialIdx ? "block" : "none",
          }}
        />
      ))}
    </div>
  );
}
