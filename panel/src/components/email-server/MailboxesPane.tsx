'use client';

/**
 * Mailboxes: the accounts themselves, with the four actions aaPanel offers -
 * Add Mailbox, Batch Create, Import, Export.
 *
 * Batch Create and Import have no server-side endpoint. Rather than a form that
 * posts into nothing, both drive the real POST /api/v1/mail-server/accounts
 * once per row and report the outcome of every row, including the ones that
 * failed. That is slower than a bulk endpoint and it is not atomic - a run that
 * stops halfway leaves the mailboxes it already made - so the screen says so
 * before the operator starts, and the gap is on the backend backlog.
 *
 * Export is built in the browser from the rows already loaded. It carries no
 * passwords: they are hashed server-side and the API never returns them.
 */

import { useCallback, useMemo, useRef, useState } from 'react';
import { Download, Pencil, Plus, Trash2, Upload, Users } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { errorMessage } from '@/lib/apiError';

import { mailServerApi } from './api';
import {
  ActionError,
  Dash,
  EmptyState,
  ErrorBlock,
  Field,
  Modal,
  Panel,
  Pill,
  ROW,
  SectionHeader,
  TableScroller,
  TableSkeleton,
  TD,
  TH,
  Toolbar,
} from './chrome';
import { formatDateTime, formatMB, quotaLabel, usagePercent } from './format';
import { useResource } from './useResource';
import type { MailAccount, MailAlias, MailDomain } from './types';

interface MailboxData {
  accounts: MailAccount[];
  domains: MailDomain[];
  aliases: MailAlias[];
}

const SELECT_CLASS =
  'h-9 w-full rounded-md border border-gray-300 bg-white px-3 text-sm text-gray-900 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500 disabled:bg-gray-50 disabled:text-gray-500';

/** One row of a bulk run, so a partial failure is legible afterwards. */
interface BulkResult {
  email: string;
  ok: boolean;
  message: string;
}

function randomPassword(length = 14): string {
  const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789';
  const out: string[] = [];
  if (typeof window !== 'undefined' && window.crypto?.getRandomValues) {
    const bytes = new Uint32Array(length);
    window.crypto.getRandomValues(bytes);
    for (let i = 0; i < length; i += 1) out.push(alphabet[bytes[i] % alphabet.length]);
  } else {
    for (let i = 0; i < length; i += 1) {
      out.push(alphabet[Math.floor(Math.random() * alphabet.length)]);
    }
  }
  return out.join('');
}

