/**
 * TerminalDock — bottom dock hosting a real xterm.js terminal connected to
 * the Go server's pod-exec WebSocket.
 *
 * Protocol (JSON text frames):
 *   send    {type:"stdin",data}  {type:"resize",cols,rows}  {type:"ping"}
 *   receive {type:"stdout",data} {type:"connected"} {type:"exit"} {type:"error",...}
 *
 * The dock takes layout space (~40vh) below the main view; it unmounts (and
 * tears the socket down) when the store's terminalSession is cleared. In demo
 * mode the server answers with an error frame, which we print gracefully.
 *
 * Look: a floating, gold-glowing "pane" — litewindow's cockpit-terminal polish
 * adapted to the Yscale language (sharp 3px radius, kubagachi's gold accent).
 * The chrome (pane glow, slim scrollbar, dock-in motion) lives in index.css
 * under `.term-pane` / `.kubagachi-dock-in`.
 */

import { useEffect, useRef, useState } from "react";
import { Terminal as TerminalIcon, X } from "lucide-react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";
import {
  useTerminalSession,
  workspaceActions,
  type TerminalSession,
} from "../store/workspace";

const PING_INTERVAL_MS = 30_000;

export default function TerminalDock() {
  const session = useTerminalSession();
  if (!session) return null;
  const key = `${session.namespace}/${session.pod}/${session.container}`;
  return <DockSession key={key} session={session} />;
}

type Status = "connecting" | "connected" | "closed";

