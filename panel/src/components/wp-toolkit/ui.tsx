'use client';

/**
 * The small set of visual pieces the WP Toolkit screens share.
 *
 * Kept local to this section on purpose: the six screens are being written
 * against a design system that other people are editing at the same time, and
 * a shared primitive that changes underneath them is a merge conflict nobody
 * asked for. Everything here is plain Tailwind on the agreed palette - white
 * surfaces, gray-200 borders, brand-600 for the one action that matters,
 * red-600 for the one that destroys something.
 */

import type { ReactNode } from 'react';
import { AlertTriangle, Check, Copy, Info, Loader2 } from 'lucide-react';
import { useState } from 'react';
import { cn } from '@/lib/utils';

// ---------------------------------------------------------------------------
// Surfaces
// ---------------------------------------------------------------------------

export function Card({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <section className={cn('rounded-lg border border-gray-200 bg-white shadow-sm', className)}>
      {children}
    </section>
  );
}

export function CardHeader({
  title,
  description,
  actions,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-3 border-b border-gray-200 px-5 py-4">
      <div className="min-w-0">
        <h2 className="text-sm font-semibold text-gray-900">{title}</h2>
        {description ? <p className="mt-1 text-sm text-gray-600">{description}</p> : null}
      </div>
      {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
    </header>
  );
}

export function CardBody({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={cn('px-5 py-4', className)}>{children}</div>;
}

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <h1 className="text-xl font-semibold text-gray-900">{title}</h1>
        {description ? <p className="mt-1 max-w-3xl text-sm text-gray-600">{description}</p> : null}
      </div>
      {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Controls
// ---------------------------------------------------------------------------

type ButtonTone = 'primary' | 'secondary' | 'danger';

const BUTTON_TONES: Record<ButtonTone, string> = {
  primary: 'bg-brand-600 text-white hover:bg-brand-700 border border-transparent',
  secondary: 'bg-white text-gray-700 hover:bg-gray-50 border border-gray-300',
  danger: 'bg-red-600 text-white hover:bg-red-700 border border-transparent',
};

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  tone?: ButtonTone;
  busy?: boolean;
  /**
   * Why this button cannot be used. Setting it disables the button AND puts the
   * sentence on it as a title and an accessible description, so a control that
   * is switched off always says who switched it off.
   */
  unavailableReason?: string | null;
}

export function Button({
  tone = 'secondary',
  busy = false,
  unavailableReason,
  className,
  disabled,
  children,
  ...rest
}: ButtonProps) {
  const off = Boolean(disabled) || busy || Boolean(unavailableReason);
  return (
    <button
      type="button"
      {...rest}
      disabled={off}
      title={unavailableReason ?? rest.title}
      aria-disabled={off}
      className={cn(
        'inline-flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium',
        'focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1',
        BUTTON_TONES[tone],
        off && 'cursor-not-allowed opacity-50 hover:bg-inherit',
        className,
      )}
    >
      {busy ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden /> : null}
      {children}
    </button>
  );
}

export function Field({
  label,
  hint,
  required,
  htmlFor,
  children,
}: {
  label: string;
  hint?: ReactNode;
  required?: boolean;
  htmlFor?: string;
  children: ReactNode;
}) {
  return (
    <div>
      <label htmlFor={htmlFor} className="mb-1.5 block text-sm font-medium text-gray-700">
        {label}
        {required ? <span className="ml-0.5 text-red-600">*</span> : null}
      </label>
      {children}
      {hint ? <p className="mt-1.5 text-xs text-gray-500">{hint}</p> : null}
    </div>
  );
}

export const inputClass =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 ' +
  'placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500 ' +
  'disabled:cursor-not-allowed disabled:bg-gray-50 disabled:text-gray-500';

export function TextInput(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={cn(inputClass, props.className)} />;
}

export function SelectInput(props: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className={cn(inputClass, 'pr-8', props.className)} />;
}

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

export type Tone = 'emerald' | 'amber' | 'red' | 'sky' | 'gray';

const BADGE_TONES: Record<Tone, string> = {
  emerald: 'bg-emerald-50 text-emerald-700 ring-emerald-200',
  amber: 'bg-amber-50 text-amber-700 ring-amber-200',
  red: 'bg-red-50 text-red-700 ring-red-200',
  sky: 'bg-sky-50 text-sky-700 ring-sky-200',
  gray: 'bg-gray-100 text-gray-600 ring-gray-200',
};

export function Badge({ tone = 'gray', children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ring-1 ring-inset',
        BADGE_TONES[tone],
      )}
    >
      {children}
    </span>
  );
}

