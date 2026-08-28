'use client';

/**
 * Mail Marketing, and its seven screens.
 *
 * The Suspend List is deliberately not the last tab in the list and not a
 * footnote on Subscribers. Sending to an address that unsubscribed, hard
 * bounced or filed a complaint is the single fastest way to get a mail server
 * onto a blocklist, and once it is there every customer on the box stops being
 * delivered - not just the one who pressed send. So the count is on the
 * Overview, it is repeated as a warning on Marketing Task, and it has a screen
 * of its own here.
 *
 * What the panel cannot do is enforce it: the backend has no sender, so there
 * is no send path to filter. That is said plainly rather than implied by a
 * reassuring green tile.
 */

import { useCallback, useMemo, useState } from 'react';

import { marketingApi } from './api';
import {
  BackendGap,
  Dash,
  EmptyState,
  ErrorBlock,
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
import { contactStatusTone, formatDate, formatPercent } from './format';
import MarketingCampaigns from './MarketingCampaigns';
import { GroupsPane, SubscribersPane, TemplatesPane } from './MarketingLibrary';
import { scanSuspended, SCAN_LIMIT, type SuspensionScan } from './suspension';
import { useResource } from './useResource';
import type { EmailCampaign, EmailStats } from './types';

export default function MailMarketingPane({ sub }: { sub: string }) {
  switch (sub) {
    case 'task':
      return <MarketingCampaigns />;
    case 'template':
      return <TemplatesPane />;
    case 'subscribers':
      return <SubscribersPane />;
    case 'groups':
      return <GroupsPane />;
    case 'suspend-list':
      return <SuspendListPane />;
    case 'automation':
      return <AutomationPane />;
    default:
      return <OverviewPane />;
  }
}

/* ------------------------------------------------------------------ *
 * Overview
 * ------------------------------------------------------------------ */

interface OverviewData {
  stats: EmailStats | null;
  campaigns: EmailCampaign[];
  suspension: SuspensionScan | null;
}

function OverviewPane() {
  const load = useCallback(async (): Promise<OverviewData> => {
    const [stats, page, suspension] = await Promise.all([
      marketingApi.stats(),
      marketingApi.listCampaigns(100, 0).catch(() => ({ items: [], total: 0 })),
      scanSuspended().catch(() => null),
    ]);
    return { stats, campaigns: page.items, suspension };
  }, []);

  const state = useResource<OverviewData>(load, 'Could not load the marketing overview.');

  const stats = state.data?.stats ?? null;
  const suspension = state.data?.suspension ?? null;
  const suspended = suspension?.suspended.length ?? 0;

  const byStatus = useMemo(() => {
    const out = new Map<string, number>();
    for (const campaign of state.data?.campaigns ?? []) {
      const key = (campaign.status || 'draft').toLowerCase();
      out.set(key, (out.get(key) ?? 0) + 1);
    }
    return Array.from(out.entries()).sort((a, b) => b[1] - a[1]);
  }, [state.data]);

  if (state.loading) {
    return (
      <div className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-24 animate-pulse rounded-lg border border-gray-200 bg-white" />
          ))}
        </div>
        <Panel>
          <TableSkeleton columns={3} rows={4} />
        </Panel>
      </div>
    );
  }

  if (state.error) {
    return (
      <Panel>
        <SectionHeader title="Overview" />
        <ErrorBlock
          title="Could not load the marketing overview"
          message={state.error}
          onRetry={state.reload}
        />
      </Panel>
    );
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatTile
          label="Campaigns"
          value={stats ? stats.total_campaigns : <Dash reason="The stats endpoint returned nothing." />}
          hint="Drafted in this account."
        />
        <StatTile
          label="Subscribers"
          value={stats ? stats.total_contacts : <Dash reason="The stats endpoint returned nothing." />}
          hint="Every address, whatever its state."
        />
        <StatTile
          label="Groups"
          value={stats ? stats.total_lists : <Dash reason="The stats endpoint returned nothing." />}
          hint="Named audiences."
        />
        <StatTile
          label="On the suspend list"
          value={
            suspension ? (
              `${suspended}${suspension.complete ? '' : '+'}`
            ) : (
              <Dash reason="The subscriber list could not be read, so this count is unknown." />
            )
          }
          hint={
            suspension && !suspension.complete
              ? `At least this many, from the first ${SCAN_LIMIT} subscribers.`
              : 'Unsubscribed, bounced or complained.'
          }
        />
      </div>

      {suspended > 0 && (
        <div
          className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800"
          role="status"
        >
          {suspended}
          {suspension && !suspension.complete ? ' or more' : ''} address
          {suspended === 1 ? '' : 'es'} must not be mailed again. Nothing on the backend enforces
          that at send time. Open the Suspend List tab before any campaign goes out.
        </div>
      )}

      <Panel>
        <SectionHeader
          title="Delivery"
          description="What the campaigns in this account have produced so far."
        />
        <Notice tone="red">
          These counters are stored on each campaign and nothing increments them: the module has no
          sender and no tracking. They will read zero until delivery is implemented, so do not read a
          zero here as &quot;nobody opened it&quot;.
        </Notice>
        <div className="grid gap-3 px-4 py-4 sm:grid-cols-2 xl:grid-cols-4">
          <StatTile label="Messages sent" value={stats ? stats.total_sent : 0} />
          <StatTile
            label="Open rate"
            value={stats ? formatPercent(stats.avg_open_rate) : '0.0%'}
            hint={stats ? `${stats.total_opened} opened` : undefined}
          />
          <StatTile
            label="Click rate"
            value={stats ? formatPercent(stats.avg_click_rate) : '0.0%'}
            hint={stats ? `${stats.total_clicked} clicked` : undefined}
          />
          <StatTile
            label="Bounce rate"
            value={stats ? formatPercent(stats.avg_bounce_rate) : '0.0%'}
            hint={
              stats
                ? `${stats.total_bounced} bounced — above 2% and receivers start filtering you`
                : undefined
            }
          />
        </div>
      </Panel>

      <Panel>
        <SectionHeader title="Campaigns by status" description="Where the drafts have got to." />
        {byStatus.length === 0 ? (
          <EmptyState
            title="No campaigns yet"
            description="Draft one on the Marketing Task tab. Build the audience under Subscribers first."
          />
        ) : (
          <div className="flex flex-wrap gap-2 px-4 py-4">
            {byStatus.map(([status, count]) => (
              <Pill key={status} tone={status === 'draft' ? 'gray' : 'sky'}>
                {status}: {count}
              </Pill>
            ))}
          </div>
        )}
      </Panel>
    </div>
  );
}

