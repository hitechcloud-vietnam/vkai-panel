'use client';

/**
 * The furniture every log section shares.
 *
 * Each tab on this page owes the operator the same three things and gets them
 * from here rather than reinventing them five times: a skeleton while the
 * request is in flight, an empty state that says what to do next, and an error
 * block that prints the server's own words instead of blanking the pane.
 *
 * GapNotice is the fourth, and the one that matters most. Where the backend
 * cannot do something yet, the section says so and names the missing route. It
 * is never replaced by a control that looks real and does nothing.
 */

import { useMemo } from 'react';
import {
  AlertTriangle,
  ChevronLeft,
  ChevronRight,
  Download,
  Search,
  ServerCrash,
} from 'lucide-react';
import { cn } from '@/lib/utils';

// ---------------------------------------------------------------------------
// Design tokens - one place, so the five tabs cannot drift apart
// ---------------------------------------------------------------------------

export const PANEL = 'rounded-lg border border-gray-200 bg-white shadow-sm';
export const PANEL_HEAD =
  'flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-5 py-4';
export const PANEL_TITLE = 'text-sm font-semibold text-gray-900';
export const TH =
  'whitespace-nowrap border-b border-gray-200 px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
export const TD = 'border-b border-gray-100 px-4 py-3 text-sm text-gray-700 align-top';
export const BTN_BASE =
  'inline-flex items-center justify-center gap-2 rounded-md px-3.5 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50';
export const BTN_PRIMARY = `${BTN_BASE} bg-brand-600 text-white hover:bg-brand-700 focus-visible:ring-brand-500`;
export const BTN_SECONDARY = `${BTN_BASE} border border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus-visible:ring-brand-500`;
export const BTN_DANGER = `${BTN_BASE} bg-red-600 text-white hover:bg-red-700 focus-visible:ring-red-500`;
export const FIELD =
  'h-9 w-full rounded-md border border-gray-300 bg-white px-3 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500';
export const FIELD_LABEL = 'mb-1 block text-xs font-medium text-gray-600';

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

export const EM_DASH = '—';

export function formatTimestamp(value: string | null | undefined): string {
  if (!value) return EM_DASH;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return EM_DASH;
  return parsed.toLocaleString();
}

export function shortId(value: string | null | undefined, length = 8): string {
  if (!value) return EM_DASH;
  return value.length > length ? `${value.slice(0, length)}…` : value;
}

/** Pull a string out of a details blob without trusting its shape. */
export function detailString(
  details: Record<string, unknown> | null | undefined,
  key: string
): string | null {
  const value = details?.[key];
  if (typeof value === 'string' && value.trim()) return value.trim();
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return null;
}

// ---------------------------------------------------------------------------
// Level and status colours
// ---------------------------------------------------------------------------

const LEVEL_TONE: Record<string, string> = {
  emergency: 'bg-red-50 text-red-700',
  alert: 'bg-red-50 text-red-700',
  critical: 'bg-red-50 text-red-700',
  crit: 'bg-red-50 text-red-700',
  error: 'bg-red-50 text-red-700',
  err: 'bg-red-50 text-red-700',
  fatal: 'bg-red-50 text-red-700',
  warning: 'bg-amber-50 text-amber-700',
  warn: 'bg-amber-50 text-amber-700',
  notice: 'bg-sky-50 text-sky-700',
  info: 'bg-sky-50 text-sky-700',
  debug: 'bg-gray-100 text-gray-600',
  trace: 'bg-gray-100 text-gray-600',
};

export function levelTone(level: string | null | undefined): string {
  return LEVEL_TONE[(level || '').toLowerCase()] ?? 'bg-gray-100 text-gray-700';
}

export function Pill({ tone, children }: { tone: string; children: React.ReactNode }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium capitalize',
        tone
      )}
    >
      {children}
    </span>
  );
}

export function LevelPill({ level }: { level: string | null | undefined }) {
  if (!level) return <span className="text-gray-400">{EM_DASH}</span>;
  return <Pill tone={levelTone(level)}>{level}</Pill>;
}

export function OutcomePill({ status }: { status: string | null | undefined }) {
  const value = (status || '').toLowerCase();
  if (!value) return <span className="text-gray-400">{EM_DASH}</span>;
  const tone =
    value === 'success'
      ? 'bg-emerald-50 text-emerald-700'
      : value === 'failure' || value === 'failed' || value === 'error'
        ? 'bg-red-50 text-red-700'
        : value === 'blocked'
          ? 'bg-amber-50 text-amber-700'
          : 'bg-gray-100 text-gray-700';
  return <Pill tone={tone}>{value}</Pill>;
}

// ---------------------------------------------------------------------------
// Loading / empty / error / gap
// ---------------------------------------------------------------------------

