'use client';

/**
 * Firewall rules: what the panel has applied, in the order it applied them,
 * with the overlaps called out.
 *
 * Three things on this pane are deliberately blunt rather than tidy.
 *
 * The backend is named. internal/service/firewall.go shells out to `iptables`
 * and nothing else - there is no ufw, firewalld or nftables driver, and no
 * detection endpoint - so the pane states iptables as a fact instead of showing
 * a backend picker that would be decoration.
 *
 * There is no reorder control. The stored rule carries no priority column and
 * the service appends with `-A`, so a drag handle here could not change
 * anything in the chain. Instead the pane shows the order the rules were
 * applied in and which rule wins where two overlap, which is the question a
 * reorder control is usually reached for in the first place.
 *
 * Saving is reported honestly. `Create` logs an iptables failure and still
 * returns the created rule, so "saved" from this API means "stored", not
 * "applied". The pane says that and puts the live `iptables -L -n -v` listing
 * on the same screen so an operator can check rather than trust.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import { AlertTriangle, ChevronDown, ChevronUp, Info, Plus, RefreshCw, Save, Trash2, X } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { errorMessage } from '@/lib/apiError';
import { cn } from '@/lib/utils';

import * as securityApi from './api';
import type { SecurityServerOption } from './api';
import {
  DANGER_BUTTON_CLASS,
  EmptyState,
  ErrorBlock,
  INPUT_CLASS,
  Panel,
  PaneHeading,
  PRIMARY_BUTTON_CLASS,
  SECONDARY_BUTTON_CLASS,
  SectionHeader,
  TableScroller,
  TableSkeleton,
  TD_CLASS,
  TH_CLASS,
} from './chrome';
import { analyseRules, type AnalysedRule } from './firewallOverlap';
import type { FirewallRule } from './types';

const PROTOCOLS = ['tcp', 'udp', 'icmp', 'all'] as const;
const ACTIONS = ['ACCEPT', 'DROP', 'REJECT', 'LOG'] as const;
const DIRECTIONS = [
  { value: 'in', label: 'Inbound (INPUT chain)' },
  { value: 'out', label: 'Outbound (OUTPUT chain)' },
] as const;

interface RuleForm {
  server_id: string;
  protocol: string;
  port: string;
  source: string;
  action: string;
  direction: string;
}

const EMPTY_FORM: RuleForm = {
  server_id: '',
  protocol: 'tcp',
  port: '',
  source: '',
  action: 'ACCEPT',
  direction: 'in',
};

function actionBadgeClass(action: string): string {
  switch ((action || '').toUpperCase()) {
    case 'ACCEPT':
      return 'border-emerald-200 bg-emerald-50 text-emerald-700';
    case 'DROP':
      return 'border-red-200 bg-red-50 text-red-700';
    case 'REJECT':
      return 'border-amber-200 bg-amber-50 text-amber-700';
    default:
      return 'border-sky-200 bg-sky-50 text-sky-700';
  }
}

/** Client-side checks that mirror validateFirewallRule in the Go service. */
function validateForm(form: RuleForm, isEdit: boolean): string {
  if (!isEdit && !form.server_id) {
    return 'Choose the server this rule belongs to. The API requires a server id.';
  }
  if (form.port.trim() !== '' && !/^\d{1,5}(:\d{1,5})?$/.test(form.port.trim())) {
    return 'Port must be a single port (80) or a range (8000:8100).';
  }
  if (form.port.trim() !== '' && form.protocol !== 'tcp' && form.protocol !== 'udp') {
    return 'A port only applies to tcp or udp. iptables is not given --dport for icmp or all.';
  }
  const source = form.source.trim().toLowerCase();
  if (source !== '' && source !== 'any') {
    const cidr = /^(\d{1,3}\.){3}\d{1,3}(\/\d{1,2})?$/;
    if (!cidr.test(source)) {
      return 'Source must be an IPv4 address or CIDR (203.0.113.5 or 10.0.0.0/8), or left empty for any.';
    }
  }
  return '';
}

