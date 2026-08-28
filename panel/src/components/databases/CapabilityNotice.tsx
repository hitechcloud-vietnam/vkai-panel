'use client';

/**
 * The honest empty state for a control the backend cannot perform.
 *
 * This panel has a documented history of interfaces wired to nothing: a
 * two-factor flow whose routes were never mounted, a settings page calling four
 * endpoints that all answered 404, an agent channel that was two TODO stubs.
 * Each looked finished. Each was discovered by an operator, in production, at
 * the moment they needed it.
 *
 * So a capability the API does not have gets a block that says which capability
 * is missing and what specifically is absent - the route, the switch arm, the
 * function that returns an error - rather than a disabled button with a
 * shrugging tooltip. An operator reading it learns whether to wait for a
 * release or to go and do the job over SSH, and whoever picks up the backlog
 * gets a ticket already written.
 */

import { Info, Lock } from 'lucide-react';

import type { CapabilityGap } from '@/types/databases';

/** A list of things this pane would offer if the backend could do them. */
export function CapabilityGaps({
  title,
  intro,
  gaps,
}: {
  title: string;
  intro: string;
  gaps: CapabilityGap[];
}) {
  if (gaps.length === 0) return null;
  return (
    <section className="rounded-lg border border-amber-200 bg-amber-50 p-4">
      <div className="flex items-start gap-3">
        <Info className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" aria-hidden="true" />
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-amber-900">{title}</h3>
          <p className="mt-1 text-sm text-amber-800">{intro}</p>
          <ul className="mt-3 space-y-2">
            {gaps.map((gap) => (
              <li key={gap.label} className="text-sm text-amber-800">
                <span className="font-medium text-amber-900">{gap.label}</span>
                <span className="text-amber-700"> — {gap.missing}</span>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  );
}

/**
 * The whole-pane version, for an engine the service cannot manage at all.
 * It states the limit first, then what the pane can still usefully show.
 */
export function EngineUnsupportedNotice({
  engineLabel,
  wouldShow,
  gaps,
}: {
  engineLabel: string;
  /** What this pane would show if the endpoints existed, in an operator's words. */
  wouldShow: string[];
  gaps: CapabilityGap[];
}) {
  return (
    <section className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <div className="border-b border-gray-200 px-4 py-3">
        <div className="flex items-start gap-3">
          <Lock className="mt-0.5 h-5 w-5 shrink-0 text-gray-400" aria-hidden="true" />
          <div>
            <h3 className="text-sm font-semibold text-gray-900">
              {engineLabel} management is not available yet
            </h3>
            <p className="mt-1 text-sm text-gray-600">
              The panel can register a {engineLabel} instance and show it here, but it
              cannot create, drop or re-password anything on it. Creating a database
              against a {engineLabel} server returns{' '}
              <span className="font-mono text-xs">unsupported database type</span> from
              the API, so this pane does not offer the control.
            </p>
          </div>
        </div>
      </div>

      <div className="grid gap-4 px-4 py-4 sm:grid-cols-2">
        <div>
          <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
            What this pane will show once it is supported
          </h4>
          <ul className="mt-2 space-y-1.5">
            {wouldShow.map((item) => (
              <li key={item} className="flex gap-2 text-sm text-gray-600">
                <span aria-hidden="true" className="text-gray-300">
                  •
                </span>
                <span>{item}</span>
              </li>
            ))}
          </ul>
        </div>
        <div>
          <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
            What is missing in the backend
          </h4>
          <ul className="mt-2 space-y-2">
            {gaps.map((gap) => (
              <li key={gap.label} className="text-sm">
                <span className="font-medium text-gray-900">{gap.label}</span>
                <span className="text-gray-500"> — {gap.missing}</span>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  );
}
