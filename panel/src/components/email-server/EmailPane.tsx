'use client';

/**
 * Email: what is moving through the server right now.
 *
 * Only one of the four sub-tabs has a backend behind it. mail_queue is real, so
 * Outbox is a real screen: queue depth, retries, and - the part that matters -
 * the failure reason in words rather than an SMTP code on its own. An operator
 * reading "550 5.1.1" cannot tell a typo from a blacklisting, and those need
 * opposite responses, so the code stays (it is what a mail admin searches for)
 * and a sentence is added next to it.
 *
 * Inbox, Spam quarantine and Sender have no routes at all. They are drawn as
 * named gaps: no message list that is always empty, no compose form that posts
 * into nothing.
 */

import { useCallback, useMemo, useState } from 'react';
import { Trash2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { errorMessage } from '@/lib/apiError';

import { mailServerApi } from './api';
import {
  ActionError,
  BackendGap,
  BlockSkeleton,
  Dash,
  EmptyState,
  ErrorBlock,
  Field,
  Notice,
  Panel,
  Pill,
  ROW,
  SectionHeader,
  StatTile,
  TableScroller,
  TableSkeleton,
  TD,
  TH,
  Toolbar,
} from './chrome';
import { explainMailFailure, formatDateTime, queueStatusTone } from './format';
import { useResource } from './useResource';
import type { MailQueueItem, MailSpamFilter, MailStats } from './types';

export default function EmailPane({ sub }: { sub: string }) {
  if (sub === 'outbox') return <OutboxPane />;
  if (sub === 'spam') return <SpamPane />;
  if (sub === 'sender') return <SenderPane />;
  return <InboxPane />;
}

/* ------------------------------------------------------------------ *
 * Inbox
 * ------------------------------------------------------------------ */

function InboxPane() {
  return (
    <Panel>
      <SectionHeader
        title="Inbox"
        description="Messages delivered to the mailboxes on this server."
      />
      <BackendGap
        title="The panel cannot read delivered mail yet"
        description="Nothing on the backend indexes or serves stored messages, so there is no list to show. Mailbox owners can still read their mail over IMAP; only this view is missing. Until the routes below exist, this tab would show an empty list no matter how much mail had arrived, which is why it shows nothing at all instead."
        missing={[
          'GET  /api/v1/mail-server/messages?mailbox=:id&folder=inbox',
          'GET  /api/v1/mail-server/messages/:id',
          'POST /api/v1/mail-server/messages/:id/spam',
          'DELETE /api/v1/mail-server/messages/:id',
        ]}
      />
    </Panel>
  );
}

/* ------------------------------------------------------------------ *
 * Outbox - the real one
 * ------------------------------------------------------------------ */

interface OutboxData {
  queue: MailQueueItem[];
  stats: MailStats | null;
}

function OutboxPane() {
  const load = useCallback(async (): Promise<OutboxData> => {
    const [queue, stats] = await Promise.all([
      mailServerApi.listQueue(),
      mailServerApi.stats().catch(() => null),
    ]);
    return { queue, stats };
  }, []);

  const state = useResource<OutboxData>(load, 'Could not load the mail queue.');
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [actionError, setActionError] = useState('');
  const [busy, setBusy] = useState(false);

  const queue = useMemo(() => state.data?.queue ?? [], [state.data]);
  const stats = state.data?.stats ?? null;

  const counts = useMemo(() => {
    const out = { queued: 0, deferred: 0, failed: 0, sent: 0 };
    for (const item of queue) {
      const key = (item.status || '').toLowerCase();
      if (key in out) out[key as keyof typeof out] += 1;
    }
    return out;
  }, [queue]);

  const rows = useMemo(() => {
    const term = search.trim().toLowerCase();
    return queue.filter((item) => {
      if (statusFilter !== 'all' && (item.status || '').toLowerCase() !== statusFilter) return false;
      if (!term) return true;
      return (
        item.to.toLowerCase().includes(term) ||
        item.from.toLowerCase().includes(term) ||
        (item.subject ?? '').toLowerCase().includes(term)
      );
    });
  }, [queue, search, statusFilter]);

  const removeItem = async (item: MailQueueItem) => {
    if (!window.confirm(`Drop the queued message to ${item.to}? It will never be delivered.`)) return;
    setActionError('');
    try {
      await mailServerApi.deleteQueueItem(item.id);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not remove that message from the queue.'));
    }
  };

  const clearFailed = async () => {
    if (
      !window.confirm(
        'Delete every failed message from the queue? They are removed, not retried, and the failure reasons go with them.'
      )
    ) {
      return;
    }
    setBusy(true);
    setActionError('');
    try {
      await mailServerApi.flushFailedQueue();
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not clear the failed messages.'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatTile
          label="Waiting in queue"
          value={stats ? stats.queue_size : counts.queued}
          hint="Accepted and not yet delivered."
        />
        <StatTile
          label="Deferred"
          value={counts.deferred}
          hint="The receiver asked to be retried later."
        />
        <StatTile
          label="Failed today"
          value={stats ? stats.failed_today : counts.failed}
          hint="Gave up. Each row below says why."
        />
        <StatTile
          label="Sent today"
          value={stats ? stats.sent_today : counts.sent}
          hint="Handed to the receiving server."
        />
      </div>

      <Panel>
        <SectionHeader
          title="Outbox"
          description="The send queue, newest first. The backend returns the most recent 100 rows."
        />

        <Toolbar
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search by address or subject"
          onRefresh={state.reload}
          refreshing={state.refreshing}
        >
          <select
            className="h-9 rounded-md border border-gray-300 bg-white px-3 text-sm text-gray-900 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            aria-label="Filter by status"
          >
            <option value="all">All statuses</option>
            <option value="queued">Queued</option>
            <option value="deferred">Deferred</option>
            <option value="failed">Failed</option>
            <option value="sent">Sent</option>
          </select>
          <Button
            type="button"
            variant="danger-outline"
            onClick={clearFailed}
            disabled={busy || counts.failed === 0}
          >
            <Trash2 className="h-4 w-4" aria-hidden="true" />
            Delete failed
          </Button>
        </Toolbar>

        {actionError && (
          <div className="px-4 pt-3">
            <ActionError message={actionError} onDismiss={() => setActionError('')} />
          </div>
        )}

        {state.loading && <TableSkeleton columns={6} rows={5} />}

        {!state.loading && state.error && (
          <ErrorBlock
            title="Could not load the mail queue"
            message={state.error}
            onRetry={state.reload}
          />
        )}

        {!state.loading && !state.error && rows.length === 0 && (
          <EmptyState
            title={queue.length === 0 ? 'The queue is empty' : 'Nothing matches that filter'}
            description={
              queue.length === 0
                ? 'Every message this server accepted has been delivered or dropped. Failures would appear here with the reason the receiving server gave.'
                : 'Widen the status filter or clear the search to see the rest of the queue.'
            }
          />
        )}

        {!state.loading && !state.error && rows.length > 0 && (
          <TableScroller>
            <table className="w-full min-w-[1000px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH} scope="col">
                    Recipient
                  </th>
                  <th className={TH} scope="col">
                    Sender
                  </th>
                  <th className={TH} scope="col">
                    Subject
                  </th>
                  <th className={TH} scope="col">
                    Status
                  </th>
                  <th className={TH} scope="col">
                    Tries
                  </th>
                  <th className={TH} scope="col">
                    Queued
                  </th>
                  <th className={TH} scope="col">
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((item) => {
                  const plain = explainMailFailure(item.last_error);
                  const queued = formatDateTime(item.created_at);
                  const sent = formatDateTime(item.sent_at);
                  return (
                    <tr key={item.id} className={ROW}>
                      <td className={TD}>
                        <span className="font-medium text-gray-900">{item.to}</span>
                      </td>
                      <td className={TD}>{item.from}</td>
                      <td className={TD}>
                        <span className="block max-w-xs truncate" title={item.subject}>
                          {item.subject || <Dash reason="This message carried no subject." />}
                        </span>
                        {item.last_error && (
                          <div className="mt-1.5 max-w-md rounded-md border border-red-200 bg-red-50 px-2 py-1.5">
                            {plain && <p className="text-xs text-red-800">{plain}</p>}
                            <p className="mt-0.5 break-words font-mono text-[11px] text-red-700">
                              {item.last_error}
                            </p>
                          </div>
                        )}
                      </td>
                      <td className={TD}>
                        <Pill tone={queueStatusTone(item.status)}>{item.status || 'unknown'}</Pill>
                      </td>
                      <td className={TD}>{item.retry_count ?? 0}</td>
                      <td className={TD}>
                        {queued ?? <Dash reason="The backend did not report a queue time." />}
                        {sent && <p className="mt-0.5 text-xs text-gray-500">Sent {sent}</p>}
                      </td>
                      <td className={TD}>
                        <Button
                          type="button"
                          variant="danger-outline"
                          size="sm"
                          onClick={() => removeItem(item)}
                          aria-label={`Remove the message to ${item.to}`}
                        >
                          <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                          Remove
                        </Button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>
    </div>
  );
}

/* ------------------------------------------------------------------ *
 * Spam
 * ------------------------------------------------------------------ */

function SpamPane() {
  const load = useCallback(() => mailServerApi.getSpamFilter(), []);
  const state = useResource<MailSpamFilter | null>(load, 'Could not load the spam filter settings.');

  const [form, setForm] = useState<{
    enabled: boolean;
    spam_threshold: string;
    reject_score: string;
    greylisting: boolean;
    blacklist: string;
    whitelist: string;
  } | null>(null);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState('');
  const [saved, setSaved] = useState(false);

  // Seed the form from the loaded record exactly once per load.
  const filter = state.data;
  const seedKey = filter ? `${filter.id}:${filter.updated_at}` : '';
  const [seededFor, setSeededFor] = useState('');
  if (filter && seedKey !== seededFor) {
    setSeededFor(seedKey);
    setForm({
      enabled: filter.enabled,
      spam_threshold: String(filter.spam_threshold ?? 5),
      reject_score: String(filter.reject_score ?? 15),
      greylisting: filter.greylisting,
      blacklist: (filter.blacklist ?? []).join('\n'),
      whitelist: (filter.whitelist ?? []).join('\n'),
    });
  }

  const save = async () => {
    if (!form) return;
    const threshold = Number(form.spam_threshold);
    const reject = Number(form.reject_score);
    if (!Number.isFinite(threshold) || !Number.isFinite(reject)) {
      setActionError('Both scores have to be numbers.');
      return;
    }
    if (reject < threshold) {
      setActionError(
        'The reject score has to be at least the spam score, otherwise mail is rejected before it is even marked.'
      );
      return;
    }
    setBusy(true);
    setActionError('');
    setSaved(false);
    try {
      await mailServerApi.updateSpamFilter({
        enabled: form.enabled,
        spam_threshold: threshold,
        reject_score: reject,
        greylisting: form.greylisting,
        blacklist: form.blacklist.split('\n').map((s) => s.trim()).filter(Boolean),
        whitelist: form.whitelist.split('\n').map((s) => s.trim()).filter(Boolean),
      });
      setSaved(true);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not save the spam filter settings.'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      <Panel>
        <SectionHeader
          title="Spam filter"
          description="How hard the server filters incoming mail, and the addresses that skip the decision entirely."
          actions={
            <Button type="button" onClick={save} disabled={busy || !form}>
              {busy ? 'Saving…' : 'Save settings'}
            </Button>
          }
        />

        {state.loading && <BlockSkeleton lines={5} />}

        {!state.loading && state.error && (
          <ErrorBlock
            title="Could not load the spam filter"
            message={state.error}
            onRetry={state.reload}
          />
        )}

        {!state.loading && !state.error && !form && (
          <EmptyState
            title="No spam filter record yet"
            description="The backend has not created a spam filter row for this tenant. Reload once the mail server has been set up."
          />
        )}

        {!state.loading && !state.error && form && (
          <div className="space-y-4 px-4 py-4">
            <ActionError message={actionError} onDismiss={() => setActionError('')} />
            {saved && (
              <p className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
                Settings saved.
              </p>
            )}

            <label className="flex items-start gap-2 text-sm text-gray-700">
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                checked={form.enabled}
                onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
              />
              <span>
                Filter incoming mail
                <span className="block text-xs text-gray-500">
                  With this off, nothing is scored and everything is delivered.
                </span>
              </span>
            </label>

            <div className="grid gap-4 sm:grid-cols-2">
              <Field
                label="Mark as spam at"
                htmlFor="spam-threshold"
                hint="A message scoring at or above this is delivered but flagged. 5 is the usual starting point."
              >
                <Input
                  id="spam-threshold"
                  type="number"
                  step="0.1"
                  value={form.spam_threshold}
                  onChange={(e) => setForm({ ...form, spam_threshold: e.target.value })}
                />
              </Field>
              <Field
                label="Reject at"
                htmlFor="reject-score"
                hint="At or above this the message is refused at the door. Set it well clear of the spam score; a low value loses real mail silently."
              >
                <Input
                  id="reject-score"
                  type="number"
                  step="0.1"
                  value={form.reject_score}
                  onChange={(e) => setForm({ ...form, reject_score: e.target.value })}
                />
              </Field>
            </div>

            <label className="flex items-start gap-2 text-sm text-gray-700">
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                checked={form.greylisting}
                onChange={(e) => setForm({ ...form, greylisting: e.target.checked })}
              />
              <span>
                Greylisting
                <span className="block text-xs text-gray-500">
                  Defer the first attempt from an unknown sender. Stops a lot of spam and delays
                  genuine first messages by a few minutes.
                </span>
              </span>
            </label>

            <div className="grid gap-4 sm:grid-cols-2">
              <Field
                label="Blocklist"
                htmlFor="spam-blacklist"
                hint="One address or domain per line. These are refused whatever they score."
              >
                <textarea
                  id="spam-blacklist"
                  rows={6}
                  className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 font-mono text-xs text-gray-900 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
                  value={form.blacklist}
                  onChange={(e) => setForm({ ...form, blacklist: e.target.value })}
                  placeholder={'spammer@example.com\nbad-domain.example'}
                />
              </Field>
              <Field
                label="Allowlist"
                htmlFor="spam-whitelist"
                hint="One address or domain per line. These skip filtering, so keep it short."
              >
                <textarea
                  id="spam-whitelist"
                  rows={6}
                  className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 font-mono text-xs text-gray-900 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
                  value={form.whitelist}
                  onChange={(e) => setForm({ ...form, whitelist: e.target.value })}
                  placeholder={'billing@partner.example\npartner.example'}
                />
              </Field>
            </div>
          </div>
        )}
      </Panel>

      <Panel>
        <SectionHeader
          title="Quarantined messages"
          description="The mail this filter caught, so a false positive can be found and released."
        />
        <BackendGap
          title="Caught messages are not listed anywhere"
          description="The filter's settings are stored and editable above, but nothing records or serves the messages it acts on. Until that exists, a legitimate message scored as spam is invisible to the panel and can only be recovered from the server's own logs."
          missing={[
            'GET  /api/v1/mail-server/spam-filter/quarantine',
            'POST /api/v1/mail-server/spam-filter/quarantine/:id/release',
          ]}
        />
      </Panel>
    </div>
  );
}

/* ------------------------------------------------------------------ *
 * Sender
 * ------------------------------------------------------------------ */

function SenderPane() {
  const load = useCallback(() => mailServerApi.getConfig(), []);
  const state = useResource(load, 'Could not load the mail server configuration.');
  const config = state.data;

  return (
    <div className="space-y-4">
      <Panel>
        <SectionHeader
          title="Sender"
          description="Send one message through this server to prove it works end to end."
        />
        <BackendGap
          title="The panel cannot send a message"
          description="There is no route that hands a message to the mail server, so there is no working form to draw here. Testing delivery today means using an SMTP client against the ports below."
          missing={['POST /api/v1/mail-server/send']}
        />
      </Panel>

      <Panel>
        <SectionHeader
          title="How to reach this server"
          description="What a mail client needs while the panel cannot send for you."
        />
        {state.loading && <BlockSkeleton lines={3} />}
        {!state.loading && state.error && (
          <ErrorBlock
            title="Could not load the server configuration"
            message={state.error}
            onRetry={state.reload}
          />
        )}
        {!state.loading && !state.error && !config && (
          <EmptyState
            title="No mail server configuration yet"
            description="Set the hostname and ports under Other Settings → Common settings, then come back."
          />
        )}
        {!state.loading && !state.error && config && (
          <>
            {!config.is_running && (
              <Notice tone="amber">
                The backend reports this mail server as stopped. A client will not connect until it
                is running.
              </Notice>
            )}
            <dl className="grid gap-4 px-4 py-4 sm:grid-cols-2 lg:grid-cols-4">
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-gray-500">
                  Hostname
                </dt>
                <dd className="mt-1 break-all font-mono text-sm text-gray-900">
                  {config.hostname || <Dash reason="No hostname has been set." />}
                </dd>
              </div>
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-gray-500">
                  SMTP submission
                </dt>
                <dd className="mt-1 font-mono text-sm text-gray-900">
                  {config.smtps_port} (STARTTLS), {config.smtp_port} (relay)
                </dd>
              </div>
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-gray-500">IMAP</dt>
                <dd className="mt-1 font-mono text-sm text-gray-900">
                  {config.imaps_port} (TLS), {config.imap_port} (plain)
                </dd>
              </div>
              <div>
                <dt className="text-xs font-medium uppercase tracking-wide text-gray-500">TLS</dt>
                <dd className="mt-1 text-sm text-gray-900">
                  {config.tls_enabled ? (
                    <Pill tone="emerald">Enabled</Pill>
                  ) : (
                    <Pill tone="red">Disabled</Pill>
                  )}
                </dd>
              </div>
            </dl>
          </>
        )}
      </Panel>
    </div>
  );
}