function RuleModal({
  open,
  editing,
  servers,
  serversError,
  onClose,
  onSubmit,
  submitting,
  error,
}: {
  open: boolean;
  editing: FirewallRule | null;
  servers: SecurityServerOption[];
  serversError: string;
  onClose: () => void;
  onSubmit: (form: RuleForm) => void;
  submitting: boolean;
  error: string;
}) {
  const [form, setForm] = useState<RuleForm>(EMPTY_FORM);
  const [localError, setLocalError] = useState('');

  useEffect(() => {
    if (!open) return;
    setLocalError('');
    if (editing) {
      setForm({
        server_id: editing.server_id,
        protocol: editing.protocol || 'tcp',
        port: editing.port || '',
        source: editing.source || '',
        action: (editing.action || 'ACCEPT').toUpperCase(),
        direction: editing.direction || 'in',
      });
    } else {
      setForm({ ...EMPTY_FORM, server_id: servers[0]?.id ?? '' });
    }
  }, [open, editing, servers]);

  if (!open) return null;

  const isEdit = editing !== null;

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    const message = validateForm(form, isEdit);
    if (message) {
      setLocalError(message);
      return;
    }
    setLocalError('');
    onSubmit(form);
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-gray-900/40 p-4 sm:p-8"
      role="dialog"
      aria-modal="true"
      aria-labelledby="firewall-rule-title"
    >
      <div className="w-full max-w-lg rounded-lg border border-gray-200 bg-white shadow-sm">
        <div className="flex items-center justify-between border-b border-gray-200 px-4 py-3">
          <h3 id="firewall-rule-title" className="text-sm font-semibold text-gray-900">
            {isEdit ? 'Edit firewall rule' : 'Add firewall rule'}
          </h3>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>

        <form onSubmit={submit} className="space-y-4 px-4 py-4">
          {!isEdit && (
            <div>
              <label htmlFor="fw-server" className="block text-sm font-medium text-gray-700">
                Server
              </label>
              {serversError ? (
                <p className="mt-1 text-sm text-red-700">{serversError}</p>
              ) : (
                <select
                  id="fw-server"
                  value={form.server_id}
                  onChange={(e) => setForm({ ...form, server_id: e.target.value })}
                  className={INPUT_CLASS}
                >
                  <option value="">Choose a server</option>
                  {servers.map((server) => (
                    <option key={server.id} value={server.id}>
                      {server.name || server.hostname || server.ip_address || server.id}
                    </option>
                  ))}
                </select>
              )}
            </div>
          )}

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor="fw-direction" className="block text-sm font-medium text-gray-700">
                Direction
              </label>
              <select
                id="fw-direction"
                value={form.direction}
                onChange={(e) => setForm({ ...form, direction: e.target.value })}
                className={INPUT_CLASS}
              >
                {DIRECTIONS.map((direction) => (
                  <option key={direction.value} value={direction.value}>
                    {direction.label}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor="fw-protocol" className="block text-sm font-medium text-gray-700">
                Protocol
              </label>
              <select
                id="fw-protocol"
                value={form.protocol}
                onChange={(e) => setForm({ ...form, protocol: e.target.value })}
                className={INPUT_CLASS}
              >
                {PROTOCOLS.map((protocol) => (
                  <option key={protocol} value={protocol}>
                    {protocol}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor="fw-port" className="block text-sm font-medium text-gray-700">
                Port
              </label>
              <input
                id="fw-port"
                value={form.port}
                onChange={(e) => setForm({ ...form, port: e.target.value })}
                placeholder="443 or 8000:8100"
                className={INPUT_CLASS}
              />
              <p className="mt-1 text-xs text-gray-500">
                Leave empty for every port. Only used for tcp and udp.
              </p>
            </div>
            <div>
              <label htmlFor="fw-source" className="block text-sm font-medium text-gray-700">
                Source
              </label>
              <input
                id="fw-source"
                value={form.source}
                onChange={(e) => setForm({ ...form, source: e.target.value })}
                placeholder="10.0.0.0/8 or empty for any"
                className={INPUT_CLASS}
              />
              <p className="mt-1 text-xs text-gray-500">
                IPv4 address or CIDR. IPv6 is rejected by the API.
              </p>
            </div>
          </div>

          <div>
            <label htmlFor="fw-action" className="block text-sm font-medium text-gray-700">
              Action
            </label>
            <select
              id="fw-action"
              value={form.action}
              onChange={(e) => setForm({ ...form, action: e.target.value })}
              className={INPUT_CLASS}
            >
              {ACTIONS.map((action) => (
                <option key={action} value={action}>
                  {action}
                </option>
              ))}
            </select>
          </div>

          {isEdit && (
            <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
              <p className="font-medium">Two things this edit does that are not obvious.</p>
              <ul className="mt-1 list-disc space-y-0.5 pl-5">
                <li>
                  The rule is removed and re-applied, which moves it to the bottom of the chain. Any
                  rule that used to sit after it now wins ahead of it.
                </li>
                <li>
                  A field left empty is ignored by the update endpoint rather than cleared. To widen
                  a source back to any, delete this rule and add it again.
                </li>
              </ul>
            </div>
          )}

          {(localError || error) && (
            <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700" role="alert">
              {localError || error}
            </div>
          )}

          <div className="flex justify-end gap-2 border-t border-gray-200 pt-4">
            <Button type="button" variant="outline" onClick={onClose} className={SECONDARY_BUTTON_CLASS}>
              Cancel
            </Button>
            <Button type="submit" disabled={submitting} className={PRIMARY_BUTTON_CLASS}>
              {submitting ? 'Saving…' : isEdit ? 'Save changes' : 'Add rule'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}

export function FirewallPane() {
  const [rules, setRules] = useState<FirewallRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');

  const [servers, setServers] = useState<SecurityServerOption[]>([]);
  const [serversError, setServersError] = useState('');

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<FirewallRule | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState('');
  const [notice, setNotice] = useState('');

  const [liveOpen, setLiveOpen] = useState(false);
  const [live, setLive] = useState('');
  const [liveLoading, setLiveLoading] = useState(false);
  const [liveError, setLiveError] = useState('');

  const [savingRules, setSavingRules] = useState(false);

  const loadRules = useCallback(async (initial: boolean) => {
    if (initial) setLoading(true);
    else setRefreshing(true);
    try {
      setRules(await securityApi.firewall.list());
      setError('');
    } catch (err) {
      setError(errorMessage(err, 'The firewall rules could not be read.'));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void loadRules(true);
    void (async () => {
      try {
        setServers(await securityApi.servers.list());
        setServersError('');
      } catch (err) {
        setServersError(
          errorMessage(err, 'The server list could not be read, so a new rule cannot be addressed.')
        );
      }
    })();
  }, [loadRules]);

  const loadLive = useCallback(async () => {
    setLiveLoading(true);
    try {
      setLive(await securityApi.firewall.active());
      setLiveError('');
    } catch (err) {
      setLiveError(errorMessage(err, 'The live iptables table could not be read.'));
    } finally {
      setLiveLoading(false);
    }
  }, []);

  const analysed: AnalysedRule[] = useMemo(() => analyseRules(rules), [rules]);
  const conflicting = analysed.filter((entry) => entry.conflicts.length > 0);

  const submit = async (form: RuleForm) => {
    setSubmitting(true);
    setFormError('');
    try {
      if (editing) {
        await securityApi.firewall.update(editing.id, {
          protocol: form.protocol,
          port: form.port.trim(),
          source: form.source.trim(),
          action: form.action,
          direction: form.direction,
        });
      } else {
        await securityApi.firewall.create({
          server_id: form.server_id,
          protocol: form.protocol,
          port: form.port.trim(),
          source: form.source.trim(),
          action: form.action,
          direction: form.direction,
        });
      }
      setModalOpen(false);
      setEditing(null);
      setNotice(
        'The rule is stored. The firewall endpoint answers success as soon as it has written the row - it does not fail the request when iptables refuses the rule, so check the live table below before you rely on it.'
      );
      await loadRules(false);
      if (liveOpen) await loadLive();
    } catch (err) {
      setFormError(errorMessage(err, 'The rule could not be saved.'));
    } finally {
      setSubmitting(false);
    }
  };

  const remove = async (rule: FirewallRule) => {
    const label = `${rule.protocol || 'all'}${rule.port ? `:${rule.port}` : ''} → ${rule.action}`;
    if (!window.confirm(`Delete the rule ${label}? It is removed from iptables and from the panel.`)) {
      return;
    }
    try {
      await securityApi.firewall.remove(rule.id);
      setNotice('');
      await loadRules(false);
      if (liveOpen) await loadLive();
    } catch (err) {
      setError(errorMessage(err, 'The rule could not be deleted.'));
    }
  };

  const persist = async () => {
    setSavingRules(true);
    try {
      await securityApi.firewall.save();
      setNotice('The live iptables table was written to /etc/iptables/rules.v4 so it survives a reboot.');
    } catch (err) {
      setError(errorMessage(err, 'The rules could not be saved to disk.'));
    } finally {
      setSavingRules(false);
    }
  };

  return (
    <div className="space-y-4">
      <PaneHeading
        title="Firewall"
        description="Rules the panel has applied to this host. The panel drives iptables directly - it does not detect or drive ufw, firewalld or nftables, so anything those tools are doing on this machine is invisible here and will not be shown as a rule."
        actions={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => void loadRules(false)}
              disabled={refreshing}
              className={SECONDARY_BUTTON_CLASS}
            >
              <RefreshCw className={cn('mr-2 h-4 w-4', refreshing && 'animate-spin')} aria-hidden="true" />
              Refresh
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => void persist()}
              disabled={savingRules}
              className={SECONDARY_BUTTON_CLASS}
            >
              <Save className="mr-2 h-4 w-4" aria-hidden="true" />
              {savingRules ? 'Saving…' : 'Persist to disk'}
            </Button>
            <Button
              type="button"
              onClick={() => {
                setEditing(null);
                setFormError('');
                setModalOpen(true);
              }}
              className={PRIMARY_BUTTON_CLASS}
            >
              <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
              Add rule
            </Button>
          </>
        }
      />

      <div className="flex flex-wrap items-center gap-2 rounded-md border border-gray-200 bg-white px-4 py-3 text-sm text-gray-600">
        <Info className="h-4 w-4 shrink-0 text-gray-400" aria-hidden="true" />
        <span>
          Backend in use: <span className="font-semibold text-gray-900">iptables</span>. Rules are
          appended to the INPUT or OUTPUT chain, so the rule applied first wins any traffic two rules
          share. There is no reorder control because the stored rule carries no priority and the
          service only appends &mdash; the order below is the order the panel applied them in.
        </span>
      </div>

      {notice && (
        <div className="flex items-start gap-3 rounded-md border border-sky-200 bg-sky-50 p-4 text-sm text-sky-800">
          <Info className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <p className="flex-1">{notice}</p>
          <button
            type="button"
            onClick={() => setNotice('')}
            aria-label="Dismiss"
            className="rounded-md p-1 hover:bg-sky-100"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      )}

      {conflicting.length > 0 && (
        <div className="rounded-md border border-amber-200 bg-amber-50 p-4">
          <div className="flex items-start gap-3">
            <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" aria-hidden="true" />
            <div className="min-w-0">
              <p className="text-sm font-semibold text-amber-900">
                {conflicting.length} of {analysed.length} rules never decide anything
              </p>
              <p className="mt-1 text-sm text-amber-800">
                An earlier rule in the chain already covers their traffic. iptables stops at the first
                match, so these rules are read and passed over. Each one is marked in the table.
              </p>
            </div>
          </div>
        </div>
      )}

      <Panel>
        <SectionHeader
          title="Rules"
          description="In the order the panel applied them. Rule 1 is evaluated first."
        />
        {loading ? (
          <TableSkeleton columns={7} />
        ) : error ? (
          <ErrorBlock
            title="The firewall rules could not be read"
            message={error}
            onRetry={() => void loadRules(false)}
          />
        ) : analysed.length === 0 ? (
          <EmptyState
            title="The panel manages no firewall rules on this host"
            description="Add a rule to have the panel apply it with iptables. An empty list does not mean this machine has no firewall - it means the panel wrote none of it. Open the live table below to see what iptables is actually carrying."
            action={
              <Button
                type="button"
                onClick={() => {
                  setEditing(null);
                  setModalOpen(true);
                }}
                className={PRIMARY_BUTTON_CLASS}
              >
                <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
                Add the first rule
              </Button>
            }
          />
        ) : (
          <TableScroller>
            <table className="w-full min-w-[900px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH_CLASS}>#</th>
                  <th className={TH_CLASS}>Direction</th>
                  <th className={TH_CLASS}>Protocol</th>
                  <th className={TH_CLASS}>Port</th>
                  <th className={TH_CLASS}>Source</th>
                  <th className={TH_CLASS}>Action</th>
                  <th className={TH_CLASS}>Status</th>
                  <th className={cn(TH_CLASS, 'text-right')}>
                    <span className="sr-only">Row actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {analysed.map((entry) => (
                  <tr key={entry.rule.id} className="border-b border-gray-200 last:border-b-0">
                    <td className={cn(TD_CLASS, 'font-mono text-gray-900')}>{entry.position}</td>
                    <td className={TD_CLASS}>{entry.rule.direction === 'out' ? 'Outbound' : 'Inbound'}</td>
                    <td className={cn(TD_CLASS, 'uppercase')}>{entry.rule.protocol || 'all'}</td>
                    <td className={cn(TD_CLASS, 'font-mono')}>{entry.rule.port || 'any'}</td>
                    <td className={cn(TD_CLASS, 'font-mono')}>{entry.rule.source || 'any'}</td>
                    <td className={TD_CLASS}>
                      <span
                        className={cn(
                          'inline-flex rounded-md border px-2 py-0.5 text-xs font-medium',
                          actionBadgeClass(entry.rule.action)
                        )}
                      >
                        {(entry.rule.action || '').toUpperCase()}
                      </span>
                      {entry.conflicts.map((conflict) => (
                        <p
                          key={`${entry.rule.id}-${conflict.winnerId}-${conflict.kind}`}
                          className="mt-1 max-w-md text-xs text-amber-700"
                        >
                          {conflict.kind === 'shadowed' ? 'Shadowed. ' : 'Redundant. '}
                          {conflict.message}
                        </p>
                      ))}
                      {entry.reappended && (
                        <p className="mt-1 max-w-md text-xs text-gray-500">
                          Edited after it was created, which re-appended it. Its real position in the
                          chain is later than shown.
                        </p>
                      )}
                    </td>
                    <td className={TD_CLASS}>{entry.rule.status || 'active'}</td>
                    <td className={cn(TD_CLASS, 'text-right')}>
                      <div className="flex justify-end gap-2">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() => {
                            setEditing(entry.rule);
                            setFormError('');
                            setModalOpen(true);
                          }}
                          className={SECONDARY_BUTTON_CLASS}
                        >
                          Edit
                        </Button>
                        <Button
                          type="button"
                          size="sm"
                          onClick={() => void remove(entry.rule)}
                          className={DANGER_BUTTON_CLASS}
                        >
                          <Trash2 className="h-4 w-4" aria-hidden="true" />
                          <span className="sr-only">Delete rule {entry.position}</span>
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>

      <Panel>
        <SectionHeader
          title="Live iptables table"
          description="What the kernel is carrying right now, from iptables -L -n -v. This includes rules the panel never wrote."
          actions={
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                const next = !liveOpen;
                setLiveOpen(next);
                if (next && !live && !liveError) void loadLive();
              }}
              className={SECONDARY_BUTTON_CLASS}
            >
              {liveOpen ? (
                <ChevronUp className="mr-2 h-4 w-4" aria-hidden="true" />
              ) : (
                <ChevronDown className="mr-2 h-4 w-4" aria-hidden="true" />
              )}
              {liveOpen ? 'Hide' : 'Show'}
            </Button>
          }
        />
        {liveOpen &&
          (liveLoading ? (
            <TableSkeleton columns={3} rows={5} />
          ) : liveError ? (
            <ErrorBlock
              title="The live table could not be read"
              message={liveError}
              onRetry={() => void loadLive()}
            />
          ) : live.trim() === '' ? (
            <EmptyState
              title="iptables returned nothing"
              description="The command ran and produced no output. On a host without iptables installed, or in a container without the capability, this is what you get."
            />
          ) : (
            <div className="overflow-x-auto p-4">
              <pre className="min-w-max rounded-md bg-gray-50 p-4 font-mono text-xs leading-relaxed text-gray-800">
                {live}
              </pre>
            </div>
          ))}
      </Panel>

      <RuleModal
        open={modalOpen}
        editing={editing}
        servers={servers}
        serversError={serversError}
        submitting={submitting}
        error={formError}
        onClose={() => {
          setModalOpen(false);
          setEditing(null);
        }}
        onSubmit={(form) => void submit(form)}
      />
    </div>
  );
}

export default FirewallPane;
