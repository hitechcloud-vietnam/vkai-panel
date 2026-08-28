'use client';

/**
 * The FTP daemon on this machine: what it is, whether it is running, and the
 * five things the panel can actually do to it.
 *
 * This is the one part of the FTP screen that is fully wired. The systemd
 * routes exist and are mounted (router.go, the /services group behind
 * RequireAdmin), and service_manager.go's `alwaysManageable` map lists
 * vsftpd, proftpd and pure-ftpd by name, so start, stop, restart, enable and
 * disable all reach a real systemctl call. Any other unit is refused
 * server-side with `service %q is not managed by this panel`, which is why the
 * controls appear only for a unit this screen recognised from that list.
 *
 * The routes are administrator-only. A non-administrator gets a 403 with a
 * message, and the message is what this card shows - not an empty card that
 * looks like a machine with no FTP server on it.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import { CircleDot, FileTerminal, Power, RotateCw, Square } from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { errorMessage } from '@/lib/apiError';
import { formatBytes } from '@/lib/utils';

import {
  getServiceLogs,
  getServiceStatus,
  listServices,
  runServiceAction,
  type ServiceAction,
} from './api';
import {
  CardSkeleton,
  DANGER_BUTTON_CLASS,
  EmptyState,
  ErrorBlock,
  Panel,
  PaneToolbar,
  PRIMARY_BUTTON_CLASS,
  SECONDARY_BUTTON_CLASS,
  SectionHeader,
} from './PaneChrome';
import {
  FTP_DAEMONS,
  isManageableFtpUnit,
  MANAGEABLE_FTP_UNITS,
  type FtpUnitName,
  type ServiceInfo,
} from './types';

const LOG_LINES = 120;

/** systemd's ActiveState, in the panel's status colours. */
function StateBadge({ state }: { state: string }) {
  const value = (state || '').toLowerCase();
  if (value === 'active') return <Badge variant="success">active</Badge>;
  if (value === 'activating' || value === 'reloading' || value === 'deactivating') {
    return <Badge variant="warning">{value}</Badge>;
  }
  if (value === 'failed') return <Badge variant="danger">failed</Badge>;
  if (value === '') return <Badge variant="neutral">unknown</Badge>;
  return <Badge variant="neutral">{value}</Badge>;
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-3 border-b border-gray-100 py-2 last:border-b-0">
      <dt className="w-40 shrink-0 text-xs font-medium uppercase tracking-wide text-gray-500">
        {label}
      </dt>
      <dd className="min-w-0 flex-1 text-sm text-gray-900">{children}</dd>
    </div>
  );
}

