'use client';

/**
 * The hardening checklist.
 *
 * Same source of truth as the Overview, shown differently: the Overview ranks
 * and groups so an operator sees the worst thing first, and this pane lists
 * every check with what the panel observed and what can be done about it.
 *
 * The Action column is where this screen could most easily lie, so it does not
 * offer one unless the panel can actually carry it out. Two checks have a real
 * one-action fix behind a real endpoint - running an integrity scan, and
 * persisting the firewall rules so they survive a reboot - and those are
 * buttons. Everything else offers a link to the place the change is made, or
 * nothing at all where the panel has no way to make it. A button that opens a
 * dialog and then does nothing would be worse than the missing feature.
 */

import { useState } from 'react';
import { ArrowRight, RefreshCw, ScanLine, Save } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { errorMessage } from '@/lib/apiError';
import { cn } from '@/lib/utils';

import * as securityApi from './api';
import {
  BlockSkeleton,
  Panel,
  PaneHeading,
  RiskBadge,
  SECONDARY_BUTTON_CLASS,
  SectionHeader,
  StateBadge,
  TableScroller,
  TD_CLASS,
  TH_CLASS,
} from './chrome';
import { buildPosture } from './posture';
import { rankPosture, type SecurityTabId } from './types';
import type { UsePostureResult } from './usePosture';

export function SystemHardeningPane({
  posture,
  onNavigate,
}: {
  posture: UsePostureResult;
  onNavigate: (tab: SecurityTabId) => void;
}) {
  const [busy, setBusy] = useState('');
  const [message, setMessage] = useState('');
  const [failure, setFailure] = useState('');

  const items = rankPosture(buildPosture(posture));

  const run = async (id: string, label: string, action: () => Promise<unknown>) => {
    setBusy(id);
    setMessage('');
    setFailure('');
    try {
      await action();
      setMessage(label);
      posture.reload();
    } catch (err) {
      setFailure(errorMessage(err, 'The action did not complete.'));
    } finally {
      setBusy('');
    }
  };

  return (
    <div className="space-y-4">
      <PaneHeading
        title="System Hardening"
        description="Every check on this screen with the state the panel could establish for it. A check the panel cannot read is listed as not verifiable and stays that way - it is never counted as passed."
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
            Re-check
          </Button>
        }
      />

      {message && (
        <div className="rounded-md border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-800">
          {message}
        </div>
      )}
      {failure && (
        <div className="rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-700" role="alert">
          {failure}
        </div>
      )}

      <Panel>
        <SectionHeader
          title="Fixes this panel can apply itself"
          description="The only two checks on this screen with an endpoint behind the fix. Everything else is a link to where the change is made by hand."
        />
        <div className="grid grid-cols-1 gap-3 px-4 py-4 sm:grid-cols-2">
          <div className="rounded-md border border-gray-200 p-4">
            <p className="text-sm font-semibold text-gray-900">Run an integrity scan now</p>
            <p className="mt-1 text-sm text-gray-600">
              Re-hashes every enabled watched path and compares it against its baseline. Nothing in
              the panel does this on a schedule, so this is how the check gets refreshed.
            </p>
            <Button
              type="button"
              className="mt-3 bg-brand-600 text-white hover:bg-brand-700"
              disabled={busy === 'scan'}
              onClick={() =>
                void run('scan', 'Every enabled path was re-hashed and compared against its baseline.', () =>
                  securityApi.tamperProof.scanAll()
                )
              }
            >
              <ScanLine className="mr-2 h-4 w-4" aria-hidden="true" />
              {busy === 'scan' ? 'Scanning…' : 'Scan every path'}
            </Button>
          </div>

          <div className="rounded-md border border-gray-200 p-4">
            <p className="text-sm font-semibold text-gray-900">Persist the firewall rules</p>
            <p className="mt-1 text-sm text-gray-600">
              Writes the live iptables table to /etc/iptables/rules.v4. Without this the rules the
              panel applied are gone at the next reboot, and nothing in the panel re-applies them.
            </p>
            <Button
              type="button"
              className="mt-3 bg-brand-600 text-white hover:bg-brand-700"
              disabled={busy === 'save'}
              onClick={() =>
                void run('save', 'The live iptables table was written to /etc/iptables/rules.v4.', () =>
                  securityApi.firewall.save()
                )
              }
            >
              <Save className="mr-2 h-4 w-4" aria-hidden="true" />
              {busy === 'save' ? 'Saving…' : 'Persist to disk'}
            </Button>
          </div>
        </div>
      </Panel>

      <Panel>
        <SectionHeader
          title="Checklist"
          description="Ranked worst first, the same order as the Overview."
        />
        {posture.loading ? (
          <BlockSkeleton rows={6} />
        ) : (
          <TableScroller>
            <table className="w-full min-w-[900px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH_CLASS}>Check</th>
                  <th className={TH_CLASS}>State</th>
                  <th className={TH_CLASS}>What the panel observed</th>
                  <th className={cn(TH_CLASS, 'text-right')}>Action</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.id} className="border-b border-gray-200 last:border-b-0">
                    <td className={cn(TD_CLASS, 'align-top')}>
                      <p className="font-medium text-gray-900">{item.title}</p>
                      <div className="mt-1">
                        <RiskBadge risk={item.risk} />
                      </div>
                    </td>
                    <td className={cn(TD_CLASS, 'align-top')}>
                      <StateBadge state={item.state} />
                    </td>
                    <td className={cn(TD_CLASS, 'align-top')}>
                      <p className="max-w-xl">{item.detail}</p>
                      {item.reason && (
                        <p className="mt-1 max-w-xl text-gray-500">
                          <span className="font-medium text-gray-600">Why not verifiable: </span>
                          {item.reason}
                        </p>
                      )}
                    </td>
                    <td className={cn(TD_CLASS, 'align-top text-right')}>
                      {item.fix ? (
                        item.fix.tab ? (
                          <button
                            type="button"
                            onClick={() => onNavigate(item.fix!.tab as SecurityTabId)}
                            className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-sm font-medium text-brand-700 hover:bg-brand-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                          >
                            {item.fix.label}
                            <ArrowRight className="h-4 w-4" aria-hidden="true" />
                          </button>
                        ) : (
                          <a
                            href={item.fix.href}
                            className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-sm font-medium text-brand-700 hover:bg-brand-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                          >
                            {item.fix.label}
                            <ArrowRight className="h-4 w-4" aria-hidden="true" />
                          </a>
                        )
                      ) : (
                        <span className="text-gray-400">—</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>
    </div>
  );
}

export default SystemHardeningPane;
