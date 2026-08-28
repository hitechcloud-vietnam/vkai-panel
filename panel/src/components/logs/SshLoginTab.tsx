'use client';

/**
 * SSH Login Logs - who got in, who tried and failed, and from where.
 *
 * WHAT THIS SHOWS, PRECISELY
 *
 * Panel sign-in events, read from the audit trail: service/auth.go records
 * auth.sign_in, auth.sign_in_failed, auth.sign_out and auth.token_refreshed
 * against resource "session", each with the source address, the account and a
 * success/failure outcome. That is real, mounted and queryable through
 * GET /api/v1/audit/search, and it is the account-takeover trail an operator
 * actually needs after an incident.
 *
 * WHAT IT DOES NOT SHOW
 *
 * Operating-system SSH sessions. sshd writes to /var/log/auth.log (or the
 * journal) and nothing in the backend reads it: there is no sshd log collector,
 * no route, no model. The panel also keeps its own authentication event log at
 * internal/middleware/authlog.go - the file fail2ban parses - but nothing
 * exposes it over HTTP either. Both gaps are stated on screen rather than left
 * for an operator to discover by trusting an incomplete list during an incident.
 *
 * Failures are given at least as much room as successes: they get their own
 * count, their own one-click filter and a red row treatment, because the reason
 * anyone opens this screen is that something went wrong.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import { KeyRound, RefreshCw, ShieldAlert } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Unavailable } from '@/components/Unavailable';
import {
  BTN_SECONDARY,
  DEFAULT_RANGE,
  EM_DASH,
  EXPORT_MAX_ROWS,
  EXPORT_PAGE_SIZE,
  EmptyState,
  ErrorBlock,
  GapNotice,
  OutcomePill,
  LogToolbar,
  PANEL,
  PANEL_HEAD,
  PANEL_TITLE,
  Pagination,
  TD,
  TH,
  TableSkeleton,
  type TimeRange,
  detailString,
  downloadText,
  formatTimestamp,
  rangeToParams,
  shortId,
  stamp,
  toCsv,
} from './common';
import { describeError, getAuditStats, searchAuditLogs } from './api';
import type { ApiFailure, AuditLog, AuditStats } from './types';

/** audit.ActionSignIn / ActionSignInFailed / ActionSignOut, from audit/actions.go. */
const ACTION_SIGN_IN = 'auth.sign_in';
const ACTION_SIGN_IN_FAILED = 'auth.sign_in_failed';
const RESOURCE_SESSION = 'session';

type View = 'all' | 'failed' | 'success';

const OUTCOME_OPTIONS = [
  { value: '', label: 'All outcomes' },
  { value: 'failure', label: 'Failure' },
  { value: 'success', label: 'Success' },
];

const RANGE_DAYS: Record<string, number> = {
  '1h': 1,
  '6h': 1,
  '24h': 1,
  '7d': 7,
  '30d': 30,
  all: 3650,
  custom: 30,
};

