'use client';

/**
 * Panel Logs - what the panel itself did.
 *
 * Wired to GET /api/v1/logs/search, which is mounted in router.go and takes
 * source, level, query, server_id, start, end, limit and offset. The filter
 * controls map one-for-one onto those parameters: "Component" is the entry's
 * `source`, which is what the panel writes the emitting subsystem into.
 *
 * Paging is the server's, not the browser's. One page is in memory at a time
 * and the export walks pages up to a hard ceiling rather than asking for
 * everything, because a log table is exactly the place where "just fetch it
 * all" turns into a locked tab.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FileText, Loader2, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import { serverApi, unwrapList } from '@/services/api';
import type { ManagedServer } from '@/types/server';
import {
  BTN_SECONDARY,
  DEFAULT_RANGE,
  EM_DASH,
  EXPORT_MAX_ROWS,
  EXPORT_PAGE_SIZE,
  EmptyState,
  ErrorBlock,
  FIELD,
  FIELD_LABEL,
  LevelPill,
  LogToolbar,
  PANEL,
  PANEL_HEAD,
  PANEL_TITLE,
  Pagination,
  TD,
  TH,
  TableSkeleton,
  type TimeRange,
  downloadText,
  formatTimestamp,
  rangeToParams,
  shortId,
  stamp,
  toCsv,
} from './common';
import { describeError, listLogSources, searchLogEntries } from './api';
import type { ApiFailure, LogEntry } from './types';

export default function PanelLogsTab() {
  const [range, setRange] = useState<TimeRange>(DEFAULT_RANGE);
  const [level, setLevel] = useState('');
  const [search, setSearch] = useState('');
  const [component, setComponent] = useState('');
  const [serverId, setServerId] = useState('');

  const [limit, setLimit] = useState(100);
  const [offset, setOffset] = useState(0);
  const [generation, setGeneration] = useState(0);

  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [total, setTotal] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [failure, setFailure] = useState<ApiFailure | null>(null);

  const [servers, setServers] = useState<ManagedServer[]>([]);
  const [components, setComponents] = useState<string[]>([]);

  const [exporting, setExporting] = useState(false);
  const [exportNote, setExportNote] = useState<string | null>(null);

  // The values the last Apply committed. Typing in a field must not refetch on
  // every keystroke, so the request reads these and not the live inputs.
  const applied = useRef({ range, level, search, component, serverId });

  const load = useCallback(async () => {
    setLoading(true);
    setFailure(null);
    const committed = applied.current;
    try {
      const result = await searchLogEntries({
        ...rangeToParams(committed.range),
        level: committed.level,
        query: committed.search,
        source: committed.component,
        server_id: committed.serverId,
        limit,
        offset,
      });
      setEntries(result.entries);
      setTotal(result.total);
    } catch (err) {
      setEntries([]);
      setTotal(null);
      setFailure(describeError(err, 'The panel log search did not answer.'));
    } finally {
      setLoading(false);
    }
  }, [limit, offset]);

  useEffect(() => {
    void load();
    // `generation` is bumped by Apply and Refresh so a committed filter change
    // refetches even when limit and offset are unchanged.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load, generation]);

  // The pickers are conveniences. Neither failing is worth an error block: the
  // filters below them still work as free text, so a failure just leaves the
  // list of suggestions empty.
  useEffect(() => {
    let cancelled = false;
    serverApi
      .list({ page: 1, per_page: 200 })
      .then((res) => {
        if (!cancelled) setServers(unwrapList<ManagedServer>(res));
      })
      .catch(() => undefined);
    listLogSources()
      .then((sources) => {
        if (cancelled) return;
        const names = new Set<string>();
        sources.forEach((source) => {
          if (source.name) names.add(source.name);
          if (source.type) names.add(source.type);
        });
        setComponents(Array.from(names).sort());
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);

  // Suggestions grow with whatever the current page actually contains, so the
  // datalist is useful even before any source has been registered.
  const componentSuggestions = useMemo(() => {
    const names = new Set(components);
    entries.forEach((entry) => {
      if (entry.source) names.add(entry.source);
    });
    return Array.from(names).sort();
  }, [components, entries]);

  const apply = () => {
    applied.current = { range, level, search, component, serverId };
    setOffset(0);
    setGeneration((value) => value + 1);
  };

  const exportCsv = async () => {
    setExporting(true);
    setExportNote(null);
    const committed = applied.current;
    const collected: LogEntry[] = [];
    try {
      for (let cursor = 0; cursor < EXPORT_MAX_ROWS; cursor += EXPORT_PAGE_SIZE) {
        const page = await searchLogEntries({
          ...rangeToParams(committed.range),
          level: committed.level,
          query: committed.search,
          source: committed.component,
          server_id: committed.serverId,
          limit: EXPORT_PAGE_SIZE,
          offset: cursor,
        });
        collected.push(...page.entries);
        if (page.entries.length < EXPORT_PAGE_SIZE) break;
      }
      downloadText(
        `panel-logs-${stamp()}.csv`,
        toCsv(
          ['Time', 'Level', 'Component', 'Server', 'Message', 'Details'],
          collected.map((entry) => [
            entry.timestamp || entry.created_at,
            entry.level,
            entry.source,
            entry.server_id,
            entry.message,
            entry.details ? JSON.stringify(entry.details) : '',
          ])
        )
      );
      setExportNote(
        collected.length >= EXPORT_MAX_ROWS
          ? `Exported the first ${EXPORT_MAX_ROWS.toLocaleString()} rows, the export ceiling. Narrow the time range to capture the rest.`
          : `Exported ${collected.length.toLocaleString()} rows.`
      );
    } catch (err) {
      const problem = describeError(err, 'The export could not be completed.');
      setExportNote(`Export failed: ${problem.message}`);
    } finally {
      setExporting(false);
    }
  };

  return (
    <div className={PANEL}>
      <div className={PANEL_HEAD}>
        <div>
          <h2 className={PANEL_TITLE}>Panel Logs</h2>
          <p className="mt-0.5 text-xs text-gray-500">
            Operations and runtime events recorded by the panel itself.
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
        level={level}
        onLevelChange={setLevel}
        search={search}
        onSearchChange={setSearch}
        searchPlaceholder="Search message text"
        searchHint="Search, level, component and time range are all applied by the server."
        onApply={apply}
        onExport={exportCsv}
        exportLabel={exporting ? 'Exporting…' : 'Export CSV'}
        exportDisabled={exporting || loading}
        exportHint={`Export walks pages of ${EXPORT_PAGE_SIZE} and stops at ${EXPORT_MAX_ROWS.toLocaleString()} rows.`}
        busy={loading}
      >
        <div className="w-full sm:w-48">
          <label className={FIELD_LABEL} htmlFor="panel-log-component">
            Component
          </label>
          <input
            id="panel-log-component"
            className={FIELD}
            list="panel-log-components"
            placeholder="Any component"
            value={component}
            onChange={(event) => setComponent(event.target.value)}
          />
          <datalist id="panel-log-components">
            {componentSuggestions.map((name) => (
              <option key={name} value={name} />
            ))}
          </datalist>
        </div>

        <div className="w-full sm:w-48">
          <label className={FIELD_LABEL} htmlFor="panel-log-server">
            Node
          </label>
          <select
            id="panel-log-server"
            className={FIELD}
            value={serverId}
            onChange={(event) => setServerId(event.target.value)}
          >
            <option value="">All nodes</option>
            {servers.map((server) => (
              <option key={server.id} value={server.id}>
                {server.hostname || server.ip_address || shortId(server.id)}
              </option>
            ))}
          </select>
        </div>
      </LogToolbar>

      {exportNote ? (
        <p className="border-b border-gray-200 bg-gray-50 px-5 py-2 text-xs text-gray-600">
          {exportNote}
        </p>
      ) : null}

      {loading ? (
        <TableSkeleton rows={8} columns={5} />
      ) : failure ? (
        <ErrorBlock
          title="Panel logs could not be loaded"
          failure={failure}
          onRetry={() => setGeneration((value) => value + 1)}
        />
      ) : entries.length === 0 ? (
        <EmptyState
          icon={<FileText size={36} aria-hidden="true" />}
          title="No panel log entries"
          message="Nothing matched these filters. Widen the time range, or clear the level and component filters. If the panel has only just been installed, nothing has been recorded yet."
        />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead className="bg-gray-50">
              <tr>
                <th className={TH}>Time</th>
                <th className={TH}>Level</th>
                <th className={TH}>Component</th>
                <th className={TH}>Node</th>
                <th className={TH}>Message</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <tr key={entry.id} className="hover:bg-gray-50">
                  <td className={cn(TD, 'whitespace-nowrap')} suppressHydrationWarning>
                    {formatTimestamp(entry.timestamp || entry.created_at)}
                  </td>
                  <td className={TD}>
                    <LevelPill level={entry.level} />
                  </td>
                  <td className={cn(TD, 'whitespace-nowrap font-medium text-gray-900')}>
                    {entry.source || EM_DASH}
                  </td>
                  <td className={cn(TD, 'whitespace-nowrap font-mono text-xs')}>
                    {shortId(entry.server_id)}
                  </td>
                  <td className={TD}>
                    <span className="break-words">{entry.message || EM_DASH}</span>
                    {entry.details && Object.keys(entry.details).length > 0 ? (
                      <details className="mt-1">
                        <summary className="cursor-pointer text-xs text-brand-700">
                          Details
                        </summary>
                        <pre className="mt-1 max-h-48 overflow-auto rounded-md bg-gray-50 p-2 text-xs text-gray-700">
                          {JSON.stringify(entry.details, null, 2)}
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

      {!loading && !failure && entries.length > 0 ? (
        <Pagination
          offset={offset}
          limit={limit}
          total={total}
          loaded={entries.length}
          onOffsetChange={setOffset}
          onLimitChange={(next) => {
            setLimit(next);
            setOffset(0);
          }}
        />
      ) : null}

      {exporting ? (
        <p className="flex items-center gap-2 border-t border-gray-200 px-5 py-3 text-xs text-gray-600">
          <Loader2 size={14} className="animate-spin" aria-hidden="true" />
          Collecting rows for the export…
        </p>
      ) : null}
    </div>
  );
}
