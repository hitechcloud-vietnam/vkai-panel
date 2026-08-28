'use client';

/**
 * Ports, TLS, and the details an operator hands to a customer.
 *
 * The most common FTP support ticket is a server that works on the machine and
 * fails from anywhere else, and it is nearly always one of three things: the
 * control port is shut, the passive data range is shut, or the daemon advertises
 * an address the client cannot reach. So this card puts the firewall's own view
 * of those ports on the screen rather than leaving an operator to guess.
 *
 * What it can and cannot say is stated on the card itself. GET /api/v1/firewall
 * returns the rules THIS PANEL holds, and GET /api/v1/firewall/active returns
 * the live `iptables -L -n -v` text. Neither is the daemon's configuration:
 * nothing in the API reads a passive range or a TLS setting out of
 * vsftpd.conf, so this card never claims to know them.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, Globe, Network, ShieldAlert } from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { errorMessage } from '@/lib/apiError';
import { cn } from '@/lib/utils';

import { getActiveFirewallRules, listFirewallRules } from './api';
import { CapabilityGaps } from './CapabilityNotice';
import { CopyButton } from './CopyButton';
import {
  CardSkeleton,
  EmptyState,
  ErrorBlock,
  Panel,
  PaneToolbar,
  SECONDARY_BUTTON_CLASS,
  SectionHeader,
  TableScroller,
  TD_CLASS,
  TH_CLASS,
} from './PaneChrome';
import { formatSpec, looksLikePassiveRange, spanOf, specCoversPort } from './ports';
import { CONFIG_GAPS, FTP_PORTS, type FirewallRule } from './types';

/** A rule only counts as opening a port when it allows inbound traffic. */
function isInboundAllow(rule: FirewallRule): boolean {
  const action = (rule.action || '').toLowerCase();
  const direction = (rule.direction || '').toLowerCase();
  return action === 'allow' && (direction === '' || direction === 'in' || direction === 'inbound');
}