export default function SshLoginTab() {
  const [view, setView] = useState<View>('all');
  const [range, setRange] = useState<TimeRange>(DEFAULT_RANGE);
  const [outcome, setOutcome] = useState('');
  const [search, setSearch] = useState('');

  const [limit, setLimit] = useState(100);
  const [offset, setOffset] = useState(0);
  const [generation, setGeneration] = useState(0);

  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [total, setTotal] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [failure, setFailure] = useState<ApiFailure | null>(null);
  const [stats, setStats] = useState<AuditStats | null>(null);
  const [exporting, setExporting] = useState(false);
  const [exportNote, setExportNote] = useState<string | null>(null);

  const queryFor = useCallback(
    (currentView: View) => ({
      ...rangeToParams(range),
      resource: RESOURCE_SESSION,
      action:
        currentView === 'failed'
          ? ACTION_SIGN_IN_FAILED
          : currentView === 'success'
            ? ACTION_SIGN_IN
            : '',
      status: currentView === 'all' ? outcome : '',
    }),
    [range, outcome]
  );

  const load = useCallback(async () => {
    setLoading(true);
    setFailure(null);
    try {
      const result = await searchAuditLogs({ ...queryFor(view), limit, offset });
      setLogs(result.logs);
      setTotal(result.total);
    } catch (err) {
      setLogs([]);
      setTotal(null);
      setFailure(describeError(err, 'The sign-in history did not answer.'));
    } finally {
      setLoading(false);
    }
  }, [limit, offset, queryFor, view]);

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load, generation]);

  useEffect(() => {
    let cancelled = false;
    getAuditStats(RANGE_DAYS[range.key] ?? 30)
      .then((value) => {
        if (!cancelled) setStats(value);
      })
      .catch(() => {
        if (!cancelled) setStats(null);
      });
    return () => {
      cancelled = true;
    };
  }, [range.key, generation]);

  const failedCount = stats?.by_action?.[ACTION_SIGN_IN_FAILED] ?? 0;
  const successCount = stats?.by_action?.[ACTION_SIGN_IN] ?? 0;

  const visibleLogs = useMemo(() => {
    const needle = search.trim().toLowerCase();
    if (!needle) return logs;
    return logs.filter((log) =>
      [
        log.ip_address,
        log.action,
        log.status,
        log.user_agent,
        detailString(log.details, 'username') ?? '',
        detailString(log.details, 'reason') ?? '',
      ]
        .join(' ')
        .toLowerCase()
        .includes(needle)
    );
  }, [logs, search]);

  const distinctAddresses = useMemo(
    () => new Set(logs.map((log) => log.ip_address).filter(Boolean)).size,
    [logs]
  );

  const exportCsv = async () => {
    setExporting(true);
    setExportNote(null);
    const collected: AuditLog[] = [];
    try {
      for (let cursor = 0; cursor < EXPORT_MAX_ROWS; cursor += EXPORT_PAGE_SIZE) {
        const page = await searchAuditLogs({
          ...queryFor(view),
          limit: EXPORT_PAGE_SIZE,
          offset: cursor,
        });
        collected.push(...page.logs);
        if (page.logs.length < EXPORT_PAGE_SIZE) break;
      }
      downloadText(
        `sign-in-history-${stamp()}.csv`,
        toCsv(
          ['Time', 'Outcome', 'Event', 'Account', 'Source address', 'Reason', 'User agent'],
          collected.map((log) => [
            log.created_at,
            log.status,
            log.action,
            detailString(log.details, 'username') ?? log.user_id ?? '',
            log.ip_address,
            detailString(log.details, 'reason') ?? '',
            log.user_agent,
          ])
        )
      );
      setExportNote(`Exported ${collected.length.toLocaleString()} events.`);
    } catch (err) {
      setExportNote(`Export failed: ${describeError(err, 'The export could not be completed.').message}`);
    } finally {
      setExporting(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <button
          type="button"
          onClick={() => {
            setView('failed');
            setOffset(0);
            setGeneration((value) => value + 1);
          }}
          className={cn(
            PANEL,
            'px-5 py-4 text-left transition-colors hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500 focus-visible:ring-offset-2',
            view === 'failed' && 'ring-2 ring-red-500'
          )}
        >
          <p className="flex items-center gap-1.5 text-xs font-medium text-gray-500">
            <ShieldAlert size={14} className="text-red-600" aria-hidden="true" />
            Failed sign-ins
          </p>
          <p className={cn('mt-1 text-2xl font-semibold', failedCount > 0 ? 'text-red-700' : 'text-gray-900')}>
            {failedCount.toLocaleString()}
          </p>
          <p className="mt-0.5 text-xs text-gray-500">Show failures only</p>
        </button>

        <button
          type="button"
          onClick={() => {
            setView('success');
            setOffset(0);
            setGeneration((value) => value + 1);
          }}
          className={cn(
            PANEL,
            'px-5 py-4 text-left transition-colors hover:bg-emerald-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500 focus-visible:ring-offset-2',
            view === 'success' && 'ring-2 ring-emerald-500'
          )}
        >
          <p className="text-xs font-medium text-gray-500">Successful sign-ins</p>
          <p className="mt-1 text-2xl font-semibold text-emerald-700">
            {successCount.toLocaleString()}
          </p>
          <p className="mt-0.5 text-xs text-gray-500">Show successes only</p>
        </button>

        <button
          type="button"
          onClick={() => {
            setView('all');
            setOffset(0);
            setGeneration((value) => value + 1);
          }}
          className={cn(
            PANEL,
            'px-5 py-4 text-left transition-colors hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2',
            view === 'all' && 'ring-2 ring-brand-500'
          )}
        >
          <p className="text-xs font-medium text-gray-500">Source addresses on this page</p>
          <p className="mt-1 text-2xl font-semibold text-gray-900">{distinctAddresses}</p>
          <p className="mt-0.5 text-xs text-gray-500">Show every session event</p>
        </button>
      </div>

      <div className={PANEL}>
        <div className={PANEL_HEAD}>
          <div>
            <h2 className={PANEL_TITLE}>Panel sign-in history</h2>
            <p className="mt-0.5 text-xs text-gray-500">
              Successes and failures, with the source address and the account that was used.
            </p>
          </div>
          <button
            type="button"
            className={BTN_SECONDARY}
            onClick={() => setGeneration((value) => value + 1)}
            disabled={loading}
          >
            <RefreshCw size={16} className={cn(loading && 'animate-spin')} aria-hidden="true" />
            Refresh
          </button>
        </div>

        <LogToolbar
          range={range}
          onRangeChange={setRange}
          level={outcome}
          onLevelChange={setOutcome}
          levelOptions={OUTCOME_OPTIONS}
          levelLabel="Outcome"
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Address, account or reason"
          searchHint={
            view === 'all'
              ? 'Time range and outcome are applied by the server. Search filters the page loaded below.'
              : 'Showing one event type only; clear the selection above to filter by outcome. Search filters the page loaded below.'
          }
          onApply={() => {
            setOffset(0);
            setGeneration((value) => value + 1);
          }}
          onExport={exportCsv}
          exportLabel={exporting ? 'Exporting…' : 'Export CSV'}
          exportDisabled={exporting || loading}
          busy={loading}
        />

        {exportNote ? (
          <p className="border-b border-gray-200 bg-gray-50 px-5 py-2 text-xs text-gray-600">
            {exportNote}
          </p>
        ) : null}

        {loading ? (
          <TableSkeleton rows={8} columns={6} />
        ) : failure ? (
          <ErrorBlock
            title="The sign-in history could not be loaded"
            failure={failure}
            onRetry={() => setGeneration((value) => value + 1)}
          />
        ) : visibleLogs.length === 0 ? (
          <EmptyState
            icon={<KeyRound size={36} aria-hidden="true" />}
            title={logs.length === 0 ? 'No sign-in events' : 'No row matches the search'}
            message={
              logs.length === 0
                ? 'Nothing was recorded in this range. Widen the time range, or clear the outcome filter.'
                : 'Clear the search box to see the whole page again.'
            }
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50">
                <tr>
                  <th className={TH}>Time</th>
                  <th className={TH}>Outcome</th>
                  <th className={TH}>Account</th>
                  <th className={TH}>Source address</th>
                  <th className={TH}>Location</th>
                  <th className={TH}>Event</th>
                </tr>
              </thead>
              <tbody>
                {visibleLogs.map((log) => {
                  const failed = (log.status || '').toLowerCase() !== 'success';
                  return (
                    <tr
                      key={log.id}
                      className={cn(
                        'hover:bg-gray-50',
                        failed && 'bg-red-50/60 hover:bg-red-50'
                      )}
                    >
                      <td className={cn(TD, 'whitespace-nowrap')} suppressHydrationWarning>
                        {formatTimestamp(log.created_at)}
                      </td>
                      <td className={TD}>
                        <OutcomePill status={log.status} />
                      </td>
                      <td className={cn(TD, 'whitespace-nowrap font-medium text-gray-900')}>
                        {detailString(log.details, 'username') ?? shortId(log.user_id)}
                      </td>
                      <td className={cn(TD, 'whitespace-nowrap font-mono text-xs')}>
                        {log.ip_address || EM_DASH}
                      </td>
                      <td className={cn(TD, 'whitespace-nowrap')}>
                        <Unavailable reason="No geo-IP lookup: the backend does not resolve source addresses to a location." />
                      </td>
                      <td className={TD}>
                        <span className="font-medium text-gray-900">{log.action || EM_DASH}</span>
                        {detailString(log.details, 'reason') ? (
                          <span className="ml-2 text-xs text-gray-600">
                            {detailString(log.details, 'reason')}
                          </span>
                        ) : null}
                        {log.user_agent ? (
                          <span className="mt-0.5 block truncate text-xs text-gray-500" title={log.user_agent}>
                            {log.user_agent}
                          </span>
                        ) : null}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}

        {!loading && !failure && logs.length > 0 ? (
          <Pagination
            offset={offset}
            limit={limit}
            total={total}
            loaded={logs.length}
            onOffsetChange={setOffset}
            onLimitChange={(next) => {
              setLimit(next);
              setOffset(0);
            }}
          />
        ) : null}

        <GapNotice title="These are panel sign-ins, not operating-system SSH sessions">
          <p>
            sshd writes its own record to /var/log/auth.log and to the journal, and the backend does
            not read either: there is no collector, no model and no route. Two further sources exist
            on disk but are not served over HTTP:
          </p>
          <ul className="mt-2 list-disc space-y-1 pl-5 text-sm">
            <li>
              the panel authentication event log at internal/middleware/authlog.go - the same file
              fail2ban parses - which records blocked and throttled attempts this table cannot show;
            </li>
            <li>sshd&apos;s own log, which is where an actual SSH intrusion would appear.</li>
          </ul>
          <p className="mt-2">
            Source addresses are also shown unresolved: nothing in the backend maps an address to a
            location.
          </p>
        </GapNotice>
      </div>
    </div>
  );
}
