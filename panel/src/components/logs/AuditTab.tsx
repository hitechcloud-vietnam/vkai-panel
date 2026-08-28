'use client';

/**
 * Logs Audit - the tamper-evident trail.
 *
 * The table is the easy half and is wired to GET /api/v1/audit/search and
 * GET /api/v1/audit/stats, both mounted in router.go.
 *
 * The half that matters is the verification state. An audit log that cannot be
 * shown as verified is just a table, so this section leads with the chain: how
 * many entries are sealed, what the head hash is, when it was last checked, and
 * - if it is broken - the sequence number where it first breaks, the reason and
 * the time.
 *
 * Those endpoints are written (handler/audit.go, RegisterAuditRoutes) but
 * router.go never calls RegisterAuditRoutes, so /api/v1/audit/chain/* answers
 * 404 today. The section therefore probes on load and does one of two things,
 * with nothing in between:
 *
 *   - the probe succeeds: the real chain state is rendered, and Verify runs a
 *     real pass;
 *   - the probe 404s: the section says the verification surface is not mounted,
 *     names the one line that would mount it, and says plainly that until then
 *     the trail below is unverified. No fake "Verified" badge, no disabled
 *     button that implies the feature is merely switched off.
 *
 * The moment the route is mounted this page starts showing real verification
 * with no further frontend change.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  CheckCircle2,
  Loader2,
  RefreshCw,
  ShieldAlert,
  ShieldCheck,
  ShieldQuestion,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import {
  BTN_PRIMARY,
  BTN_SECONDARY,
  BlockSkeleton,
  DEFAULT_RANGE,
  EM_DASH,
  EXPORT_MAX_ROWS,
  EXPORT_PAGE_SIZE,
  EmptyState,
  ErrorBlock,
  FIELD,
  FIELD_LABEL,
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
import {
  describeError,
  exportChainBundle,
  getAuditStats,
  getChainStatus,
  searchAuditLogs,
  verifyChain,
} from './api';
import type { ApiFailure, AuditLog, AuditStats, ChainStatus, VerifyReport } from './types';

const STATUS_OPTIONS = [
  { value: '', label: 'All outcomes' },
  { value: 'success', label: 'Success' },
  { value: 'failure', label: 'Failure' },
];

export default function AuditTab() {
  const [range, setRange] = useState<TimeRange>(DEFAULT_RANGE);
  const [status, setStatus] = useState('');
  const [action, setAction] = useState('');
  const [resource, setResource] = useState('');
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

  const applied = useRef({ range, status, action, resource });

  const load = useCallback(async () => {
    setLoading(true);
    setFailure(null);
    const committed = applied.current;
    try {
      const result = await searchAuditLogs({
        ...rangeToParams(committed.range),
        status: committed.status,
        action: committed.action,
        resource: committed.resource,
        limit,
        offset,
      });
      setLogs(result.logs);
      setTotal(result.total);
    } catch (err) {
      setLogs([]);
      setTotal(null);
      setFailure(describeError(err, 'The audit search did not answer.'));
    } finally {
      setLoading(false);
    }
  }, [limit, offset]);

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load, generation]);

  useEffect(() => {
    let cancelled = false;
    getAuditStats(30)
      .then((value) => {
        if (!cancelled) setStats(value);
      })
      .catch(() => {
        if (!cancelled) setStats(null);
      });
    return () => {
      cancelled = true;
    };
  }, [generation]);

  const apply = () => {
    applied.current = { range, status, action, resource };
    setOffset(0);
    setGeneration((value) => value + 1);
  };

  // The audit API has no full-text parameter, so this filter runs over the page
  // in hand and the toolbar says so rather than implying a server-side search.
  const visibleLogs = useMemo(() => {
    const needle = search.trim().toLowerCase();
    if (!needle) return logs;
    return logs.filter((log) => {
      const haystack = [
        log.action,
        log.resource,
        log.ip_address,
        log.status,
        log.user_agent,
        log.details ? JSON.stringify(log.details) : '',
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(needle);
    });
  }, [logs, search]);

  const actionSuggestions = useMemo(
    () => Object.keys(stats?.by_action ?? {}).sort(),
    [stats]
  );
  const resourceSuggestions = useMemo(
    () => Object.keys(stats?.by_resource ?? {}).sort(),
    [stats]
  );

  const exportCsv = async () => {
    setExporting(true);
    setExportNote(null);
    const committed = applied.current;
    const collected: AuditLog[] = [];
    try {
      for (let cursor = 0; cursor < EXPORT_MAX_ROWS; cursor += EXPORT_PAGE_SIZE) {
        const page = await searchAuditLogs({
          ...rangeToParams(committed.range),
          status: committed.status,
          action: committed.action,
          resource: committed.resource,
          limit: EXPORT_PAGE_SIZE,
          offset: cursor,
        });
        collected.push(...page.logs);
        if (page.logs.length < EXPORT_PAGE_SIZE) break;
      }
      downloadText(
        `audit-${stamp()}.csv`,
        toCsv(
          ['Time', 'Action', 'Resource', 'Resource ID', 'User', 'IP address', 'Outcome', 'Details'],
          collected.map((log) => [
            log.created_at,
            log.action,
            log.resource,
            log.resource_id ?? '',
            log.user_id ?? '',
            log.ip_address,
            log.status,
            log.details ? JSON.stringify(log.details) : '',
          ])
        )
      );
      setExportNote(
        collected.length >= EXPORT_MAX_ROWS
          ? `Exported the first ${EXPORT_MAX_ROWS.toLocaleString()} entries, the export ceiling.`
          : `Exported ${collected.length.toLocaleString()} entries. A CSV is a convenience copy and is not the tamper-evident bundle.`
      );
    } catch (err) {
      setExportNote(`Export failed: ${describeError(err, 'The export could not be completed.').message}`);
    } finally {
      setExporting(false);
    }
  };

  const statusCounts = stats?.by_status ?? {};

  return (
    <div className="space-y-4">
      <ChainPanel />

      {stats ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatTile label="Entries (30 days)" value={stats.total_logs?.toLocaleString() ?? EM_DASH} />
          <StatTile
            label="Successful"
            value={(statusCounts.success ?? 0).toLocaleString()}
            tone="text-emerald-700"
          />
          <StatTile
            label="Failed"
            value={(statusCounts.failure ?? 0).toLocaleString()}
            tone={(statusCounts.failure ?? 0) > 0 ? 'text-red-700' : 'text-gray-900'}
          />
          <StatTile label="Distinct actors" value={Object.keys(stats.by_user ?? {}).length.toLocaleString()} />
        </div>
      ) : null}

      <div className={PANEL}>
        <div className={PANEL_HEAD}>
          <div>
            <h2 className={PANEL_TITLE}>Audit trail</h2>
            <p className="mt-0.5 text-xs text-gray-500">
              Every security-relevant action, in the order it was recorded.
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
          level={status}
          onLevelChange={setStatus}
          levelOptions={STATUS_OPTIONS}
          levelLabel="Outcome"
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Filter the rows below"
          searchHint="Time range, outcome, action and resource are applied by the server. Search filters the page loaded below - the audit API has no full-text parameter."
          onApply={apply}
          onExport={exportCsv}
          exportLabel={exporting ? 'Exporting…' : 'Export CSV'}
          exportDisabled={exporting || loading}
          busy={loading}
        >
          <div className="w-full sm:w-48">
            <label className={FIELD_LABEL} htmlFor="audit-action">
              Action
            </label>
            <input
              id="audit-action"
              className={FIELD}
              list="audit-actions"
              placeholder="Any action"
              value={action}
              onChange={(event) => setAction(event.target.value)}
            />
            <datalist id="audit-actions">
              {actionSuggestions.map((name) => (
                <option key={name} value={name} />
              ))}
            </datalist>
          </div>
          <div className="w-full sm:w-44">
            <label className={FIELD_LABEL} htmlFor="audit-resource">
              Resource
            </label>
            <input
              id="audit-resource"
              className={FIELD}
              list="audit-resources"
              placeholder="Any resource"
              value={resource}
              onChange={(event) => setResource(event.target.value)}
            />
            <datalist id="audit-resources">
              {resourceSuggestions.map((name) => (
                <option key={name} value={name} />
              ))}
            </datalist>
          </div>
        </LogToolbar>

        {exportNote ? (
          <p className="border-b border-gray-200 bg-gray-50 px-5 py-2 text-xs text-gray-600">
            {exportNote}
          </p>
        ) : null}

        {loading ? (
          <TableSkeleton rows={8} columns={6} />
        ) : failure ? (
          <ErrorBlock
            title="The audit trail could not be loaded"
            failure={failure}
            onRetry={() => setGeneration((value) => value + 1)}
          />
        ) : visibleLogs.length === 0 ? (
          <EmptyState
            icon={<ShieldQuestion size={36} aria-hidden="true" />}
            title={logs.length === 0 ? 'No audit entries' : 'No row matches the search'}
            message={
              logs.length === 0
                ? 'Nothing matched these filters. Widen the time range, or clear the action and resource filters.'
                : 'Clear the search box to see the whole page again.'
            }
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50">
                <tr>
                  <th className={TH}>Time</th>
                  <th className={TH}>Action</th>
                  <th className={TH}>Resource</th>
                  <th className={TH}>Actor</th>
                  <th className={TH}>Source address</th>
                  <th className={TH}>Outcome</th>
                </tr>
              </thead>
              <tbody>
                {visibleLogs.map((log) => (
                  <tr key={log.id} className="hover:bg-gray-50">
                    <td className={cn(TD, 'whitespace-nowrap')} suppressHydrationWarning>
                      {formatTimestamp(log.created_at)}
                    </td>
                    <td className={cn(TD, 'whitespace-nowrap font-medium text-gray-900')}>
                      {log.action || EM_DASH}
                    </td>
                    <td className={cn(TD, 'whitespace-nowrap')}>
                      {log.resource || EM_DASH}
                      {log.resource_id ? (
                        <span className="ml-1 font-mono text-xs text-gray-500">
                          {shortId(log.resource_id)}
                        </span>
                      ) : null}
                    </td>
                    <td className={cn(TD, 'whitespace-nowrap')}>
                      {detailString(log.details, 'username') ?? shortId(log.user_id)}
                    </td>
                    <td className={cn(TD, 'whitespace-nowrap font-mono text-xs')}>
                      {log.ip_address || EM_DASH}
                    </td>
                    <td className={TD}>
                      <OutcomePill status={log.status} />
                      {log.details && Object.keys(log.details).length > 0 ? (
                        <details className="mt-1">
                          <summary className="cursor-pointer text-xs text-brand-700">
                            Details
                          </summary>
                          <pre className="mt-1 max-h-48 overflow-auto rounded-md bg-gray-50 p-2 text-xs text-gray-700">
                            {JSON.stringify(log.details, null, 2)}
                          </pre>
                        </details>
                      ) : null}
                    </td>
                  </tr>
                ))}
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
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Chain verification
// ---------------------------------------------------------------------------

function ChainPanel() {
  const [chain, setChain] = useState<ChainStatus | null>(null);
  const [probing, setProbing] = useState(true);
  const [probeFailure, setProbeFailure] = useState<ApiFailure | null>(null);

  const [report, setReport] = useState<VerifyReport | null>(null);
  const [verifying, setVerifying] = useState(false);
  const [verifyFailure, setVerifyFailure] = useState<ApiFailure | null>(null);

  const [deep, setDeep] = useState(true);
  const [bundling, setBundling] = useState(false);
  const [bundleNote, setBundleNote] = useState<string | null>(null);

  const probe = useCallback(async () => {
    setProbing(true);
    setProbeFailure(null);
    try {
      setChain(await getChainStatus());
    } catch (err) {
      setChain(null);
      setProbeFailure(describeError(err, 'The chain status endpoint did not answer.'));
    } finally {
      setProbing(false);
    }
  }, []);

  useEffect(() => {
    void probe();
  }, [probe]);

  const runVerify = async () => {
    setVerifying(true);
    setVerifyFailure(null);
    try {
      // A broken chain comes back as a 409 carrying the report; the API layer
      // returns it as data. Only a pass that could not RUN lands here as an error.
      setReport(await verifyChain({ deep, full: false }));
      await probe();
    } catch (err) {
      setReport(null);
      setVerifyFailure(describeError(err, 'The verification pass could not be run.'));
    } finally {
      setVerifying(false);
    }
  };

  const downloadBundle = async () => {
    setBundling(true);
    setBundleNote(null);
    try {
      const { payload, verified } = await exportChainBundle();
      downloadText(
        `audit-bundle-${stamp()}.json`,
        JSON.stringify(payload, null, 2),
        'application/json;charset=utf-8'
      );
      setBundleNote(
        verified
          ? 'Bundle downloaded. It verifies against the published procedure with a SHA-256 implementation and nothing else.'
          : 'Bundle downloaded, but it DID NOT verify. It is preserved as evidence; do not treat it as a clean export.'
      );
    } catch (err) {
      setBundleNote(`Bundle export failed: ${describeError(err, 'The export did not answer.').message}`);
    } finally {
      setBundling(false);
    }
  };

  if (probing) {
    return (
      <div className={PANEL}>
        <div className={PANEL_HEAD}>
          <h2 className={PANEL_TITLE}>Chain verification</h2>
        </div>
        <BlockSkeleton lines={4} />
      </div>
    );
  }

  // The route is not mounted. Say exactly that, and exactly what would mount it.
  if (probeFailure?.notMounted) {
    return (
      <div className={PANEL}>
        <div className={PANEL_HEAD}>
          <h2 className={PANEL_TITLE}>Chain verification</h2>
          <span className="inline-flex items-center gap-1.5 rounded-md bg-amber-50 px-2 py-1 text-xs font-medium text-amber-800">
            <ShieldQuestion size={14} aria-hidden="true" />
            Not verifiable
          </span>
        </div>
        <GapNotice
          title="The tamper-evident chain cannot be checked from here"
          endpoints={[
            'GET  /api/v1/audit/chain/status',
            'GET  /api/v1/audit/chain/verify',
            'GET  /api/v1/audit/chain/export',
            'GET  /api/v1/audit/chain/seals',
          ]}
        >
          <p>
            All four answer 404. The handlers are written, in
            core/internal/handler/audit.go, but the group they live in is never
            mounted: RegisterAuditRoutes is defined and never called. One line inside the protected
            group in core/internal/handler/router.go would mount them —
            <code className="mx-1 rounded bg-amber-100 px-1 py-0.5 font-mono text-xs">
              RegisterAuditRoutes(protected, r.auditHandler)
            </code>
            .
          </p>
          <p className="mt-2">
            Until then the trail below is <strong>unverified</strong>: it can be read, but nobody
            can show that it has not been edited.
          </p>
        </GapNotice>
      </div>
    );
  }

  if (probeFailure) {
    return (
      <div className={PANEL}>
        <div className={PANEL_HEAD}>
          <h2 className={PANEL_TITLE}>Chain verification</h2>
        </div>
        <ErrorBlock
          title="Chain status could not be read"
          failure={probeFailure}
          onRetry={() => void probe()}
        />
      </div>
    );
  }

  const last = report ?? chain?.last_verification ?? null;
  const intact = last?.ok === true;
  const broken = last?.ok === false;

  return (
    <div className={PANEL}>
      <div className={PANEL_HEAD}>
        <div>
          <h2 className={PANEL_TITLE}>Chain verification</h2>
          <p className="mt-0.5 text-xs text-gray-500">
            Whether the audit trail can be shown to be unedited, and where it first breaks if not.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <label className="flex items-center gap-1.5 text-xs text-gray-600">
            <input
              type="checkbox"
              className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
              checked={deep}
              onChange={(event) => setDeep(event.target.checked)}
            />
            Deep pass
          </label>
          <button type="button" className={BTN_PRIMARY} onClick={runVerify} disabled={verifying}>
            {verifying ? (
              <Loader2 size={16} className="animate-spin" aria-hidden="true" />
            ) : (
              <ShieldCheck size={16} aria-hidden="true" />
            )}
            {verifying ? 'Verifying…' : 'Verify now'}
          </button>
          <button
            type="button"
            className={BTN_SECONDARY}
            onClick={downloadBundle}
            disabled={bundling}
          >
            {bundling ? 'Preparing…' : 'Export chain bundle'}
          </button>
        </div>
      </div>

      <div
        className={cn(
          'flex items-start gap-3 border-b border-gray-200 px-5 py-4',
          broken ? 'bg-red-50' : intact ? 'bg-emerald-50' : 'bg-gray-50'
        )}
      >
        {broken ? (
          <ShieldAlert className="mt-0.5 shrink-0 text-red-600" size={20} aria-hidden="true" />
        ) : intact ? (
          <CheckCircle2 className="mt-0.5 shrink-0 text-emerald-600" size={20} aria-hidden="true" />
        ) : (
          <ShieldQuestion className="mt-0.5 shrink-0 text-gray-400" size={20} aria-hidden="true" />
        )}
        <div className="min-w-0">
          <p
            className={cn(
              'text-sm font-semibold',
              broken ? 'text-red-800' : intact ? 'text-emerald-800' : 'text-gray-900'
            )}
          >
            {broken
              ? 'The chain is BROKEN'
              : intact
                ? 'The chain is intact'
                : 'The chain has not been verified yet'}
          </p>
          {broken ? (
            <dl className="mt-2 grid grid-cols-1 gap-x-6 gap-y-1 text-sm text-red-800 sm:grid-cols-2">
              <Field label="First break at sequence" value={last?.break_seq?.toString() ?? EM_DASH} />
              <Field label="Reason" value={last?.break_reason ?? EM_DASH} />
              <Field label="Recorded at" value={formatTimestamp(last?.break_at)} />
              <Field label="Entry" value={shortId(last?.break_log_id, 12)} />
            </dl>
          ) : (
            <p className="mt-1 text-sm text-gray-700">
              {report?.note ||
                (last
                  ? `${last.checked.toLocaleString()} entries checked in ${last.mode} mode, ${formatTimestamp(last.ran_at)}.`
                  : 'Run a pass to establish whether the trail has been edited.')}
            </p>
          )}
        </div>
      </div>

      {verifyFailure ? (
        <ErrorBlock title="The verification pass could not be run" failure={verifyFailure} />
      ) : null}

      {bundleNote ? (
        <p className="border-b border-gray-200 bg-gray-50 px-5 py-2 text-xs text-gray-600">
          {bundleNote}
        </p>
      ) : null}

      {chain ? (
        <dl className="grid grid-cols-2 gap-x-6 gap-y-3 px-5 py-4 text-sm sm:grid-cols-3 lg:grid-cols-5">
          <Field label="Entries in chain" value={chain.entries.toLocaleString()} />
          <Field label="Sequence range" value={`${chain.first_seq} – ${chain.last_seq}`} />
          <Field label="Head sequence" value={chain.head_seq.toString()} />
          <Field label="Head hash" value={chain.head_hash ? `${chain.head_hash.slice(0, 16)}…` : EM_DASH} mono />
          <Field label="Seals" value={chain.seals.toLocaleString()} />
          <Field label="Oldest entry" value={formatTimestamp(chain.oldest_at)} />
          <Field label="Newest entry" value={formatTimestamp(chain.newest_at)} />
          <Field
            label="Last pass"
            value={
              chain.last_verification
                ? `${chain.last_verification.mode}, ${formatTimestamp(chain.last_verification.ran_at)}`
                : 'Never'
            }
          />
        </dl>
      ) : null}
    </div>
  );
}

function Field({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-gray-500">{label}</dt>
      <dd
        className={cn(
          'truncate text-sm text-gray-900',
          mono && 'font-mono text-xs'
        )}
        title={value}
      >
        {value}
      </dd>
    </div>
  );
}

function StatTile({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div className={cn(PANEL, 'px-5 py-4')}>
      <p className="text-xs font-medium text-gray-500">{label}</p>
      <p className={cn('mt-1 text-2xl font-semibold', tone ?? 'text-gray-900')}>{value}</p>
    </div>
  );
}
