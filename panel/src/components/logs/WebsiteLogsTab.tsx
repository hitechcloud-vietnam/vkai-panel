'use client';

/**
 * Website Logs - access and error lines for one site.
 *
 * WHAT IS REAL HERE, AND WHAT IS NOT
 *
 * Real: the site picker (GET /api/v1/websites), the list of log files the panel
 * has been told about for that site's node (GET /api/v1/logs/sources), and the
 * line table itself (GET /api/v1/logs/search), which is filtered by node, by
 * source and by the shared time/level/search controls and paged by the server.
 *
 * TAILING: implemented as polling, not streaming, and only over lines that have
 * been ingested into the panel. Every few seconds the newest page is re-fetched
 * with the same bounded `limit`. That is a deliberate choice over the obvious
 * alternative - there is no log-streaming socket in the backend (the /ws hub is
 * a generic pub/sub with nothing publishing log lines into it), and the file
 * read endpoint that does exist, GET /api/v1/files/read, returns the ENTIRE file
 * in one JSON string and refuses anything over 10 MB. Building a tail on that
 * would mean re-downloading the whole file on every poll, which is the exact
 * defect this screen is supposed to avoid. So the tail here re-requests N rows,
 * never a file, and the raw-file gap is stated on screen rather than papered
 * over with a control that appears to work.
 *
 * DOWNLOAD: not offered, because it cannot be honoured. GET /api/v1/files/download
 * exists but the file manager is jailed to the web root
 * (/vkai-panel/www/domains by default, service/file_manager.go), and access and
 * error logs live outside it. A download button pointed at that route would 400
 * on every site. What is offered instead is an export of the ingested lines,
 * which is data the panel actually holds.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Globe, Play, RefreshCw, Square } from 'lucide-react';
import { cn } from '@/lib/utils';
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
  GapNotice,
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
  stamp,
  toCsv,
} from './common';
import { describeError, listLogSources, listWebsites, searchLogEntries } from './api';
import type { ApiFailure, LogEntry, LogSource, WebsiteSummary } from './types';

/** How often a running tail re-requests the newest page. */
const TAIL_INTERVAL_MS = 5000;

