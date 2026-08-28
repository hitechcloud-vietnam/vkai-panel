'use client';

/**
 * Docker.
 *
 * One page per area, a strip of tabs above one pane per area - the layout an
 * operator arriving from aaPanel already knows, so nothing has to be relearned
 * on the way across.
 *
 * The selected tab lives in the URL. aaPanel keeps it in the DOM, which means a
 * reload drops you back on the first tab and there is nothing to send a
 * colleague; `/docker?tab=volume` is a link, and a reload lands where it left
 * off.
 *
 * This screen manages Docker for the customer. The panel itself does not run in
 * a container and does not ship one.
 *
 * Almost nothing here is wired, and that is deliberate. Twenty Docker routes
 * are mounted in core/internal/handler/router.go and every handler behind them
 * is a literal: the lists return empty slices, the actions write a log line and
 * answer "Container started". This panel has already shipped that defect three
 * times - two-factor routes that were never mounted behind a finished-looking
 * settings page, a panel settings page calling four endpoints that answered
 * 404, an agent channel that was two TODO stubs beside a real route - and each
 * time it was found by an operator in production. So each pane states which
 * handler is empty instead of offering a button that reports success for work
 * that never happened.
 */

import { Suspense, useCallback } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';

import { DOCKER_TABS, parseDockerTab, type DockerTabId } from '@/types/docker';
import { CloudImagePane } from '@/components/docker/CloudImagePane';
import { ComposePane } from '@/components/docker/ComposePane';
import { ContainerPane } from '@/components/docker/ContainerPane';
import { DockerTabs } from '@/components/docker/DockerTabs';
import { LocalImagePane } from '@/components/docker/LocalImagePane';
import { NetworkPane } from '@/components/docker/NetworkPane';
import { OneClickPane } from '@/components/docker/OneClickPane';
import { OverviewPane } from '@/components/docker/OverviewPane';
import { RepositoryPane } from '@/components/docker/RepositoryPane';
import { SettingsPane } from '@/components/docker/SettingsPane';
import { VolumePane } from '@/components/docker/VolumePane';

/** Areas with no working API behind them, marked in the tab strip. */
const UNAVAILABLE_TABS: DockerTabId[] = ['one-click', 'cloud-image', 'repository', 'settings'];

function PaneFor({ tab }: { tab: DockerTabId }) {
  switch (tab) {
    case 'overview':
      return <OverviewPane />;
    case 'container':
      return <ContainerPane />;
    case 'one-click':
      return <OneClickPane />;
    case 'cloud-image':
      return <CloudImagePane />;
    case 'local-image':
      return <LocalImagePane />;
    case 'compose':
      return <ComposePane />;
    case 'network':
      return <NetworkPane />;
    case 'volume':
      return <VolumePane />;
    case 'repository':
      return <RepositoryPane />;
    case 'settings':
      return <SettingsPane />;
    default:
      return <OverviewPane />;
  }
}

function DockerScreen() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const active = parseDockerTab(searchParams.get('tab'));

  const selectTab = useCallback(
    (id: DockerTabId) => {
      const params = new URLSearchParams(searchParams.toString());
      params.set('tab', id);
      // replace, not push: flicking across ten tabs should not bury the page an
      // operator came from under ten history entries.
      router.replace(`${pathname}?${params.toString()}`, { scroll: false });
    },
    [router, pathname, searchParams]
  );

  const label = DOCKER_TABS.find((tab) => tab.id === active)?.label ?? 'Overview';

  return (
    <div className="space-y-4">
      <header>
        <h1 className="text-xl font-semibold text-gray-900">Docker</h1>
        <p className="mt-1 text-sm text-gray-600">
          Containers, images, networks and volumes on this host.
        </p>
      </header>

      <DockerTabs active={active} onSelect={selectTab} unavailable={UNAVAILABLE_TABS} />

      <div
        role="tabpanel"
        id={`docker-panel-${active}`}
        aria-labelledby={`docker-tab-${active}`}
        aria-label={label}
      >
        {/* Keyed so switching tabs remounts the pane and its request starts clean. */}
        <PaneFor key={active} tab={active} />
      </div>
    </div>
  );
}

/**
 * The shell while the router resolves the query string. useSearchParams needs a
 * Suspense boundary or the route cannot be prerendered, and this is what the
 * boundary shows.
 */
function DockerScreenSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true" aria-live="polite">
      <span className="sr-only">Loading Docker</span>
      <div className="h-6 w-32 animate-pulse rounded bg-gray-100" />
      <div className="h-12 animate-pulse rounded-lg border border-gray-200 bg-white shadow-sm" />
      <div className="h-64 animate-pulse rounded-lg border border-gray-200 bg-white shadow-sm" />
    </div>
  );
}

export default function DockerPage() {
  return (
    <Suspense fallback={<DockerScreenSkeleton />}>
      <DockerScreen />
    </Suspense>
  );
}
