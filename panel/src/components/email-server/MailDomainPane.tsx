'use client';

/**
 * Mail Domain: every sending domain with the state of the three DNS records
 * that decide whether its mail is delivered or silently filed as spam.
 *
 * This screen exists because a mail server with wrong DNS *appears to work*.
 * Postfix accepts the message, the queue drains, the panel reports success, and
 * the mail lands in a spam folder nobody reads. The operator finds out weeks
 * later from a customer. So SPF, DKIM and DMARC are each shown as present or
 * missing, with the exact record to publish next to the verdict.
 *
 * Honesty note, and it is a big one: the backend stores spf_record, dmarc_record
 * and dkim_enabled on mail_domains but nothing ever writes them - CreateDomain
 * inserts the domain and nothing else, and there is no route that resolves DNS.
 * So "not published" here means "the panel has no record of it", not "the world
 * says it is missing". That distinction is printed on the screen rather than
 * hidden behind a green tick, and the missing endpoint is named.
 */

import { Fragment, useCallback, useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, Plus, Trash2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { errorMessage } from '@/lib/apiError';

import { mailServerApi } from './api';
import {
  ActionError,
  CopyableRecord,
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
import { formatDate } from './format';
import { useResource } from './useResource';
import type { MailAccount, MailDomain, MailServerConfig } from './types';

interface DomainData {
  domains: MailDomain[];
  accounts: MailAccount[];
  config: MailServerConfig | null;
}

/** The record we tell the operator to publish, per mechanism. */
interface RecordAdvice {
  type: 'MX' | 'TXT' | 'CNAME';
  host: string;
  value: string | null;
  /** Why we cannot print a value, when we cannot. */
  unavailable?: string;
  note?: string;
}

function spfAdvice(domain: string): RecordAdvice {
  return {
    type: 'TXT',
    host: domain,
    value: 'v=spf1 mx a ~all',
    note:
      'Authorises this domain’s own MX and A hosts to send for it. Add any third party that sends on your behalf (for example include:_spf.google.com) before publishing, and keep exactly one SPF record on the domain.',
  };
}

function dmarcAdvice(domain: string): RecordAdvice {
  return {
    type: 'TXT',
    host: `_dmarc.${domain}`,
    value: `v=DMARC1; p=none; rua=mailto:postmaster@${domain}`,
    note:
      'Start at p=none so reports arrive without mail being rejected. Once the reports show SPF and DKIM passing for everything you send, tighten it to p=quarantine and then p=reject.',
  };
}

function dkimAdvice(domain: string): RecordAdvice {
  return {
    type: 'TXT',
    host: `default._domainkey.${domain}`,
    value: null,
    unavailable:
      'The DKIM public key is what makes this record; it is held in mail_dkim_keys and no endpoint returns it. Printing a placeholder here would be worse than printing nothing.',
  };
}

function mxAdvice(domain: string, hostname: string | null): RecordAdvice {
  return {
    type: 'MX',
    host: domain,
    value: hostname ? `10 ${hostname}.` : null,
    unavailable: hostname
      ? undefined
      : 'The mail hostname is not set. Set it under Other Settings → Common settings first; the MX record has to point at it.',
  };
}

function StateCell({ present, label }: { present: boolean; label: string }) {
  return present ? (
    <Pill tone="emerald">{label} present</Pill>
  ) : (
    <Pill tone="amber">{label} missing</Pill>
  );
}

function RecordRow({ title, advice }: { title: string; advice: RecordAdvice }) {
  return (
    <div className="rounded-md border border-gray-200 bg-white p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-semibold text-gray-900">{title}</span>
        <Pill tone="gray">{advice.type}</Pill>
      </div>
      <dl className="mt-2 space-y-2 text-sm">
        <div>
          <dt className="text-xs font-medium uppercase tracking-wide text-gray-500">Host</dt>
          <dd className="mt-1">
            <CopyableRecord value={advice.host} label={`${title} host`} />
          </dd>
        </div>
        <div>
          <dt className="text-xs font-medium uppercase tracking-wide text-gray-500">Value</dt>
          <dd className="mt-1">
            {advice.value ? (
              <CopyableRecord value={advice.value} label={`${title} value`} />
            ) : (
              <p className="rounded-md border border-amber-200 bg-amber-50 px-2 py-1.5 text-xs text-amber-800">
                {advice.unavailable}
              </p>
            )}
          </dd>
        </div>
      </dl>
      {advice.note && <p className="mt-2 text-xs text-gray-500">{advice.note}</p>}
    </div>
  );
}

export default function MailDomainPane() {
  const load = useCallback(async (): Promise<DomainData> => {
    const [domains, accounts, config] = await Promise.all([
      mailServerApi.listDomains(),
      mailServerApi.listAccounts(),
      mailServerApi.getConfig().catch(() => null),
    ]);
    return { domains, accounts, config };
  }, []);

  const state = useResource<DomainData>(load, 'Could not load mail domains.');

  const [search, setSearch] = useState('');
  const [expanded, setExpanded] = useState<string | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [newDomain, setNewDomain] = useState('');
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState('');

  const hostname = state.data?.config?.hostname?.trim() || null;

  const mailboxCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const account of state.data?.accounts ?? []) {
      counts.set(account.domain_id, (counts.get(account.domain_id) ?? 0) + 1);
    }
    return counts;
  }, [state.data]);

  const rows = useMemo(() => {
    const all = state.data?.domains ?? [];
    const term = search.trim().toLowerCase();
    return term ? all.filter((d) => d.domain.toLowerCase().includes(term)) : all;
  }, [state.data, search]);

  const submitDomain = async () => {
    const value = newDomain.trim().toLowerCase();
    if (!value) {
      setActionError('Enter a domain name, for example example.vn');
      return;
    }
    setBusy(true);
    setActionError('');
    try {
      await mailServerApi.createDomain(value);
      setAddOpen(false);
      setNewDomain('');
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not add the domain.'));
    } finally {
      setBusy(false);
    }
  };

  const removeDomain = async (domain: MailDomain) => {
    const count = mailboxCounts.get(domain.id) ?? 0;
    const warning =
      count > 0
        ? `Delete ${domain.domain}? Its ${count} mailbox${count === 1 ? '' : 'es'} are deleted with it.`
        : `Delete ${domain.domain}?`;
    if (!window.confirm(warning)) return;
    setActionError('');
    try {
      await mailServerApi.deleteDomain(domain.id);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not delete the domain.'));
    }
  };

  return (
    <div className="space-y-4">
      <Panel>
        <SectionHeader
          title="Mail domains"
          description="Each domain that sends or receives mail on this server, and the DNS records that decide whether receivers trust it."
        />

        <Notice tone="amber">
          <p className="font-medium">The panel does not read DNS yet.</p>
          <p className="mt-1">
            SPF, DKIM and DMARC below reflect what this panel has recorded, not what is published in
            the zone. Nothing writes those fields today, so a correctly configured domain will still
            read as missing here. Publish the records shown and verify them with an external lookup
            until{' '}
            <span className="font-mono text-xs">POST /api/v1/mail-server/domains/:id/verify</span>{' '}
            exists.
          </p>
        </Notice>

        <Toolbar
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search domains"
          onRefresh={state.reload}
          refreshing={state.refreshing}
        >
          <Button type="button" onClick={() => setAddOpen(true)}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            Add Domain
          </Button>
        </Toolbar>

        {actionError && !addOpen && (
          <div className="px-4 pt-3">
            <ActionError message={actionError} onDismiss={() => setActionError('')} />
          </div>
        )}

        {state.loading && <TableSkeleton columns={6} rows={4} />}

        {!state.loading && state.error && (
          <ErrorBlock
            title="Could not load mail domains"
            message={state.error}
            onRetry={state.reload}
          />
        )}

        {!state.loading && !state.error && rows.length === 0 && (
          <EmptyState
            title={search ? 'No domain matches that search' : 'No mail domain yet'}
            description={
              search
                ? 'Clear the search to see every domain on this server.'
                : 'Add the domain your mailboxes will live under. Once it is here, publish the MX, SPF and DMARC records this screen prints for it, then create mailboxes.'
            }
            action={
              !search && (
                <Button type="button" onClick={() => setAddOpen(true)}>
                  <Plus className="h-4 w-4" aria-hidden="true" />
                  Add Domain
                </Button>
              )
            }
          />
        )}

        {!state.loading && !state.error && rows.length > 0 && (
          <TableScroller>
            <table className="w-full min-w-[900px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH} scope="col">
                    Domain
                  </th>
                  <th className={TH} scope="col">
                    Mailboxes
                  </th>
                  <th className={TH} scope="col">
                    MX
                  </th>
                  <th className={TH} scope="col">
                    SPF
                  </th>
                  <th className={TH} scope="col">
                    DKIM
                  </th>
                  <th className={TH} scope="col">
                    DMARC
                  </th>
                  <th className={TH} scope="col">
                    Added
                  </th>
                  <th className={TH} scope="col">
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((domain) => {
                  const open = expanded === domain.id;
                  const added = formatDate(domain.created_at);
                  return (
                    <Fragment key={domain.id}>
                      <tr className={ROW}>
                        <td className={TD}>
                          <button
                            type="button"
                            onClick={() => setExpanded(open ? null : domain.id)}
                            aria-expanded={open}
                            className="flex items-center gap-1.5 text-left text-sm font-medium text-gray-900 hover:text-brand-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                          >
                            {open ? (
                              <ChevronDown className="h-4 w-4 text-gray-400" aria-hidden="true" />
                            ) : (
                              <ChevronRight className="h-4 w-4 text-gray-400" aria-hidden="true" />
                            )}
                            {domain.domain}
                          </button>
                          {!domain.is_active && (
                            <span className="ml-6 mt-1 block">
                              <Pill tone="gray">Disabled</Pill>
                            </span>
                          )}
                        </td>
                        <td className={TD}>{mailboxCounts.get(domain.id) ?? 0}</td>
                        <td className={TD}>
                          <StateCell present={Boolean(domain.mx_record)} label="MX" />
                        </td>
                        <td className={TD}>
                          <StateCell present={Boolean(domain.spf_record)} label="SPF" />
                        </td>
                        <td className={TD}>
                          <StateCell present={domain.dkim_enabled} label="DKIM" />
                        </td>
                        <td className={TD}>
                          <StateCell present={Boolean(domain.dmarc_record)} label="DMARC" />
                        </td>
                        <td className={TD}>
                          {added ?? <Dash reason="The backend did not report a creation date." />}
                        </td>
                        <td className={TD}>
                          <Button
                            type="button"
                            variant="danger-outline"
                            size="sm"
                            onClick={() => removeDomain(domain)}
                            aria-label={`Delete ${domain.domain}`}
                          >
                            <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                            Delete
                          </Button>
                        </td>
                      </tr>
                      {open && (
                        <tr className="border-b border-gray-100">
                          <td colSpan={8} className="bg-gray-50 px-4 py-4">
                            <p className="text-sm font-semibold text-gray-900">
                              Records to publish for {domain.domain}
                            </p>
                            <p className="mt-1 text-sm text-gray-500">
                              Add these at the DNS provider that holds the zone. TXT values must be
                              published exactly as shown, on one line.
                            </p>
                            <div className="mt-3 grid gap-3 lg:grid-cols-2">
                              <RecordRow title="MX" advice={mxAdvice(domain.domain, hostname)} />
                              <RecordRow title="SPF" advice={spfAdvice(domain.domain)} />
                              <RecordRow title="DKIM" advice={dkimAdvice(domain.domain)} />
                              <RecordRow title="DMARC" advice={dmarcAdvice(domain.domain)} />
                            </div>
                            {domain.mx_record && (
                              <p className="mt-3 text-xs text-gray-500">
                                Stored MX:{' '}
                                <span className="font-mono text-gray-700">{domain.mx_record}</span>
                              </p>
                            )}
                            {domain.spf_record && (
                              <p className="mt-1 text-xs text-gray-500">
                                Stored SPF:{' '}
                                <span className="font-mono text-gray-700">{domain.spf_record}</span>
                              </p>
                            )}
                            {domain.dmarc_record && (
                              <p className="mt-1 text-xs text-gray-500">
                                Stored DMARC:{' '}
                                <span className="font-mono text-gray-700">
                                  {domain.dmarc_record}
                                </span>
                              </p>
                            )}
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>

      <Modal
        open={addOpen}
        title="Add mail domain"
        description="The domain part of the addresses this server will host, without a hostname in front."
        onClose={() => {
          setAddOpen(false);
          setActionError('');
        }}
        footer={
          <>
            <Button type="button" variant="secondary" onClick={() => setAddOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={submitDomain} disabled={busy}>
              {busy ? 'Adding…' : 'Add Domain'}
            </Button>
          </>
        }
      >
        <ActionError message={actionError} onDismiss={() => setActionError('')} />
        <Field
          label="Domain name"
          htmlFor="new-mail-domain"
          hint="For example example.vn. Adding it here does not change DNS; publish the records this screen prints afterwards."
        >
          <Input
            id="new-mail-domain"
            value={newDomain}
            onChange={(e) => setNewDomain(e.target.value)}
            placeholder="example.vn"
            autoComplete="off"
          />
        </Field>
      </Modal>
    </div>
  );
}