function DockSession({ session }: { session: TerminalSession }) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  // `status` drives the connecting banner; `focused` drives the gold pane glow.
  const [status, setStatus] = useState<Status>("connecting");
  const [focused, setFocused] = useState(true);

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: '"JetBrains Mono", ui-monospace, Menlo, monospace',
      fontSize: 12,
      lineHeight: 1.25,
      scrollback: 5000,
      theme: {
        background: "#08090b",
        foreground: "#ecebe4",
        cursor: "#c9b88a",
        cursorAccent: "#08090b",
        selectionBackground: "rgba(201,184,138,0.30)",
        selectionForeground: "#0a0a0a",
        black: "#08090b",
        red: "#d88a8a",
        green: "#7eb87e",
        yellow: "#d4b46a",
        blue: "#8aa3c9",
        magenta: "#c9a3c0",
        cyan: "#8ac0b8",
        white: "#e8e6e0",
        brightBlack: "#5a564e",
        brightRed: "#e0a0a0",
        brightGreen: "#9ec79a",
        brightYellow: "#d8c89a",
        brightBlue: "#a0b8d8",
        brightMagenta: "#d8b8d0",
        brightCyan: "#a0d0c8",
        brightWhite: "#f4f2ec",
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    // Make any URL in the output tappable — opens in a new tab (noopener).
    term.loadAddon(
      new WebLinksAddon((_e, uri) => window.open(uri, "_blank", "noopener")),
    );
    term.open(host);

    let ws: WebSocket | null = null;
    let closed = false;
    let lastCols = -1;
    let lastRows = -1;
    let pingTimer: number | null = null;
    let resizeTimer: number | null = null;

    const sendResize = (): void => {
      if (!ws || ws.readyState !== WebSocket.OPEN) return;
      const { cols, rows } = term;
      if (cols === lastCols && rows === lastRows) return;
      lastCols = cols;
      lastRows = rows;
      ws.send(JSON.stringify({ type: "resize", cols, rows }));
    };

    const doFit = (): void => {
      try {
        fit.fit();
      } catch {
        /* host may be 0-sized mid-layout */
      }
      sendResize();
    };

    const onWindowResize = (): void => {
      if (resizeTimer !== null) window.clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(doFit, 120);
    };

    // The shell is "active" (gold glow) whenever focus lives inside the pane.
    const onFocusIn = (): void => setFocused(true);
    const onFocusOut = (): void => setFocused(false);
    host.addEventListener("focusin", onFocusIn);
    host.addEventListener("focusout", onFocusOut);

    // Initial fit before connecting so we can report a real size.
    try {
      fit.fit();
    } catch {
      /* ignore */
    }

    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const qs = new URLSearchParams({
      namespace: session.namespace,
      pod: session.pod,
      container: session.container,
    });
    const url = `${proto}//${window.location.host}/api/exec?${qs.toString()}`;

    try {
      ws = new WebSocket(url);
    } catch {
      term.writeln("\x1b[31mfailed to open exec socket\x1b[0m");
      term.writeln("\x1b[90m[session ended]\x1b[0m");
      setStatus("closed");
    }

    if (ws) {
      const socket = ws;
      socket.onopen = () => {
        sendResize();
        pingTimer = window.setInterval(() => {
          if (socket.readyState === WebSocket.OPEN) {
            socket.send(JSON.stringify({ type: "ping" }));
          }
        }, PING_INTERVAL_MS);
      };
      socket.onmessage = (ev: MessageEvent<string>) => {
        let frame: { type?: string; data?: string; message?: string; error?: string };
        try {
          frame = JSON.parse(ev.data) as typeof frame;
        } catch {
          return;
        }
        switch (frame.type) {
          case "stdout":
            if (frame.data) term.write(frame.data);
            break;
          case "connected":
            setStatus("connected");
            break;
          case "error":
            term.writeln(`\x1b[31m${frame.message ?? frame.error ?? frame.data ?? "exec error"}\x1b[0m`);
            setStatus("closed");
            break;
          case "exit":
            term.writeln("\r\n\x1b[90m[session ended]\x1b[0m");
            closed = true;
            setStatus("closed");
            break;
          default:
            break;
        }
      };
      socket.onerror = () => {
        if (!closed) term.writeln("\r\n\x1b[31mexec socket error\x1b[0m");
        setStatus("closed");
      };
      socket.onclose = () => {
        if (!closed) {
          term.writeln("\r\n\x1b[90m[session ended]\x1b[0m");
          closed = true;
        }
        setStatus("closed");
      };
    }

    const dataSub = term.onData((data) => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "stdin", data }));
      }
    });

    window.addEventListener("resize", onWindowResize);
    // One more fit after fonts settle / layout stabilizes.
    const settleTimer = window.setTimeout(doFit, 50);
    term.focus();

    return () => {
      window.removeEventListener("resize", onWindowResize);
      host.removeEventListener("focusin", onFocusIn);
      host.removeEventListener("focusout", onFocusOut);
      window.clearTimeout(settleTimer);
      if (pingTimer !== null) window.clearInterval(pingTimer);
      if (resizeTimer !== null) window.clearTimeout(resizeTimer);
      dataSub.dispose();
      try {
        ws?.close();
      } catch {
        /* ignore */
      }
      term.dispose();
    };
  }, [session.namespace, session.pod, session.container]);

  return (
    <div className="shrink-0 h-[40vh] min-h-[200px] p-1.5 sm:p-2 z-20 kubagachi-dock-in">
      <div
        data-focused={focused ? "true" : "false"}
        className="term-pane h-full flex flex-col rounded-md overflow-hidden bg-[#08090b]"
      >
        {/* Head bar — terminal glyph, ns/pod · container, faint close→danger. */}
        <div className="shrink-0 flex items-center gap-2 px-3 py-2 border-b border-border bg-bg-panel/85 font-mono text-[11px]">
          <TerminalIcon size={12} className="text-accent shrink-0" />
          <span className="text-text truncate">
            {session.namespace}/{session.pod}
          </span>
          <span className="text-text-muted truncate">· {session.container}</span>
          <span className="flex-1" />
          <button
            type="button"
            aria-label="Close terminal"
            onClick={() => workspaceActions.closeTerminal()}
            className="inline-flex items-center justify-center h-6 w-6 rounded text-text-muted hover:text-status-error hover:bg-bg-panel2 transition-colors"
          >
            <X size={13} />
          </button>
        </div>

        {/* Body — xterm host with a fade-in "connecting" banner on top. */}
        <div className="relative flex-1 min-h-0">
          <div ref={hostRef} className="absolute inset-0 px-2 py-1.5 overflow-hidden" />
          <div
            aria-hidden={status !== "connecting"}
            className="pointer-events-none absolute inset-0 flex items-center justify-center transition-opacity duration-500"
            style={{ opacity: status === "connecting" ? 1 : 0 }}
          >
            <div className="flex items-center gap-2.5 px-3.5 py-2 rounded-md border border-border bg-bg-panel/90 backdrop-blur-sm font-mono text-[11px] text-text-muted shadow-lg">
              <span className="kubagachi-pip block h-1.5 w-1.5 rounded-full bg-accent" />
              <span>
                connecting to{" "}
                <span className="text-text">
                  {session.namespace}/{session.pod}
                </span>{" "}
                <span className="text-text-muted">({session.container})</span>…
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
