'use client';

/**
 * The four states every pane on this screen needs - loading, failed, empty,
 * populated - plus the surfaces and the button classes they share.
 *
 * The failure state is the one that earns its place. A page that blanks
 * instead of naming what went wrong has already cost this codebase an
 * incident, so ErrorBlock always renders a string (see lib/apiError, which
 * never hands back an object) and always offers the retry.
 */

import { AlertTriangle, Inbox, RefreshCw } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

export const PRIMARY_BUTTON_CLASS =
  'bg-brand-600 text-white hover:bg-brand-700 focus-visible:ring-brand-500';
export const SECONDARY_BUTTON_CLASS =
  'border border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus-visible:ring-brand-500';
export const DANGER_BUTTON_CLASS =
  'bg-red-600 text-white hover:bg-red-700 focus-visible:ring-red-500';
export const TH_CLASS =
  'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
export const TD_CLASS = 'px-4 py-3 text-sm text-gray-600';

/** A white surface with a hairline border. Every section sits in one. */
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

/** A section heading with a one-line explanation and room for a toolbar. */
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

/** The refresh control every pane carries, with room for pane-specific actions. */
export function PaneToolbar({
  onRefresh,
  refreshing,
  children,
}: {
  onRefresh: () => void;
  refreshing?: boolean;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {children}
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onRefresh}
        disabled={refreshing}
        className={SECONDARY_BUTTON_CLASS}
      >
        <RefreshCw className={cn('h-4 w-4', refreshing && 'animate-spin')} aria-hidden="true" />
        Refresh
      </Button>
    </div>
  );
}

/**
 * The shape of the table while it loads: grey bars in the row geometry the real
 * table will use, so nothing jumps when the data lands.
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
              style={{ maxWidth: colIndex === 0 ? '20%' : undefined }}
            />
          ))}
        </div>
      ))}
    </div>
  );
}

/** The same idea for a card of key/value rows rather than a table. */
export function CardSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="space-y-3 px-4 py-4" aria-busy="true" aria-live="polite">
      <span className="sr-only">Loading</span>
      {Array.from({ length: rows }).map((_, index) => (
        <div key={index} className="flex items-center gap-4">
          <div className="h-3 w-28 animate-pulse rounded bg-gray-100" />
          <div className="h-3 flex-1 animate-pulse rounded bg-gray-100" />
        </div>
      ))}
    </div>
  );
}

/**
 * A failed request, named. The message the API sent is the only thing that
 * tells an operator whether to retry or to go and fix a server.
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
              size="sm"
              onClick={onRetry}
              className="mt-3 border-red-300 bg-white text-red-700 hover:bg-red-100 focus-visible:ring-red-500"
            >
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
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

/** A horizontal scroller so a wide table never makes the page scroll sideways. */
export function TableScroller({ children }: { children: React.ReactNode }) {
  return <div className="overflow-x-auto">{children}</div>;
}
