'use client';

/**
 * FTP Manager.
 *
 * aaPanel's FTP screen is one tab strip over one table with a toolbar. This
 * screen keeps that structure - a horizontal strip of panes, one content pane
 * below - and departs from it in exactly two ways, both deliberate.
 *
 * First, the selected pane lives in the query string rather than the DOM, so
 * `/ftp?tab=service` survives a reload and can be sent to a colleague.
 *
 * Second, the accounts pane does not draw an account table. The API has no FTP
 * routes: nothing under /api/v1/ftp is mounted, no handler mentions FTP, no
 * model stores an account. An empty table would read as "this server has no FTP
 * accounts", which is a different and false statement. This panel has shipped
 * that class of defect three times already - two-factor routes that were never
 * mounted behind a finished-looking settings page, four panel-settings
 * endpoints that all answered 404, an agent channel that was two TODO stubs -
 * so the pane names the missing routes instead.
 *
 * What IS wired here is real: the systemd routes that start and stop the FTP
 * daemon, the firewall rules that decide whether anyone outside the machine can
 * reach it, and the site roots that an account would be confined to.
 */

import { useCallback, useEffect, useState } from 'react';
import { Server } from 'lucide-react';

import { AccountsPane } from '@/components/ftp/AccountsPane';
import { FtpTabs } from '@/components/ftp/FtpTabs';
import { ServicePane } from '@/components/ftp/ServicePane';
import { FTP_TABS, isFtpTabId, type FtpTabId } from '@/components/ftp/types';

const DEFAULT_TAB: FtpTabId = 'accounts';

/**
 * Keep the selected pane in `?tab=`.
 *
 * Written against the history API rather than useSearchParams: in the app
 * router that hook opts the whole route into client-side rendering and fails
 * the production build unless the page is wrapped in a Suspense boundary, and
 * this page has no other reason to need one. replaceState rather than pushState
 * so the browser Back button leaves the FTP screen instead of walking back
 * through the tabs.
 */
function useTabParam(): [FtpTabId, (tab: FtpTabId) => void] {
  const [tab, setTab] = useState<FtpTabId>(DEFAULT_TAB);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const read = () => {
      const value = new URLSearchParams(window.location.search).get('tab');
      setTab(isFtpTabId(value) ? value : DEFAULT_TAB);
    };
    read();
    window.addEventListener('popstate', read);
    return () => window.removeEventListener('popstate', read);
  }, []);

  const select = useCallback((next: FtpTabId) => {
    setTab(next);
    if (typeof window === 'undefined') return;
    const url = new URL(window.location.href);
    url.searchParams.set('tab', next);
    window.history.replaceState(window.history.state, '', url.toString());
  }, []);

  return [tab, select];
}

export default function FtpPage() {
  const [tab, setTab] = useTabParam();
  const active = FTP_TABS.find((entry) => entry.id === tab) ?? FTP_TABS[0];

  return (
    <div className="min-h-full bg-gray-50">
      <div className="mx-auto max-w-7xl space-y-4 px-4 py-6 sm:px-6 lg:px-8">
        <header className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h1 className="flex items-center gap-2 text-xl font-semibold text-gray-900">
              <Server className="h-5 w-5 text-gray-400" aria-hidden="true" />
              FTP Manager
            </h1>
            <p className="mt-1 max-w-3xl text-sm text-gray-600">
              FTP accounts for this machine, the directory each one is confined to, and the
              daemon and ports behind them.
            </p>
          </div>
        </header>

        <FtpTabs active={tab} onSelect={setTab} />

        <p className="text-sm text-gray-500">{active.blurb}</p>

        <div
          role="tabpanel"
          id={`ftp-panel-${tab}`}
          aria-labelledby={`ftp-tab-${tab}`}
          tabIndex={-1}
        >
          {tab === 'accounts' ? <AccountsPane /> : <ServicePane />}
        </div>
      </div>
    </div>
  );
}