export function PortsCard({ unit }: { unit: string | null }) {
  const [rules, setRules] = useState<FirewallRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [showLive, setShowLive] = useState(false);
  const [live, setLive] = useState<string | null>(null);
  const [liveLoading, setLiveLoading] = useState(false);
  const [liveError, setLiveError] = useState<string | null>(null);

  // Read on the client only: the server render has no hostname to print, and
  // guessing one would put a wrong address in front of a customer.
  const [host, setHost] = useState<string | null>(null);
  useEffect(() => {
    if (typeof window !== 'undefined') setHost(window.location.hostname || null);
  }, []);

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    else setLoading(true);
    setError(null);
    try {
      setRules(await listFirewallRules());
    } catch (err) {
      setError(errorMessage(err, 'Could not load the firewall rules.'));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const openRules = useMemo(() => rules.filter(isInboundAllow), [rules]);

  const wellKnown = useMemo(
    () =>
      FTP_PORTS.map((entry) => ({
        ...entry,
        covering: openRules.filter((rule) => specCoversPort(rule.port || '', entry.port)),
      })),
    [openRules]
  );

  const passiveCandidates = useMemo(
    () =>
      openRules
        .filter((rule) => looksLikePassiveRange(rule.port || ''))
        .sort((a, b) => spanOf(b.port || '') - spanOf(a.port || '')),
    [openRules]
  );

  const loadLive = useCallback(async () => {
    setLiveLoading(true);
    setLiveError(null);
    try {
      setLive(await getActiveFirewallRules());
    } catch (err) {
      setLive(null);
      setLiveError(errorMessage(err, 'Could not read the live firewall table.'));
    } finally {
      setLiveLoading(false);
    }
  }, []);

  // Fetched on first open and kept, so re-collapsing does not re-run iptables.
  const toggleLive = useCallback(() => {
    setShowLive((open) => {
      if (!open && live === null && !liveLoading) void loadLive();
      return !open;
    });
  }, [live, liveLoading, loadLive]);

  return (
    <div className="space-y-4">
      <Panel>
        <SectionHeader
          title="Ports through the firewall"
          description="An FTP server that works on the machine and fails from a customer's laptop is almost always a closed port. These are the rules this panel holds."
          actions={<PaneToolbar onRefresh={() => void load(true)} refreshing={refreshing} />}
        />

        {loading ? (
          <CardSkeleton rows={4} />
        ) : error ? (
          <ErrorBlock
            title="Could not load firewall rules"
            message={error}
            onRetry={() => void load(true)}
          />
        ) : rules.length === 0 ? (
          <EmptyState
            title="This panel holds no firewall rules"
            description="Nothing here says the ports are shut - it says the panel is not the thing managing them. Add the FTP rules under Firewall so the panel can show them, or check the host firewall and any cloud security group by hand."
          />
        ) : (
          <>
            <TableScroller>
              <table className="w-full min-w-[680px] border-collapse">
                <thead className="bg-gray-50">
                  <tr className="border-b border-gray-200">
                    <th className={TH_CLASS}>Port</th>
                    <th className={TH_CLASS}>What uses it</th>
                    <th className={TH_CLASS}>Panel rule</th>
                  </tr>
                </thead>
                <tbody>
                  {wellKnown.map((entry) => (
                    <tr key={entry.port} className="border-b border-gray-200">
                      <td className={cn(TD_CLASS, 'font-mono text-sm text-gray-900')}>
                        {entry.port}
                      </td>
                      <td className={TD_CLASS}>
                        <span className="text-gray-900">{entry.label}</span>
                        <p className="mt-0.5 text-xs text-gray-500">{entry.note}</p>
                      </td>
                      <td className={TD_CLASS}>
                        {entry.covering.length > 0 ? (
                          <div className="space-y-1">
                            <Badge variant="success">Allowed</Badge>
                            {entry.covering.map((rule) => (
                              <p key={rule.id} className="text-xs text-gray-500">
                                {(rule.protocol || 'tcp').toLowerCase()}{' '}
                                {formatSpec(rule.port || '')} from{' '}
                                <span className="font-mono">{rule.source || 'any'}</span>
                              </p>
                            ))}
                          </div>
                        ) : (
                          <Badge variant="neutral">No rule covers it</Badge>
                        )}
                      </td>
                    </tr>
                  ))}

                  <tr className="border-b border-gray-200 last:border-b-0">
                    <td className={cn(TD_CLASS, 'font-mono text-sm text-gray-900')}>
                      range
                    </td>
                    <td className={TD_CLASS}>
                      <span className="text-gray-900">Passive data ports</span>
                      <p className="mt-0.5 text-xs text-gray-500">
                        In passive mode the client opens a second connection on a high
                        port the server names. Shut, the login succeeds and every
                        directory listing hangs - which is why this reads as &quot;works
                        for me&quot; from the machine itself.
                      </p>
                    </td>
                    <td className={TD_CLASS}>
                      {passiveCandidates.length > 0 ? (
                        <div className="space-y-1">
                          <Badge variant="success">
                            {passiveCandidates.length === 1
                              ? '1 range open'
                              : `${passiveCandidates.length} ranges open`}
                          </Badge>
                          {passiveCandidates.map((rule) => (
                            <p key={rule.id} className="text-xs text-gray-500">
                              {(rule.protocol || 'tcp').toLowerCase()}{' '}
                              <span className="font-mono">{formatSpec(rule.port || '')}</span>{' '}
                              ({spanOf(rule.port || '')} ports) from{' '}
                              <span className="font-mono">{rule.source || 'any'}</span>
                            </p>
                          ))}
                        </div>
                      ) : (
                        <div>
                          <Badge variant="warning">No range open</Badge>
                          <p className="mt-1 text-xs text-gray-500">
                            No panel rule opens a range above port 1024.
                          </p>
                        </div>
                      )}
                    </td>
                  </tr>
                </tbody>
              </table>
            </TableScroller>

            <div className="border-t border-gray-200 px-4 py-3">
              <p className="flex items-start gap-2 text-xs text-gray-500">
                <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-gray-400" aria-hidden="true" />
                <span>
                  These are the rules the panel stores, not the kernel&apos;s. A cloud
                  security group or a firewall managed outside the panel can still be
                  shut, and the panel cannot see it. The live table below is the
                  machine&apos;s own answer.
                </span>
              </p>

              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={toggleLive}
                className={cn(SECONDARY_BUTTON_CLASS, 'mt-3')}
                aria-expanded={showLive}
              >
                {showLive ? (
                  <ChevronDown className="h-4 w-4" aria-hidden="true" />
                ) : (
                  <ChevronRight className="h-4 w-4" aria-hidden="true" />
                )}
                Live iptables table
              </Button>

              {showLive && (
                <div className="mt-3">
                  {liveLoading && (
                    <div className="space-y-2" aria-busy="true">
                      {Array.from({ length: 4 }).map((_, index) => (
                        <div key={index} className="h-3 animate-pulse rounded bg-gray-100" />
                      ))}
                    </div>
                  )}
                  {!liveLoading && liveError && (
                    <ErrorBlock
                      title="Could not read the live firewall table"
                      message={liveError}
                      onRetry={() => void loadLive()}
                    />
                  )}
                  {!liveLoading && !liveError && live !== null && (
                    live.trim() === '' ? (
                      <p className="text-sm text-gray-500">
                        iptables returned nothing. The host may use nftables or firewalld
                        directly, in which case this endpoint cannot see its rules.
                      </p>
                    ) : (
                      <pre className="max-h-72 overflow-auto rounded-md border border-gray-200 bg-gray-50 p-3 font-mono text-xs leading-relaxed text-gray-700">
                        {live}
                      </pre>
                    )
                  )}
                </div>
              )}
            </div>
          </>
        )}
      </Panel>

      <CapabilityGaps
        title="What the panel cannot read about the FTP daemon"
        intro={
          unit
            ? `${unit} is running on this machine, but no endpoint reads its configuration file, so these settings can only be changed on the machine itself.`
            : 'No endpoint reads an FTP daemon configuration file, so these settings can only be changed on the machine itself.'
        }
        gaps={CONFIG_GAPS}
      />

      <Panel>
        <SectionHeader
          title="Connection details"
          description="What a customer needs in order to connect, and what the panel cannot issue for them yet."
        />
        <div className="px-4 py-3">
          <dl className="m-0">
            <div className="flex items-start gap-3 border-b border-gray-100 py-2">
              <dt className="w-40 shrink-0 text-xs font-medium uppercase tracking-wide text-gray-500">
                Host
              </dt>
              <dd className="flex min-w-0 flex-1 items-center gap-1 text-sm text-gray-900">
                <Globe className="h-4 w-4 shrink-0 text-gray-400" aria-hidden="true" />
                {host ? (
                  <>
                    <span className="font-mono">{host}</span>
                    <CopyButton label="Host" getValue={() => host} />
                  </>
                ) : (
                  <span className="text-gray-500">Reading…</span>
                )}
              </dd>
            </div>
            <div className="flex items-start gap-3 border-b border-gray-100 py-2">
              <dt className="w-40 shrink-0 text-xs font-medium uppercase tracking-wide text-gray-500">
                Port
              </dt>
              <dd className="min-w-0 flex-1 text-sm text-gray-900">
                <span className="font-mono">21</span>
                <span className="ml-2 text-xs text-gray-500">
                  The standard control port. The panel cannot read the port the daemon is
                  actually bound to, so confirm it on the machine before sending this to a
                  customer.
                </span>
              </dd>
            </div>
            <div className="flex items-start gap-3 border-b border-gray-100 py-2">
              <dt className="w-40 shrink-0 text-xs font-medium uppercase tracking-wide text-gray-500">
                Encryption
              </dt>
              <dd className="min-w-0 flex-1 text-sm text-gray-900">
                <span className="text-gray-500">
                  Unknown. Whether the daemon requires AUTH TLS lives in its configuration
                  file, which no endpoint reads. Plain FTP sends the password in clear
                  text, so confirm the setting before handing these details out.
                </span>
              </dd>
            </div>
            <div className="flex items-start gap-3 border-b border-gray-100 py-2">
              <dt className="w-40 shrink-0 text-xs font-medium uppercase tracking-wide text-gray-500">
                Mode
              </dt>
              <dd className="min-w-0 flex-1 text-sm text-gray-900">
                Passive
                <span className="ml-2 text-xs text-gray-500">
                  {passiveCandidates.length > 0
                    ? `Data ports open in the panel firewall: ${passiveCandidates
                        .map((rule) => formatSpec(rule.port || ''))
                        .join(', ')}.`
                    : 'No passive range is open in the panel firewall, so a client may connect and then hang on the first listing.'}
                </span>
              </dd>
            </div>
            <div className="flex items-start gap-3 border-b border-gray-100 py-2 last:border-b-0">
              <dt className="w-40 shrink-0 text-xs font-medium uppercase tracking-wide text-gray-500">
                Username and password
              </dt>
              <dd className="min-w-0 flex-1 text-sm">
                <span className="inline-flex items-center gap-2">
                  <Network className="h-4 w-4 shrink-0 text-gray-400" aria-hidden="true" />
                  <span className="text-gray-500">
                    Not issued by the panel. No endpoint creates an FTP account or returns
                    a password, so there is no credential here to reveal or copy. Create
                    the account on the machine and hand over the password from there.
                  </span>
                </span>
              </dd>
            </div>
          </dl>
        </div>
      </Panel>
    </div>
  );
}

export default PortsCard;
