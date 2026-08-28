'use client';

/**
 * The honest states for a control the backend cannot perform.
 *
 * This panel has a documented history of interfaces wired to nothing: a
 * two-factor flow whose routes were never mounted, a settings page calling four
 * endpoints that all answered 404, an agent channel that was two TODO stubs
 * beside a real route. Each looked finished. Each was found by an operator, in
 * production, at the moment they needed it.
 *
 * Docker is the largest instance of that pattern in the codebase right now:
 * twenty routes are mounted and every handler is a literal. So these components
 * say which handler is empty rather than offering a disabled button with a
 * shrugging tooltip. An operator reading one learns whether to wait for a
 * release or to go and do the job over SSH, and whoever picks up the backlog
 * gets a ticket already written.
 */

import { FileWarning, Info, Lock } from 'lucide-react';

import type { CapabilityGap } from '@/types/docker';

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
 * A list endpoint that answered, but whose handler is a hardcoded empty slice.
 *
 * This replaces the ordinary empty state, and the difference is the whole
 * point. "No containers yet, create one" and "the panel cannot see your
 * containers" are opposite facts, and printing the first when the second is
 * true is how an operator ends up creating a second copy of something that was
 * already running.
 */
export function StubResultNotice({
  resource,
  handler,
  detail,
}: {
  /** What the operator was looking for, e.g. "containers". */
  resource: string;
  /** The Go function that is empty, e.g. "ListContainers". */
  handler: string;
  detail: string;
}) {
  return (
    <div className="m-4 rounded-md border border-amber-200 bg-amber-50 p-4">
      <div className="flex items-start gap-3">
        <FileWarning className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" aria-hidden="true" />
        <div className="min-w-0">
          <p className="text-sm font-semibold text-amber-900">
            This list is empty because the panel is not reading Docker yet
          </p>
          <p className="mt-1 text-sm text-amber-800">
            The request for {resource} succeeded, so your session and permissions are
            fine. It came back empty because{' '}
            <span className="font-mono text-xs">{handler}</span> in{' '}
            <span className="font-mono text-xs">core/internal/handler/docker.go</span> is a
            stub: {detail} Do not read this as &ldquo;there are no {resource}&rdquo; — check
            the host directly until the handler is implemented.
          </p>
        </div>
      </div>
    </div>
  );
}

/**
 * A whole pane the API has no route for at all. States the limit first, then
 * what the pane will show once the route exists, then the backlog.
 */
export function PaneUnavailable({
  title,
  summary,
  wouldShow,
  gaps,
}: {
  title: string;
  summary: React.ReactNode;
  /** What this pane will show once the endpoints exist, in an operator's words. */
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
