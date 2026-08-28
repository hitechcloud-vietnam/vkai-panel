'use client';

/**
 * The pieces every Security pane is built from.
 *
 * Four of them are not optional, because this screen has to survive a backend
 * that answers slowly, answers with a 403, answers with nothing, or does not
 * answer at all:
 *
 *   TableSkeleton  the shape of the data while it loads
 *   ErrorBlock     the message the API sent, never a blank pane
 *   EmptyState     nothing here yet, and what to do about it
 *   BackendGap     the endpoint does not exist - said plainly, with no form
 *
 * BackendGap is the one that matters most. A control rendered over a missing
 * endpoint is the defect this codebase has already shipped three times (a
 * two-factor page whose routes were never mounted, a settings page calling four
 * endpoints that all 404'd, an agent channel that was two TODO stubs). So where
 * the backend cannot do the thing, this screen renders the gap and no control
 * at all - never a form that pretends to work.
 */

import {
  AlertTriangle,
  CheckCircle2,
  CircleSlash,
  HelpCircle,
  Inbox,
  RefreshCw,
  ShieldOff,
  Wrench,
} from 'lucide-react';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

import type { PostureRisk, PostureState } from './types';

export const PRIMARY_BUTTON_CLASS =
  'bg-brand-600 text-white hover:bg-brand-700 focus-visible:ring-brand-500';
export const SECONDARY_BUTTON_CLASS =
  'border border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus-visible:ring-brand-500';
export const DANGER_BUTTON_CLASS =
  'bg-red-600 text-white hover:bg-red-700 focus-visible:ring-red-500';
export const INPUT_CLASS =
  'h-9 w-full rounded-md border border-gray-300 bg-white px-3 text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none focus:ring-1 focus:ring-brand-500';
export const TH_CLASS =
  'whitespace-nowrap px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
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

