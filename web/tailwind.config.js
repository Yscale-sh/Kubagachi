/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    // Yscale: sharp radii everywhere — panels and buttons sit at 2-3px. The
    // ramp is intentionally capped at 3px so a stray rounded-xl can't quietly
    // soften the sharp TUI identity.
    borderRadius: {
      none: "0",
      sm: "2px",
      DEFAULT: "2px",
      md: "3px",
      lg: "3px",
      xl: "3px",
      "2xl": "3px",
      "3xl": "3px",
      full: "9999px",
    },
    extend: {
      colors: {
        bg: {
          base: "#0a0a0a",
          panel: "#141414",
          panel2: "#1c1c1c",
        },
        border: {
          DEFAULT: "#262626",
          strong: "#3a3a3a",
        },
        text: {
          DEFAULT: "#e8e6e0",
          muted: "#8a857a",
        },
        accent: {
          DEFAULT: "#c9b88a",
          bright: "#d8c89a",
          // Fill tier — the gold finally gets to be a *surface*, not just a
          // hover hint. `dim` for chip/active fills, `soft` for hairlines.
          dim: "rgba(201, 184, 138, 0.14)",
          soft: "rgba(201, 184, 138, 0.45)",
        },
        // TUI palette — matches the KUBE-TUI mockup: cyan section headers,
        // pink pod-detail names, and vivid terminal status colors.
        tui: {
          cyan: "#5db8e8",
          pink: "#e07b9a",
        },
        status: {
          running: "#5ec46b",
          pending: "#e0b83a",
          completed: "#4ec8c8",
          crashloop: "#e05a5a",
          backoff: "#e0903a",
          terminating: "#9a9a9a",
          unknown: "#6a6a6a",
          error: "#e05a5a",
        },
      },
      fontFamily: {
        sans: ['"Inter Tight"', "system-ui", "-apple-system", "sans-serif"],
        mono: [
          '"JetBrains Mono"',
          "ui-monospace",
          "SFMono-Regular",
          "Menlo",
          "Monaco",
          "Consolas",
          "monospace",
        ],
        serif: ['"Fraunces"', "Georgia", "serif"],
      },
      transitionTimingFunction: {
        DEFAULT: "cubic-bezier(0.16, 1, 0.3, 1)",
        yscale: "cubic-bezier(0.16, 1, 0.3, 1)",
      },
      transitionDuration: {
        DEFAULT: "220ms",
      },
    },
  },
  plugins: [],
};
