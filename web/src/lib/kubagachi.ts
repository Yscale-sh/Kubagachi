/**
 * Kubagachi — thin helper module.
 *
 * The original "tamagotchi" mood/vitals/care model has been removed. The
 * Kubagachi view is now a pure pod-status mascot view: each pinned pod shows
 * its big animated critter with its literal current pod status. The only
 * actions are real Kubernetes ops (restart / logs / shell) plus unpin.
 *
 * This module exists solely to expose those k8s actions as a single helper
 * so components can render the same action set without reinventing the list.
 */

import type { Pod } from "./types";

export type CareKind = "restart" | "logs" | "shell" | "unpin";

export interface CareAction {
  label: string;
  kind: CareKind;
}

/**
 * Returns the always-available pod actions (k8s ops only — no feed/pet/play).
 * The `pod` parameter is currently unused but kept so this can grow into a
 * context-aware list (e.g. hide Shell when the pod isn't Running).
 */
export function careActions(_pod: Pod): CareAction[] {
  return [
    { label: "Restart", kind: "restart" },
    { label: "Logs", kind: "logs" },
    { label: "Shell", kind: "shell" },
    { label: "Unpin", kind: "unpin" },
  ];
}
