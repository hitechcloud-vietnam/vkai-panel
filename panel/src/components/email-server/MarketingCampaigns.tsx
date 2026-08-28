'use client';

/**
 * Marketing Task: the campaigns, what they cost in reputation, and the one
 * thing an operator must understand before pressing anything here.
 *
 * EmailMarketingService.SendCampaign does exactly one thing - it sets
 * status = 'sending'. There is no worker that reads that status, no recipient
 * resolution, no suppression check and no SMTP call anywhere in the module. So
 * the button is labelled for what it does ("Mark as sending") and the banner
 * above it says the rest. A button labelled "Send" here would be the worst
 * defect this section could ship: an operator would believe a campaign had gone
 * out, and would never look again.
 */

import { useCallback, useMemo, useState } from 'react';
import { Pause, Play, Plus, Trash2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { errorMessage } from '@/lib/apiError';

import { marketingApi } from './api';
import {
  ActionError,
  Dash,
  EmptyState,
  ErrorBlock,
  Field,
  Modal,
  Notice,
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
import { campaignStatusTone, formatDateTime } from './format';
import { scanSuspended, type SuspensionScan } from './suspension';
import { useResource } from './useResource';
import type { EmailCampaign } from './types';

interface CampaignData {
  campaigns: EmailCampaign[];
  total: number;
  suspension: SuspensionScan | null;
}

export default function MarketingCampaigns() {
  const load = useCallback(async (): Promise<CampaignData> => {
    const [page, suspension] = await Promise.all([
      marketingApi.listCampaigns(100, 0),
      scanSuspended().catch(() => null),
    ]);
    return { campaigns: page.items, total: page.total, suspension };
  }, []);

  const state = useResource<CampaignData>(load, 'Could not load campaigns.');

  const [search, setSearch] = useState('');
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<EmailCampaign | null>(null);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState('');
  const [form, setForm] = useState({
    name: '',
    subject: '',
    from_name: '',
    from_email: '',
    reply_to: '',
    html_content: '',
  });

  const campaigns = useMemo(() => state.data?.campaigns ?? [], [state.data]);
  const suspension = state.data?.suspension ?? null;
  const suspendedCount = suspension?.suspended.length ?? 0;

  const rows = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return campaigns;
    return campaigns.filter(
      (c) =>
        c.name.toLowerCase().includes(term) ||
        c.subject.toLowerCase().includes(term) ||
        c.from_email.toLowerCase().includes(term)
    );
  }, [campaigns, search]);

  const openCreate = () => {
    setEditing(null);
    setForm({ name: '', subject: '', from_name: '', from_email: '', reply_to: '', html_content: '' });
    setActionError('');
    setOpen(true);
  };

  const openEdit = (campaign: EmailCampaign) => {
    setEditing(campaign);
    setForm({
      name: campaign.name,
      subject: campaign.subject,
      from_name: campaign.from_name,
      from_email: campaign.from_email,
      reply_to: campaign.reply_to ?? '',
      html_content: campaign.html_content,
    });
    setActionError('');
    setOpen(true);
  };

  const submit = async () => {
    if (!form.name.trim()) {
      setActionError('Give the campaign a name you will recognise later.');
      return;
    }
    if (!form.subject.trim()) {
      setActionError('Enter the subject line recipients will see.');
      return;
    }
    if (!form.from_name.trim()) {
      setActionError('Enter the sender name recipients will see.');
      return;
    }
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(form.from_email.trim())) {
      setActionError('Enter a valid From address. The backend rejects anything else.');
      return;
    }
    if (!form.html_content.trim()) {
      setActionError('The message body cannot be empty.');
      return;
    }
    setBusy(true);
    setActionError('');
    try {
      if (editing) {
        await marketingApi.updateCampaign(editing.id, {
          name: form.name.trim(),
          subject: form.subject.trim(),
          from_name: form.from_name.trim(),
          from_email: form.from_email.trim(),
          reply_to: form.reply_to.trim(),
          html_content: form.html_content,
        });
      } else {
        await marketingApi.createCampaign({
          name: form.name.trim(),
          subject: form.subject.trim(),
          from_name: form.from_name.trim(),
          from_email: form.from_email.trim(),
          reply_to: form.reply_to.trim() || undefined,
          html_content: form.html_content,
        });
      }
      setOpen(false);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not save the campaign.'));
    } finally {
      setBusy(false);
    }
  };

  const start = async (campaign: EmailCampaign) => {
    const warning = [
      `Mark "${campaign.name}" as sending?`,
      '',
      'This sets the campaign status only. No message is dispatched: the backend has no sender, and it does not exclude unsubscribed or bounced addresses.',
    ];
    if (suspendedCount > 0) {
      warning.push(
        '',
        `${suspendedCount} address${suspendedCount === 1 ? '' : 'es'} in this account ${suspendedCount === 1 ? 'is' : 'are'} on the suspend list and would not be filtered out.`
      );
    }
    if (!window.confirm(warning.join('\n'))) return;
    setActionError('');
    try {
      await marketingApi.startCampaign(campaign.id);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not change the campaign status.'));
    }
  };

  const pause = async (campaign: EmailCampaign) => {
    setActionError('');
    try {
      await marketingApi.pauseCampaign(campaign.id);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not pause the campaign.'));
    }
  };

  const remove = async (campaign: EmailCampaign) => {
    if (!window.confirm(`Delete "${campaign.name}"? Its statistics go with it.`)) return;
    setActionError('');
    try {
      await marketingApi.deleteCampaign(campaign.id);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not delete the campaign.'));
    }
  };

  return (
    <div className="space-y-4">
      <Panel>
        <SectionHeader
          title="Marketing tasks"
          description="Each campaign, its audience and what happened to the mail it produced."
        />

        <Notice tone="red">
          <p className="font-medium">Campaigns are stored, not sent.</p>
          <p className="mt-1">
            The backend has no delivery worker: starting a campaign writes{' '}
            <span className="font-mono text-xs">status = &apos;sending&apos;</span> and stops there.
            Nothing is dispatched, no recipients are resolved, and the suspend list is not applied.
            The counters below stay at zero for the same reason. Treat this tab as a drafting table
            until{' '}
            <span className="font-mono text-xs">POST /api/v1/email-marketing/campaigns/:id/send</span>{' '}
            actually sends.
          </p>
        </Notice>

        {suspendedCount > 0 && (
          <Notice tone="amber">
            {suspendedCount} address{suspendedCount === 1 ? '' : 'es'} in this account{' '}
            {suspendedCount === 1 ? 'has' : 'have'} unsubscribed, bounced or complained. Sending to
            them is what gets a mail server blocklisted, and nothing on the backend filters them out
            yet. See the Suspend List tab.
          </Notice>
        )}

        <Toolbar
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search campaigns"
          onRefresh={state.reload}
          refreshing={state.refreshing}
        >
          <Button type="button" onClick={openCreate}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            Add Task
          </Button>
        </Toolbar>

        {actionError && !open && (
          <div className="px-4 pt-3">
            <ActionError message={actionError} onDismiss={() => setActionError('')} />
          </div>
        )}

        {state.loading && <TableSkeleton columns={6} rows={4} />}

        {!state.loading && state.error && (
          <ErrorBlock title="Could not load campaigns" message={state.error} onRetry={state.reload} />
        )}

        {!state.loading && !state.error && rows.length === 0 && (
          <EmptyState
            title={search ? 'No campaign matches that search' : 'No campaigns yet'}
            description={
              search
                ? 'Clear the search to see every campaign.'
                : 'Draft a campaign with its subject, sender and body. Build the audience under Subscribers and Groups first, so the campaign has somewhere to go once delivery works.'
            }
            action={
              !search && (
                <Button type="button" onClick={openCreate}>
                  <Plus className="h-4 w-4" aria-hidden="true" />
                  Add Task
                </Button>
              )
            }
          />
        )}

        {!state.loading && !state.error && rows.length > 0 && (
          <TableScroller>
            <table className="w-full min-w-[1100px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH} scope="col">
                    Campaign
                  </th>
                  <th className={TH} scope="col">
                    Sender
                  </th>
                  <th className={TH} scope="col">
                    Status
                  </th>
                  <th className={TH} scope="col">
                    Recipients
                  </th>
                  <th className={TH} scope="col">
                    Opened / clicked
                  </th>
                  <th className={TH} scope="col">
                    Bounced / unsubscribed
                  </th>
                  <th className={TH} scope="col">
                    Last change
                  </th>
                  <th className={TH} scope="col">
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((campaign) => {
                  const updated = formatDateTime(campaign.updated_at);
                  const sent = formatDateTime(campaign.sent_at);
                  const status = (campaign.status || 'draft').toLowerCase();
                  return (
                    <tr key={campaign.id} className={ROW}>
                      <td className={TD}>
                        <p className="font-medium text-gray-900">{campaign.name}</p>
                        <p className="mt-0.5 max-w-xs truncate text-xs text-gray-500" title={campaign.subject}>
                          {campaign.subject}
                        </p>
                      </td>
                      <td className={TD}>
                        <p className="text-gray-900">{campaign.from_name}</p>
                        <p className="mt-0.5 font-mono text-xs text-gray-500">
                          {campaign.from_email}
                        </p>
                      </td>
                      <td className={TD}>
                        <Pill tone={campaignStatusTone(status)}>{status}</Pill>
                      </td>
                      <td className={TD}>
                        {campaign.sent_count} of {campaign.total_recipients}
                      </td>
                      <td className={TD}>
                        {campaign.open_count} / {campaign.click_count}
                      </td>
                      <td className={TD}>
                        <span className={campaign.bounce_count > 0 ? 'text-red-700' : undefined}>
                          {campaign.bounce_count}
                        </span>
                        {' / '}
                        <span className={campaign.unsubscribe_count > 0 ? 'text-amber-700' : undefined}>
                          {campaign.unsubscribe_count}
                        </span>
                      </td>
                      <td className={TD}>
                        {updated ?? <Dash reason="The backend did not report a change time." />}
                        {sent && <p className="mt-0.5 text-xs text-gray-500">Sent {sent}</p>}
                      </td>
                      <td className={TD}>
                        <div className="flex flex-wrap items-center gap-2">
                          {status === 'sending' ? (
                            <Button
                              type="button"
                              variant="secondary"
                              size="sm"
                              onClick={() => pause(campaign)}
                            >
                              <Pause className="h-3.5 w-3.5" aria-hidden="true" />
                              Pause
                            </Button>
                          ) : (
                            <Button
                              type="button"
                              variant="secondary"
                              size="sm"
                              onClick={() => start(campaign)}
                              title="Sets the campaign status to sending. The backend does not dispatch mail yet."
                            >
                              <Play className="h-3.5 w-3.5" aria-hidden="true" />
                              Mark as sending
                            </Button>
                          )}
                          <Button
                            type="button"
                            variant="secondary"
                            size="sm"
                            onClick={() => openEdit(campaign)}
                          >
                            Edit
                          </Button>
                          <Button
                            type="button"
                            variant="danger-outline"
                            size="sm"
                            onClick={() => remove(campaign)}
                            aria-label={`Delete ${campaign.name}`}
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

      <Modal
        open={open}
        wide
        title={editing ? `Edit ${editing.name}` : 'New marketing task'}
        onClose={() => setOpen(false)}
        footer={
          <>
            <Button type="button" variant="secondary" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={submit} disabled={busy}>
              {busy ? 'Saving…' : editing ? 'Save changes' : 'Create task'}
            </Button>
          </>
        }
      >
        <ActionError message={actionError} onDismiss={() => setActionError('')} />
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Campaign name" htmlFor="camp-name" hint="Internal only. Recipients never see it.">
            <Input
              id="camp-name"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="March newsletter"
            />
          </Field>
          <Field label="Subject" htmlFor="camp-subject">
            <Input
              id="camp-subject"
              value={form.subject}
              onChange={(e) => setForm({ ...form, subject: e.target.value })}
              placeholder="What changed this month"
            />
          </Field>
          <Field label="From name" htmlFor="camp-from-name">
            <Input
              id="camp-from-name"
              value={form.from_name}
              onChange={(e) => setForm({ ...form, from_name: e.target.value })}
              placeholder="Example Support"
            />
          </Field>
          <Field
            label="From address"
            htmlFor="camp-from-email"
            hint="Use an address on a domain whose SPF and DKIM are published, or the mail will be filtered."
          >
            <Input
              id="camp-from-email"
              type="email"
              value={form.from_email}
              onChange={(e) => setForm({ ...form, from_email: e.target.value })}
              placeholder="news@example.vn"
            />
          </Field>
        </div>
        <Field label="Reply-to" htmlFor="camp-reply" hint="Optional. Defaults to the From address.">
          <Input
            id="camp-reply"
            type="email"
            value={form.reply_to}
            onChange={(e) => setForm({ ...form, reply_to: e.target.value })}
            placeholder="support@example.vn"
          />
        </Field>
        <Field
          label="Message body (HTML)"
          htmlFor="camp-body"
          hint="Include an unsubscribe link. Bulk mail without one is reported as spam and costs the whole server its reputation."
        >
          <textarea
            id="camp-body"
            rows={10}
            className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 font-mono text-xs text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            value={form.html_content}
            onChange={(e) => setForm({ ...form, html_content: e.target.value })}
            placeholder={'<p>Hello,</p>\n<p>…</p>\n<p><a href="{{unsubscribe_url}}">Unsubscribe</a></p>'}
          />
        </Field>
      </Modal>
    </div>
  );
}