/**
 * A figure the panel could not read.
 *
 * Never print 0 for something unknown: an installation with no outdated plugins
 * and one whose plugin list could not be read both come out as "0", and an
 * operator who cannot tell them apart has been actively misled.
 */
export function Unknown({ reason }: { reason: string }) {
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

export function Notice({
  tone = 'sky',
  title,
  children,
}: {
  tone?: Tone;
  title?: ReactNode;
  children: ReactNode;
}) {
  const frame: Record<Tone, string> = {
    emerald: 'border-emerald-200 bg-emerald-50 text-emerald-900',
    amber: 'border-amber-200 bg-amber-50 text-amber-900',
    red: 'border-red-200 bg-red-50 text-red-900',
    sky: 'border-sky-200 bg-sky-50 text-sky-900',
    gray: 'border-gray-200 bg-gray-50 text-gray-800',
  };
  const Icon = tone === 'red' || tone === 'amber' ? AlertTriangle : Info;
  return (
    <div className={cn('flex gap-3 rounded-md border px-4 py-3 text-sm', frame[tone])}>
      <Icon className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
      <div className="min-w-0 space-y-1">
        {title ? <p className="font-medium">{title}</p> : null}
        <div className="[&_code]:rounded [&_code]:bg-white/60 [&_code]:px-1 [&_code]:py-0.5 [&_code]:text-[12px]">
          {children}
        </div>
      </div>
    </div>
  );
}

export function ErrorNote({ children }: { children: ReactNode }) {
  if (!children) return null;
  return (
    <Notice tone="red" title="That request failed">
      {children}
    </Notice>
  );
}

export function Loading({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2 px-5 py-8 text-sm text-gray-500">
      <Loader2 className="h-4 w-4 animate-spin" aria-hidden />
      {label}
    </div>
  );
}

export function EmptyState({
  title,
  children,
  action,
}: {
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="px-5 py-10 text-center">
      <p className="text-sm font-medium text-gray-900">{title}</p>
      {children ? <div className="mx-auto mt-2 max-w-xl text-sm text-gray-600">{children}</div> : null}
      {action ? <div className="mt-4 flex justify-center">{action}</div> : null}
    </div>
  );
}

// ---------------------------------------------------------------------------
// The honest gap
// ---------------------------------------------------------------------------

export interface MissingCapability {
  /** The HTTP route that would have to exist, e.g. "POST /api/v1/wordpress/migrations". */
  endpoint: string;
  /** What it would have to do, in one line. */
  purpose: string;
}

/**
 * What a screen renders instead of a form when the API behind it cannot do the
 * job yet.
 *
 * The point is the endpoint list. "Coming soon" tells an operator to wait and
 * tells a backend engineer nothing; a named route with a stated job is a ticket
 * somebody can pick up, and it is checkable - the day the route exists, this
 * panel is wrong and somebody will notice.
 */
export function NotBuiltYet({
  what,
  because,
  endpoints,
  footnote,
}: {
  what: string;
  because: ReactNode;
  endpoints: MissingCapability[];
  footnote?: ReactNode;
}) {
  return (
    <Card>
      <CardHeader
        title={
          <span className="flex items-center gap-2">
            <AlertTriangle className="h-4 w-4 text-amber-600" aria-hidden />
            {what} is not available on this panel yet
          </span>
        }
        description={because}
      />
      <CardBody className="space-y-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">
            What the API would have to provide
          </p>
          <ul className="mt-2 divide-y divide-gray-100 rounded-md border border-gray-200">
            {endpoints.map((item) => (
              <li key={item.endpoint} className="px-4 py-3">
                <code className="block break-all font-mono text-xs text-gray-900">{item.endpoint}</code>
                <p className="mt-1 text-sm text-gray-600">{item.purpose}</p>
              </li>
            ))}
          </ul>
        </div>
        {footnote ? <div className="text-sm text-gray-600">{footnote}</div> : null}
        <p className="text-sm text-gray-500">
          No form is shown here on purpose. A screen that collects credentials and then cannot use
          them is worse than an empty one, because the operator only finds out after handing over a
          root password.
        </p>
      </CardBody>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Secrets
// ---------------------------------------------------------------------------

export function CopyButton({ value, label = 'Copy' }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      tone="secondary"
      onClick={() => {
        try {
          void navigator.clipboard?.writeText(value);
          setCopied(true);
          window.setTimeout(() => setCopied(false), 2000);
        } catch {
          /* a browser that refuses the clipboard still shows the value on screen */
        }
      }}
    >
      {copied ? <Check className="h-4 w-4" aria-hidden /> : <Copy className="h-4 w-4" aria-hidden />}
      {copied ? 'Copied' : label}
    </Button>
  );
}

/**
 * A value shown once and then gone.
 *
 * The caller owns the "once": it renders this, and when the operator dismisses
 * it the caller drops the value from state so no re-render can bring it back.
 * There is no prop here that would let a parent redisplay it later.
 */
export function ShownOnce({
  title,
  description,
  entries,
  onDismiss,
}: {
  title: string;
  description: ReactNode;
  entries: { label: string; value: string; secret?: boolean }[];
  onDismiss: () => void;
}) {
  return (
    <Card className="border-amber-200">
      <CardHeader title={title} description={description} />
      <CardBody className="space-y-3">
        <dl className="divide-y divide-gray-100 rounded-md border border-gray-200">
          {entries.map((entry) => (
            <div key={entry.label} className="flex flex-wrap items-center gap-3 px-4 py-3">
              <dt className="w-40 shrink-0 text-sm text-gray-600">{entry.label}</dt>
              <dd className="min-w-0 flex-1 break-all font-mono text-sm text-gray-900">
                {entry.value}
              </dd>
              <CopyButton value={entry.value} />
            </div>
          ))}
        </dl>
        <div className="flex justify-end">
          <Button tone="primary" onClick={onDismiss}>
            I have saved these
          </Button>
        </div>
      </CardBody>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Tables
// ---------------------------------------------------------------------------

export function Table({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="min-w-full divide-y divide-gray-200 text-sm">{children}</table>
    </div>
  );
}

export function Th({ children, className }: { children?: ReactNode; className?: string }) {
  return (
    <th
      scope="col"
      className={cn(
        'bg-gray-50 px-4 py-2.5 text-left text-xs font-semibold uppercase tracking-wide text-gray-500',
        className,
      )}
    >
      {children}
    </th>
  );
}

export function Td({ children, className }: { children?: ReactNode; className?: string }) {
  return <td className={cn('px-4 py-3 align-middle text-gray-700', className)}>{children}</td>;
}

/** Generates a password in the browser, from the platform CSPRNG. */
export function generatePassword(length = 20): string {
  const alphabet = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#%^*_-+=';
  const out: string[] = [];
  const bytes = new Uint32Array(length);
  if (typeof window !== 'undefined' && window.crypto?.getRandomValues) {
    window.crypto.getRandomValues(bytes);
  } else {
    // Server-side render of an empty form; the value is replaced on mount.
    return '';
  }
  for (let i = 0; i < length; i += 1) {
    out.push(alphabet[bytes[i] % alphabet.length]);
  }
  return out.join('');
}
