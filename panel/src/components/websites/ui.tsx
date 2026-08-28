'use client';

/**
 * The pieces every project-type panel shares: the states a list can be in, the
 * badges, the dialog shell and the input styles.
 *
 * They live in one file so that the five tabs look like one screen rather than
 * five, and so an empty state that has to be honest is honest in the same shape
 * everywhere.
 */

import { AlertTriangle, Info, Loader2, X } from 'lucide-react';
import type { ReactNode } from 'react';

import { Unavailable } from '@/components/Unavailable';

export const INPUT_CLASS =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500 disabled:bg-gray-50 disabled:text-gray-500';
export const LABEL_CLASS = 'mb-1.5 block text-sm font-medium text-gray-700';
export const HINT_CLASS = 'mt-1 text-xs text-gray-500';
export const BTN_PRIMARY =
  'inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60';
export const BTN_SECONDARY =
  'inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 disabled:cursor-not-allowed disabled:opacity-60';
export const BTN_DANGER =
  'inline-flex items-center gap-2 rounded-md bg-red-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-red-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60';
export const BTN_ROW =
  'inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-2.5 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 disabled:cursor-not-allowed disabled:border-gray-200 disabled:bg-gray-50 disabled:text-gray-400';

/** The surface every tab's content sits on. */
export function Surface({ children, className = '' }: { children: ReactNode; className?: string }) {
  return (
    <div className={`rounded-lg border border-gray-200 bg-white shadow-sm ${className}`}>
      {children}
    </div>
  );
}

