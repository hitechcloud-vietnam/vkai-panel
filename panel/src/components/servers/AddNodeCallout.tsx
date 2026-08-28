'use client';

/**
 * "You already have one machine. Here is how to add another, if you ever need
 * one."
 *
 * The panel manages the machine it is installed on, so an operator is never
 * blocked waiting for a second server. Extra machines, clustering and high
 * availability are a layer added on top when a fleet grows - not a step on the
 * way to a working install. This block says that in the two places an operator
 * would look for it, so the cluster menu item stops reading as an unexplained
 * requirement.
 */

import { useState } from 'react';
import Link from 'next/link';
import { ChevronDown, ChevronRight, Network } from 'lucide-react';

import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

/**
 * The enrolment commands. A shell snippet is a machine value: it is the same in
 * every locale and is never translated.
 */
const ENROLMENT_COMMAND =
  'curl -sSL https://install.vkai.vn/agent.sh | bash\n' +
  'VKAI_PANEL_URL=https://panel.example.vn \\\n' +
  'VKAI_AGENT_ENROLMENT_TOKEN=vkai-enrol.v1.... \\\n' +
  '  sudo systemctl start vkai-agent';

export interface AddNodeCopy {
  title: string;
  /** One line: this panel already manages its own host. */
  lead: string;
  /** Label on the disclosure control. */
  toggleShow: string;
  toggleHide: string;
  /** Ordered steps for enrolling another machine. */
  steps: string[];
  /** The optional-layer sentence about clustering and HA. */
  optionalNote: string;
  clustersLink: string;
  docsLink: string;
}

/**
 * Deprecated. Every string now comes from the dictionary; this export remains
 * only so the screens that still pass `copy` keep compiling, and passing it
 * changes nothing. Drop the prop at the call site and this goes with it.
 */
export const ADD_NODE_COPY_EN: Partial<AddNodeCopy> = {};

export interface AddNodeCalloutProps {
  /** Deprecated per-field override; omit it and the dictionary is used. */
  copy?: Partial<AddNodeCopy>;
  docsHref?: string;
  className?: string;
}

export default function AddNodeCallout({
  copy,
  docsHref = 'https://hitechcloud.vn/docs',
  className,
}: AddNodeCalloutProps) {
  const t = useT();
  const [open, setOpen] = useState(false);

  const title = copy?.title ?? t('servers.addNode.title');
  const lead = copy?.lead ?? t('servers.addNode.lead');
  const toggleShow = copy?.toggleShow ?? t('servers.addNode.toggleShow');
  const toggleHide = copy?.toggleHide ?? t('servers.addNode.toggleHide');
  const optionalNote = copy?.optionalNote ?? t('servers.addNode.optionalNote');
  const clustersLink = copy?.clustersLink ?? t('servers.addNode.clustersLink');
  const docsLink = copy?.docsLink ?? t('common.documentation');
  const steps = copy?.steps ?? [
    t('servers.addNode.step1'),
    t('servers.addNode.step2'),
    t('servers.addNode.step3'),
  ];

  return (
    <section
      className={cn('rounded-lg border border-gray-200 bg-white shadow-sm', className)}
      aria-label={title}
    >
      <div className="flex flex-wrap items-start gap-3 px-5 py-4">
        <div className="mt-0.5 rounded-md border border-gray-200 bg-gray-50 p-2 text-gray-600">
          <Network size={18} aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-semibold text-gray-900">{title}</h2>
          <p className="mt-1 text-sm text-gray-600">{lead}</p>
          <p className="mt-2 text-sm text-gray-600">{optionalNote}</p>

          <div className="mt-3 flex flex-wrap items-center gap-3">
            <button
              type="button"
              onClick={() => setOpen((prev) => !prev)}
              aria-expanded={open}
              className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              {open ? (
                <ChevronDown size={14} aria-hidden="true" />
              ) : (
                <ChevronRight size={14} aria-hidden="true" />
              )}
              {open ? toggleHide : toggleShow}
            </button>
            <Link
              href="/clusters"
              className="rounded-md text-sm font-medium text-brand-700 hover:text-brand-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              {clustersLink}
            </Link>
            <a
              href={docsHref}
              target="_blank"
              rel="noreferrer"
              className="rounded-md text-sm font-medium text-brand-700 hover:text-brand-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              {docsLink}
            </a>
          </div>

          {open && (
            <div className="mt-4 border-t border-gray-200 pt-4">
              <ol className="list-decimal space-y-1.5 pl-5 text-sm text-gray-700">
                {steps.map((step) => (
                  <li key={step}>{step}</li>
                ))}
              </ol>
              <pre className="mt-3 overflow-x-auto rounded-md border border-gray-200 bg-gray-50 p-3 font-mono text-xs text-gray-700">
                {ENROLMENT_COMMAND}
              </pre>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
