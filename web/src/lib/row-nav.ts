/**
 * row-nav — tiny registry that lets the global keyboard layer (j/k/enter/g/G)
 * talk to whichever table view is currently mounted, without the store having
 * to know row counts.
 *
 * The active view registers its visible row ids (in render order) plus an
 * onEnter handler; the keyboard layer reads the registry to clamp the cursor
 * and to activate the selected row.
 */

export interface RowNavRegistration {
  /** Stable ids for the visible rows, in render order. */
  ids: string[];
  /** Called when the user presses Enter on the selected row. */
  onEnter: (id: string, index: number) => void;
}

let current: RowNavRegistration | null = null;

export function registerRowNav(reg: RowNavRegistration): void {
  current = reg;
}

export function clearRowNav(reg?: RowNavRegistration): void {
  // Only clear if the unmounting view still owns the registration.
  if (!reg || current === reg) current = null;
}

export function getRowNav(): RowNavRegistration | null {
  return current;
}

export function rowNavCount(): number {
  return current?.ids.length ?? 0;
}

/** Scroll the currently-selected row into view (called after j/k moves). */
export function scrollSelectedRowIntoView(): void {
  requestAnimationFrame(() => {
    const el = document.querySelector<HTMLElement>('[data-row-selected="true"]');
    el?.scrollIntoView({ block: "nearest" });
  });
}
