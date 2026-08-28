'use client';

/**
 * Brute force protection.
 *
 * The panel already runs a credential limiter and this pane is wired to what it
 * leaves behind rather than to a second limiter of its own. Two layers exist in
 * the backend today:
 *
 *   middleware.ProtectCredentialEndpoints - mounted on the engine before the
 *   routes, layered per address, per account and per address-account pair,
 *   backed by a shared store so several panel instances count together, and
 *   fail-closed when that store is unreachable;
 *
 *   AuthService's own per-account failure tracker, which locks an account after
 *   repeated failures inside one process and writes an audit record when it
 *   refuses an attempt for that reason.
 *
 * Only the second one is readable over HTTP, and only indirectly: every refused
 * sign-in is written to the audit log with the source address, the account
 * tried and the reason. That is what the tables below show. The limits in
 * force, the addresses currently locked and the allow list are NOT readable
 * from any endpoint, and this pane says so instead of printing the compiled-in
 * defaults as though they were live - they can be overridden by environment
 * variables, so printing them would be a number the operator could not act on.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import { RefreshCw } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { errorMessage } from '@/lib/apiError';
import { cn } from '@/lib/utils';

import * as securityApi from './api';
import {
  BackendGap,
  EmptyState,
  ErrorBlock,
  Panel,
  PaneHeading,
  SECONDARY_BUTTON_CLASS,
  SectionHeader,
  StatTile,
  TableScroller,
  TableSkeleton,
  TD_CLASS,
  TH_CLASS,
} from './chrome';
import type { AuditLog } from './types';

const WINDOW_HOURS = 24;

/** The reasons AuthService records, in the words an operator would use. */
const REASON_LABELS: Record<string, string> = {
  locked_out: 'Refused: account already locked',
  bad_password: 'Wrong password',
  unknown_user: 'No such account',
  inactive_account: 'Account not active',
};

function reasonOf(row: AuditLog): string {
  const raw = String(row.details?.reason ?? '');
  return REASON_LABELS[raw] ?? (raw || 'Rejected');
}

function usernameOf(row: AuditLog): string {
  const raw = row.details?.username;
  return typeof raw === 'string' && raw.trim() !== '' ? raw : '(not recorded)';
}

function isLockout(row: AuditLog): boolean {
  return String(row.details?.reason ?? '') === 'locked_out';
}

function formatTime(iso: string): string {
  const at = Date.parse(iso);
  if (Number.isNaN(at)) return iso;
  return new Date(at).toLocaleString();
}

interface AddressSummary {
  address: string;
  attempts: number;
  lockouts: number;
  accounts: string[];
  lastSeen: string;
}

function summarise(rows: AuditLog[]): AddressSummary[] {
  const byAddress = new Map<string, AddressSummary>();
  for (const row of rows) {
    const address = row.ip_address || '(not recorded)';
    const existing = byAddress.get(address);
    const username = usernameOf(row);
    if (existing) {
      existing.attempts += 1;
      if (isLockout(row)) existing.lockouts += 1;
      if (!existing.accounts.includes(username)) existing.accounts.push(username);
      if (Date.parse(row.created_at) > Date.parse(existing.lastSeen)) {
        existing.lastSeen = row.created_at;
      }
    } else {
      byAddress.set(address, {
        address,
        attempts: 1,
        lockouts: isLockout(row) ? 1 : 0,
        accounts: [username],
        lastSeen: row.created_at,
      });
    }
  }
  return [...byAddress.values()].sort((a, b) => b.attempts - a.attempts);
}