export default function WebsiteLogsTab() {
  const [sites, setSites] = useState<WebsiteSummary[]>([]);
  const [sitesLoading, setSitesLoading] = useState(true);
  const [sitesFailure, setSitesFailure] = useState<ApiFailure | null>(null);
  const [siteFilter, setSiteFilter] = useState('');
  const [selectedId, setSelectedId] = useState('');

  const [sources, setSources] = useState<LogSource[]>([]);
  const [sourceName, setSourceName] = useState('');

  const [range, setRange] = useState<TimeRange>(DEFAULT_RANGE);
  const [level, setLevel] = useState('');
  const [search, setSearch] = useState('');
  const [limit, setLimit] = useState(100);
  const [offset, setOffset] = useState(0);
  const [generation, setGeneration] = useState(0);

  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [total, setTotal] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [failure, setFailure] = useState<ApiFailure | null>(null);

  const [tailing, setTailing] = useState(false);
  const [lastTailAt, setLastTailAt] = useState<string | null>(null);
  const [exporting, setExporting] = useState(false);
  const [exportNote, setExportNote] = useState<string | null>(null);

  const applied = useRef({ range, level, search, sourceName });

  const selected = useMemo(
    () => sites.find((site) => site.id === selectedId) ?? null,
    [sites, selectedId]
  );

  useEffect(() => {
    let cancelled = false;
    setSitesLoading(true);
    setSitesFailure(null);
    listWebsites()
      .then((list) => {
        if (cancelled) return;
        setSites(list);
        setSelectedId((current) => current || list[0]?.id || '');
      })
      .catch((err) => {
        if (!cancelled) setSitesFailure(describeError(err, 'The website list did not answer.'));
      })
      .finally(() => {
        if (!cancelled) setSitesLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Which log files the panel knows about on the node this site lives on. A
  // failure here is not fatal: it only empties the file picker.
  useEffect(() => {
    if (!selected?.server_id) {
      setSources([]);
      return;
    }
    let cancelled = false;
    listLogSources(selected.server_id)
      .then((list) => {
        if (!cancelled) setSources(list);
      })
      .catch(() => {
        if (!cancelled) setSources([]);
      });
    return () => {
      cancelled = true;
    };
  }, [selected?.server_id]);

  const load = useCallback(
    async (quiet = false) => {
      if (!selected) return;
      if (!quiet) setLoading(true);
      setFailure(null);
      const committed = applied.current;
      try {
        const result = await searchLogEntries({
          ...rangeToParams(committed.range),
          level: committed.level,
          query: committed.search,
          source: committed.sourceName,
          server_id: selected.server_id,
          limit,
          offset: quiet ? 0 : offset,
        });
        setEntries(result.entries);
        setTotal(result.total);
        if (quiet) setLastTailAt(new Date().toLocaleTimeString());
      } catch (err) {
        setEntries([]);
        setTotal(null);
        setFailure(describeError(err, 'The website log search did not answer.'));
        setTailing(false);
      } finally {
        if (!quiet) setLoading(false);
      }
    },
    [limit, offset, selected]
  );

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load, generation]);

  // The tail. It asks for the newest page on an interval; it never asks for a
  // file, and it stops itself the moment a request fails.
  useEffect(() => {
    if (!tailing || !selected) return;
    const timer = window.setInterval(() => {
      void load(true);
    }, TAIL_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [tailing, selected, load]);

  const apply = () => {
    applied.current = { range, level, search, sourceName };
    setOffset(0);
    setGeneration((value) => value + 1);
  };

  const visibleSites = useMemo(() => {
    const needle = siteFilter.trim().toLowerCase();
    if (!needle) return sites;
    return sites.filter((site) => (site.domain || '').toLowerCase().includes(needle));
  }, [sites, siteFilter]);

  const exportCsv = async () => {
    if (!selected) return;
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
          source: committed.sourceName,
          server_id: selected.server_id,
          limit: EXPORT_PAGE_SIZE,
          offset: cursor,
        });
        collected.push(...page.entries);
        if (page.entries.length < EXPORT_PAGE_SIZE) break;
      }
      downloadText(
        `${selected.domain || 'website'}-logs-${stamp()}.csv`,
        toCsv(
          ['Time', 'Level', 'Source', 'Message'],
          collected.map((entry) => [
            entry.timestamp || entry.created_at,
            entry.level,
            entry.source,
            entry.message,
          ])
        )
      );
      setExportNote(
        collected.length >= EXPORT_MAX_ROWS
          ? `Exported the first ${EXPORT_MAX_ROWS.toLocaleString()} ingested lines, the export ceiling.`
          : `Exported ${collected.length.toLocaleString()} ingested lines.`
      );
    } catch (err) {
      setExportNote(`Export failed: ${describeError(err, 'The export could not be completed.').message}`);
    } finally {
      setExporting(false);
    }
  };

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-[18rem_minmax(0,1fr)]">
      {/* Site picker */}
      <div className={cn(PANEL, 'self-start')}>
        <div className="border-b border-gray-200 px-4 py-3">
          <h2 className={PANEL_TITLE}>Websites</h2>
          <input
            type="search"
            className={cn(FIELD, 'mt-2')}
            placeholder="Filter by domain"
            aria-label="Filter websites"
            value={siteFilter}
            onChange={(event) => setSiteFilter(event.target.value)}
          />
        </div>

        {sitesLoading ? (
          <TableSkeleton rows={6} columns={1} />
        ) : sitesFailure ? (
          <ErrorBlock title="Websites could not be listed" failure={sitesFailure} />
        ) : visibleSites.length === 0 ? (
          <EmptyState
            icon={<Globe size={36} aria-hidden="true" />}
            title={sites.length === 0 ? 'No websites' : 'No match'}
            message={
              sites.length === 0
                ? 'Add a website first; its logs appear here once the site exists.'
                : 'No domain matches that filter.'
            }
          />
        ) : (
          <ul className="max-h-[28rem] overflow-y-auto py-1" role="listbox" aria-label="Websites">
            {visibleSites.map((site) => {
              const active = site.id === selectedId;
              return (
                <li key={site.id}>
                  <button
                    type="button"
                    role="option"
                    aria-selected={active}
                    onClick={() => {
                      setSelectedId(site.id);
                      setSourceName('');
                      setOffset(0);
                      setTailing(false);
                      applied.current = { range, level, search, sourceName: '' };
                      setGeneration((value) => value + 1);
                    }}
                    className={cn(
                      'w-full border-l-2 px-4 py-2 text-left text-sm transition-colors',
                      active
                        ? 'border-brand-600 bg-brand-50 font-medium text-brand-700'
                        : 'border-transparent text-gray-700 hover:bg-gray-50'
                    )}
                  >
                    <span className="block truncate">{site.domain || EM_DASH}</span>
                    <span className="block truncate text-xs text-gray-500">
                      {site.web_server_type || 'unknown web server'}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      {/* Lines */}
      <div className={PANEL}>
        <div className={PANEL_HEAD}>
          <div className="min-w-0">
            <h2 className={cn(PANEL_TITLE, 'truncate')}>
              {selected ? selected.domain : 'Website logs'}
            </h2>
            <p className="mt-0.5 text-xs text-gray-500">
              Lines ingested into the panel for this site&apos;s node.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              className={BTN_SECONDARY}
              onClick={() => setTailing((value) => !value)}
              disabled={!selected || Boolean(failure)}
              aria-pressed={tailing}
            >
              {tailing ? (
                <>
                  <Square size={16} aria-hidden="true" />
                  Stop tail
                </>
              ) : (
                <>
                  <Play size={16} aria-hidden="true" />
                  Start tail
                </>
              )}
            </button>
            <button
              type="button"
              className={BTN_SECONDARY}
              onClick={() => setGeneration((value) => value + 1)}
              disabled={loading || !selected}
            >
              <RefreshCw size={16} className={cn(loading && 'animate-spin')} aria-hidden="true" />
              Refresh
            </button>
          </div>
        </div>

        {tailing ? (
          <p className="border-b border-gray-200 bg-emerald-50 px-5 py-2 text-xs text-emerald-800">
            Tailing: the newest {limit} lines are re-requested every {TAIL_INTERVAL_MS / 1000}{' '}
            seconds. The whole file is never fetched.
            {lastTailAt ? ` Last updated ${lastTailAt}.` : ''}
          </p>
        ) : null}

        <LogToolbar
          range={range}
          onRangeChange={setRange}
          level={level}
          onLevelChange={setLevel}
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search request lines"
          searchHint="All four filters are applied by the server, over one page at a time."
          onApply={apply}
          onExport={exportCsv}
          exportLabel={exporting ? 'Exporting…' : 'Export CSV'}
          exportDisabled={exporting || loading || !selected}
          exportHint={`Export walks pages of ${EXPORT_PAGE_SIZE} and stops at ${EXPORT_MAX_ROWS.toLocaleString()} rows.`}
          busy={loading}
        >
          <div className="w-full sm:w-52">
            <label className={FIELD_LABEL} htmlFor="website-log-source">
              Log file
            </label>
            <select
              id="website-log-source"
              className={FIELD}
              value={sourceName}
              onChange={(event) => setSourceName(event.target.value)}
              disabled={sources.length === 0}
            >
              <option value="">
                {sources.length === 0 ? 'No log files registered' : 'All log files'}
              </option>
              {sources.map((source) => (
                <option key={source.id} value={source.name}>
                  {source.name}
                  {source.type ? ` (${source.type})` : ''}
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

        {!selected ? (
          <EmptyState
            icon={<Globe size={36} aria-hidden="true" />}
            title="Choose a website"
            message="Pick a site on the left to see the access and error lines recorded for it."
          />
        ) : loading ? (
          <TableSkeleton rows={8} columns={4} />
        ) : failure ? (
          <ErrorBlock
            title="Website logs could not be loaded"
            failure={failure}
            onRetry={() => setGeneration((value) => value + 1)}
          />
        ) : entries.length === 0 ? (
          <EmptyState
            icon={<Globe size={36} aria-hidden="true" />}
            title="No lines for this site"
            message="Nothing has been ingested for this node in the selected range. Register the site's access and error logs as log sources, and make sure something is shipping their lines to POST /api/v1/logs/servers/{server_id}/entries."
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50">
                <tr>
                  <th className={TH}>Time</th>
                  <th className={TH}>Level</th>
                  <th className={TH}>Log file</th>
                  <th className={TH}>Line</th>
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
                    <td className={cn(TD, 'font-mono text-xs')}>
                      <span className="break-all">{entry.message || EM_DASH}</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {selected && !loading && !failure && entries.length > 0 ? (
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

        {selected && sources.length > 0 ? (
          <div className="border-t border-gray-200 px-5 py-4">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
              Registered log files on this node
            </h3>
            <div className="mt-2 overflow-x-auto">
              <table className="w-full">
                <thead className="bg-gray-50">
                  <tr>
                    <th className={TH}>Name</th>
                    <th className={TH}>Type</th>
                    <th className={TH}>Path</th>
                    <th className={TH}>Active</th>
                    <th className={TH}>Last read</th>
                  </tr>
                </thead>
                <tbody>
                  {sources.map((source) => (
                    <tr key={source.id}>
                      <td className={cn(TD, 'font-medium text-gray-900')}>{source.name}</td>
                      <td className={TD}>{source.type || EM_DASH}</td>
                      <td className={cn(TD, 'font-mono text-xs')}>{source.path || EM_DASH}</td>
                      <td className={TD}>{source.is_active ? 'Yes' : 'No'}</td>
                      <td className={cn(TD, 'whitespace-nowrap')} suppressHydrationWarning>
                        {formatTimestamp(source.last_read_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ) : null}

        <GapNotice title="Raw log files cannot be read or downloaded from here yet">
          <p>
            The table above shows lines that have been <strong>ingested</strong> into the panel, and
            the tail re-requests the newest page rather than streaming a file. Reading or
            downloading the raw access and error files needs backend work that does not exist yet:
          </p>
          <ul className="mt-2 list-disc space-y-1 pl-5 text-sm">
            <li>
              a range read, so the last N lines of a large file can be fetched without loading the
              whole file (GET /api/v1/files/read returns the entire file and refuses anything over
              10 MB);
            </li>
            <li>
              a download that is allowed to leave the web-root jail, which today confines
              GET /api/v1/files/download to /vkai-panel/www/domains while logs live elsewhere;
            </li>
            <li>an ingester that ships access and error lines into the panel at all.</li>
          </ul>
        </GapNotice>
      </div>
    </div>
  );
}
