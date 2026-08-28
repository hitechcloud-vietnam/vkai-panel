'use client';

/**
 * Anti Intrusion: file integrity monitoring, wired to the tamper-proof module.
 *
 * This is the one section on this screen where the backend does the real work.
 * TamperProofService walks each protected path, hashes every file it finds with
 * the configured algorithm, stores that as a baseline, and on the next scan
 * reports what was modified, created or deleted, raising an alert per finding.
 * Everything below is that module's own data: nothing is derived, nothing is
 * assumed, and where the module has never scanned, the pane says so rather than
 * showing a clean result.
 *
 * One thing it will not do is imply a schedule. Scans run when somebody presses
 * the button here; nothing in the panel runs them periodically. A baseline that
 * is never re-checked detects nothing, so the pane states when each path was
 * last scanned and leaves the operator to draw the conclusion.
 */

import { useCallback, useEffect, useState } from 'react';
import { CheckCircle2, Play, RefreshCw, ScanLine } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { errorMessage } from '@/lib/apiError';
import { cn } from '@/lib/utils';

import * as securityApi from './api';
import {
  CaveatBlock,
  EmptyState,
  ErrorBlock,
  Panel,
  PaneHeading,
  PRIMARY_BUTTON_CLASS,
  SECONDARY_BUTTON_CLASS,
  SectionHeader,
  StatTile,
  TableScroller,
  TableSkeleton,
  TD_CLASS,
  TH_CLASS,
} from './chrome';
import type { ProtectedPath, TamperAlert, TamperScanResult, TamperStats } from './types';

function formatTime(iso: string | null): string {
  if (!iso) return 'never';
  const at = Date.parse(iso);
  if (Number.isNaN(at)) return iso;
  return new Date(at).toLocaleString();
}

function severityClass(severity: string): string {
  switch ((severity || '').toLowerCase()) {
    case 'critical':
      return 'border-red-200 bg-red-50 text-red-700';
    case 'high':
      return 'border-amber-200 bg-amber-50 text-amber-700';
    case 'medium':
      return 'border-sky-200 bg-sky-50 text-sky-700';
    default:
      return 'border-gray-200 bg-gray-100 text-gray-700';
  }
}

