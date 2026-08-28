'use client';

/**
 * Soft Logs - the journals of the installed software.
 *
 * Wired to GET /api/v1/services (the systemd units on the host) and
 * GET /api/v1/services/{name}/logs?lines=N, both mounted in router.go and both
 * administrator-only.
 *
 * SIZE: this is the one section where the backend already does the right thing.
 * The handler runs `journalctl -u <unit> -n <lines>`, so the response is
 * truncated on the SERVER before it is serialised - the panel receives at most N
 * lines and there is no code path that could ask for a whole journal. The picker
 * stops at 5000 because service_manager.go caps there and silently falls back to
 * 100 above it, and offering 50000 would mean a control that quietly returns
 * something other than what it says.
 *
 * The level filter, the search box and the time range run in the browser, over
 * those N lines and nothing else: `journalctl --since/--until` and `-p` are not
 * exposed by the route. The toolbar says so rather than implying a server-side
 * search over the whole journal.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import { RefreshCw, Server, Terminal } from 'lucide-react';
import { cn } from '@/lib/utils';
import {
  BTN_SECONDARY,
  BlockSkeleton,
  DEFAULT_RANGE,
  EmptyState,
  ErrorBlock,
  FIELD,
  FIELD_LABEL,
  GapNotice,
  LogToolbar,
  PANEL,
  PANEL_HEAD,
  PANEL_TITLE,
  TableSkeleton,
  type TimeRange,
  downloadText,
  rangeToBounds,
  stamp,
} from './common';
import { describeError, getServiceLogs, listServices } from './api';
import type { ApiFailure, ServiceInfo } from './types';

/**
 * The units this screen is about: the web server, the database, PHP-FPM and the
 * other services a hosting operator installs. Anything else is still reachable
 * through "All units", but the default list is not 300 systemd targets.
 */
const SOFTWARE_PATTERNS = [
  /^nginx/i,
  /^apache2?/i,
  /^httpd/i,
  /^openlitespeed/i,
  /^lsws/i,
  /^caddy/i,
  /^traefik/i,
  /^mysqld?/i,
  /^mariadb/i,
  /^postgresql/i,
  /^redis/i,
  /^memcached/i,
  /^mongod/i,
  /php.*fpm/i,
  /^pure-ftpd/i,
  /^vsftpd/i,
  /^proftpd/i,
  /^dovecot/i,
  /^postfix/i,
  /^vkai/i,
];

/** journalctl caps at this in the backend; asking for more silently gives 100. */
const LINE_CHOICES = [100, 500, 1000, 5000];

const LEVEL_WORDS: Record<string, RegExp> = {
  debug: /\bdebug\b/i,
  info: /\binfo(rmation)?\b/i,
  notice: /\bnotice\b/i,
  warning: /\bwarn(ing)?\b/i,
  error: /\berr(or)?\b/i,
  critical: /\b(crit(ical)?|fatal|emerg(ency)?|alert)\b/i,
};

interface ParsedLine {
  raw: string;
  at: number | null;
}

/** `--output=short-iso` puts an ISO timestamp first; anything else stays unparsed. */
function parseLine(raw: string): ParsedLine {
  const token = raw.slice(0, 40).split(/\s+/, 1)[0];
  if (!token) return { raw, at: null };
  const parsed = Date.parse(token);
  return { raw, at: Number.isNaN(parsed) ? null : parsed };
}