/* ------------------------------------------------------------------ *
 * Suspend List
 * ------------------------------------------------------------------ */

function SuspendListPane() {
  const load = useCallback(() => scanSuspended(), []);
  const state = useResource<SuspensionScan>(load, 'Could not read the subscriber list.');
  const [search, setSearch] = useState('');

  const scan = state.data;
  const rows = useMemo(() => {
    const all = scan?.suspended ?? [];
    const term = search.trim().toLowerCase();
    if (!term) return all;
    return all.filter((c) => c.email.toLowerCase().includes(term));
  }, [scan, search]);

  const counts = useMemo(() => {
    const out = { unsubscribed: 0, bounced: 0, complained: 0 };
    for (const contact of scan?.suspended ?? []) {
      const key = (contact.status || '').toLowerCase();
      if (key in out) out[key as keyof typeof out] += 1;
    }
    return out;
  }, [scan]);

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-3">
        <StatTile
          label="Unsubscribed"
          value={counts.unsubscribed}
          hint="Asked to stop. Mailing them again is a legal problem as well as a reputation one."
        />
        <StatTile
          label="Bounced"
          value={counts.bounced}
          hint="The address does not exist. Repeat attempts are what receivers score you on."
        />
        <StatTile
          label="Complained"
          value={counts.complained}
          hint="Marked your mail as spam. The most expensive signal a receiver has."
        />
      </div>

      <Panel>
        <SectionHeader
          title="Suspend list"
          description="Addresses that must never receive another campaign from this account."
        />

        <Notice tone="red">
          <p className="font-medium">Nothing enforces this list yet.</p>
          <p className="mt-1">
            The backend has no sender, so there is no send path that could exclude these addresses,
            and nothing in{' '}
            <span className="font-mono text-xs">POST /api/v1/email-marketing/campaigns/:id/send</span>{' '}
            looks at contact status. Until that changes, treat this screen as a list to check by hand
            before any bulk send — one complaint rate above roughly 0.3% is enough for a large
            provider to start filtering everything the server sends, for every customer on it.
          </p>
        </Notice>

        {scan && !scan.complete && (
          <Notice tone="amber">
            This list was built by reading the first {scan.scanned} of {scan.total} subscribers and
            sorting them here, because the contacts endpoint has no status filter. There may be
            suspended addresses it never saw, so the counts above are a floor, not a total. The
            missing capability is a status parameter on{' '}
            <span className="font-mono text-xs">GET /api/v1/email-marketing/contacts</span>.
          </Notice>
        )}

        <Toolbar
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search suspended addresses"
          onRefresh={state.reload}
          refreshing={state.refreshing}
        />

        {state.loading && <TableSkeleton columns={4} rows={5} />}

        {!state.loading && state.error && (
          <ErrorBlock
            title="Could not read the subscriber list"
            message={state.error}
            onRetry={state.reload}
          />
        )}

        {!state.loading && !state.error && rows.length === 0 && (
          <EmptyState
            title={
              search ? 'No suspended address matches that search' : 'Nobody is suspended'
            }
            description={
              search
                ? 'Clear the search to see the whole list.'
                : 'No subscriber in this account has unsubscribed, bounced or complained. Keep it that way: confirmed opt-in on every group, and an unsubscribe link in every message.'
            }
          />
        )}

        {!state.loading && !state.error && rows.length > 0 && (
          <TableScroller>
            <table className="w-full min-w-[760px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH} scope="col">
                    Address
                  </th>
                  <th className={TH} scope="col">
                    Reason
                  </th>
                  <th className={TH} scope="col">
                    Came from
                  </th>
                  <th className={TH} scope="col">
                    Added
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((contact) => {
                  const added = formatDate(contact.updated_at || contact.created_at);
                  return (
                    <tr key={contact.id} className={ROW}>
                      <td className={TD}>
                        <span className="font-medium text-gray-900">{contact.email}</span>
                      </td>
                      <td className={TD}>
                        <Pill tone={contactStatusTone(contact.status)}>{contact.status}</Pill>
                      </td>
                      <td className={TD}>{contact.source || 'manual'}</td>
                      <td className={TD}>
                        {added ?? <Dash reason="The backend did not report a date." />}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>

      <Panel>
        <SectionHeader
          title="Managing the list"
          description="Adding a suppression by hand, and lifting one."
        />
        <BackendGap
          title="Entries cannot be added or lifted from the panel"
          description="A contact's status is set once at creation and there is no route that changes it, so an address that asks to be removed by phone cannot be suppressed here, and someone suppressed by mistake cannot be restored. Deleting the contact is not a substitute: that throws away the record that they must not be mailed, and the next import will happily add them back."
          missing={[
            'PUT  /api/v1/email-marketing/contacts/:id  (status)',
            'POST /api/v1/email-marketing/suppressions  (add an address without a contact record)',
            'GET  /api/v1/email-marketing/contacts?status=unsubscribed  (a server-side filter)',
            'and: SendCampaign must exclude suspended addresses before it dispatches anything',
          ]}
        />
      </Panel>
    </div>
  );
}

/* ------------------------------------------------------------------ *
 * Automation
 * ------------------------------------------------------------------ */

function AutomationPane() {
  return (
    <Panel>
      <SectionHeader
        title="Automation"
        description="Messages that go out on a trigger rather than on a schedule you press."
      />
      <BackendGap
        title="Automations have a table and a model, and no routes"
        description="email_automations exists in migration 015 and models.EmailAutomation describes it, but nothing is mounted in router.go and no service method touches it. Every control on this screen would post into a 404, so there are none. This is the same shape of defect the panel has shipped before — a settings page whose four endpoints all returned 404 — and it is not repeated here."
        missing={[
          'GET    /api/v1/email-marketing/automations',
          'POST   /api/v1/email-marketing/automations',
          'PUT    /api/v1/email-marketing/automations/:id',
          'DELETE /api/v1/email-marketing/automations/:id',
          'POST   /api/v1/email-marketing/automations/:id/pause',
        ]}
      />
    </Panel>
  );
}