export function AntiIntrusionPane() {
  const [stats, setStats] = useState<TamperStats | null>(null);
  const [paths, setPaths] = useState<ProtectedPath[]>([]);
  const [alerts, setAlerts] = useState<TamperAlert[]>([]);
  const [scans, setScans] = useState<TamperScanResult[]>([]);

  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');
  const [scanning, setScanning] = useState(false);
  const [notice, setNotice] = useState('');
  const [busyAlert, setBusyAlert] = useState('');

  const load = useCallback(async (initial: boolean) => {
    if (initial) setLoading(true);
    else setRefreshing(true);
    try {
      const [nextStats, nextPaths, nextAlerts, nextScans] = await Promise.all([
        securityApi.tamperProof.stats(),
        securityApi.tamperProof.paths(),
        securityApi.tamperProof.alerts(false),
        securityApi.tamperProof.scanResults(20),
      ]);
      setStats(nextStats);
      setPaths(nextPaths);
      setAlerts(nextAlerts);
      setScans(nextScans);
      setError('');
    } catch (err) {
      setError(errorMessage(err, 'The file integrity data could not be read.'));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void load(true);
  }, [load]);

  const scanAll = async () => {
    setScanning(true);
    setNotice('');
    try {
      await securityApi.tamperProof.scanAll();
      setNotice('Every enabled path was re-hashed and compared against its baseline.');
      await load(false);
    } catch (err) {
      setError(errorMessage(err, 'The scan could not be run.'));
    } finally {
      setScanning(false);
    }
  };

  const scanOne = async (path: ProtectedPath) => {
    setBusyAlert(path.id);
    try {
      await securityApi.tamperProof.scanPath(path.id);
      await load(false);
    } catch (err) {
      setError(errorMessage(err, `The scan of ${path.path} could not be run.`));
    } finally {
      setBusyAlert('');
    }
  };

  const resolve = async (alert: TamperAlert) => {
    const notes = window.prompt(
      `Resolving the alert on ${alert.file_path}. Say why this change was expected - it is written into the audit trail.`,
      ''
    );
    if (notes === null) return;
    setBusyAlert(alert.id);
    try {
      await securityApi.tamperProof.resolveAlert(alert.id, notes);
      await load(false);
    } catch (err) {
      setError(errorMessage(err, 'The alert could not be resolved.'));
    } finally {
      setBusyAlert('');
    }
  };

  const neverScanned = paths.filter((path) => path.is_enabled && !path.last_scan_at);

  return (
    <div className="space-y-4">
      <PaneHeading
        title="Anti Intrusion"
        description="File integrity monitoring, from the panel's tamper-proof module. Each watched path is hashed file by file and compared against a stored baseline; anything that changed, appeared or disappeared becomes an alert here."
        actions={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => void load(false)}
              disabled={refreshing}
              className={SECONDARY_BUTTON_CLASS}
            >
              <RefreshCw className={cn('mr-2 h-4 w-4', refreshing && 'animate-spin')} aria-hidden="true" />
              Refresh
            </Button>
            <Button
              type="button"
              onClick={() => void scanAll()}
              disabled={scanning || paths.length === 0}
              className={PRIMARY_BUTTON_CLASS}
            >
              <ScanLine className={cn('mr-2 h-4 w-4', scanning && 'animate-pulse')} aria-hidden="true" />
              {scanning ? 'Scanning…' : 'Scan every path'}
            </Button>
          </>
        }
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        <StatTile label="Watched paths" value={stats ? stats.enabled_paths : null} hint={stats ? `${stats.protected_paths} configured` : 'Not read.'} />
        <StatTile label="Files under watch" value={stats ? stats.total_files : null} hint={stats ? undefined : 'Not read.'} />
        <StatTile label="Unresolved alerts" value={stats ? stats.active_alerts : null} hint={stats ? `${stats.alerts_today} raised today` : 'Not read.'} />
        <StatTile label="Scans run" value={stats ? stats.total_scans : null} hint={stats ? `${stats.violation_scans} found violations` : 'Not read.'} />
        <StatTile
          label="Last scan"
          value={stats ? (stats.last_scan_at ? formatTime(stats.last_scan_at) : 'never') : null}
          hint={stats ? undefined : 'Not read.'}
        />
      </div>

      {notice && (
        <div className="flex items-start gap-3 rounded-md border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-800">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <p>{notice}</p>
        </div>
      )}

      {!loading && !error && paths.length > 0 && (
        <CaveatBlock title="Scans are on demand only">
          <p>
            Nothing in this panel runs an integrity scan on a schedule. A baseline that is never
            re-checked records what the files looked like once and detects nothing afterwards, so
            until a scheduled scan exists, the &ldquo;last scan&rdquo; column above is the age of
            everything this section can tell you.
          </p>
          {neverScanned.length > 0 && (
            <p>
              {neverScanned.length} enabled{' '}
              {neverScanned.length === 1 ? 'path has' : 'paths have'} never been scanned, so there is
              no baseline to compare against yet.
            </p>
          )}
        </CaveatBlock>
      )}

      <Panel>
        <SectionHeader
          title="What has been flagged"
          description="Unresolved integrity alerts. Resolving one records who signed it off and why."
        />
        {loading ? (
          <TableSkeleton columns={5} />
        ) : error ? (
          <ErrorBlock
            title="The alerts could not be read"
            message={error}
            onRetry={() => void load(false)}
          />
        ) : alerts.length === 0 ? (
          <EmptyState
            title={paths.length === 0 ? 'Nothing is being watched' : 'No unresolved alerts'}
            description={
              paths.length === 0
                ? 'Add a path on the tamper-proof screen and run a scan to establish a baseline. Until then a change to a site file or a system binary would leave no record.'
                : 'Every path with a baseline matched it at its last scan. Check the last scan time above before reading that as current.'
            }
            action={
              paths.length === 0 ? (
                <a
                  href="/tamper-proof"
                  className="inline-flex items-center rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700"
                >
                  Add a watched path
                </a>
              ) : undefined
            }
          />
        ) : (
          <TableScroller>
            <table className="w-full min-w-[900px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH_CLASS}>File</th>
                  <th className={TH_CLASS}>Change</th>
                  <th className={TH_CLASS}>Severity</th>
                  <th className={TH_CLASS}>Detected</th>
                  <th className={cn(TH_CLASS, 'text-right')}>
                    <span className="sr-only">Row actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {alerts.map((alert) => (
                  <tr key={alert.id} className="border-b border-gray-200 last:border-b-0">
                    <td className={cn(TD_CLASS, 'font-mono text-gray-900')}>
                      <span className="break-all">{alert.file_path}</span>
                      {alert.old_mode && alert.new_mode && alert.old_mode !== alert.new_mode && (
                        <p className="mt-1 text-xs text-gray-500">
                          mode {alert.old_mode} → {alert.new_mode}
                        </p>
                      )}
                    </td>
                    <td className={cn(TD_CLASS, 'capitalize')}>
                      {(alert.alert_type || '').replace(/_/g, ' ')}
                    </td>
                    <td className={TD_CLASS}>
                      <span
                        className={cn(
                          'inline-flex rounded-md border px-2 py-0.5 text-xs font-medium capitalize',
                          severityClass(alert.severity)
                        )}
                      >
                        {alert.severity || 'unknown'}
                      </span>
                    </td>
                    <td className={cn(TD_CLASS, 'whitespace-nowrap')}>{formatTime(alert.created_at)}</td>
                    <td className={cn(TD_CLASS, 'text-right')}>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={busyAlert === alert.id}
                        onClick={() => void resolve(alert)}
                        className={SECONDARY_BUTTON_CLASS}
                      >
                        {busyAlert === alert.id ? 'Resolving…' : 'Resolve'}
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>

      <Panel>
        <SectionHeader
          title="Watched paths"
          description="What the module is hashing, and when it last did so."
        />
        {loading ? (
          <TableSkeleton columns={5} />
        ) : error ? (
          <ErrorBlock title="The watched paths could not be read" message={error} onRetry={() => void load(false)} />
        ) : paths.length === 0 ? (
          <EmptyState
            title="No path is being watched"
            description="Paths are added on the tamper-proof screen. Start with the web roots, the panel's own configuration and the system binaries an attacker would replace."
            action={
              <a
                href="/tamper-proof"
                className="inline-flex items-center rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700"
              >
                Open tamper-proof
              </a>
            }
          />
        ) : (
          <TableScroller>
            <table className="w-full min-w-[900px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH_CLASS}>Path</th>
                  <th className={TH_CLASS}>State</th>
                  <th className={TH_CLASS}>Algorithm</th>
                  <th className={TH_CLASS}>Files</th>
                  <th className={TH_CLASS}>Last scan</th>
                  <th className={cn(TH_CLASS, 'text-right')}>
                    <span className="sr-only">Row actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {paths.map((path) => (
                  <tr key={path.id} className="border-b border-gray-200 last:border-b-0">
                    <td className={cn(TD_CLASS, 'font-mono text-gray-900')}>
                      <span className="break-all">{path.path}</span>
                      {path.recursive && <span className="ml-2 text-xs text-gray-500">recursive</span>}
                    </td>
                    <td className={TD_CLASS}>
                      <span
                        className={cn(
                          'inline-flex rounded-md border px-2 py-0.5 text-xs font-medium',
                          path.is_enabled
                            ? 'border-emerald-200 bg-emerald-50 text-emerald-700'
                            : 'border-gray-200 bg-gray-100 text-gray-700'
                        )}
                      >
                        {path.is_enabled ? 'Watching' : 'Disabled'}
                      </span>
                    </td>
                    <td className={cn(TD_CLASS, 'uppercase')}>{path.algorithm || '—'}</td>
                    <td className={TD_CLASS}>{path.file_count}</td>
                    <td className={cn(TD_CLASS, 'whitespace-nowrap')}>
                      {path.last_scan_at ? (
                        formatTime(path.last_scan_at)
                      ) : (
                        <span className="text-amber-700">never — no baseline</span>
                      )}
                    </td>
                    <td className={cn(TD_CLASS, 'text-right')}>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={busyAlert === path.id || !path.is_enabled}
                        onClick={() => void scanOne(path)}
                        className={SECONDARY_BUTTON_CLASS}
                      >
                        <Play className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                        {busyAlert === path.id ? 'Scanning…' : 'Scan'}
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>

      <Panel>
        <SectionHeader title="Recent scans" description="What each run compared, and what it found." />
        {loading ? (
          <TableSkeleton columns={6} />
        ) : error ? (
          <ErrorBlock title="The scan history could not be read" message={error} onRetry={() => void load(false)} />
        ) : scans.length === 0 ? (
          <EmptyState
            title="No scan has run"
            description="Until a scan runs there is no baseline, and without a baseline nothing here can detect a change. Press Scan every path above."
          />
        ) : (
          <TableScroller>
            <table className="w-full min-w-[800px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH_CLASS}>When</th>
                  <th className={TH_CLASS}>Result</th>
                  <th className={TH_CLASS}>Files scanned</th>
                  <th className={TH_CLASS}>Modified</th>
                  <th className={TH_CLASS}>New</th>
                  <th className={TH_CLASS}>Deleted</th>
                </tr>
              </thead>
              <tbody>
                {scans.map((scan) => (
                  <tr key={scan.id} className="border-b border-gray-200 last:border-b-0">
                    <td className={cn(TD_CLASS, 'whitespace-nowrap')}>{formatTime(scan.created_at)}</td>
                    <td className={TD_CLASS}>
                      <span
                        className={cn(
                          'inline-flex rounded-md border px-2 py-0.5 text-xs font-medium',
                          scan.status === 'clean'
                            ? 'border-emerald-200 bg-emerald-50 text-emerald-700'
                            : scan.status === 'error'
                              ? 'border-red-200 bg-red-50 text-red-700'
                              : 'border-amber-200 bg-amber-50 text-amber-700'
                        )}
                      >
                        {(scan.status || 'unknown').replace(/_/g, ' ')}
                      </span>
                    </td>
                    <td className={TD_CLASS}>
                      {scan.scanned_files} of {scan.total_files}
                    </td>
                    <td className={TD_CLASS}>{scan.modified_files}</td>
                    <td className={TD_CLASS}>{scan.new_files}</td>
                    <td className={TD_CLASS}>{scan.deleted_files}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>
    </div>
  );
}

export default AntiIntrusionPane;
