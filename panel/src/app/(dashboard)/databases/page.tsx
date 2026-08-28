'use client';

/**
 * Databases, one managed layout per engine.
 *
 * The screen is arranged the way aaPanel arranges its own - a strip of engines
 * across the top, one pane below - because that is the shape operators coming
 * from aaPanel already know. Three things are done differently, and all three
 * are deliberate.
 *
 * 1. The selected engine lives in the URL. aaPanel keeps it in the DOM, so a
 *    reload drops you back on MySQL and there is nothing to bookmark or paste
 *    into a ticket. Here `?engine=postgresql` is a link.
 *
 * 2. Each pane is built for its engine rather than for a generic table. Redis
 *    in particular is not a list of databases with a size column; it is memory,
 *    a policy, a persistence mode and sixteen numbered slots.
 *
 * 3. A control appears only where an endpoint exists behind it. The backend
 *    manages exactly two engines - DatabaseService.CreateDatabase switches on
 *    the instance type and has arms for "mysql" and "postgresql" and a default
 *    that returns "unsupported database type" - so SQL Server, MongoDB and
 *    Redis get an honest pane that says what is missing and names it, instead
 *    of buttons that would answer 500.
 *
 * Endpoints this page uses, all of them verified in
 * core/internal/handler/router.go:
 *
 *   GET    /api/v1/databases/servers
 *   POST   /api/v1/databases/servers
 *   DELETE /api/v1/databases/servers/:id
 *   GET    /api/v1/databases
 *   POST   /api/v1/databases
 *   DELETE /api/v1/databases/:id
 *   POST   /api/v1/databases/:id/change-password
 *   GET    /api/v1/servers                        (to name the node an instance runs on)
 *
 * It calls nothing else, because there is nothing else.
 */

import { Suspense, useCallback, useEffect, useMemo, useState } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { CheckCircle2, Database, X } from 'lucide-react';

import { api, unwrap, unwrapList } from '@/services/api';
import { errorMessage } from '@/lib/apiError';
import type { ManagedServer } from '@/types/server';
import {
  DATABASE_ENGINES,
  DEFAULT_ENGINE,
  engineById,
  parseEngineId,
  type CreateDatabasePayload,
  type DBEntry,
  type DBServer,
  type DatabaseEngineId,
} from '@/types/databases';

import EngineTabs from '@/components/databases/EngineTabs';
import MySQLPane from '@/components/databases/MySQLPane';
import PostgresPane from '@/components/databases/PostgresPane';
import RedisPane from '@/components/databases/RedisPane';
import RegistryOnlyPane from '@/components/databases/RegistryOnlyPane';
import { TableSkeleton } from '@/components/databases/PaneChrome';
import {
  MONGODB_GAPS,
  MONGODB_WOULD_SHOW,
  SQLSERVER_GAPS,
  SQLSERVER_WOULD_SHOW,
} from '@/components/databases/gaps';
import { entriesForEngine, serversForEngine } from '@/components/databases/helpers';

/** The query parameter that carries the selected engine. */
const ENGINE_PARAM = 'engine';