export default function SoftLogsTab() {
  const [services, setServices] = useState<ServiceInfo[]>([]);
  const [servicesLoading, setServicesLoading] = useState(true);
  const [servicesFailure, setServicesFailure] = useState<ApiFailure | null>(null);
  const [showAllUnits, setShowAllUnits] = useState(false);
  const [unitFilter, setUnitFilter] = useState('');
  const [selected, setSelected] = useState('');

  const [lines, setLines] = useState(500);
  const [text, setText] = useState('');
  const [loading, setLoading] = useState(false);
  const [failure, setFailure] = useState<ApiFailure | null>(null);
  const [generation, setGeneration] = useState(0);

  const [range, setRange] = useState<TimeRange>({ ...DEFAULT_RANGE, key: 'all' });
  const [level, setLevel] = useState('');
  const [search, setSearch] = useState('');

  useEffect(() => {
    let cancelled = false;
    setServicesLoading(true);
    setServicesFailure(null);
    listServices()
      .then((list) => {
        if (cancelled) return;
        setServices(list);
        const preferred = list.find((service) =>
          SOFTWARE_PATTERNS.some((pattern) => pattern.test(service.name))
        );
        setSelected((current) => current || preferred?.name || list[0]?.name || '');
      })
      .catch((err) => {
        if (!cancelled) setServicesFailure(describeError(err, 'The service list did not answer.'));
      })
      .finally(() => {
        if (!cancelled) setServicesLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const load = useCallback(async () => {
    if (!selected) return;
    setLoading(true);
    setFailure(null);
    try {
      setText(await getServiceLogs(selected, lines));
    } catch (err) {
      setText('');
      setFailure(describeError(err, 'The journal for this unit did not answer.'));
    } finally {
      setLoading(false);
    }
  }, [selected, lines]);

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load, generation]);

  const visibleUnits = useMemo(() => {
    const needle = unitFilter.trim().toLowerCase();
    return services
      .filter((service) =>
        showAllUnits ? true : SOFTWARE_PATTERNS.some((pattern) => pattern.test(service.name))
      )
      .filter((service) => (needle ? service.name.toLowerCase().includes(needle) : true))
      .sort((a, b) => a.name.localeCompare(b.name));
  }, [services, showAllUnits, unitFilter]);

  const parsed = useMemo(
    () =>
      text
        .split('\n')
        .filter((line) => line.trim().length > 0)
        .map(parseLine),
    [text]
  );

  const filtered = useMemo(() => {
    const bounds = rangeToBounds(range);
    const needle = search.trim().toLowerCase();
    const levelPattern = level ? LEVEL_WORDS[level] : null;
    return parsed.filter((line) => {
      if (bounds.start !== null && line.at !== null && line.at < bounds.start) return false;
      if (bounds.end !== null && line.at !== null && line.at > bounds.end) return false;
      if (levelPattern && !levelPattern.test(line.raw)) return false;
      if (needle && !line.raw.toLowerCase().includes(needle)) return false;
      return true;
    });
  }, [parsed, range, level, search]);

  const exportLog = () => {
    downloadText(
      `${selected || 'unit'}-${stamp()}.log`,
      `${filtered.map((line) => line.raw).join('\n')}\n`,
      'text/plain;charset=utf-8'
    );
  };

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-[18rem_minmax(0,1fr)]">
      <div className={cn(PANEL, 'self-start')}>
        <div className="border-b border-gray-200 px-4 py-3">
          <h2 className={PANEL_TITLE}>Installed software</h2>
          <input
            type="search"
            className={cn(FIELD, 'mt-2')}
            placeholder="Filter units"
            aria-label="Filter units"
            value={unitFilter}
            onChange={(event) => setUnitFilter(event.target.value)}
          />
          <label className="mt-2 flex items-center gap-2 text-xs text-gray-600">
            <input
              type="checkbox"
              className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
              checked={showAllUnits}
              onChange={(event) => setShowAllUnits(event.target.checked)}
            />
            Show every systemd unit
          </label>
        </div>

        {servicesLoading ? (
          <TableSkeleton rows={6} columns={1} />
        ) : servicesFailure ? (
          <ErrorBlock
            title={
              servicesFailure.forbidden
                ? 'This account cannot list services'
                : 'Services could not be listed'
            }
            failure={{
              ...servicesFailure,
              message: servicesFailure.forbidden
                ? 'GET /api/v1/services is restricted to administrators. Software logs need an administrator account.'
                : servicesFailure.message,
            }}
          />
        ) : visibleUnits.length === 0 ? (
          <EmptyState
            icon={<Server size={36} aria-hidden="true" />}
            title="No matching units"
            message={
              showAllUnits
                ? 'No systemd unit matches that filter.'
                : 'No web server, database or PHP-FPM unit was found. Tick "Show every systemd unit" to see the rest.'
            }
          />
        ) : (
          <ul className="max-h-[28rem] overflow-y-auto py-1" role="listbox" aria-label="Units">
            {visibleUnits.map((service) => {
              const active = service.name === selected;
              const running = (service.active_state || '').toLowerCase() === 'active';
              return (
                <li key={service.name}>
                  <button
                    type="button"
                    role="option"
                    aria-selected={active}
                    onClick={() => {
                      setSelected(service.name);
                      setGeneration((value) => value + 1);
                    }}
                    className={cn(
                      'flex w-full items-center justify-between gap-2 border-l-2 px-4 py-2 text-left text-sm transition-colors',
                      active
                        ? 'border-brand-600 bg-brand-50 font-medium text-brand-700'
                        : 'border-transparent text-gray-700 hover:bg-gray-50'
                    )}
                  >
                    <span className="truncate">{service.name}</span>
                    <span
                      className={cn(
                        'shrink-0 rounded-md px-1.5 py-0.5 text-xs font-medium',
                        running ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-600'
                      )}
                    >
                      {service.active_state || 'unknown'}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <div className={PANEL}>
        <div className={PANEL_HEAD}>
          <div className="min-w-0">
            <h2 className={cn(PANEL_TITLE, 'truncate')}>{selected || 'Software logs'}</h2>
            <p className="mt-0.5 text-xs text-gray-500">
              The last {lines.toLocaleString()} journal lines, truncated by the server.
            </p>
          </div>
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

        <LogToolbar
          range={range}
          onRangeChange={setRange}
          level={level}
          onLevelChange={setLevel}
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search the fetched lines"
          searchHint={`Level, search and time range are applied in the browser, over the ${lines.toLocaleString()} lines the server returned - the route exposes no journalctl --since or -p.`}
          onApply={() => setGeneration((value) => value + 1)}
          onExport={exportLog}
          exportLabel="Export .log"
          exportDisabled={filtered.length === 0}
          exportHint="The export contains exactly the lines shown below."
          busy={loading}
        >
          <div className="w-full sm:w-40">
            <label className={FIELD_LABEL} htmlFor="soft-log-lines">
              Lines
            </label>
            <select
              id="soft-log-lines"
              className={FIELD}
              value={lines}
              onChange={(event) => {
                setLines(Number(event.target.value));
                setGeneration((value) => value + 1);
              }}
            >
              {LINE_CHOICES.map((choice) => (
                <option key={choice} value={choice}>
                  Last {choice.toLocaleString()}
                </option>
              ))}
            </select>
          </div>
        </LogToolbar>

        {!selected ? (
          <EmptyState
            icon={<Server size={36} aria-hidden="true" />}
            title="Choose a service"
            message="Pick the web server, database or PHP-FPM unit on the left to read its journal."
          />
        ) : loading ? (
          <BlockSkeleton lines={12} />
        ) : failure ? (
          <ErrorBlock
            title="The journal could not be read"
            failure={{
              ...failure,
              message: failure.forbidden
                ? 'Reading a unit journal is restricted to administrators.'
                : failure.message,
            }}
            onRetry={() => setGeneration((value) => value + 1)}
          />
        ) : parsed.length === 0 ? (
          <EmptyState
            icon={<Terminal size={36} aria-hidden="true" />}
            title="The journal is empty"
            message="journalctl returned nothing for this unit. It may never have started, or its journal may have been rotated away."
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={<Terminal size={36} aria-hidden="true" />}
            title="No line matches these filters"
            message="Clear the level filter, the search box or the time range to see the fetched lines again."
          />
        ) : (
          <>
            <pre className="max-h-[32rem] overflow-auto whitespace-pre-wrap break-words border-b border-gray-200 px-5 py-4 font-mono text-xs leading-relaxed text-gray-800">
              {filtered.map((line) => line.raw).join('\n')}
            </pre>
            <p className="px-5 py-3 text-xs text-gray-500">
              Showing {filtered.length.toLocaleString()} of {parsed.length.toLocaleString()} fetched
              lines.
            </p>
          </>
        )}

        <GapNotice title="The journal is read in fixed-size chunks, not paged">
          <p>
            The route takes a line count and nothing else, so there is no way to walk backwards
            through an older part of a journal, and no server-side level or time filter. What is
            missing:
          </p>
          <ul className="mt-2 list-disc space-y-1 pl-5 text-sm">
            <li>
              since/until and priority parameters on GET /api/v1/services/{'{name}'}/logs, passed
              through to journalctl;
            </li>
            <li>a cursor, so a second page of older lines can be requested;</li>
            <li>
              a route for software that logs to files rather than to the journal - a MySQL slow
              query log, for instance, is not in journalctl at all.
            </li>
          </ul>
        </GapNotice>
      </div>
    </div>
  );
}
