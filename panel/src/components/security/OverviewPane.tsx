'use client';

/**
 * The posture summary.
 *
 * This is the screen an operator uses to decide the machine is safe, so it is
 * the screen that has to be honest. It shows five states, not two:
 *
 *   On                    read from an endpoint and applied by something
 *   Needs attention       read, on, and carrying a caveat worth acting on
 *   Off                   read, and switched off
 *   Stored, not enforced  the panel holds the setting; nothing applies it
 *   Not verifiable        the panel cannot read it at all
 *
 * The last two are what a two-state screen would have to lie about. A WAF whose
 * rules are rows in a table gets "stored, not enforced" rather than a green
 * tick, and a control with no endpoint gets "not verifiable" rather than
 * silence - silence would let an operator conclude the panel had checked.
 *
 * Ranked by risk within those groups, so what needs doing is at the top.
 */

import { ArrowRight, RefreshCw } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

import {
  BlockSkeleton,
  Panel,
  PaneHeading,
  RiskBadge,
  SectionHeader,
  StateBadge,
  StateIcon,
  SECONDARY_BUTTON_CLASS,
} from './chrome';
import { buildPosture } from './posture';
import { countStates, rankPosture, type PostureItem, type SecurityTabId } from './types';
import type { UsePostureResult } from './usePosture';

const SUMMARY_ORDER: { key: keyof ReturnType<typeof countStates>; label: string; tone: string }[] = [
  { key: 'off', label: 'Off', tone: 'border-red-200 bg-red-50 text-red-700' },
  {
    key: 'unenforced',
    label: 'Stored, not enforced',
    tone: 'border-red-200 bg-red-50 text-red-700',
  },
  {
    key: 'attention',
    label: 'Needs attention',
    tone: 'border-amber-200 bg-amber-50 text-amber-700',
  },
  { key: 'unknown', label: 'Not verifiable', tone: 'border-gray-200 bg-gray-100 text-gray-700' },
  { key: 'ok', label: 'On', tone: 'border-emerald-200 bg-emerald-50 text-emerald-700' },
];

function PostureRow({
  item,
  onNavigate,
}: {
  item: PostureItem;
  onNavigate: (tab: SecurityTabId) => void;
}) {
  return (
    <li className="flex flex-wrap items-start gap-3 border-b border-gray-100 px-4 py-3 last:border-b-0">
      <StateIcon state={item.state} className="mt-0.5 shrink-0" />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-sm font-semibold text-gray-900">{item.title}</p>
          <StateBadge state={item.state} />
          <RiskBadge risk={item.risk} />
        </div>
        <p className="mt-1 text-sm text-gray-600">{item.detail}</p>
        {item.reason && (
          <p className="mt-1 text-sm text-gray-500">
            <span className="font-medium text-gray-600">Why the panel cannot say: </span>
            {item.reason}
          </p>
        )}
      </div>
      {item.fix &&
        (item.fix.tab ? (
          <button
            type="button"
            onClick={() => onNavigate(item.fix!.tab as SecurityTabId)}
            className="inline-flex shrink-0 items-center gap-1 rounded-md px-2 py-1 text-sm font-medium text-brand-700 hover:bg-brand-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            {item.fix.label}
            <ArrowRight className="h-4 w-4" aria-hidden="true" />
          </button>
        ) : (
          <a
            href={item.fix.href}
            className="inline-flex shrink-0 items-center gap-1 rounded-md px-2 py-1 text-sm font-medium text-brand-700 hover:bg-brand-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            {item.fix.label}
            <ArrowRight className="h-4 w-4" aria-hidden="true" />
          </a>
        ))}
    </li>
  );
}

export function OverviewPane({
  posture,
  onNavigate,
}: {
  posture: UsePostureResult;
  onNavigate: (tab: SecurityTabId) => void;
}) {
  const items = rankPosture(buildPosture(posture));
  const counts = countStates(items);

  const problems = items.filter(
    (item) => item.state === 'off' || item.state === 'unenforced' || item.state === 'attention'
  );
  const unverifiable = items.filter((item) => item.state === 'unknown');
  const healthy = items.filter((item) => item.state === 'ok');

  return (
    <div className="space-y-4">
      <PaneHeading
        title="Posture"
        description="What this panel can actually prove about the security of this machine, ranked with the worst first. A control is only marked On when the panel read it from an endpoint and something applies it; anything the panel stores without applying, or cannot read at all, says so instead of showing a tick."
        actions={
          <Button
            type="button"
            variant="outline"
            onClick={posture.reload}
            disabled={posture.loading || posture.refreshing}
            className={SECONDARY_BUTTON_CLASS}
          >
            <RefreshCw
              className={cn('mr-2 h-4 w-4', posture.refreshing && 'animate-spin')}
              aria-hidden="true"
            />
            Refresh
          </Button>
        }
      />

      {posture.loading ? (
        <Panel>
          <SectionHeader title="Reading the posture" />
          <BlockSkeleton rows={5} />
        </Panel>
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
            {SUMMARY_ORDER.map((entry) => (
              <div
                key={entry.key}
                className={cn('rounded-md border px-4 py-3', entry.tone)}
              >
                <p className="text-xs font-medium uppercase tracking-wide opacity-80">
                  {entry.label}
                </p>
                <p className="mt-1 text-2xl font-semibold">{counts[entry.key]}</p>
              </div>
            ))}
          </div>

          <Panel>
            <SectionHeader
              title="Needs doing"
              description="Switched off, stored without being applied, or on with a caveat. Worst risk first."
            />
            {problems.length === 0 ? (
              <p className="px-4 py-6 text-sm text-gray-600">
                Nothing the panel can read is switched off or misconfigured. That is not the same as
                &ldquo;this machine is secure&rdquo; &mdash; see the section below for what the panel
                cannot check at all.
              </p>
            ) : (
              <ul>
                {problems.map((item) => (
                  <PostureRow key={item.id} item={item} onNavigate={onNavigate} />
                ))}
              </ul>
            )}
          </Panel>

          <Panel>
            <SectionHeader
              title="Not verifiable from this panel"
              description="Controls this screen deliberately shows no state for. Either no endpoint exists, or the request that would have answered failed. None of these is safe to assume."
            />
            {unverifiable.length === 0 ? (
              <p className="px-4 py-6 text-sm text-gray-600">
                Every control on this screen was read from a live endpoint.
              </p>
            ) : (
              <ul>
                {unverifiable.map((item) => (
                  <PostureRow key={item.id} item={item} onNavigate={onNavigate} />
                ))}
              </ul>
            )}
          </Panel>

          <Panel>
            <SectionHeader
              title="On and applied"
              description="Read from an endpoint, switched on, and enforced by something outside the panel database."
            />
            {healthy.length === 0 ? (
              <p className="px-4 py-6 text-sm text-gray-600">
                Nothing on this screen currently meets that bar.
              </p>
            ) : (
              <ul>
                {healthy.map((item) => (
                  <PostureRow key={item.id} item={item} onNavigate={onNavigate} />
                ))}
              </ul>
            )}
          </Panel>
        </>
      )}
    </div>
  );
}

export default OverviewPane;
