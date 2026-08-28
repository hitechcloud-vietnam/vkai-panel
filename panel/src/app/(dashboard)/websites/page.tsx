'use client';

/**
 * Websites, split into one layout per project type.
 *
 * The screen used to be a single table with a single form, where a Node.js
 * project and a PHP site were told apart by a free-text "site type" field and
 * shown the same nine columns - a document root for a proxy that has none, a
 * PHP version for an application that never runs PHP. This replaces it with one
 * panel per type, each asking for and showing only what that type has.
 *
 * The selected type lives in the URL as ?type=, so a link to the Node.js tab
 * opens on the Node.js tab and the back button steps between them.
 *
 * Two tabs have no backend behind them. They are still here, and they say so:
 * see UnavailableProjectType and PROJECT_TYPE_GAPS. A tab that quietly offered
 * a form for a capability the API does not have would be the same defect this
 * codebase has shipped before.
 */

import { Suspense, useCallback, useMemo, useState } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { Cpu, Terminal } from 'lucide-react';

import NodeProjects from '@/components/websites/NodeProjects';
import PhpProjects from '@/components/websites/PhpProjects';
import ProjectTypeTabs from '@/components/websites/ProjectTypeTabs';
import ProxyProjects from '@/components/websites/ProxyProjects';
import UnavailableProjectType from '@/components/websites/UnavailableProjectType';
import {
  GO_REQUIREMENTS,
  PHP_TAB_SITE_TYPES,
  PROJECT_TYPES,
  PROJECT_TYPE_GAPS,
  PYTHON_REQUIREMENTS,
  projectType,
  projectTypeFromSlug,
  type ProjectTypeId,
} from '@/components/websites/projectTypes';
import { useSiteContext } from '@/components/websites/useSiteContext';
import { useServers } from '@/hooks/useServers';
import { serverLabel } from '@/lib/servers';

function WebsitesScreen() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const active = projectTypeFromSlug(searchParams.get('type'));
  const activeDef = projectType(active);

  const { servers, localNode, defaultId, singleNode, loading: serversLoading } = useServers();
  const ctx = useSiteContext();

  const [counts, setCounts] = useState<Partial<Record<ProjectTypeId, number | null>>>({});

  /*
   * One stable callback per type. The panels put these in their effect
   * dependencies, so a new function identity on every render would reload the
   * list forever.
   */
  const setNodeCount = useCallback(
    (count: number | null) => setCounts((prev) => ({ ...prev, nodejs: count })),
    []
  );
  const setProxyCount = useCallback(
    (count: number | null) => setCounts((prev) => ({ ...prev, proxy: count })),
    []
  );

  /** The PHP count needs no callback: the websites list is already on this page. */
  const phpCount = ctx.websitesLoading
    ? null
    : ctx.websites.filter((s) =>
        PHP_TAB_SITE_TYPES.includes(String(s.site_type || '').toLowerCase())
      ).length;

  const tabCounts = useMemo(
    () => ({ ...counts, php: ctx.websitesError ? null : phpCount }),
    [counts, phpCount, ctx.websitesError]
  );

  const select = (id: ProjectTypeId) => {
    const params = new URLSearchParams(Array.from(searchParams.entries()));
    params.set('type', projectType(id).slug);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">Websites</h1>
        <p className="mt-1 text-sm text-gray-600">
          {singleNode && localNode
            ? `Projects hosted on ${serverLabel(localNode)}, the machine this panel runs on.`
            : 'Projects hosted on the machines this panel manages.'}
        </p>
      </div>

      <ProjectTypeTabs
        types={PROJECT_TYPES}
        active={active}
        onSelect={select}
        counts={tabCounts}
      />

      <div
        role="tabpanel"
        id={`project-panel-${active}`}
        aria-labelledby={`project-tab-${active}`}
        className="space-y-4"
      >
        <p className="text-sm text-gray-600">{activeDef.summary}</p>

        {active === 'php' && (
          <PhpProjects servers={servers} defaultServerId={defaultId} ctx={ctx} />
        )}

        {active === 'nodejs' && (
          <NodeProjects
            servers={servers}
            defaultServerId={defaultId}
            ctx={ctx}
            onCount={setNodeCount}
          />
        )}

        {active === 'proxy' && (
          <ProxyProjects
            servers={servers}
            defaultServerId={defaultId}
            ctx={ctx}
            onCount={setProxyCount}
          />
        )}

        {active === 'go' && (
          <UnavailableProjectType
            title="Go projects"
            description="A compiled binary run as a service unit, with its working directory and port."
            icon={<Cpu size={40} aria-hidden="true" />}
            reason="The panel cannot manage a Go project because nothing in the API knows what one is. There is no route group, no model and no service, so there is no list to show and no form worth offering."
            requirements={GO_REQUIREMENTS}
            fields={[
              'Built binary',
              'Working directory',
              'Port',
              'Service unit',
              'Environment',
              'Status and logs',
            ]}
            notes={PROJECT_TYPE_GAPS.go}
          />
        )}

        {active === 'python' && (
          <UnavailableProjectType
            title="Python projects"
            description="A WSGI or ASGI application served out of a virtual environment."
            icon={<Terminal size={40} aria-hidden="true" />}
            reason="The panel cannot manage a Python project because nothing in the API knows what one is. There is no route group, no model and no service, so there is no list to show and no form worth offering."
            requirements={PYTHON_REQUIREMENTS}
            fields={[
              'Interpreter version',
              'Virtual environment path',
              'WSGI or ASGI server',
              'Entry module',
              'Port',
              'Status and logs',
            ]}
            notes={PROJECT_TYPE_GAPS.python}
          />
        )}
      </div>

      {serversLoading && (
        <p className="text-sm text-gray-500">Still reading the server list...</p>
      )}
    </div>
  );
}

/**
 * useSearchParams needs a Suspense boundary in the app router; without one the
 * whole route falls back to client rendering at build time and Next fails the
 * build rather than shipping it.
 */
export default function WebsitesPage() {
  return (
    <Suspense
      fallback={
        <div className="flex h-64 items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-b-brand-600" />
        </div>
      }
    >
      <WebsitesScreen />
    </Suspense>
  );
}