export function TableSkeleton({ rows = 6, columns = 5 }: { rows?: number; columns?: number }) {
  return (
    <div className="px-5 py-4" aria-busy="true" aria-live="polite">
      <span className="sr-only">Loading</span>
      <div className="space-y-3">
        {Array.from({ length: rows }).map((_, rowIndex) => (
          <div key={rowIndex} className="flex gap-4">
            {Array.from({ length: columns }).map((__, colIndex) => (
              <div
                key={colIndex}
                className="h-4 flex-1 animate-pulse rounded bg-gray-100"
                style={{ maxWidth: colIndex === columns - 1 ? '100%' : '12rem' }}
              />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

export function BlockSkeleton({ lines = 8 }: { lines?: number }) {
  return (
    <div className="space-y-2 px-5 py-4" aria-busy="true" aria-live="polite">
      <span className="sr-only">Loading</span>
      {Array.from({ length: lines }).map((_, index) => (
        <div
          key={index}
          className="h-3 animate-pulse rounded bg-gray-100"
          style={{ width: `${60 + ((index * 13) % 40)}%` }}
        />
      ))}
    </div>
  );
}

export function EmptyState({
  icon,
  title,
  message,
  action,
}: {
  icon?: React.ReactNode;
  title: string;
  message: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="px-5 py-12 text-center">
      <div className="mx-auto mb-3 flex h-10 w-10 items-center justify-center text-gray-300">
        {icon ?? <Search size={36} aria-hidden="true" />}
      </div>
      <h3 className="text-sm font-semibold text-gray-900">{title}</h3>
      <p className="mx-auto mt-1 max-w-xl text-sm text-gray-600">{message}</p>
      {action ? <div className="mt-4 flex justify-center">{action}</div> : null}
    </div>
  );
}

/**
 * A request that failed, printed rather than swallowed.
 *
 * The status code is shown next to the message because "404" and "403" send an
 * operator to two completely different places, and neither is "the page is
 * broken".
 */
export function ErrorBlock({
  title = 'Request failed',
  failure,
  onRetry,
}: {
  title?: string;
  failure: { message: string; status: number | null };
  onRetry?: () => void;
}) {
  return (
    <div className="m-5 rounded-md border border-red-200 bg-red-50 px-4 py-3" role="alert">
      <div className="flex items-start gap-3">
        <ServerCrash className="mt-0.5 shrink-0 text-red-600" size={18} aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-red-800">
            {title}
            {failure.status !== null ? ` (HTTP ${failure.status})` : ''}
          </p>
          <p className="mt-1 break-words text-sm text-red-700">{failure.message}</p>
        </div>
        {onRetry ? (
          <button type="button" onClick={onRetry} className={cn(BTN_SECONDARY, 'shrink-0')}>
            Retry
          </button>
        ) : null}
      </div>
    </div>
  );
}

/**
 * Something the backend cannot do yet, named precisely.
 *
 * This is deliberately not styled as an error: nothing has gone wrong, the
 * capability simply is not built. Listing the exact routes means the person who
 * builds them knows what this screen is waiting for.
 */
export function GapNotice({
  title,
  children,
  endpoints,
}: {
  title: string;
  children?: React.ReactNode;
  endpoints?: string[];
}) {
  return (
    <div className="m-5 rounded-md border border-amber-200 bg-amber-50 px-4 py-3">
      <div className="flex items-start gap-3">
        <AlertTriangle className="mt-0.5 shrink-0 text-amber-600" size={18} aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-amber-900">{title}</p>
          {children ? <div className="mt-1 text-sm text-amber-800">{children}</div> : null}
          {endpoints && endpoints.length > 0 ? (
            <ul className="mt-2 space-y-1">
              {endpoints.map((endpoint) => (
                <li key={endpoint} className="font-mono text-xs text-amber-900">
                  {endpoint}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Time range
// ---------------------------------------------------------------------------

export type RangeKey = '1h' | '6h' | '24h' | '7d' | '30d' | 'all' | 'custom';

export const RANGE_OPTIONS: Array<{ value: RangeKey; label: string }> = [
  { value: '1h', label: 'Last hour' },
  { value: '6h', label: 'Last 6 hours' },
  { value: '24h', label: 'Last 24 hours' },
  { value: '7d', label: 'Last 7 days' },
  { value: '30d', label: 'Last 30 days' },
  { value: 'all', label: 'All time' },
  { value: 'custom', label: 'Custom range' },
];

const RANGE_MS: Partial<Record<RangeKey, number>> = {
  '1h': 3600_000,
  '6h': 6 * 3600_000,
  '24h': 24 * 3600_000,
  '7d': 7 * 24 * 3600_000,
  '30d': 30 * 24 * 3600_000,
};

export interface TimeRange {
  key: RangeKey;
  /** Only read when key === 'custom'. Values come from datetime-local inputs. */
  from: string;
  to: string;
}

export const DEFAULT_RANGE: TimeRange = { key: '24h', from: '', to: '' };

/** Resolve a range into the RFC3339 pair the Go handlers parse, or nothing. */
export function rangeToParams(range: TimeRange): { start?: string; end?: string } {
  if (range.key === 'all') return {};
  if (range.key === 'custom') {
    const out: { start?: string; end?: string } = {};
    const from = range.from ? new Date(range.from) : null;
    const to = range.to ? new Date(range.to) : null;
    if (from && !Number.isNaN(from.getTime())) out.start = from.toISOString();
    if (to && !Number.isNaN(to.getTime())) out.end = to.toISOString();
    return out;
  }
  const span = RANGE_MS[range.key];
  if (!span) return {};
  return { start: new Date(Date.now() - span).toISOString() };
}

/** The same range as a pair of epoch bounds, for filtering rows in the browser. */
export function rangeToBounds(range: TimeRange): { start: number | null; end: number | null } {
  const params = rangeToParams(range);
  const start = params.start ? new Date(params.start).getTime() : null;
  const end = params.end ? new Date(params.end).getTime() : null;
  return {
    start: start !== null && !Number.isNaN(start) ? start : null,
    end: end !== null && !Number.isNaN(end) ? end : null,
  };
}

export const LEVEL_OPTIONS = [
  { value: '', label: 'All levels' },
  { value: 'debug', label: 'Debug' },
  { value: 'info', label: 'Info' },
  { value: 'notice', label: 'Notice' },
  { value: 'warning', label: 'Warning' },
  { value: 'error', label: 'Error' },
  { value: 'critical', label: 'Critical' },
];

// ---------------------------------------------------------------------------
// Toolbar
// ---------------------------------------------------------------------------

export interface ToolbarProps {
  range: TimeRange;
  onRangeChange: (range: TimeRange) => void;
  level: string;
  onLevelChange: (level: string) => void;
  levelOptions?: Array<{ value: string; label: string }>;
  levelLabel?: string;
  search: string;
  onSearchChange: (value: string) => void;
  searchPlaceholder?: string;
  /** Explains where the search runs - on the server, or over the loaded rows. */
  searchHint?: string;
  onApply: () => void;
  onExport?: () => void;
  exportLabel?: string;
  exportDisabled?: boolean;
  exportHint?: string;
  busy?: boolean;
  /** Section-specific filters, rendered before the shared ones. */
  children?: React.ReactNode;
}

export function LogToolbar({
  range,
  onRangeChange,
  level,
  onLevelChange,
  levelOptions = LEVEL_OPTIONS,
  levelLabel = 'Level',
  search,
  onSearchChange,
  searchPlaceholder = 'Search messages',
  searchHint,
  onApply,
  onExport,
  exportLabel = 'Export CSV',
  exportDisabled,
  exportHint,
  busy,
  children,
}: ToolbarProps) {
  return (
    <form
      className="border-b border-gray-200 px-5 py-4"
      onSubmit={(event) => {
        event.preventDefault();
        onApply();
      }}
    >
      <div className="flex flex-wrap items-end gap-3">
        {children}

        <div className="w-full sm:w-44">
          <label className={FIELD_LABEL} htmlFor="log-range">
            Time range
          </label>
          <select
            id="log-range"
            className={FIELD}
            value={range.key}
            onChange={(event) =>
              onRangeChange({ ...range, key: event.target.value as RangeKey })
            }
          >
            {RANGE_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </div>

        {range.key === 'custom' ? (
          <>
            <div className="w-full sm:w-52">
              <label className={FIELD_LABEL} htmlFor="log-range-from">
                From
              </label>
              <input
                id="log-range-from"
                type="datetime-local"
                className={FIELD}
                value={range.from}
                onChange={(event) => onRangeChange({ ...range, from: event.target.value })}
              />
            </div>
            <div className="w-full sm:w-52">
              <label className={FIELD_LABEL} htmlFor="log-range-to">
                To
              </label>
              <input
                id="log-range-to"
                type="datetime-local"
                className={FIELD}
                value={range.to}
                onChange={(event) => onRangeChange({ ...range, to: event.target.value })}
              />
            </div>
          </>
        ) : null}

        <div className="w-full sm:w-40">
          <label className={FIELD_LABEL} htmlFor="log-level">
            {levelLabel}
          </label>
          <select
            id="log-level"
            className={FIELD}
            value={level}
            onChange={(event) => onLevelChange(event.target.value)}
          >
            {levelOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </div>

        <div className="min-w-[14rem] flex-1">
          <label className={FIELD_LABEL} htmlFor="log-search">
            Search
          </label>
          <input
            id="log-search"
            type="search"
            className={FIELD}
            placeholder={searchPlaceholder}
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
          />
        </div>

        <div className="flex items-center gap-2">
          <button type="submit" className={BTN_PRIMARY} disabled={busy}>
            <Search size={16} aria-hidden="true" />
            Apply
          </button>
          {onExport ? (
            <button
              type="button"
              className={BTN_SECONDARY}
              onClick={onExport}
              disabled={exportDisabled}
              title={exportHint}
            >
              <Download size={16} aria-hidden="true" />
              {exportLabel}
            </button>
          ) : null}
        </div>
      </div>

      {searchHint || exportHint ? (
        <p className="mt-2 text-xs text-gray-500">
          {[searchHint, exportHint].filter(Boolean).join(' ')}
        </p>
      ) : null}
    </form>
  );
}

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------

export const PAGE_SIZES = [50, 100, 200, 500];

/**
 * Server-side paging, shown as pages.
 *
 * The offset/limit pair is what the Go handlers take, so the browser only ever
 * holds one page. That is the whole defence against a table with a million rows
 * in it: there is no code path here that asks for everything.
 */
export function Pagination({
  offset,
  limit,
  total,
  loaded,
  onOffsetChange,
  onLimitChange,
}: {
  offset: number;
  limit: number;
  /** Row count the server reported, or null when it does not report one. */
  total: number | null;
  /** Rows actually in hand, used to decide whether a next page can exist. */
  loaded: number;
  onOffsetChange: (offset: number) => void;
  onLimitChange: (limit: number) => void;
}) {
  const page = Math.floor(offset / Math.max(limit, 1)) + 1;
  const pages = total !== null && total > 0 ? Math.max(1, Math.ceil(total / Math.max(limit, 1))) : null;
  const hasPrev = offset > 0;
  const hasNext = pages !== null ? page < pages : loaded >= limit;

  const summary = useMemo(() => {
    if (loaded === 0) return 'No rows';
    const first = offset + 1;
    const last = offset + loaded;
    if (total !== null) return `${first}–${last} of ${total}`;
    return `${first}–${last}`;
  }, [loaded, offset, total]);

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-t border-gray-200 px-5 py-3">
      <p className="text-xs text-gray-500">{summary}</p>
      <div className="flex items-center gap-2">
        <label className="text-xs text-gray-500" htmlFor="log-page-size">
          Rows per page
        </label>
        <select
          id="log-page-size"
          className="h-8 rounded-md border border-gray-300 bg-white px-2 text-sm text-gray-900 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          value={limit}
          onChange={(event) => onLimitChange(Number(event.target.value))}
        >
          {PAGE_SIZES.map((size) => (
            <option key={size} value={size}>
              {size}
            </option>
          ))}
        </select>
        <button
          type="button"
          className={cn(BTN_SECONDARY, 'px-2 py-1.5')}
          disabled={!hasPrev}
          onClick={() => onOffsetChange(Math.max(0, offset - limit))}
          aria-label="Previous page"
        >
          <ChevronLeft size={16} aria-hidden="true" />
        </button>
        <span className="text-xs text-gray-600">
          Page {page}
          {pages !== null ? ` of ${pages}` : ''}
        </span>
        <button
          type="button"
          className={cn(BTN_SECONDARY, 'px-2 py-1.5')}
          disabled={!hasNext}
          onClick={() => onOffsetChange(offset + limit)}
          aria-label="Next page"
        >
          <ChevronRight size={16} aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Export
// ---------------------------------------------------------------------------

function csvCell(value: unknown): string {
  if (value === null || value === undefined) return '';
  const text = typeof value === 'string' ? value : JSON.stringify(value);
  // A leading =, +, - or @ is executed by spreadsheet software when the file is
  // opened. Log messages are attacker-influenced, so they are neutralised here.
  const guarded = /^[=+\-@\t\r]/.test(text) ? `'${text}` : text;
  return `"${guarded.replace(/"/g, '""')}"`;
}

export function toCsv(headers: string[], rows: Array<Array<unknown>>): string {
  const lines = [headers.map(csvCell).join(',')];
  rows.forEach((row) => lines.push(row.map(csvCell).join(',')));
  return `﻿${lines.join('\r\n')}\r\n`;
}

/** Hand the browser a file. Used only for data already in memory. */
export function downloadText(filename: string, contents: string, mime = 'text/csv;charset=utf-8') {
  const blob = new Blob([contents], { type: mime });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  // Revoked on the next tick so the click has taken the URL first.
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

export function stamp(): string {
  return new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
}

/**
 * The ceiling on any export.
 *
 * An export walks pages instead of asking for everything at once, and it stops
 * here. Pulling an unbounded result set into a browser tab to build a CSV is
 * the same defect as rendering it, only slower.
 */
export const EXPORT_MAX_ROWS = 10000;
export const EXPORT_PAGE_SIZE = 500;
