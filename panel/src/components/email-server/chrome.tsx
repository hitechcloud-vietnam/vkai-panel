'use client';

/**
 * The furniture every pane in the Email Server section is built from.
 *
 * Four states are mandatory on every section here: loading, failed, empty, and
 * populated. `ErrorBlock` always renders a string produced by lib/apiError, so
 * an API failure names itself instead of blanking the page - this codebase has
 * already paid once for a page that threw React #31 on an error object.
 *
 * `BackendGap` is the fifth state and the one specific to this section: a piece
 * of the screen whose endpoint does not exist yet. It says which route is
 * missing rather than showing a form that would post into nothing.
 */

import * as React from 'react';
import {
  AlertTriangle,
  Check,
  Copy,
  Inbox,
  Plug,
  RefreshCw,
  Search,
  X,
} from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';

export const TH = 'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
export const TD = 'px-4 py-3 text-sm text-gray-600 align-top';
export const ROW = 'border-b border-gray-100 last:border-b-0 hover:bg-gray-50';

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
        {description && <p className="mt-0.5 max-w-3xl text-sm text-gray-500">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

export function Toolbar({
  search,
  onSearchChange,
  searchPlaceholder,
  onRefresh,
  refreshing,
  children,
}: {
  search?: string;
  onSearchChange?: (value: string) => void;
  searchPlaceholder?: string;
  onRefresh: () => void;
  refreshing?: boolean;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 border-b border-gray-200 px-4 py-3">
      {onSearchChange && (
        <div className="relative min-w-[200px] flex-1">
          <Search
            className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
            aria-hidden="true"
          />
          <Input
            value={search ?? ''}
            onChange={(e) => onSearchChange(e.target.value)}
            placeholder={searchPlaceholder ?? 'Search'}
            aria-label={searchPlaceholder ?? 'Search'}
            className="pl-9"
          />
        </div>
      )}
      {!onSearchChange && <div className="flex-1" />}
      <Button type="button" variant="secondary" onClick={onRefresh} disabled={refreshing}>
        <RefreshCw className={cn('h-4 w-4', refreshing && 'animate-spin')} aria-hidden="true" />
        Refresh
      </Button>
      {children}
    </div>
  );
}

/** Grey bars in the geometry the real table will use, so nothing jumps. */
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

/** Blocks of grey where a form or a set of tiles will land. */
export function BlockSkeleton({ lines = 3 }: { lines?: number }) {
  return (
    <div className="space-y-3 px-4 py-4" aria-busy="true" aria-live="polite">
      <span className="sr-only">Loading</span>
      {Array.from({ length: lines }).map((_, i) => (
        <div key={i} className="h-8 animate-pulse rounded bg-gray-100" />
      ))}
    </div>
  );
}

/** A failed request, named, with the retry one click away. */
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
              variant="danger-outline"
              onClick={onRetry}
              className="mt-3"
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
      <p className="mt-1 max-w-lg text-sm text-gray-500">{description}</p>
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

/**
 * A section the panel cannot drive yet.
 *
 * The alternative - a form wired to nothing - is the defect this project has
 * shipped twice already (a settings page calling four endpoints that all 404'd,
 * a two-factor screen whose routes were never mounted). A control an operator
 * trusts and that does nothing is worse than an absent one, so the endpoint
 * that is missing is printed here by name.
 */
export function BackendGap({
  title,
  description,
  missing,
}: {
  title: string;
  description: string;
  /** The routes that would have to exist, written the way router.go writes them. */
  missing: string[];
}) {
  return (
    <div className="flex flex-col items-center justify-center px-4 py-12 text-center">
      <Plug className="h-8 w-8 text-gray-300" aria-hidden="true" />
      <p className="mt-3 text-sm font-semibold text-gray-900">{title}</p>
      <p className="mt-1 max-w-xl text-sm text-gray-500">{description}</p>
      {missing.length > 0 && (
        <div className="mt-4 w-full max-w-xl rounded-md border border-gray-200 bg-gray-50 px-4 py-3 text-left">
          <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">
            Endpoints this needs
          </p>
          <ul className="mt-2 space-y-1">
            {missing.map((route) => (
              <li key={route} className="font-mono text-xs text-gray-600">
                {route}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

/** A short warning that stays visible above working controls. */
export function Notice({
  tone = 'amber',
  children,
}: {
  tone?: 'amber' | 'sky' | 'red';
  children: React.ReactNode;
}) {
  const tones = {
    amber: 'border-amber-200 bg-amber-50 text-amber-800',
    sky: 'border-sky-200 bg-sky-50 text-sky-800',
    red: 'border-red-200 bg-red-50 text-red-800',
  } as const;
  return (
    <div className={cn('flex items-start gap-2 border-b px-4 py-3 text-sm', tones[tone])}>
      <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
      <div className="min-w-0">{children}</div>
    </div>
  );
}

export function StatTile({
  label,
  value,
  hint,
}: {
  label: string;
  value: React.ReactNode;
  hint?: string;
}) {
  return (
    <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
      <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{label}</p>
      <p className="mt-1 text-2xl font-semibold text-gray-900">{value}</p>
      {hint && <p className="mt-1 text-xs text-gray-500">{hint}</p>}
    </div>
  );
}

export type PillTone = 'emerald' | 'amber' | 'red' | 'sky' | 'gray';

const PILL_TONES: Record<PillTone, string> = {
  emerald: 'border-emerald-200 bg-emerald-50 text-emerald-700',
  amber: 'border-amber-200 bg-amber-50 text-amber-700',
  red: 'border-red-200 bg-red-50 text-red-700',
  sky: 'border-sky-200 bg-sky-50 text-sky-700',
  gray: 'border-gray-200 bg-gray-100 text-gray-700',
};

export function Pill({
  tone,
  children,
  className,
}: {
  tone: PillTone;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 whitespace-nowrap rounded-md border px-2 py-0.5 text-xs font-medium',
        PILL_TONES[tone],
        className
      )}
    >
      {children}
    </span>
  );
}

/** A value the backend has not reported. Never printed as 0. */
export function Dash({ reason }: { reason: string }) {
  return (
    <span
      title={reason}
      aria-label={reason}
      className="cursor-help text-gray-400 underline decoration-dotted underline-offset-2"
    >
      &mdash;
    </span>
  );
}

/** A DNS record value with a copy button, because these are retyped by hand otherwise. */
export function CopyableRecord({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = React.useState(false);

  const copy = React.useCallback(() => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) return;
    navigator.clipboard.writeText(value).then(
      () => {
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1500);
      },
      () => setCopied(false)
    );
  }, [value]);

  return (
    <div className="flex items-start gap-2">
      <code className="min-w-0 flex-1 break-all rounded-md border border-gray-200 bg-gray-50 px-2 py-1 font-mono text-xs text-gray-700">
        {value}
      </code>
      <Button
        type="button"
        variant="secondary"
        size="sm"
        onClick={copy}
        aria-label={`Copy ${label}`}
      >
        {copied ? (
          <Check className="h-3.5 w-3.5 text-emerald-600" aria-hidden="true" />
        ) : (
          <Copy className="h-3.5 w-3.5" aria-hidden="true" />
        )}
        {copied ? 'Copied' : 'Copy'}
      </Button>
    </div>
  );
}

/** A plain modal. Escape closes it; the backdrop click does too. */
export function Modal({
  open,
  title,
  description,
  onClose,
  children,
  footer,
  wide,
}: {
  open: boolean;
  title: string;
  description?: string;
  onClose: () => void;
  children: React.ReactNode;
  footer?: React.ReactNode;
  wide?: boolean;
}) {
  React.useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-gray-900/40 p-4 sm:items-center"
      role="presentation"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={cn(
          'w-full rounded-lg border border-gray-200 bg-white shadow-sm',
          wide ? 'max-w-2xl' : 'max-w-md'
        )}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-3 border-b border-gray-200 px-4 py-3">
          <div className="min-w-0">
            <h2 className="text-sm font-semibold text-gray-900">{title}</h2>
            {description && <p className="mt-0.5 text-sm text-gray-500">{description}</p>}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="rounded-md p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
        <div className="space-y-4 px-4 py-4">{children}</div>
        {footer && (
          <div className="flex flex-wrap items-center justify-end gap-2 border-t border-gray-200 px-4 py-3">
            {footer}
          </div>
        )}
      </div>
    </div>
  );
}

export function Field({
  label,
  hint,
  htmlFor,
  children,
}: {
  label: string;
  hint?: string;
  htmlFor?: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label htmlFor={htmlFor} className="block text-sm font-medium text-gray-700">
        {label}
      </label>
      <div className="mt-1">{children}</div>
      {hint && <p className="mt-1 text-xs text-gray-500">{hint}</p>}
    </div>
  );
}

/** A horizontal scroller so a wide table never makes the page scroll sideways. */
export function TableScroller({ children }: { children: React.ReactNode }) {
  return <div className="overflow-x-auto">{children}</div>;
}

/** Inline failure text for an action, so a failed POST is not silent. */
export function ActionError({ message, onDismiss }: { message: string; onDismiss?: () => void }) {
  if (!message) return null;
  return (
    <div
      className="flex items-start gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
      role="alert"
    >
      <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
      <span className="min-w-0 flex-1 break-words">{message}</span>
      {onDismiss && (
        <button
          type="button"
          onClick={onDismiss}
          aria-label="Dismiss"
          className="rounded p-0.5 text-red-500 hover:bg-red-100"
        >
          <X className="h-3.5 w-3.5" aria-hidden="true" />
        </button>
      )}
    </div>
  );
}