export function BruteForcePane() {
  const [rows, setRows] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async (initial: boolean) => {
    if (initial) setLoading(true);
    else setRefreshing(true);
    try {
      setRows(await securityApi.audit.signInFailures(200));
      setError('');
    } catch (err) {
      setError(errorMessage(err, 'The audit log could not be read.'));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void load(true);
  }, [load]);

  const recent = useMemo(
    () =>
      rows.filter((row) => {
        const at = Date.parse(row.created_at);
        return !Number.isNaN(at) && Date.now() - at <= WINDOW_HOURS * 60 * 60 * 1000;
      }),
    [rows]
  );
  const addresses = useMemo(() => summarise(recent), [recent]);
  const lockouts = recent.filter(isLockout).length;

  return (
    <div className="space-y-4">
      <PaneHeading
        title="Brute force protection"
        description="The panel already guards every credential endpoint with a layered limiter that counts failures per address, per account and per address-account pair, and fails closed when its store is unreachable. This tab reads what that guard and the sign-in path leave in the audit log. It does not add a second limiter."
        actions={
          <Button
            type="button"
            variant="outline"
            onClick={() => void load(false)}
            disabled={refreshing}
            className={SECONDARY_BUTTON_CLASS}
          >
            <RefreshCw className={cn('mr-2 h-4 w-4', refreshing && 'animate-spin')} aria-hidden="true" />
            Refresh
          </Button>
        }
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile
          label={`Rejections / ${WINDOW_HOURS}h`}
          value={loading || error ? null : recent.length}
          hint={error ? 'The audit log could not be read.' : undefined}
        />
        <StatTile
          label="Source addresses"
          value={loading || error ? null : addresses.length}
          hint={error ? 'The audit log could not be read.' : undefined}
        />
        <StatTile
          label="Refused while locked"
          value={loading || error ? null : lockouts}
          hint={error ? 'The audit log could not be read.' : 'Attempts the account lockout turned away.'}
        />
        <StatTile
          label="Currently blocked"
          value={null}
          hint="No endpoint reports the addresses the limiter is holding."
        />
      </div>

      <Panel>
        <SectionHeader
          title="Limits, blocks and the allow list"
          description="The three things this tab is supposed to show, and the reason it shows none of them."
        />
        <BackendGap
          title="The limiter's live state is not readable over the API"
          summary="The guard is mounted on every credential route and it is doing its job, but nothing exposes the thresholds it is using, the address-account pairs it currently has locked, or an allow list to exempt an office address. The thresholds can be changed by environment variables, so this screen will not print the built-in defaults as if they were the live numbers."
          controls={[
            'The thresholds in force, read from the running guard rather than from the source',
            'Address-account pairs currently locked, with the time each lock expires',
            'Addresses over the per-address budget and accounts over the per-account budget',
            'Release one lock, for the support call where a real operator is locked out',
            'An allow list of addresses that are never delayed or locked, and an audit record for every change to it',
            'Whether the counter store is reachable, because the guard fails closed when it is not and every sign-in is refused',
          ]}
          endpoints={[
            'GET  /api/v1/security/brute-force/policy',
            'GET  /api/v1/security/brute-force/locks',
            'DELETE /api/v1/security/brute-force/locks/:key',
            'GET  /api/v1/security/brute-force/allow-list',
            'PUT  /api/v1/security/brute-force/allow-list',
            'GET  /api/v1/security/brute-force/store-health',
          ]}
          note="Two limiters exist in the backend and they count separately: the middleware guard, and a per-account failure tracker inside AuthService. Only the second writes an audit record, which is why the tables below can show a lockout but not a throttle. Reconciling them is part of the same piece of work."
        />
      </Panel>

      <Panel>
        <SectionHeader
          title="By source address"
          description={`Rejected sign-ins in the last ${WINDOW_HOURS} hours, grouped by where they came from. Busiest first.`}
        />
        {loading ? (
          <TableSkeleton columns={5} />
        ) : error ? (
          <ErrorBlock
            title="The audit log could not be read"
            message={error}
            onRetry={() => void load(false)}
          />
        ) : addresses.length === 0 ? (
          <EmptyState
            title="No rejected sign-in in this window"
            description={`Nothing has been turned away in the last ${WINDOW_HOURS} hours. This counts panel sign-ins only - SSH and mail rejections are not in this log.`}
          />
        ) : (
          <TableScroller>
            <table className="w-full min-w-[760px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH_CLASS}>Source address</th>
                  <th className={TH_CLASS}>Attempts</th>
                  <th className={TH_CLASS}>Refused while locked</th>
                  <th className={TH_CLASS}>Accounts tried</th>
                  <th className={TH_CLASS}>Last seen</th>
                </tr>
              </thead>
              <tbody>
                {addresses.map((entry) => (
                  <tr key={entry.address} className="border-b border-gray-200 last:border-b-0">
                    <td className={cn(TD_CLASS, 'font-mono font-medium text-gray-900')}>
                      {entry.address}
                    </td>
                    <td className={TD_CLASS}>{entry.attempts}</td>
                    <td className={TD_CLASS}>
                      {entry.lockouts > 0 ? (
                        <span className="inline-flex rounded-md border border-amber-200 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">
                          {entry.lockouts}
                        </span>
                      ) : (
                        <span className="text-gray-400">0</span>
                      )}
                    </td>
                    <td className={TD_CLASS}>
                      <span className="break-words">{entry.accounts.join(', ')}</span>
                    </td>
                    <td className={TD_CLASS}>{formatTime(entry.lastSeen)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>

      <Panel>
        <SectionHeader
          title="Recent failures"
          description="The individual rejections, newest first. These are what a lockout is built out of."
        />
        {loading ? (
          <TableSkeleton columns={4} />
        ) : error ? (
          <ErrorBlock
            title="The audit log could not be read"
            message={error}
            onRetry={() => void load(false)}
          />
        ) : rows.length === 0 ? (
          <EmptyState
            title="No rejected sign-in recorded"
            description="The audit log holds no auth.sign_in_failed entry for this tenant. Either nobody has failed a sign-in, or the audit service was not attached to the authentication service when the panel started."
          />
        ) : (
          <TableScroller>
            <table className="w-full min-w-[760px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH_CLASS}>When</th>
                  <th className={TH_CLASS}>Source address</th>
                  <th className={TH_CLASS}>Account tried</th>
                  <th className={TH_CLASS}>Outcome</th>
                </tr>
              </thead>
              <tbody>
                {rows.slice(0, 100).map((row) => (
                  <tr key={row.id} className="border-b border-gray-200 last:border-b-0">
                    <td className={cn(TD_CLASS, 'whitespace-nowrap')}>{formatTime(row.created_at)}</td>
                    <td className={cn(TD_CLASS, 'font-mono')}>{row.ip_address || '—'}</td>
                    <td className={TD_CLASS}>{usernameOf(row)}</td>
                    <td className={TD_CLASS}>
                      <span
                        className={cn(
                          'inline-flex rounded-md border px-2 py-0.5 text-xs font-medium',
                          isLockout(row)
                            ? 'border-amber-200 bg-amber-50 text-amber-700'
                            : 'border-gray-200 bg-gray-100 text-gray-700'
                        )}
                      >
                        {reasonOf(row)}
                      </span>
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

export default BruteForcePane;