function DatabasesScreen() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const activeEngineId: DatabaseEngineId = parseEngineId(searchParams.get(ENGINE_PARAM));
  const activeEngine = engineById(activeEngineId);

  const [servers, setServers] = useState<DBServer[]>([]);
  const [entries, setEntries] = useState<DBEntry[]>([]);
  const [nodes, setNodes] = useState<ManagedServer[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  /**
   * Passwords typed in THIS browser tab, keyed by database id.
   *
   * The API returns no password ever - models.DatabaseEntry hides the field
   * from JSON - so this is the only window in which the panel can show one.
   * It is deliberately not persisted anywhere: no localStorage, no cookie,
   * nothing that survives the tab.
   */
  const [sessionSecrets, setSessionSecrets] = useState<Record<string, string>>({});

  /** Search term per engine, so switching tabs does not carry a filter across. */
  const [searchByEngine, setSearchByEngine] = useState<Record<string, string>>({});
  const search = searchByEngine[activeEngineId] || '';
  const setSearch = useCallback(
    (value: string) => {
      setSearchByEngine((prev) => ({ ...prev, [activeEngineId]: value }));
    },
    [activeEngineId]
  );

  const selectEngine = useCallback(
    (id: DatabaseEngineId) => {
      const params = new URLSearchParams(searchParams.toString());
      if (id === DEFAULT_ENGINE) {
        params.delete(ENGINE_PARAM);
      } else {
        params.set(ENGINE_PARAM, id);
      }
      const query = params.toString();
      router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
    },
    [router, pathname, searchParams]
  );

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    else setLoading(true);
    setError(null);
    try {
      const [serverRes, entryRes, nodeRes] = await Promise.all([
        api.get('/api/v1/databases/servers'),
        api.get('/api/v1/databases'),
        // GET /api/v1/servers is paginated. Read it through unwrapList or the
        // machine the panel runs on never reaches the instance table.
        api.get('/api/v1/servers', { params: { page: 1, per_page: 200 } }),
      ]);
      setServers(unwrapList<DBServer>(serverRes));
      setEntries(unwrapList<DBEntry>(entryRes));
      setNodes(unwrapList<ManagedServer>(nodeRes));
    } catch (err) {
      setServers([]);
      setEntries([]);
      setNodes([]);
      setError(errorMessage(err, 'The database list could not be loaded.'));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!toast) return;
    const timer = setTimeout(() => setToast(null), 5000);
    return () => clearTimeout(timer);
  }, [toast]);

  // -------------------------------------------------------------------------
  // Actions. Each one throws a plain Error carrying the API's own message, so
  // the dialog that called it can show what the server said rather than a
  // generic failure - and so the object-in-state bug that lib/apiError exists
  // to prevent cannot come back through this page.
  // -------------------------------------------------------------------------

  const registerInstance = useCallback(
    async (payload: { server_id: string; type: string; version: string; port: number }) => {
      try {
        await api.post('/api/v1/databases/servers', payload);
      } catch (err) {
        throw new Error(errorMessage(err, 'The instance could not be registered.'));
      }
      setToast('Instance registered.');
      await load(true);
    },
    [load]
  );

  const deleteInstance = useCallback(
    async (server: DBServer) => {
      try {
        await api.delete(`/api/v1/databases/servers/${server.id}`);
      } catch (err) {
        throw new Error(errorMessage(err, 'The instance could not be removed.'));
      }
      setToast('Instance removed from the panel.');
      await load(true);
    },
    [load]
  );

  const createDatabase = useCallback(
    async (payload: CreateDatabasePayload) => {
      let created: DBEntry | null = null;
      try {
        const res = await api.post('/api/v1/databases', payload);
        created = unwrap<DBEntry>(res);
      } catch (err) {
        throw new Error(errorMessage(err, 'The database could not be created.'));
      }
      // Hold the password for this tab only. It is the single moment the panel
      // will ever be able to show it.
      if (created?.id) {
        setSessionSecrets((prev) => ({ ...prev, [created!.id]: payload.password }));
      }
      setToast(`Database ${payload.name} created.`);
      await load(true);
    },
    [load]
  );

  const changePassword = useCallback(
    async (entry: DBEntry, password: string) => {
      try {
        await api.post(`/api/v1/databases/${entry.id}/change-password`, { password });
      } catch (err) {
        throw new Error(errorMessage(err, 'The password could not be changed.'));
      }
      setSessionSecrets((prev) => ({ ...prev, [entry.id]: password }));
      setToast(`Password changed for ${entry.name}.`);
      await load(true);
    },
    [load]
  );

  const dropDatabase = useCallback(
    async (entry: DBEntry) => {
      try {
        await api.delete(`/api/v1/databases/${entry.id}`);
      } catch (err) {
        throw new Error(errorMessage(err, 'The database could not be dropped.'));
      }
      setSessionSecrets((prev) => {
        const next = { ...prev };
        delete next[entry.id];
        return next;
      });
      setToast(`Database ${entry.name} dropped.`);
      await load(true);
    },
    [load]
  );

  // -------------------------------------------------------------------------
  // Per-engine slices
  // -------------------------------------------------------------------------

  const engineServers = useMemo(
    () => serversForEngine(servers, activeEngine),
    [servers, activeEngine]
  );
  const engineEntries = useMemo(
    () => entriesForEngine(entries, engineServers),
    [entries, engineServers]
  );

  /** The number on each tab: databases where the engine is managed, instances where it is not. */
  const counts = useMemo(() => {
    const out: Partial<Record<DatabaseEngineId, number>> = {};
    DATABASE_ENGINES.forEach((engine) => {
      const own = serversForEngine(servers, engine);
      out[engine.id] =
        engine.support === 'managed' ? entriesForEngine(entries, own).length : own.length;
    });
    return out;
  }, [servers, entries]);

  const sharedPaneProps = {
    engine: activeEngine,
    servers: engineServers,
    nodes,
    loading,
    refreshing,
    error,
    onRefresh: () => load(true),
    search,
    onSearchChange: setSearch,
  };

  const managedPaneProps = {
    ...sharedPaneProps,
    entries: engineEntries,
    sessionSecrets,
    onRegisterInstance: registerInstance,
    onDeleteInstance: deleteInstance,
    onCreateDatabase: createDatabase,
    onChangePassword: changePassword,
    onDropDatabase: dropDatabase,
  };

  return (
    <div className="min-h-full bg-gray-50">
      <div className="mx-auto max-w-7xl space-y-4 px-4 py-6 sm:px-6 lg:px-8">
        <header className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 className="flex items-center gap-2 text-xl font-semibold text-gray-900">
              <Database className="h-5 w-5 text-brand-600" aria-hidden="true" />
              Databases
            </h1>
            <p className="mt-1 text-sm text-gray-600">
              One pane per engine. MySQL and PostgreSQL are managed by the panel; the
              rest are recorded but not driven, and each pane says so.
            </p>
          </div>
        </header>

        <EngineTabs active={activeEngineId} onSelect={selectEngine} counts={counts} />

        <div
          role="tabpanel"
          id={`engine-panel-${activeEngineId}`}
          aria-labelledby={`engine-tab-${activeEngineId}`}
        >
          <p className="mb-4 text-sm text-gray-600">{activeEngine.blurb}</p>

          {activeEngineId === 'mysql' && <MySQLPane {...managedPaneProps} />}

          {activeEngineId === 'postgresql' && <PostgresPane {...managedPaneProps} />}

          {activeEngineId === 'redis' && (
            <RedisPane
              {...sharedPaneProps}
              onRegister={registerInstance}
              onDeleteInstance={deleteInstance}
            />
          )}

          {activeEngineId === 'sqlserver' && (
            <RegistryOnlyPane
              {...sharedPaneProps}
              entries={engineEntries}
              wouldShow={SQLSERVER_WOULD_SHOW}
              gaps={SQLSERVER_GAPS}
              onRegister={registerInstance}
              onDeleteInstance={deleteInstance}
            />
          )}

          {activeEngineId === 'mongodb' && (
            <RegistryOnlyPane
              {...sharedPaneProps}
              entries={engineEntries}
              wouldShow={MONGODB_WOULD_SHOW}
              gaps={MONGODB_GAPS}
              onRegister={registerInstance}
              onDeleteInstance={deleteInstance}
            />
          )}
        </div>
      </div>

      {toast && (
        <div
          role="status"
          className="fixed bottom-4 right-4 z-50 flex items-start gap-3 rounded-md border border-emerald-200 bg-white px-4 py-3 shadow-sm"
        >
          <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-emerald-600" aria-hidden="true" />
          <p className="text-sm text-gray-900">{toast}</p>
          <button
            type="button"
            onClick={() => setToast(null)}
            aria-label="Dismiss"
            className="rounded-md p-0.5 text-gray-400 hover:bg-gray-100 hover:text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      )}
    </div>
  );
}

/**
 * useSearchParams opts a route into client rendering, and Next refuses to build
 * the page unless the boundary is explicit. The fallback is the same skeleton
 * the panes use, so a slow first paint looks like a loading table rather than a
 * blank screen.
 */
export default function DatabasesPage() {
  return (
    <Suspense
      fallback={
        <div className="min-h-full bg-gray-50">
          <div className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
            <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
              <TableSkeleton columns={6} rows={5} />
            </div>
          </div>
        </div>
      }
    >
      <DatabasesScreen />
    </Suspense>
  );
}