/** A section heading with a one-line explanation and room for actions. */
export function SectionHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: React.ReactNode;
  actions?: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3 border-b border-gray-200 px-4 py-3">
      <div className="min-w-0">
        <h3 className="text-sm font-semibold text-gray-900">{title}</h3>
        {description && <div className="mt-0.5 text-sm text-gray-500">{description}</div>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

/** The heading above a whole pane, with the pane's own explanation. */
export function PaneHeading({
  title,
  description,
  actions,
}: {
  title: string;
  description: string;
  actions?: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <h2 className="text-base font-semibold text-gray-900">{title}</h2>
        <p className="mt-1 max-w-3xl text-sm text-gray-600">{description}</p>
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

/** A horizontal scroller, so a wide table never makes the page scroll sideways. */
export function TableScroller({ children }: { children: React.ReactNode }) {
  return <div className="overflow-x-auto">{children}</div>;
}

/**
 * The shape of the table while it loads: grey bars in the geometry the real
 * rows will use, so the pane does not jump when the data lands.
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

/** The same idea for a block of cards rather than a table. */
export function BlockSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="space-y-3 px-4 py-4" aria-busy="true" aria-live="polite">
      <span className="sr-only">Loading</span>
      {Array.from({ length: rows }).map((_, index) => (
        <div key={index} className="space-y-2 rounded-md border border-gray-100 p-3">
          <div className="h-3 w-1/3 animate-pulse rounded bg-gray-100" />
          <div className="h-3 w-2/3 animate-pulse rounded bg-gray-100" />
        </div>
      ))}
    </div>
  );
}

/**
 * A failed request, named. The message the API sent is the only thing that
 * tells an operator whether to retry or to go and fix a server, so it is always
 * rendered and the pane around it stays on screen.
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

/**
 * A section the backend cannot serve yet.
 *
 * It names the controls that belong here, the endpoints that would have to
 * exist, and it renders no inputs. An operator who reads this knows the panel
 * is not managing the thing, which is the whole point: the alternative - a
 * switch that toggles and saves nothing - reads as protection that is not
 * there.
 */
export function BackendGap({
  title,
  summary,
  controls,
  endpoints,
  note,
}: {
  title: string;
  /** One sentence: what is missing, stated as a fact about this panel. */
  summary: string;
  /** The controls this section would carry once the backend exists. */
  controls: string[];
  /** The endpoints that have to exist first. */
  endpoints: string[];
  /** Anything an operator should do in the meantime. */
  note?: string;
}) {
  return (
    <div className="m-4 rounded-md border border-amber-200 bg-amber-50 p-4">
      <div className="flex items-start gap-3">
        <Wrench className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-amber-900">{title}</p>
          <p className="mt-1 text-sm text-amber-800">{summary}</p>

          <p className="mt-3 text-xs font-semibold uppercase tracking-wide text-amber-700">
            Controls this section will carry
          </p>
          <ul className="mt-1 list-disc space-y-0.5 pl-5 text-sm text-amber-800">
            {controls.map((control) => (
              <li key={control}>{control}</li>
            ))}
          </ul>

          <p className="mt-3 text-xs font-semibold uppercase tracking-wide text-amber-700">
            Endpoints it needs
          </p>
          <ul className="mt-1 space-y-0.5 pl-1 text-sm text-amber-800">
            {endpoints.map((endpoint) => (
              <li key={endpoint}>
                <code className="break-all rounded bg-amber-100 px-1.5 py-0.5 font-mono text-xs text-amber-900">
                  {endpoint}
                </code>
              </li>
            ))}
          </ul>

          {note && <p className="mt-3 text-sm text-amber-800">{note}</p>}
        </div>
      </div>
    </div>
  );
}

/**
 * A warning about something the panel CAN see but an operator would misread -
 * chiefly a setting stored in the database that nothing applies to the machine.
 */
export function CaveatBlock({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="m-4 rounded-md border border-amber-200 bg-amber-50 p-4">
      <div className="flex items-start gap-3">
        <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-amber-900">{title}</p>
          <div className="mt-1 space-y-1 text-sm text-amber-800">{children}</div>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// State presentation
// ---------------------------------------------------------------------------

const STATE_PRESENTATION: Record<
  PostureState,
  { label: string; badge: string; icon: typeof CheckCircle2; iconClass: string }
> = {
  ok: {
    label: 'On',
    badge: 'border-emerald-200 bg-emerald-50 text-emerald-700',
    icon: CheckCircle2,
    iconClass: 'text-emerald-600',
  },
  attention: {
    label: 'Needs attention',
    badge: 'border-amber-200 bg-amber-50 text-amber-700',
    icon: AlertTriangle,
    iconClass: 'text-amber-600',
  },
  off: {
    label: 'Off',
    badge: 'border-red-200 bg-red-50 text-red-700',
    icon: CircleSlash,
    iconClass: 'text-red-600',
  },
  unenforced: {
    label: 'Stored, not enforced',
    badge: 'border-red-200 bg-red-50 text-red-700',
    icon: ShieldOff,
    iconClass: 'text-red-600',
  },
  unknown: {
    label: 'Not verifiable',
    badge: 'border-gray-200 bg-gray-100 text-gray-700',
    icon: HelpCircle,
    iconClass: 'text-gray-500',
  },
};

export function stateLabel(state: PostureState): string {
  return STATE_PRESENTATION[state].label;
}

export function StateBadge({ state, className }: { state: PostureState; className?: string }) {
  const presentation = STATE_PRESENTATION[state];
  return (
    <span
      className={cn(
        'inline-flex items-center whitespace-nowrap rounded-md border px-2 py-0.5 text-xs font-medium',
        presentation.badge,
        className
      )}
    >
      {presentation.label}
    </span>
  );
}

export function StateIcon({ state, className }: { state: PostureState; className?: string }) {
  const presentation = STATE_PRESENTATION[state];
  const Icon = presentation.icon;
  return <Icon className={cn('h-4 w-4', presentation.iconClass, className)} aria-hidden="true" />;
}

const RISK_PRESENTATION: Record<PostureRisk, string> = {
  critical: 'border-red-200 bg-red-50 text-red-700',
  high: 'border-amber-200 bg-amber-50 text-amber-700',
  medium: 'border-sky-200 bg-sky-50 text-sky-700',
  low: 'border-gray-200 bg-gray-100 text-gray-600',
};

export function RiskBadge({ risk }: { risk: PostureRisk }) {
  return (
    <span
      className={cn(
        'inline-flex items-center whitespace-nowrap rounded-md border px-2 py-0.5 text-xs font-medium capitalize',
        RISK_PRESENTATION[risk]
      )}
    >
      {risk} risk
    </span>
  );
}

/** A single figure with its label. Renders an em dash when the value is unknown. */
export function StatTile({
  label,
  value,
  hint,
}: {
  label: string;
  value: number | string | null;
  hint?: string;
}) {
  const missing = value === null || value === '';
  return (
    <div className="rounded-md border border-gray-200 bg-white px-4 py-3">
      <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{label}</p>
      <p
        className={cn('mt-1 text-xl font-semibold', missing ? 'text-gray-400' : 'text-gray-900')}
        title={missing ? hint : undefined}
      >
        {missing ? '—' : value}
      </p>
      {hint && !missing && <p className="mt-0.5 text-xs text-gray-500">{hint}</p>}
    </div>
  );
}

/** A label/value row, for read-only configuration. */
export function FactRow({
  label,
  value,
  hint,
}: {
  label: string;
  value: React.ReactNode;
  hint?: string;
}) {
  return (
    <div className="flex flex-wrap items-baseline justify-between gap-2 border-b border-gray-100 px-4 py-2.5 last:border-b-0">
      <div className="min-w-0">
        <p className="text-sm text-gray-600">{label}</p>
        {hint && <p className="mt-0.5 text-xs text-gray-500">{hint}</p>}
      </div>
      <div className="text-sm font-medium text-gray-900">{value}</div>
    </div>
  );
}