function csvCell(value: string): string {
  return /[",\n]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value;
}

export default function MailboxesPane() {
  const load = useCallback(async (): Promise<MailboxData> => {
    const [accounts, domains, aliases] = await Promise.all([
      mailServerApi.listAccounts(),
      mailServerApi.listDomains(),
      mailServerApi.listAliases(),
    ]);
    return { accounts, domains, aliases };
  }, []);

  const state = useResource<MailboxData>(load, 'Could not load mailboxes.');

  const [search, setSearch] = useState('');
  const [actionError, setActionError] = useState('');
  const [busy, setBusy] = useState(false);

  const [addOpen, setAddOpen] = useState(false);
  const [addForm, setAddForm] = useState({ domain_id: '', local: '', password: '', quota_mb: '1024' });

  const [editing, setEditing] = useState<MailAccount | null>(null);
  const [editForm, setEditForm] = useState({
    quota_mb: '1024',
    is_active: true,
    forward_to: '',
    auto_reply: false,
    auto_reply_msg: '',
  });

  const [batchOpen, setBatchOpen] = useState(false);
  const [batchForm, setBatchForm] = useState({
    domain_id: '',
    prefix: 'user',
    start: '1',
    count: '10',
    quota_mb: '1024',
    password: '',
  });

  const [importOpen, setImportOpen] = useState(false);
  const [importDomain, setImportDomain] = useState('');
  const [importText, setImportText] = useState('');
  const fileInput = useRef<HTMLInputElement | null>(null);

  const [bulkResults, setBulkResults] = useState<BulkResult[] | null>(null);
  const [bulkProgress, setBulkProgress] = useState<{ done: number; total: number } | null>(null);

  const domains = useMemo(() => state.data?.domains ?? [], [state.data]);
  const accounts = useMemo(() => state.data?.accounts ?? [], [state.data]);

  const domainName = useCallback(
    (id: string) => domains.find((d) => d.id === id)?.domain ?? null,
    [domains]
  );

  /** Aliases that deliver INTO a mailbox - "who else arrives here". */
  const aliasesFor = useMemo(() => {
    const byTarget = new Map<string, string[]>();
    for (const alias of state.data?.aliases ?? []) {
      const key = alias.destination.trim().toLowerCase();
      const bucket = byTarget.get(key);
      if (bucket) bucket.push(alias.source);
      else byTarget.set(key, [alias.source]);
    }
    return byTarget;
  }, [state.data]);

  const rows = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return accounts;
    return accounts.filter((a) => a.email.toLowerCase().includes(term));
  }, [accounts, search]);

  const resetBulk = () => {
    setBulkResults(null);
    setBulkProgress(null);
  };

  /* ---------------------------------------------------------------- *
   * Single mailbox
   * ---------------------------------------------------------------- */

  const openAdd = () => {
    setAddForm({
      domain_id: domains[0]?.id ?? '',
      local: '',
      password: '',
      quota_mb: '1024',
    });
    setActionError('');
    setAddOpen(true);
  };

  const submitAdd = async () => {
    const local = addForm.local.trim().toLowerCase();
    const domain = domainName(addForm.domain_id);
    if (!addForm.domain_id || !domain) {
      setActionError('Choose the domain this mailbox belongs to.');
      return;
    }
    if (!local) {
      setActionError('Enter the part of the address before the @.');
      return;
    }
    if (addForm.password.length < 8) {
      setActionError('Use a password of at least 8 characters.');
      return;
    }
    setBusy(true);
    setActionError('');
    try {
      await mailServerApi.createAccount({
        domain_id: addForm.domain_id,
        email: `${local}@${domain}`,
        password: addForm.password,
        quota_mb: Number(addForm.quota_mb) || 0,
      });
      setAddOpen(false);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not create the mailbox.'));
    } finally {
      setBusy(false);
    }
  };

  const openEdit = (account: MailAccount) => {
    setEditing(account);
    setEditForm({
      quota_mb: String(account.quota_mb ?? 0),
      is_active: account.is_active,
      forward_to: account.forward_to ?? '',
      auto_reply: account.auto_reply,
      auto_reply_msg: account.auto_reply_msg ?? '',
    });
    setActionError('');
  };

  const submitEdit = async () => {
    if (!editing) return;
    setBusy(true);
    setActionError('');
    try {
      await mailServerApi.updateAccount(editing.id, {
        quota_mb: Number(editForm.quota_mb) || 0,
        is_active: editForm.is_active,
        forward_to: editForm.forward_to.trim(),
        auto_reply: editForm.auto_reply,
        auto_reply_msg: editForm.auto_reply_msg,
      });
      setEditing(null);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not save the mailbox.'));
    } finally {
      setBusy(false);
    }
  };

  const removeAccount = async (account: MailAccount) => {
    if (!window.confirm(`Delete ${account.email}? Stored mail for this address goes with it.`)) {
      return;
    }
    setActionError('');
    try {
      await mailServerApi.deleteAccount(account.id);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not delete the mailbox.'));
    }
  };

  /* ---------------------------------------------------------------- *
   * Bulk: one real POST per row, reported row by row
   * ---------------------------------------------------------------- */

  const runBulk = async (
    entries: { email: string; password: string; quota_mb: number }[],
    domainId: string
  ) => {
    setBusy(true);
    resetBulk();
    setBulkProgress({ done: 0, total: entries.length });
    const results: BulkResult[] = [];
    for (let i = 0; i < entries.length; i += 1) {
      const entry = entries[i];
      try {
        await mailServerApi.createAccount({
          domain_id: domainId,
          email: entry.email,
          password: entry.password,
          quota_mb: entry.quota_mb,
        });
        results.push({ email: entry.email, ok: true, message: 'Created' });
      } catch (err) {
        results.push({
          email: entry.email,
          ok: false,
          message: errorMessage(err, 'The panel refused this row.'),
        });
      }
      setBulkProgress({ done: i + 1, total: entries.length });
      setBulkResults([...results]);
    }
    setBusy(false);
    state.reload();
  };

  const openBatch = () => {
    setBatchForm({
      domain_id: domains[0]?.id ?? '',
      prefix: 'user',
      start: '1',
      count: '10',
      quota_mb: '1024',
      password: '',
    });
    setActionError('');
    resetBulk();
    setBatchOpen(true);
  };

  const submitBatch = async () => {
    const domain = domainName(batchForm.domain_id);
    const count = Number(batchForm.count);
    const start = Number(batchForm.start);
    if (!domain) {
      setActionError('Choose the domain the mailboxes belong to.');
      return;
    }
    if (!batchForm.prefix.trim()) {
      setActionError('Enter a name prefix, for example sales.');
      return;
    }
    if (!Number.isFinite(count) || count < 1 || count > 200) {
      setActionError('Create between 1 and 200 mailboxes in one run.');
      return;
    }
    if (!Number.isFinite(start) || start < 0) {
      setActionError('The first number must be 0 or more.');
      return;
    }
    if (batchForm.password && batchForm.password.length < 8) {
      setActionError('Use a password of at least 8 characters, or leave it empty for random ones.');
      return;
    }
    const quota = Number(batchForm.quota_mb) || 0;
    const entries = Array.from({ length: count }, (_, i) => ({
      email: `${batchForm.prefix.trim().toLowerCase()}${start + i}@${domain}`,
      password: batchForm.password || randomPassword(),
      quota_mb: quota,
    }));
    setActionError('');
    await runBulk(entries, batchForm.domain_id);
  };

  const openImport = () => {
    setImportDomain(domains[0]?.id ?? '');
    setImportText('');
    setActionError('');
    resetBulk();
    setImportOpen(true);
  };

  const readFile = (file: File) => {
    const reader = new FileReader();
    reader.onload = () => setImportText(String(reader.result ?? ''));
    reader.onerror = () => setActionError('That file could not be read.');
    reader.readAsText(file);
  };

  const submitImport = async () => {
    const domain = domainName(importDomain);
    if (!domain) {
      setActionError('Choose the domain the imported mailboxes belong to.');
      return;
    }
    const lines = importText
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter((line) => line && !line.toLowerCase().startsWith('email,'));
    if (lines.length === 0) {
      setActionError('Nothing to import. One row per line: address, password, quota in MB.');
      return;
    }
    if (lines.length > 500) {
      setActionError('Import at most 500 rows in one run.');
      return;
    }
    const entries: { email: string; password: string; quota_mb: number }[] = [];
    for (const line of lines) {
      const parts = line.split(',').map((p) => p.trim());
      const address = parts[0]?.toLowerCase() ?? '';
      if (!address) continue;
      const email = address.includes('@') ? address : `${address}@${domain}`;
      entries.push({
        email,
        password: parts[1] && parts[1].length >= 8 ? parts[1] : randomPassword(),
        quota_mb: Number(parts[2]) || 1024,
      });
    }
    if (entries.length === 0) {
      setActionError('No usable rows found. One row per line: address, password, quota in MB.');
      return;
    }
    setActionError('');
    await runBulk(entries, importDomain);
  };

  const exportCsv = () => {
    const header = ['email', 'domain', 'quota_mb', 'used_mb', 'active', 'forward_to', 'last_login_at'];
    const lines = [header.join(',')];
    for (const account of rows) {
      lines.push(
        [
          csvCell(account.email),
          csvCell(domainName(account.domain_id) ?? ''),
          String(account.quota_mb ?? 0),
          String(account.used_mb ?? 0),
          account.is_active ? 'yes' : 'no',
          csvCell(account.forward_to ?? ''),
          csvCell(account.last_login_at ?? ''),
        ].join(',')
      );
    }
    const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `mailboxes-${new Date().toISOString().slice(0, 10)}.csv`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  };

  const noDomains = !state.loading && !state.error && domains.length === 0;

  return (
    <div className="space-y-4">
      <Panel>
        <SectionHeader
          title="Mailboxes"
          description="Every address this server delivers to, with what it is allowed to hold and what it is using."
        />

        <Toolbar
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search mailboxes"
          onRefresh={state.reload}
          refreshing={state.refreshing}
        >
          <Button type="button" onClick={openAdd} disabled={domains.length === 0}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            Add Mailbox
          </Button>
          <Button
            type="button"
            variant="secondary"
            onClick={openBatch}
            disabled={domains.length === 0}
          >
            <Users className="h-4 w-4" aria-hidden="true" />
            Batch Create
          </Button>
          <Button
            type="button"
            variant="secondary"
            onClick={openImport}
            disabled={domains.length === 0}
          >
            <Upload className="h-4 w-4" aria-hidden="true" />
            Import
          </Button>
          <Button
            type="button"
            variant="secondary"
            onClick={exportCsv}
            disabled={rows.length === 0}
          >
            <Download className="h-4 w-4" aria-hidden="true" />
            Export
          </Button>
        </Toolbar>

        {actionError && !addOpen && !editing && !batchOpen && !importOpen && (
          <div className="px-4 pt-3">
            <ActionError message={actionError} onDismiss={() => setActionError('')} />
          </div>
        )}

        {state.loading && <TableSkeleton columns={7} rows={5} />}

        {!state.loading && state.error && (
          <ErrorBlock
            title="Could not load mailboxes"
            message={state.error}
            onRetry={state.reload}
          />
        )}

        {noDomains && (
          <EmptyState
            title="Add a mail domain first"
            description="A mailbox belongs to a domain. Add one on the Mail Domain tab, publish its records, then create addresses under it."
          />
        )}

        {!state.loading && !state.error && domains.length > 0 && rows.length === 0 && (
          <EmptyState
            title={search ? 'No mailbox matches that search' : 'No mailbox yet'}
            description={
              search
                ? 'Clear the search to see every mailbox on this server.'
                : 'Create the first address under one of your mail domains. Batch Create makes a numbered run of them in one pass.'
            }
            action={
              !search && (
                <Button type="button" onClick={openAdd}>
                  <Plus className="h-4 w-4" aria-hidden="true" />
                  Add Mailbox
                </Button>
              )
            }
          />
        )}

        {!state.loading && !state.error && rows.length > 0 && (
          <TableScroller>
            <table className="w-full min-w-[1000px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH} scope="col">
                    Mailbox
                  </th>
                  <th className={TH} scope="col">
                    Quota and use
                  </th>
                  <th className={TH} scope="col">
                    Aliases
                  </th>
                  <th className={TH} scope="col">
                    Forwarding
                  </th>
                  <th className={TH} scope="col">
                    Auto responder
                  </th>
                  <th className={TH} scope="col">
                    Last login
                  </th>
                  <th className={TH} scope="col">
                    State
                  </th>
                  <th className={TH} scope="col">
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((account) => {
                  const pct = usagePercent(account.used_mb ?? 0, account.quota_mb ?? 0);
                  const aliasList = aliasesFor.get(account.email.toLowerCase()) ?? [];
                  const lastLogin = formatDateTime(account.last_login_at);
                  return (
                    <tr key={account.id} className={ROW}>
                      <td className={TD}>
                        <span className="font-medium text-gray-900">{account.email}</span>
                      </td>
                      <td className={TD}>
                        <div className="min-w-[160px]">
                          <p className="text-sm text-gray-700">
                            {formatMB(account.used_mb ?? 0)} of {quotaLabel(account.quota_mb ?? 0)}
                          </p>
                          {pct === null ? (
                            <p className="mt-1 text-xs text-gray-500">No quota set</p>
                          ) : (
                            <div
                              className="mt-1 h-1.5 w-full overflow-hidden rounded bg-gray-100"
                              role="img"
                              aria-label={`${pct}% of quota used`}
                            >
                              <div
                                className={
                                  pct >= 90
                                    ? 'h-full rounded bg-red-500'
                                    : pct >= 75
                                      ? 'h-full rounded bg-amber-500'
                                      : 'h-full rounded bg-emerald-500'
                                }
                                style={{ width: `${pct}%` }}
                              />
                            </div>
                          )}
                        </div>
                      </td>
                      <td className={TD}>
                        {aliasList.length === 0 ? (
                          <span className="text-gray-500">None</span>
                        ) : (
                          <div className="space-y-0.5">
                            {aliasList.slice(0, 3).map((source) => (
                              <p key={source} className="font-mono text-xs text-gray-700">
                                {source}
                              </p>
                            ))}
                            {aliasList.length > 3 && (
                              <p className="text-xs text-gray-500">
                                and {aliasList.length - 3} more
                              </p>
                            )}
                          </div>
                        )}
                      </td>
                      <td className={TD}>
                        {account.forward_to ? (
                          <span className="font-mono text-xs text-gray-700">
                            {account.forward_to}
                          </span>
                        ) : (
                          <span className="text-gray-500">Off</span>
                        )}
                      </td>
                      <td className={TD}>
                        {account.auto_reply ? <Pill tone="sky">On</Pill> : <span className="text-gray-500">Off</span>}
                      </td>
                      <td className={TD}>
                        {lastLogin ?? (
                          <Dash reason="This mailbox has never signed in, or the server has not reported a login." />
                        )}
                      </td>
                      <td className={TD}>
                        {account.is_active ? (
                          <Pill tone="emerald">Active</Pill>
                        ) : (
                          <Pill tone="gray">Suspended</Pill>
                        )}
                      </td>
                      <td className={TD}>
                        <div className="flex items-center gap-2">
                          <Button
                            type="button"
                            variant="secondary"
                            size="sm"
                            onClick={() => openEdit(account)}
                            aria-label={`Edit ${account.email}`}
                          >
                            <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
                            Edit
                          </Button>
                          <Button
                            type="button"
                            variant="danger-outline"
                            size="sm"
                            onClick={() => removeAccount(account)}
                            aria-label={`Delete ${account.email}`}
                          >
                            <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                            Delete
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>

      {/* Add ---------------------------------------------------------- */}
      <Modal
        open={addOpen}
        title="Add mailbox"
        onClose={() => setAddOpen(false)}
        footer={
          <>
            <Button type="button" variant="secondary" onClick={() => setAddOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={submitAdd} disabled={busy}>
              {busy ? 'Creating…' : 'Create'}
            </Button>
          </>
        }
      >
        <ActionError message={actionError} onDismiss={() => setActionError('')} />
        <Field label="Domain" htmlFor="add-domain">
          <select
            id="add-domain"
            className={SELECT_CLASS}
            value={addForm.domain_id}
            onChange={(e) => setAddForm({ ...addForm, domain_id: e.target.value })}
          >
            {domains.map((d) => (
              <option key={d.id} value={d.id}>
                {d.domain}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Address" htmlFor="add-local" hint="The part before the @.">
          <div className="flex items-center gap-2">
            <Input
              id="add-local"
              value={addForm.local}
              onChange={(e) => setAddForm({ ...addForm, local: e.target.value })}
              placeholder="sales"
              autoComplete="off"
            />
            <span className="whitespace-nowrap text-sm text-gray-500">
              @{domainName(addForm.domain_id) ?? '…'}
            </span>
          </div>
        </Field>
        <Field
          label="Password"
          htmlFor="add-password"
          hint="At least 8 characters. It is stored by the server; the panel cannot show it again."
        >
          <div className="flex items-center gap-2">
            <Input
              id="add-password"
              type="text"
              value={addForm.password}
              onChange={(e) => setAddForm({ ...addForm, password: e.target.value })}
              autoComplete="new-password"
            />
            <Button
              type="button"
              variant="secondary"
              onClick={() => setAddForm({ ...addForm, password: randomPassword() })}
            >
              Generate
            </Button>
          </div>
        </Field>
        <Field label="Quota (MB)" htmlFor="add-quota" hint="0 means no limit.">
          <Input
            id="add-quota"
            type="number"
            min={0}
            value={addForm.quota_mb}
            onChange={(e) => setAddForm({ ...addForm, quota_mb: e.target.value })}
          />
        </Field>
      </Modal>

      {/* Edit --------------------------------------------------------- */}
      <Modal
        open={editing !== null}
        title={editing ? `Edit ${editing.email}` : 'Edit mailbox'}
        onClose={() => setEditing(null)}
        footer={
          <>
            <Button type="button" variant="secondary" onClick={() => setEditing(null)}>
              Cancel
            </Button>
            <Button type="button" onClick={submitEdit} disabled={busy}>
              {busy ? 'Saving…' : 'Save'}
            </Button>
          </>
        }
      >
        <ActionError message={actionError} onDismiss={() => setActionError('')} />
        <Field label="Quota (MB)" htmlFor="edit-quota" hint="0 means no limit.">
          <Input
            id="edit-quota"
            type="number"
            min={0}
            value={editForm.quota_mb}
            onChange={(e) => setEditForm({ ...editForm, quota_mb: e.target.value })}
          />
        </Field>
        <Field
          label="Forward to"
          htmlFor="edit-forward"
          hint="Leave empty to stop forwarding. Mail is still delivered to this mailbox as well."
        >
          <Input
            id="edit-forward"
            value={editForm.forward_to}
            onChange={(e) => setEditForm({ ...editForm, forward_to: e.target.value })}
            placeholder="someone@example.vn"
            autoComplete="off"
          />
        </Field>
        <label className="flex items-start gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
            checked={editForm.is_active}
            onChange={(e) => setEditForm({ ...editForm, is_active: e.target.checked })}
          />
          <span>
            Active
            <span className="block text-xs text-gray-500">
              A suspended mailbox keeps its mail but cannot sign in.
            </span>
          </span>
        </label>
        <label className="flex items-start gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
            checked={editForm.auto_reply}
            onChange={(e) => setEditForm({ ...editForm, auto_reply: e.target.checked })}
          />
          <span>Auto responder</span>
        </label>
        {editForm.auto_reply && (
          <Field label="Auto reply message" htmlFor="edit-reply">
            <textarea
              id="edit-reply"
              rows={4}
              className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
              value={editForm.auto_reply_msg}
              onChange={(e) => setEditForm({ ...editForm, auto_reply_msg: e.target.value })}
              placeholder="I am away until 3 March. For anything urgent, write to support@example.vn."
            />
          </Field>
        )}
      </Modal>

      {/* Batch -------------------------------------------------------- */}
      <Modal
        open={batchOpen}
        wide
        title="Batch create mailboxes"
        description="A numbered run of addresses under one domain."
        onClose={() => {
          setBatchOpen(false);
          resetBulk();
        }}
        footer={
          <>
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                setBatchOpen(false);
                resetBulk();
              }}
            >
              Close
            </Button>
            <Button type="button" onClick={submitBatch} disabled={busy}>
              {busy ? 'Creating…' : 'Create mailboxes'}
            </Button>
          </>
        }
      >
        <ActionError message={actionError} onDismiss={() => setActionError('')} />
        <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
          There is no bulk endpoint on the backend yet, so this creates the mailboxes one at a time.
          If a row fails the run carries on and the ones already made stay. Every row&rsquo;s result
          is listed below when it finishes.
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Domain" htmlFor="batch-domain">
            <select
              id="batch-domain"
              className={SELECT_CLASS}
              value={batchForm.domain_id}
              onChange={(e) => setBatchForm({ ...batchForm, domain_id: e.target.value })}
            >
              {domains.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.domain}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Name prefix" htmlFor="batch-prefix" hint="Addresses become prefix1, prefix2, …">
            <Input
              id="batch-prefix"
              value={batchForm.prefix}
              onChange={(e) => setBatchForm({ ...batchForm, prefix: e.target.value })}
            />
          </Field>
          <Field label="First number" htmlFor="batch-start">
            <Input
              id="batch-start"
              type="number"
              min={0}
              value={batchForm.start}
              onChange={(e) => setBatchForm({ ...batchForm, start: e.target.value })}
            />
          </Field>
          <Field label="How many" htmlFor="batch-count" hint="Up to 200 per run.">
            <Input
              id="batch-count"
              type="number"
              min={1}
              max={200}
              value={batchForm.count}
              onChange={(e) => setBatchForm({ ...batchForm, count: e.target.value })}
            />
          </Field>
          <Field label="Quota (MB)" htmlFor="batch-quota" hint="0 means no limit.">
            <Input
              id="batch-quota"
              type="number"
              min={0}
              value={batchForm.quota_mb}
              onChange={(e) => setBatchForm({ ...batchForm, quota_mb: e.target.value })}
            />
          </Field>
          <Field
            label="Password"
            htmlFor="batch-password"
            hint="Leave empty to give each mailbox a different random password."
          >
            <Input
              id="batch-password"
              type="text"
              value={batchForm.password}
              onChange={(e) => setBatchForm({ ...batchForm, password: e.target.value })}
              autoComplete="new-password"
            />
          </Field>
        </div>
        <BulkReport progress={bulkProgress} results={bulkResults} />
      </Modal>

      {/* Import ------------------------------------------------------- */}
      <Modal
        open={importOpen}
        wide
        title="Import mailboxes"
        description="One address per line: address, password, quota in MB."
        onClose={() => {
          setImportOpen(false);
          resetBulk();
        }}
        footer={
          <>
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                setImportOpen(false);
                resetBulk();
              }}
            >
              Close
            </Button>
            <Button type="button" onClick={submitImport} disabled={busy}>
              {busy ? 'Importing…' : 'Import'}
            </Button>
          </>
        }
      >
        <ActionError message={actionError} onDismiss={() => setActionError('')} />
        <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
          The file is read in your browser and each row is created through the normal mailbox
          endpoint, one at a time. Nothing is uploaded. A row with no password, or one shorter than 8
          characters, gets a random one.
        </div>
        <Field label="Domain" htmlFor="import-domain" hint="Used for rows written without an @.">
          <select
            id="import-domain"
            className={SELECT_CLASS}
            value={importDomain}
            onChange={(e) => setImportDomain(e.target.value)}
          >
            {domains.map((d) => (
              <option key={d.id} value={d.id}>
                {d.domain}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Rows" htmlFor="import-text">
          <textarea
            id="import-text"
            rows={8}
            className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 font-mono text-xs text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            value={importText}
            onChange={(e) => setImportText(e.target.value)}
            placeholder={'sales@example.vn,Str0ngPass!,2048\nsupport,An0therPass!,1024'}
          />
        </Field>
        <div>
          <input
            ref={fileInput}
            type="file"
            accept=".csv,.txt,text/csv,text/plain"
            className="hidden"
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) readFile(file);
              e.target.value = '';
            }}
          />
          <Button type="button" variant="secondary" onClick={() => fileInput.current?.click()}>
            <Upload className="h-4 w-4" aria-hidden="true" />
            Choose a CSV file
          </Button>
        </div>
        <BulkReport progress={bulkProgress} results={bulkResults} />
      </Modal>
    </div>
  );
}

/** Progress and the per-row outcome of a bulk run. */
function BulkReport({
  progress,
  results,
}: {
  progress: { done: number; total: number } | null;
  results: BulkResult[] | null;
}) {
  if (!progress && !results) return null;
  const failed = (results ?? []).filter((r) => !r.ok);
  return (
    <div className="rounded-md border border-gray-200">
      <div className="border-b border-gray-200 bg-gray-50 px-3 py-2">
        <p className="text-sm font-medium text-gray-900">
          {progress ? `${progress.done} of ${progress.total} processed` : 'Finished'}
          {failed.length > 0 && (
            <span className="ml-2 text-red-700">
              {failed.length} failed
            </span>
          )}
        </p>
      </div>
      <div className="max-h-56 overflow-y-auto">
        <table className="w-full border-collapse">
          <tbody>
            {(results ?? []).map((row) => (
              <tr key={row.email} className="border-b border-gray-100 last:border-b-0">
                <td className="px-3 py-1.5 font-mono text-xs text-gray-700">{row.email}</td>
                <td className="px-3 py-1.5 text-xs">
                  {row.ok ? (
                    <span className="text-emerald-700">{row.message}</span>
                  ) : (
                    <span className="text-red-700">{row.message}</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
