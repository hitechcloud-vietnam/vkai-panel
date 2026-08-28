'use client';

/**
 * Email Server: one page for the whole mail system.
 *
 * This replaces two separate pages - /mail-server and /email-marketing - that
 * split one job in half. An operator diagnosing "our newsletter goes to spam"
 * had to look at the campaign on one screen and the domain's DNS on another,
 * with no path between them. aaPanel keeps all of it under Mail, and so does
 * this: a strip of tabs across the top, one content pane under it, and the
 * marketing area carrying its own second strip.
 *
 * The selected tab lives in the URL (?tab=…&sub=…), so a reload keeps it and a
 * link to a particular screen can be sent to somebody. Both old paths redirect
 * here, so a bookmark still lands somewhere useful.
 */

import { Suspense, useCallback } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';

import EmailPane from '@/components/email-server/EmailPane';
import MailDomainPane from '@/components/email-server/MailDomainPane';
import MailboxesPane from '@/components/email-server/MailboxesPane';
import MailMarketingPane from '@/components/email-server/MailMarketingPane';
import OtherSettingsPane from '@/components/email-server/OtherSettingsPane';
import { SubNav, TopTabs } from '@/components/email-server/nav';
import {
  SUBTABS,
  TOP_TABS,
  defaultSub,
  isTopTab,
  normaliseSub,
  type TopTabId,
} from '@/components/email-server/types';

export default function EmailServerPage() {
  return (
    <Suspense fallback={<PageSkeleton />}>
      <EmailServerSection />
    </Suspense>
  );
}

function EmailServerSection() {
  const router = useRouter();
  const params = useSearchParams();

  const rawTab = params.get('tab');
  const tab: TopTabId = isTopTab(rawTab) ? rawTab : 'mail-marketing';
  const sub = normaliseSub(tab, params.get('sub'));
  const subItems = SUBTABS[tab];

  const go = useCallback(
    (nextTab: TopTabId, nextSub: string) => {
      const query = new URLSearchParams();
      query.set('tab', nextTab);
      if (nextSub) query.set('sub', nextSub);
      router.replace(`/email-server?${query.toString()}`, { scroll: false });
    },
    [router]
  );

  const selectTab = useCallback(
    (id: string) => {
      if (!isTopTab(id)) return;
      go(id, defaultSub(id));
    },
    [go]
  );

  const selectSub = useCallback((id: string) => go(tab, id), [go, tab]);

  return (
    <div className="space-y-4">
      <header>
        <h1 className="text-xl font-semibold text-gray-900">Email Server</h1>
        <p className="mt-1 max-w-3xl text-sm text-gray-600">
          Mail domains and their DNS, the mailboxes under them, what is moving through the queue, and
          the campaigns that use it — in one place, because a delivery problem is never in only one
          of them.
        </p>
      </header>

      <TopTabs items={TOP_TABS} active={tab} onSelect={selectTab} />

      {subItems.length > 0 && (
        <SubNav
          items={subItems}
          active={sub}
          onSelect={selectSub}
          label={`${TOP_TABS.find((t) => t.id === tab)?.label ?? ''} sections`}
        />
      )}

      <div
        role="tabpanel"
        id={`email-panel-${tab}`}
        aria-labelledby={`email-tab-${tab}`}
        tabIndex={-1}
      >
        {tab === 'mail-marketing' && <MailMarketingPane sub={sub} />}
        {tab === 'mail-domain' && <MailDomainPane />}
        {tab === 'mailboxes' && <MailboxesPane />}
        {tab === 'email' && <EmailPane sub={sub} />}
        {tab === 'other-settings' && <OtherSettingsPane sub={sub} />}
      </div>
    </div>
  );
}

/** Shown for the instant before the URL's tab is known. */
function PageSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true">
      <div className="h-7 w-48 animate-pulse rounded bg-gray-100" />
      <div className="h-12 animate-pulse rounded-lg border border-gray-200 bg-white" />
      <div className="h-64 animate-pulse rounded-lg border border-gray-200 bg-white" />
    </div>
  );
}
