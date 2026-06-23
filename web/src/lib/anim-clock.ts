/**
 * anim-clock — a single global animation clock for every critter, modelled on
 * box-network's "nori" renderer: one fixed-FPS tick advances the frame index
 * for all mascots in lockstep (`frame = tick % count`), so the habitat reads
 * like a synchronized TUI rather than a field of independently-jittering
 * sprites. It also replaces N per-component timers with exactly one — a real
 * win when 100+ critters are on screen.
 */

const FPS = 12;
const listeners = new Set<() => void>();
let tick = 0;
let timer: ReturnType<typeof setInterval> | null = null;

function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

function ensureRunning(): void {
  if (timer !== null || typeof window === "undefined") return;
  // Honor reduced-motion: leave `tick` frozen at 0 so every critter holds a
  // single rest frame instead of cycling. The one-shot swap each CritterPlayer
  // runs on mount still paints that frame.
  if (prefersReducedMotion()) return;
  timer = setInterval(() => {
    tick++;
    for (const l of listeners) l();
  }, Math.round(1000 / FPS));
}

// React live when the user flips the OS reduced-motion setting: start the clock
// when motion is re-enabled, stop (and freeze) it when it's turned off.
if (typeof window !== "undefined" && typeof window.matchMedia === "function") {
  const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
  const onChange = () => {
    if (mq.matches) {
      if (timer !== null) {
        clearInterval(timer);
        timer = null;
      }
    } else if (listeners.size > 0) {
      ensureRunning();
    }
  };
  if (typeof mq.addEventListener === "function") mq.addEventListener("change", onChange);
}

/** Subscribe to the global tick; returns an unsubscribe fn. */
export function subscribeAnim(cb: () => void): () => void {
  listeners.add(cb);
  ensureRunning();
  return () => {
    listeners.delete(cb);
  };
}

/** The current frame index for an animation of `count` frames. */
export function animFrame(count: number): number {
  return count > 0 ? tick % count : 0;
}

/**
 * The raw global tick. Callers add a stable per-critter phase offset before
 * taking `% count`, so the fleet shares one clock but doesn't stutter in
 * lockstep — each critter animates on its own beat.
 */
export function animTick(): number {
  return tick;
}
