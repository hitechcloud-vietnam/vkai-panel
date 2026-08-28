'use client';

/**
 * The honest empty state for a control the backend cannot perform.
 *
 * A disabled button with a shrugging tooltip tells an operator nothing. A block
 * that names the missing route tells them whether to wait for a release or go
 * and do the job over SSH, and hands whoever picks up the backlog a ticket that
 * is already written.
 */

import { Info, Lock } from 'lucide-react';

import type { CapabilityGap } from './types';

/** A list of things a pane would offer if the backend could do them. */
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
 * The whole-pane version, for a feature the API cannot do at all: what the pane
 * will show once it works, beside what is missing today.
 */
export function FeatureUnavailableNotice({
  title,
  summary,
  wouldShow,
  gaps,
}: {
  title: string;
  summary: React.ReactNode;
  /** The columns and controls this pane will carry, in an operator's words. */
  wouldShow: string[];
  gaps: CapabilityGap[];
}) {
  return (
    <section className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <div className="border-b border-gray-200 px-4 py-3">
        <div className="flex items-start gap-3">
          <Lock className="mt-0.5 h-5 w-5 shrink-0 text-gray-400" aria-hidden="true" />
          <div className="min-w-0">
            <h3 className="text-sm font-semibold text-gray-900">{title}</h3>
            <p className="mt-1 text-sm text-gray-600">{summary}</p>
          </div>
        </div>
      </div>

      <div className="grid gap-6 px-4 py-4 sm:grid-cols-2">
        <div>
          <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
            What this pane will show once it is wired
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