export function SurfaceHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3 border-b border-gray-200 px-5 py-4">
      <div>
        <h2 className="text-sm font-semibold text-gray-900">{title}</h2>
        {description && <p className="mt-1 text-sm text-gray-600">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

/** The list is still loading. */
export function PanelLoading({ label }: { label: string }) {
  return (
    <div className="flex items-center justify-center gap-3 px-6 py-14 text-sm text-gray-600">
      <Loader2 size={18} className="animate-spin text-brand-600" aria-hidden="true" />
      <span>{label}</span>
    </div>
  );
}

/** The request failed. The message is the API's, never a swallowed object. */
export function PanelError({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div role="alert" className="px-6 py-10 text-center">
      <AlertTriangle className="mx-auto text-red-500" size={32} aria-hidden="true" />
      <h3 className="mt-3 text-sm font-semibold text-gray-900">Could not load this list</h3>
      <p className="mx-auto mt-2 max-w-xl text-sm text-gray-600">{message}</p>
      {onRetry && (
        <button type="button" onClick={onRetry} className={`mt-4 ${BTN_SECONDARY}`}>
          Try again
        </button>
      )}
    </div>
  );
}

/** Nothing of this type exists yet, and the backend could hold one if it did. */
export function PanelEmpty({
  icon,
  title,
  body,
  action,
}: {
  icon: ReactNode;
  title: string;
  body: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="px-6 py-14 text-center">
      <div className="mx-auto flex h-10 w-10 items-center justify-center text-gray-300">{icon}</div>
      <h3 className="mt-3 text-sm font-semibold text-gray-900">{title}</h3>
      <div className="mx-auto mt-2 max-w-xl text-sm text-gray-600">{body}</div>
      {action && <div className="mt-4 flex flex-wrap justify-center gap-3">{action}</div>}
    </div>
  );
}

/**
 * The empty state for a type the backend cannot manage at all.
 *
 * It says the capability is not available, names exactly what is missing, and
 * offers no control that would pretend otherwise. This is the shape the brief
 * asks for and the reason there is no "Add Go project" button.
 */
export function CapabilityGap({
  icon,
  title,
  reason,
  requirements,
  footer,
}: {
  icon: ReactNode;
  title: string;
  reason: string;
  requirements: readonly string[];
  footer?: ReactNode;
}) {
  return (
    <div className="px-6 py-12">
      <div className="mx-auto max-w-2xl text-center">
        <div className="mx-auto flex h-10 w-10 items-center justify-center text-gray-300">
          {icon}
        </div>
        <h3 className="mt-3 text-sm font-semibold text-gray-900">{title}</h3>
        <p className="mt-2 text-sm text-gray-600">{reason}</p>
      </div>
      <div className="mx-auto mt-6 max-w-2xl rounded-md border border-gray-200 bg-gray-50 px-4 py-4">
        <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
          What has to exist first
        </h4>
        <ul className="mt-2 space-y-2">
          {requirements.map((line) => (
            <li key={line} className="flex gap-2 text-sm text-gray-600">
              <span aria-hidden="true" className="mt-2 h-1 w-1 shrink-0 rounded-full bg-gray-400" />
              <span>{line}</span>
            </li>
          ))}
        </ul>
      </div>
      {footer && <div className="mx-auto mt-4 max-w-2xl">{footer}</div>}
    </div>
  );
}

/** A standing note about something the backend does only halfway. */
export function Notice({
  tone = 'amber',
  title,
  children,
}: {
  tone?: 'amber' | 'sky';
  title: string;
  children: ReactNode;
}) {
  const tones = {
    amber: 'border-amber-200 bg-amber-50 text-amber-800',
    sky: 'border-sky-200 bg-sky-50 text-sky-800',
  } as const;
  const Icon = tone === 'amber' ? AlertTriangle : Info;
  return (
    <div className={`flex gap-3 rounded-md border px-4 py-3 text-sm ${tones[tone]}`}>
      <Icon size={16} className="mt-0.5 shrink-0" aria-hidden="true" />
      <div>
        <p className="font-medium">{title}</p>
        <div className="mt-1 space-y-1">{children}</div>
      </div>
    </div>
  );
}

const STATUS_TONES: Record<string, string> = {
  active: 'bg-emerald-50 text-emerald-700',
  running: 'bg-emerald-50 text-emerald-700',
  online: 'bg-emerald-50 text-emerald-700',
  pending: 'bg-amber-50 text-amber-700',
  starting: 'bg-amber-50 text-amber-700',
  suspended: 'bg-amber-50 text-amber-700',
  stopped: 'bg-gray-100 text-gray-600',
  inactive: 'bg-gray-100 text-gray-600',
  error: 'bg-red-50 text-red-700',
  failed: 'bg-red-50 text-red-700',
};

export function StatusBadge({ status }: { status: string | null | undefined }) {
  const value = String(status || '').toLowerCase();
  if (!value) {
    return <Unavailable reason="Not available: the API did not report a status for this project." />;
  }
  const tone = STATUS_TONES[value] || 'bg-gray-100 text-gray-600';
  return (
    <span className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${tone}`}>
      {value}
    </span>
  );
}

/** A small neutral label - project kind, runtime version, protocol. */
export function Tag({ children, tone = 'gray' }: { children: ReactNode; tone?: 'gray' | 'sky' }) {
  const tones = {
    gray: 'bg-gray-100 text-gray-700',
    sky: 'bg-sky-50 text-sky-700',
  } as const;
  return (
    <span className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${tones[tone]}`}>
      {children}
    </span>
  );
}

export function TH({ children, className = '' }: { children: ReactNode; className?: string }) {
  return (
    <th
      scope="col"
      className={`whitespace-nowrap px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 ${className}`}
    >
      {children}
    </th>
  );
}

export function TD({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <td className={`px-4 py-3 text-sm text-gray-700 ${className}`}>{children}</td>;
}

/** The dialog shell every form and confirmation on this screen uses. */
export function Modal({
  title,
  onClose,
  children,
  wide = false,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-gray-900/40 p-4 sm:items-center"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={`w-full ${wide ? 'max-w-3xl' : 'max-w-lg'} rounded-lg border border-gray-200 bg-white p-6 shadow-sm`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-5 flex items-start justify-between gap-4">
          <h2 className="text-sm font-semibold text-gray-900">{title}</h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close dialog"
            className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            <X size={16} aria-hidden="true" />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

export function FormError({ message }: { message: string }) {
  if (!message) return null;
  return (
    <div role="alert" className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
      {message}
    </div>
  );
}

/** A labelled input row, so every form on the screen aligns the same way. */
export function Field({
  id,
  label,
  required = false,
  hint,
  children,
}: {
  id: string;
  label: string;
  required?: boolean;
  hint?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div>
      <label htmlFor={id} className={LABEL_CLASS}>
        {label} {required && <span className="text-red-600">*</span>}
      </label>
      {children}
      {hint && <p className={HINT_CLASS}>{hint}</p>}
    </div>
  );
}