export function DaemonCard({ onUnitChange }: { onUnitChange?: (unit: string | null) => void }) {
  const [units, setUnits] = useState<FtpUnitName[]>([]);
  const [unit, setUnit] = useState<FtpUnitName | null>(null);
  const [status, setStatus] = useState<ServiceInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [pending, setPending] = useState<ServiceAction | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionNote, setActionNote] = useState<string | null>(null);
  const [confirmStop, setConfirmStop] = useState(false);

  const [logs, setLogs] = useState<string | null>(null);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsError, setLogsError] = useState<string | null>(null);

  const load = useCallback(
    async (isRefresh = false) => {
      if (isRefresh) setRefreshing(true);
      else setLoading(true);
      setError(null);
      try {
        const services = await listServices();
        const found = services
          .map((service) => (service.name || '').replace(/\.service$/, ''))
          .filter(isManageableFtpUnit);
        const unique = Array.from(new Set(found));
        setUnits(unique);
        const chosen = unique[0] ?? null;
        setUnit(chosen);
        onUnitChange?.(chosen);
        setStatus(chosen ? await getServiceStatus(chosen) : null);
      } catch (err) {
        setError(errorMessage(err, 'Could not read the services on this machine.'));
        setUnits([]);
        setUnit(null);
        setStatus(null);
        onUnitChange?.(null);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [onUnitChange]
  );

  useEffect(() => {
    void load();
  }, [load]);

  const act = useCallback(
    async (action: ServiceAction) => {
      if (!unit) return;
      setPending(action);
      setActionError(null);
      setActionNote(null);
      try {
        await runServiceAction(unit, action);
        setActionNote(`${unit}: ${action} completed.`);
        setStatus(await getServiceStatus(unit));
      } catch (err) {
        setActionError(errorMessage(err, `Could not ${action} ${unit}.`));
      } finally {
        setPending(null);
        setConfirmStop(false);
      }
    },
    [unit]
  );

  const loadLogs = useCallback(async () => {
    if (!unit) return;
    setLogsLoading(true);
    setLogsError(null);
    try {
      setLogs(await getServiceLogs(unit, LOG_LINES));
    } catch (err) {
      setLogsError(errorMessage(err, `Could not read the journal for ${unit}.`));
    } finally {
      setLogsLoading(false);
    }
  }, [unit]);

  const daemon = unit ? FTP_DAEMONS[unit] : null;
  const activeState = (status?.active_state || status?.status || '').toLowerCase();
  const running = activeState === 'active';
  const busy = pending !== null;

  const memory = useMemo(() => {
    const value = status?.memory;
    // systemd reports MemoryCurrent as [not set] for a stopped unit, which
    // Sscanf leaves at 0 - not the same as a unit using no memory.
    if (typeof value !== 'number' || value <= 0) return null;
    return formatBytes(value);
  }, [status?.memory]);

  return (
    <Panel>
      <SectionHeader
        title="FTP server"
        description="The daemon this panel can start and stop through systemd."
        actions={<PaneToolbar onRefresh={() => void load(true)} refreshing={refreshing} />}
      />

      {loading ? (
        <CardSkeleton rows={4} />
      ) : error ? (
        <ErrorBlock
          title="Could not read the service list"
          message={error}
          onRetry={() => void load(true)}
        />
      ) : !unit ? (
        <EmptyState
          title="No FTP server on this machine"
          description={`None of ${MANAGEABLE_FTP_UNITS.join(', ')} appears in systemd on this host. The panel has no package endpoint, so the daemon has to be installed over SSH - then refresh this pane and the controls appear.`}
        />
      ) : (
        <>
          <div className="px-4 py-3">
            <dl className="m-0">
              <DetailRow label="Unit">
                <span className="font-mono text-sm">{unit}.service</span>
                {units.length > 1 && (
                  <span className="ml-2 text-xs text-amber-700">
                    {units.length} FTP daemons are installed ({units.join(', ')}). Two
                    daemons fighting over port 21 is its own support ticket.
                  </span>
                )}
              </DetailRow>
              <DetailRow label="State">
                <span className="inline-flex items-center gap-2">
                  <StateBadge state={activeState} />
                  {status?.sub_state && (
                    <span className="text-xs text-gray-500">{status.sub_state}</span>
                  )}
                </span>
              </DetailRow>
              <DetailRow label="Description">
                {status?.description ? (
                  status.description
                ) : (
                  <span className="text-gray-500">Not reported by systemd.</span>
                )}
              </DetailRow>
              <DetailRow label="Main PID">
                {status?.pid ? (
                  <span className="font-mono text-sm">{status.pid}</span>
                ) : (
                  <span className="text-gray-500">Not running.</span>
                )}
              </DetailRow>
              <DetailRow label="Memory">
                {memory ?? <span className="text-gray-500">Not reported.</span>}
              </DetailRow>
              <DetailRow label="Configuration">
                <span className="font-mono text-xs">{daemon?.configPath}</span>
                <span className="ml-2 text-xs text-gray-500">
                  Read and edited on the machine. No panel endpoint exposes it.
                </span>
              </DetailRow>
            </dl>
          </div>

          <div className="flex flex-wrap items-center gap-2 border-t border-gray-200 px-4 py-3">
            <Button
              type="button"
              size="sm"
              onClick={() => void act('start')}
              disabled={busy || running}
              className={PRIMARY_BUTTON_CLASS}
            >
              <Power className="h-4 w-4" aria-hidden="true" />
              {pending === 'start' ? 'Starting…' : 'Start'}
            </Button>

            {confirmStop ? (
              <span className="inline-flex flex-wrap items-center gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-1.5">
                <span className="text-sm text-red-800">
                  Stop {unit}? Every connected client is dropped mid-transfer.
                </span>
                <Button
                  type="button"
                  size="sm"
                  onClick={() => void act('stop')}
                  disabled={busy}
                  className={DANGER_BUTTON_CLASS}
                >
                  {pending === 'stop' ? 'Stopping…' : 'Stop it'}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setConfirmStop(false)}
                  disabled={busy}
                  className={SECONDARY_BUTTON_CLASS}
                >
                  Cancel
                </Button>
              </span>
            ) : (
              <Button
                type="button"
                size="sm"
                variant="danger-outline"
                onClick={() => setConfirmStop(true)}
                disabled={busy || !running}
              >
                <Square className="h-4 w-4" aria-hidden="true" />
                Stop
              </Button>
            )}

            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void act('restart')}
              disabled={busy}
              className={SECONDARY_BUTTON_CLASS}
            >
              <RotateCw className="h-4 w-4" aria-hidden="true" />
              {pending === 'restart' ? 'Restarting…' : 'Restart'}
            </Button>

            <span className="mx-1 hidden h-5 w-px bg-gray-200 sm:inline-block" aria-hidden="true" />

            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void act('enable')}
              disabled={busy}
              className={SECONDARY_BUTTON_CLASS}
              title="Start this unit automatically on boot."
            >
              <CircleDot className="h-4 w-4" aria-hidden="true" />
              {pending === 'enable' ? 'Enabling…' : 'Enable on boot'}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void act('disable')}
              disabled={busy}
              className={SECONDARY_BUTTON_CLASS}
              title="Leave this unit stopped after a reboot."
            >
              {pending === 'disable' ? 'Disabling…' : 'Disable on boot'}
            </Button>

            <span className="text-xs text-gray-500">
              systemd does not report whether a unit is enabled, so the panel cannot show
              the current boot setting - only change it.
            </span>
          </div>

          {actionError && (
            <ErrorBlock title="The action failed" message={actionError} onRetry={() => void load(true)} />
          )}
          {actionNote && !actionError && (
            <p className="mx-4 mb-3 rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
              {actionNote}
            </p>
          )}

          <div className="border-t border-gray-200 px-4 py-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="min-w-0">
                <h4 className="text-sm font-semibold text-gray-900">Recent journal</h4>
                <p className="mt-0.5 text-sm text-gray-500">
                  The last {LOG_LINES} lines from {unit}. A refused login and a TLS
                  handshake that failed both land here.
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void loadLogs()}
                disabled={logsLoading}
                className={SECONDARY_BUTTON_CLASS}
              >
                <FileTerminal className="h-4 w-4" aria-hidden="true" />
                {logsLoading ? 'Reading…' : logs === null ? 'Read journal' : 'Read again'}
              </Button>
            </div>

            {logsLoading && (
              <div className="mt-3 space-y-2" aria-busy="true">
                {Array.from({ length: 4 }).map((_, index) => (
                  <div key={index} className="h-3 animate-pulse rounded bg-gray-100" />
                ))}
              </div>
            )}
            {!logsLoading && logsError && (
              <ErrorBlock title="Could not read the journal" message={logsError} onRetry={() => void loadLogs()} />
            )}
            {!logsLoading && !logsError && logs !== null && (
              logs.trim() === '' ? (
                <p className="mt-3 text-sm text-gray-500">
                  journalctl returned nothing for this unit. It may have been started
                  before the current boot, or the journal may be empty.
                </p>
              ) : (
                <pre className="mt-3 max-h-72 overflow-auto rounded-md border border-gray-200 bg-gray-50 p-3 font-mono text-xs leading-relaxed text-gray-700">
                  {logs}
                </pre>
              )
            )}
          </div>
        </>
      )}
    </Panel>
  );
}

export default DaemonCard;
