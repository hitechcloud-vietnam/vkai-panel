'use client';

/**
 * The tab for a project type the backend cannot manage.
 *
 * It exists, it is reachable, and it tells the truth: what the type would hold,
 * why nothing is listed, and what has to be built first. There is no create
 * button, no form and no disabled table - a control that cannot work is not
 * offered at all, and the reason is written out instead.
 *
 * This is the same shape for Go and Python, so it is one component with the
 * facts passed in rather than two files that will drift apart.
 */

import Link from 'next/link';
import type { ReactNode } from 'react';

import { CapabilityGap, Notice, Surface, SurfaceHeader, Tag } from './ui';
import { BTN_SECONDARY } from './ui';

export interface UnavailableProjectTypeProps {
  title: string;
  description: string;
  icon: ReactNode;
  /** Why there is nothing to list, in one sentence an operator can act on. */
  reason: string;
  /** The concrete missing pieces. Rendered as the backlog. */
  requirements: readonly string[];
  /** The fields this type would carry, so the intent is not lost. */
  fields: readonly string[];
  /** Everything the backend does have that is adjacent, named honestly. */
  notes: readonly string[];
}

export default function UnavailableProjectType({
  title,
  description,
  icon,
  reason,
  requirements,
  fields,
  notes,
}: UnavailableProjectTypeProps) {
  return (
    <div className="space-y-4">
      <Notice tone="amber" title="This project type is not available yet">
        {notes.map((note) => (
          <p key={note}>{note}</p>
        ))}
      </Notice>

      <Surface>
        <SurfaceHeader title={title} description={description} />
        <CapabilityGap
          icon={icon}
          title="Nothing to list, because nothing can be created"
          reason={reason}
          requirements={requirements}
          footer={
            <div className="rounded-md border border-gray-200 bg-white px-4 py-4">
              <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                What this tab will show once it exists
              </h4>
              <div className="mt-2 flex flex-wrap gap-2">
                {fields.map((field) => (
                  <Tag key={field}>{field}</Tag>
                ))}
              </div>
              <p className="mt-3 text-sm text-gray-600">
                Until then, a service of this kind is run from a systemd unit written by hand and
                watched on the{' '}
                <Link
                  href="/monitoring"
                  className="font-medium text-brand-700 underline underline-offset-2 hover:text-brand-800"
                >
                  Monitoring
                </Link>{' '}
                screen. A hostname in front of it can be recorded as a proxy project.
              </p>
              <Link href="/websites?type=proxy" className={`${BTN_SECONDARY} mt-3 w-fit`}>
                Open the Proxy Project tab
              </Link>
            </div>
          }
        />
      </Surface>
    </div>
  );
}
