'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import {
  Terminal as TerminalIcon,
  Plus,
  X,
  Wifi,
  WifiOff,
  Loader2,
  ChevronLeft,
  ChevronRight,
  Monitor,
  Maximize2,
  Minimize2,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { brand } from '@/lib/brand';

// ---------------------------------------------------------------------------
// Dynamic import for xterm (SSR-safe)
// ---------------------------------------------------------------------------
let XTerminal: any = null;
let FitAddon: any = null;
let WebLinksAddon: any = null;

async function loadXterm() {
  if (typeof window === 'undefined') return;
  const xterm = await import('xterm');
  const fit = await import('xterm-addon-fit');
  const webLinks = await import('xterm-addon-web-links');
  await import('xterm/css/xterm.css');
  XTerminal = xterm.Terminal;
  FitAddon = fit.FitAddon;
  WebLinksAddon = webLinks.WebLinksAddon;
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
type ConnectionStatus = 'connecting' | 'connected' | 'disconnected';

interface TerminalSession {
  id: string;
  name: string;
  terminal: any | null;
  fitAddon: any | null;
  ws: WebSocket | null;
  status: ConnectionStatus;
  createdAt: Date;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function generateId(): string {
  return `term-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function getWsUrl(sessionId: string): string {
  if (typeof window === 'undefined') return '';
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  let token = '';
  try {
    token = window.localStorage.getItem('access_token') || '';
  } catch {
    token = '';
  }
  return `${protocol}//${host}/api/ws?token=${encodeURIComponent(token)}&session=${sessionId}`;
}

function formatTime(date: Date): string {
  try {
    return date.toLocaleTimeString('en-US', {
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    });
  } catch {
    return '';
  }
}

// ---------------------------------------------------------------------------
// TerminalTheme — the console surface stays dark on purpose (terminal is the
// one exception to the light panel palette); everything around it is light.
// ---------------------------------------------------------------------------
const TERMINAL_THEME = {
  background: '#0f172a',
  foreground: '#e2e8f0',
  cursor: '#3b82f6',
  cursorAccent: '#0f172a',
  selectionBackground: '#1e40af55',
  selectionForeground: '#f8fafc',
  black: '#1e293b',
  red: '#ef4444',
  green: '#22c55e',
  yellow: '#eab308',
  blue: '#3b82f6',
  magenta: '#a855f7',
  cyan: '#06b6d4',
  white: '#e2e8f0',
  brightBlack: '#475569',
  brightRed: '#f87171',
  brightGreen: '#4ade80',
  brightYellow: '#facc15',
  brightBlue: '#60a5fa',
  brightMagenta: '#c084fc',
  brightCyan: '#22d3ee',
  brightWhite: '#f8fafc',
};

// ---------------------------------------------------------------------------
// StatusIndicator
// ---------------------------------------------------------------------------
function StatusIndicator({ status }: { status: ConnectionStatus }) {
  const config = {
    connecting: {
      dot: 'bg-amber-500',
      icon: <Loader2 className="h-3 w-3 animate-spin text-amber-600" />,
      label: 'Connecting',
      text: 'text-amber-700',
    },
    connected: {
      dot: 'bg-emerald-500',
      icon: <Wifi className="h-3 w-3 text-emerald-600" />,
      label: 'Connected',
      text: 'text-emerald-700',
    },
    disconnected: {
      dot: 'bg-red-500',
      icon: <WifiOff className="h-3 w-3 text-red-600" />,
      label: 'Disconnected',
      text: 'text-red-700',
    },
  };

  const c = config[status];

  return (
    <div className="flex items-center gap-1.5">
      <span className={cn('h-2 w-2 rounded-full', c.dot)} />
      <span className={cn('text-xs font-medium', c.text)}>{c.label}</span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// TerminalPage
// ---------------------------------------------------------------------------
export default function TerminalPage() {
  const [sessions, setSessions] = useState<TerminalSession[]>([]);
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [xtermLoaded, setXtermLoaded] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [loadError, setLoadError] = useState('');

  const terminalContainerRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const sessionCounterRef = useRef(0);
  const sessionsRef = useRef<TerminalSession[]>([]);

  // Keep ref in sync
  useEffect(() => {
    sessionsRef.current = sessions;
  }, [sessions]);

  // Load xterm.js on mount
  useEffect(() => {
    let cancelled = false;
    loadXterm()
      .then(() => {
        if (!cancelled) setXtermLoaded(true);
      })
      .catch((err: any) => {
        console.error('Failed to load terminal engine:', err);
        if (!cancelled) {
          setLoadError(err?.message || 'Failed to load the terminal engine.');
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // ── Create a new terminal session ──────────────────────────────────────
  const createSession = useCallback(
    (autoConnect = true) => {
      if (!xtermLoaded || !XTerminal) return;

      sessionCounterRef.current += 1;
      const id = generateId();
      const name = `Session ${sessionCounterRef.current}`;

      const term = new XTerminal({
        theme: TERMINAL_THEME,
        fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', Menlo, monospace",
        fontSize: 14,
        lineHeight: 1.2,
        cursorBlink: true,
        cursorStyle: 'bar',
        cursorWidth: 2,
        scrollback: 10000,
        tabStopWidth: 4,
        bellStyle: 'none',
        allowTransparency: true,
        convertEol: true,
      });

      const fitAddonInstance = new FitAddon();
      const webLinksAddonInstance = new WebLinksAddon();
      term.loadAddon(fitAddonInstance);
      term.loadAddon(webLinksAddonInstance);

      const session: TerminalSession = {
        id,
        name,
        terminal: term,
        fitAddon: fitAddonInstance,
        ws: null,
        status: 'disconnected',
        createdAt: new Date(),
      };

      setSessions((prev) => [...prev, session]);
      setActiveSessionId(id);

      if (autoConnect) {
        setTimeout(() => connectSession(id), 50);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [xtermLoaded],
  );

  // ── Connect a session's WebSocket ──────────────────────────────────────
  const connectSession = useCallback((sessionId: string) => {
    const session = sessionsRef.current.find((s) => s.id === sessionId);
    if (!session || !session.terminal) return;

    // Mark connecting
    setSessions((prev) =>
      prev.map((s) =>
        s.id === sessionId ? { ...s, status: 'connecting' as ConnectionStatus } : s,
      ),
    );

    const wsUrl = getWsUrl(sessionId);
    if (!wsUrl) return;

    let ws: WebSocket;
    try {
      ws = new WebSocket(wsUrl);
    } catch (err) {
      console.error('Failed to open terminal socket:', err);
      setSessions((prev) =>
        prev.map((s) =>
          s.id === sessionId ? { ...s, status: 'disconnected' as ConnectionStatus } : s,
        ),
      );
      session.terminal.writeln('\x1b[1;31m[Connection error]\x1b[0m');
      return;
    }

    ws.onopen = () => {
      setSessions((prev) =>
        prev.map((s) =>
          s.id === sessionId ? { ...s, status: 'connected' as ConnectionStatus, ws } : s,
        ),
      );

      // Join terminal room
      ws.send(
        JSON.stringify({
          type: 'join_room',
          payload: { room: `terminal:${sessionId}` },
        }),
      );

      const bannerText = `${brand.productName} Terminal — Đã kết nối phiên làm việc`;
      const bannerRule = '═'.repeat(bannerText.length + 2);
      session.terminal.writeln(`\x1b[1;34m╔${bannerRule}╗\x1b[0m`);
      session.terminal.writeln(
        `\x1b[1;34m║\x1b[0m \x1b[1;36m${bannerText}\x1b[0m \x1b[1;34m║\x1b[0m`,
      );
      session.terminal.writeln(`\x1b[1;34m╚${bannerRule}╝\x1b[0m`);
      session.terminal.writeln('');
    };

    ws.onmessage = (event: MessageEvent) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg?.type === 'terminal_output' && msg?.payload?.data) {
          session.terminal.write(msg.payload.data);
        } else if (msg?.type === 'error') {
          session.terminal.writeln(
            `\x1b[1;31mError: ${msg?.payload?.message || 'Unknown error'}\x1b[0m`,
          );
        }
      } catch {
        // Raw terminal data
        session.terminal.write(event.data);
      }
    };

    ws.onclose = () => {
      setSessions((prev) =>
        prev.map((s) =>
          s.id === sessionId ? { ...s, status: 'disconnected' as ConnectionStatus, ws: null } : s,
        ),
      );
      session.terminal.writeln('');
      session.terminal.writeln('\x1b[1;31m[Connection closed]\x1b[0m');
    };

    ws.onerror = () => {
      setSessions((prev) =>
        prev.map((s) =>
          s.id === sessionId ? { ...s, status: 'disconnected' as ConnectionStatus } : s,
        ),
      );
      session.terminal.writeln('\x1b[1;31m[Connection error]\x1b[0m');
    };

    // Wire terminal input → WebSocket
    session.terminal.onData((data: string) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({
            type: 'terminal_input',
            room: `terminal:${sessionId}`,
            payload: { data },
          }),
        );
      }
    });

    // Wire terminal resize → WebSocket
    session.terminal.onResize(({ cols, rows }: { cols: number; rows: number }) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({
            type: 'terminal_resize',
            room: `terminal:${sessionId}`,
            payload: { cols, rows },
          }),
        );
      }
    });

    setSessions((prev) =>
      prev.map((s) => (s.id === sessionId ? { ...s, ws } : s)),
    );
  }, []);

  // ── Mount terminal to DOM ──────────────────────────────────────────────
  useEffect(() => {
    if (!activeSessionId) return;
    const session = sessions.find((s) => s.id === activeSessionId);
    if (!session || !session.terminal) return;

    const container = terminalContainerRefs.current.get(activeSessionId);
    if (!container) return;

    // Only open if not already attached
    if (container.childElementCount === 0) {
      session.terminal.open(container);
      requestAnimationFrame(() => {
        try {
          session.fitAddon?.fit();
        } catch {
          /* ignore */
        }
        session.terminal.focus();
      });
    }
  }, [activeSessionId, sessions]);

  // ── Handle window resize ───────────────────────────────────────────────
  useEffect(() => {
    const handleResize = () => {
      if (!activeSessionId) return;
      const session = sessionsRef.current.find((s) => s.id === activeSessionId);
      if (session?.fitAddon) {
        try {
          session.fitAddon.fit();
        } catch {
          /* ignore fit errors during transitions */
        }
      }
    };

    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, [activeSessionId]);

  // ── Close a session ────────────────────────────────────────────────────
  const closeSession = useCallback(
    (sessionId: string) => {
      const session = sessionsRef.current.find((s) => s.id === sessionId);
      if (!session) return;

      if (session.ws) session.ws.close();
      if (session.terminal) {
        try {
          session.terminal.dispose();
        } catch {
          /* ignore dispose errors */
        }
      }

      setSessions((prev) => {
        const remaining = prev.filter((s) => s.id !== sessionId);
        if (activeSessionId === sessionId) {
          setActiveSessionId(remaining.length > 0 ? remaining[remaining.length - 1].id : null);
        }
        return remaining;
      });
    },
    [activeSessionId],
  );

  // ── Reconnect a session ────────────────────────────────────────────────
  const reconnectSession = useCallback(
    (sessionId: string) => {
      const session = sessionsRef.current.find((s) => s.id === sessionId);
      if (!session) return;

      if (session.ws) session.ws.close();
      session.terminal?.clear();
      connectSession(sessionId);
    },
    [connectSession],
  );

  // ── Create first session on xterm load ─────────────────────────────────
  useEffect(() => {
    if (xtermLoaded && sessions.length === 0) {
      createSession();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [xtermLoaded]);

  // ── Fullscreen toggle ──────────────────────────────────────────────────
  const toggleFullscreen = useCallback(() => {
    try {
      if (!document.fullscreenElement) {
        document.documentElement.requestFullscreen();
        setIsFullscreen(true);
      } else {
        document.exitFullscreen();
        setIsFullscreen(false);
      }
    } catch {
      /* fullscreen may be blocked by the browser */
    }
  }, []);

  useEffect(() => {
    const handler = () => setIsFullscreen(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', handler);
    return () => document.removeEventListener('fullscreenchange', handler);
  }, []);

  const activeSession = sessions.find((s) => s.id === activeSessionId);

  // ── Load error state ───────────────────────────────────────────────────
  if (loadError) {
    return (
      <div className="flex items-center justify-center h-[calc(100vh-8rem)]">
        <div
          role="alert"
          className="max-w-md rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
        >
          {loadError}
        </div>
      </div>
    );
  }

  // ── Loading state ──────────────────────────────────────────────────────
  if (!xtermLoaded) {
    return (
      <div className="flex items-center justify-center h-[calc(100vh-8rem)]">
        <div className="flex flex-col items-center gap-3">
          <Loader2 className="h-8 w-8 animate-spin text-blue-600" aria-hidden="true" />
          <p className="text-sm text-gray-600">Loading terminal...</p>
        </div>
      </div>
    );
  }

  // ── Render ─────────────────────────────────────────────────────────────
  return (
    <div
      className={cn(
        'flex flex-col border border-gray-200 bg-white overflow-hidden',
        isFullscreen
          ? 'fixed inset-0 z-50'
          : 'h-[calc(100vh-8rem)] rounded-lg shadow-sm',
      )}
    >
      {/* ── Top Bar ─────────────────────────────────────────────────────── */}
      <div className="flex items-center justify-between border-b border-gray-200 bg-white px-4 py-2.5">
        <div className="flex items-center gap-3">
          <TerminalIcon className="h-4 w-4 text-gray-600" aria-hidden="true" />
          <h1 className="text-sm font-semibold text-gray-900">Web Terminal</h1>
          {activeSession && <StatusIndicator status={activeSession.status} />}
        </div>

        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => createSession()}
            className="h-8 gap-1.5 border-gray-300 bg-white text-xs text-gray-700 hover:bg-gray-50"
          >
            <Plus className="h-3.5 w-3.5" aria-hidden="true" />
            New Session
          </Button>
          <div className="h-4 w-px bg-gray-200" />
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="h-8 w-8 p-0 text-gray-600 hover:bg-gray-100 hover:text-gray-900"
            aria-label={sidebarOpen ? 'Hide session sidebar' : 'Show session sidebar'}
            title={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
          >
            {sidebarOpen ? (
              <ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />
            ) : (
              <ChevronLeft className="h-3.5 w-3.5" aria-hidden="true" />
            )}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={toggleFullscreen}
            className="h-8 w-8 p-0 text-gray-600 hover:bg-gray-100 hover:text-gray-900"
            aria-label={isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
            title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}
          >
            {isFullscreen ? (
              <Minimize2 className="h-3.5 w-3.5" aria-hidden="true" />
            ) : (
              <Maximize2 className="h-3.5 w-3.5" aria-hidden="true" />
            )}
          </Button>
        </div>
      </div>

      <div className="flex flex-1 overflow-hidden">
        {/* ── Sidebar ──────────────────────────────────────────────────── */}
        {sidebarOpen && (
          <div className="w-64 flex-shrink-0 border-r border-gray-200 bg-white flex flex-col">
            <div className="px-3 py-2.5 border-b border-gray-200 bg-gray-50">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                  Sessions
                </span>
                <span className="text-xs text-gray-500">{sessions.length}</span>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto p-2 space-y-1">
              {sessions.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-8 text-gray-500">
                  <Monitor className="h-8 w-8 mb-2 text-gray-300" aria-hidden="true" />
                  <p className="text-xs">No active sessions</p>
                </div>
              ) : (
                sessions.map((session) => (
                  <div
                    key={session.id}
                    onClick={() => setActiveSessionId(session.id)}
                    className={cn(
                      'group flex items-center gap-2 rounded-md px-3 py-2 cursor-pointer border',
                      session.id === activeSessionId
                        ? 'bg-blue-50 border-blue-200 text-blue-700'
                        : 'border-transparent text-gray-700 hover:bg-gray-50',
                    )}
                  >
                    <span
                      className={cn(
                        'h-2 w-2 rounded-full flex-shrink-0',
                        session.status === 'connected'
                          ? 'bg-emerald-500'
                          : session.status === 'connecting'
                            ? 'bg-amber-500'
                            : 'bg-red-500',
                      )}
                    />
                    <div className="flex-1 min-w-0">
                      <p className="text-xs font-medium truncate">{session.name}</p>
                      <p className="text-[10px] text-gray-500" suppressHydrationWarning>
                        {formatTime(session.createdAt)}
                      </p>
                    </div>
                    <div className="flex items-center gap-1">
                      {session.status === 'disconnected' && (
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            reconnectSession(session.id);
                          }}
                          aria-label={`Reconnect ${session.name}`}
                          className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-emerald-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                          title="Reconnect"
                        >
                          <Wifi className="h-3 w-3" aria-hidden="true" />
                        </button>
                      )}
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          closeSession(session.id);
                        }}
                        aria-label={`Close ${session.name}`}
                        className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-red-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                        title="Close session"
                      >
                        <X className="h-3 w-3" aria-hidden="true" />
                      </button>
                    </div>
                  </div>
                ))
              )}
            </div>

            <div className="p-3 border-t border-gray-200">
              <Button
                variant="outline"
                size="sm"
                onClick={() => createSession()}
                className="w-full h-8 text-xs border-gray-300 bg-white text-gray-700 hover:bg-gray-50"
              >
                <Plus className="h-3.5 w-3.5 mr-1.5" aria-hidden="true" />
                New Session
              </Button>
            </div>
          </div>
        )}

        {/* ── Terminal Area ────────────────────────────────────────────── */}
        <div className="flex-1 flex flex-col overflow-hidden bg-white">
          {/* Session Tabs (only when multiple sessions) */}
          {sessions.length > 1 && (
            <div className="flex items-center border-b border-gray-200 bg-gray-50 overflow-x-auto">
              {sessions.map((session) => (
                <div
                  key={session.id}
                  onClick={() => setActiveSessionId(session.id)}
                  className={cn(
                    'group flex items-center gap-2 px-4 py-2 text-xs cursor-pointer border-r border-gray-200 min-w-[120px]',
                    session.id === activeSessionId
                      ? 'bg-white text-gray-900 border-b-2 border-b-blue-600'
                      : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900',
                  )}
                >
                  <span
                    className={cn(
                      'h-1.5 w-1.5 rounded-full flex-shrink-0',
                      session.status === 'connected'
                        ? 'bg-emerald-500'
                        : session.status === 'connecting'
                          ? 'bg-amber-500'
                          : 'bg-red-500',
                    )}
                  />
                  <span className="truncate flex-1">{session.name}</span>
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      closeSession(session.id);
                    }}
                    aria-label={`Close ${session.name}`}
                    className="rounded-md p-0.5 text-gray-500 hover:bg-gray-200 hover:text-red-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                  >
                    <X className="h-3 w-3" aria-hidden="true" />
                  </button>
                </div>
              ))}
            </div>
          )}

          {/* Terminal containers */}
          <div className="flex-1 relative overflow-hidden bg-slate-900">
            {sessions.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-full bg-white text-gray-600">
                <TerminalIcon className="h-12 w-12 mb-4 text-gray-300" aria-hidden="true" />
                <p className="text-sm font-semibold text-gray-900 mb-1">No terminal sessions</p>
                <p className="text-sm text-gray-600 mb-5">Create a new session to get started</p>
                <Button
                  onClick={() => createSession()}
                  className="gap-2 bg-blue-600 text-white hover:bg-blue-700"
                >
                  <Plus className="h-4 w-4" aria-hidden="true" />
                  New Session
                </Button>
              </div>
            ) : (
              sessions.map((session) => (
                <div
                  key={session.id}
                  ref={(el) => {
                    if (el) {
                      terminalContainerRefs.current.set(session.id, el);
                    } else {
                      terminalContainerRefs.current.delete(session.id);
                    }
                  }}
                  className={cn(
                    'absolute inset-0 p-2',
                    session.id === activeSessionId ? 'visible' : 'invisible',
                  )}
                  style={{ height: '100%', width: '100%' }}
                />
              ))
            )}
          </div>

          {/* Bottom Status Bar */}
          {activeSession && (
            <div className="flex items-center justify-between border-t border-gray-200 bg-white px-4 py-2">
              <div className="flex items-center gap-4 text-[11px] text-gray-500">
                <span className="flex items-center gap-1.5">
                  <TerminalIcon className="h-3 w-3" aria-hidden="true" />
                  {activeSession.name}
                </span>
                <span>
                  {activeSession.terminal
                    ? `${activeSession.terminal.cols}×${activeSession.terminal.rows}`
                    : '—'}
                </span>
              </div>
              <div className="flex items-center gap-4 text-[11px] text-gray-500">
                {activeSession.status === 'disconnected' && (
                  <button
                    type="button"
                    onClick={() => reconnectSession(activeSession.id)}
                    className="flex items-center gap-1 rounded-md px-1.5 py-0.5 font-medium text-blue-700 hover:bg-blue-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                  >
                    <Wifi className="h-3 w-3" aria-hidden="true" />
                    Reconnect
                  </button>
                )}
                <span>{brand.productName} Terminal</span>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
