'use client';

/**
 * The four states every Docker pane needs - loading, failed, empty, populated -
 * plus the surfaces they sit on.
 *
 * The failure state exists because this panel has already shipped pages that
 * blanked instead of naming what went wrong: an error object went into React as
 * a child and took the whole route down. `ErrorBlock` only ever renders a
 * string (see lib/apiError) and always offers the retry, so a failed request
 * costs an operator one click rather than a page reload.
 */

import { AlertTriangle, Inbox, RefreshCw, Search } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';

export const TH_CLASS =
  'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
export const TD_CLASS = 'px-4 py-3 text-sm text-gray-600 align-top';
export const SECONDARY_BUTTON_CLASS =
  'border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus-visible:ring-brand-500';

/** A white surface with a hairline border. Every pane section sits in one. */
export function Panel({
  className,
  children,
}: {
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <section className={cn('rounded-lg border border-gray-200 bg-white shadow-sm', className)}>
      {children}
    </section>
  );
}

/** A section heading with a one-line explanation and room for actions. */
export function SectionHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3 border-b border-gray-200 px-4 py-3">
      <div className="min-w-0">
        <h3 className="text-sm font-semibold text-gray-900">{title}</h3>
        {description && <p className="mt-0.5 text-sm text-gray-500">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

/** Search box plus refresh, the two controls every list pane carries. */
export function PaneToolbar({
  search,
  onSearchChange,
  searchPlaceholder,
  onRefresh,
  refreshing,
  children,
}: {
  search: string;
  onSearchChange: (value: string) => void;
  searchPlaceholder: string;
  onRefresh: () => void;
  refreshing?: boolean;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-[200px] flex-1">
        <Search
          className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
          aria-hidden="true"
        />
        <Input
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder={searchPlaceholder}
          aria-label={searchPlaceholder}
          className="pl-9"
        />
      </div>
      <Button
        type="button"
        variant="outline"
        onClick={onRefresh}
        disabled={refreshing}
        className={SECONDARY_BUTTON_CLASS}
      >
        <RefreshCw className={cn('mr-2 h-4 w-4', refreshing && 'animate-spin')} aria-hidden="true" />
        Refresh
      </Button>
      {children}
    </div>
  );
}

/**
 * The shape of the table while it loads: grey bars in the row geometry the real
 * table will use, so the pane does not jump when the data lands.
 */
export function TableSkeleton({ columns, rows = 4 }: { columns: number; rows?: number }) {
  return (
    <div className="px-4 py-3" aria-busy="true" aria-live="polite">
      <span className="sr-only">Loading</span>
      {Array.from({ length: rows }).map((_, rowIndex) => (
        <div
          key={rowIndex}
          className="flex items-center gap-4 border-b border-gray-100 py-3 last:border-b-0"
        >
          {Array.from({ length: columns }).map((__, colIndex) => (
            <div
              key={colIndex}
              className="h-3 flex-1 animate-pulse rounded bg-gray-100"
              style={{ maxWidth: colIndex === 0 ? '18%' : undefined }}
            />
          ))}
        </div>
      ))}
    </div>
  );
}

/** The card version of the skeleton, for Overview's tile row. */
export function TileSkeleton({ tiles = 4 }: { tiles?: number }) {
  return (
    <div
      className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4"
      aria-busy="true"
      aria-live="polite"
    >
      <span className="sr-only">Loading</span>
      {Array.from({ length: tiles }).map((_, index) => (
        <div key={index} className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
          <div className="h-3 w-24 animate-pulse rounded bg-gray-100" />
          <div className="mt-3 h-6 w-16 animate-pulse rounded bg-gray-100" />
        </div>
      ))}
    </div>
  );
}

/**
 * A failed request, named. Never a blank pane: the message the API sent is the
 * only thing that tells an operator whether to retry or go and fix a daemon.
 */
export function ErrorBlock({
  title,
  message,
  onRetry,
}: {
  title: string;
  message: string;
  onRetry?: () => void;
}) {
  return (
    <div className="m-4 rounded-md border border-red-200 bg-red-50 p-4" role="alert">
      <div className="flex items-start gap-3">
        <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-red-600" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-red-800">{title}</p>
          <p className="mt-1 break-words text-sm text-red-700">{message}</p>
          {onRetry && (
            <Button
              type="button"
              variant="outline"
              onClick={onRetry}
              className="mt-3 border-red-300 bg-white text-red-700 hover:bg-red-100 focus-visible:ring-red-500"
            >
              <RefreshCw className="mr-2 h-4 w-4" aria-hidden="true" />
              Try again
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

/** Nothing here yet, and what to do about it. */
export function EmptyState({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center px-4 py-12 text-center">
      <Inbox className="h-8 w-8 text-gray-300" aria-hidden="true" />
      <p className="mt-3 text-sm font-semibold text-gray-900">{title}</p>
      <p className="mt-1 max-w-md text-sm text-gray-500">{description}</p>
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

/** A horizontal scroller, so a wide table never makes the page scroll sideways. */
export function TableScroller({ children }: { children: React.ReactNode }) {
  return <div className="overflow-x-auto">{children}</div>;
}
