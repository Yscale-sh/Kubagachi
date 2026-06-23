/**
 * ConfirmButton — a destructive-action button with an inline two-step confirm,
 * matching the k9s/TUI vibe (no modal). First click "arms" it (label flips to
 * `confirmLabel` for ~3s); a second click within that window fires `onConfirm`.
 * Clicking elsewhere / waiting disarms it.
 */

import { useEffect, useRef, useState } from "react";

interface ConfirmButtonProps {
  onConfirm: () => void;
  label: React.ReactNode;
  /** Shown while armed; defaults to "confirm?". */
  confirmLabel?: React.ReactNode;
  className?: string;
  /** Optional class applied only while armed (e.g. a louder danger style). */
  armedClassName?: string;
  title?: string;
  "aria-label"?: string;
}

export default function ConfirmButton({
  onConfirm,
  label,
  confirmLabel = "confirm?",
  className = "",
  armedClassName,
  title,
  "aria-label": ariaLabel,
}: ConfirmButtonProps) {
  const [armed, setArmed] = useState(false);
  const timer = useRef<number | null>(null);

  useEffect(
    () => () => {
      if (timer.current) window.clearTimeout(timer.current);
    },
    [],
  );

  const onClick: React.MouseEventHandler<HTMLButtonElement> = (e) => {
    e.stopPropagation();
    if (!armed) {
      setArmed(true);
      timer.current = window.setTimeout(() => setArmed(false), 3000);
      return;
    }
    if (timer.current) window.clearTimeout(timer.current);
    setArmed(false);
    onConfirm();
  };

  return (
    <button
      type="button"
      title={title}
      aria-label={ariaLabel}
      onClick={onClick}
      className={`${className} ${armed && armedClassName ? armedClassName : ""}`}
    >
      {armed ? confirmLabel : label}
    </button>
  );
}
