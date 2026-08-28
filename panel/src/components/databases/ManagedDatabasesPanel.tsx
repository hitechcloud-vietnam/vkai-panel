'use client';

/**
 * The databases the panel really created, for the two engines it really drives.
 *
 * MySQL and PostgreSQL share this table because the service creates them
 * through the same three steps - create the database, create the account, grant
 * one to the other - and differ only in what an operator calls the account and
 * in whether a character set means anything. Those two differences are props.
 * Everything else that looks like a difference between the engines lives in the
 * pane around this table, not inside it.
 */

import { Fragment, useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, KeyRound, Plus, Trash2 } from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Unavailable } from '@/components/Unavailable';
import { cn } from '@/lib/utils';
import type { ManagedServer } from '@/types/server';
import type { DBEntry, DBServer, DatabaseEngine } from '@/types/databases';

import ConnectionDetails from './ConnectionDetails';
import {
  EmptyState,
  ErrorBlock,
  Panel,
  PRIMARY_BUTTON_CLASS,
  PaneToolbar,
  SectionHeader,
  TableScroller,
  TableSkeleton,
  TD_CLASS,
  TH_CLASS,
} from './PaneChrome';
import {
  DASH,
  connectionString,
  formatDate,
  instanceHost,
  instancePort,
  matchesSearch,
  nodeName,
  statusBadgeVariant,
} from './helpers';

/**
 * The size column is never a number.
 *
 * models.DatabaseEntry carries a `size` field and the table has the column, but
 * nothing in the repository or the service ever writes it - there is no
 * UpdateEntrySize, and no collector that measures a database. The value is
 * therefore always 0, and printing "0 B" would tell an operator their database
 * is empty when the panel simply never looked. It renders as unavailable, with
 * the reason attached, which is the rule the rest of this panel already follows.
 */
const SIZE_REASON =
  'The API never measures database size: models.DatabaseEntry.size is written once as 0 and never updated.';

export interface ManagedDatabasesPanelProps {
  engine: DatabaseEngine;
  entries: DBEntry[];
  servers: DBServer[];
  nodes: ManagedServer[];
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  onRefresh: () => void;
  search: string;
  onSearchChange: (value: string) => void;
  /** Passwords typed in this browser session, keyed by database id. */
  sessionSecrets: Record<string, string>;
  onCreate: () => void;
  onSetPassword: (entry: DBEntry) => void;
  onDelete: (entry: DBEntry) => void;
  /** Whether the character set and collation columns say anything true. */
  showCharsetColumns: boolean;
  /** Replaces the charset columns on PostgreSQL, where they would mislead. */
  extraColumnHeader?: string;
  extraColumnCell?: (entry: DBEntry) => React.ReactNode;
}

export function ManagedDatabasesPanel({
  engine,
  entries,
  servers,
  nodes,
  loading,
  refreshing,
  error,
  onRefresh,
  search,
  onSearchChange,
  sessionSecrets,
  onCreate,
  onSetPassword,
  onDelete,
  showCharsetColumns,
  extraColumnHeader,
  extraColumnCell,
}: ManagedDatabasesPanelProps) {
  const [expanded, setExpanded] = useState<string | null>(null);

  const visible = useMemo(
    () =>
      entries.filter((entry) =>
        matchesSearch(search, entry.name, entry.username, entry.charset, entry.collation)
      ),
    [entries, search]
  );

  const serverById = useMemo(() => {
    const map: Record<string, DBServer> = {};
    servers.forEach((s) => {
      map[String(s.id)] = s;
    });
    return map;
  }, [servers]);

  /*
   * Nine fixed columns - expander, name, account, size, instance, status,
   * created, actions - plus the two charset columns and the one engine-specific
   * column when the pane asks for them. Kept as one number so the skeleton, the
   * header and the detail row's colSpan can never drift apart.
   */
  const totalColumns =
    8 + (showCharsetColumns ? 2 : 0) + (extraColumnHeader ? 1 : 0);

  const canCreate = servers.length > 0;

  return (
    <Panel>
      <SectionHeader
        title="Databases"
        description={`Created through the panel, each with its own ${engine.accountNoun.toLowerCase()} and grant.`}
        actions={
          <Button
            type="button"
            onClick={onCreate}
            disabled={!canCreate}
            title={
              canCreate
                ? undefined
                : `Register a ${engine.label} instance first - there is nowhere to create a database.`
            }
            className={PRIMARY_BUTTON_CLASS}
          >
            <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
            Create database
          </Button>
        }
      />

      <div className="border-b border-gray-200 px-4 py-3">
        <PaneToolbar
          search={search}
          onSearchChange={onSearchChange}
          searchPlaceholder={`Search ${engine.label} databases by name or ${engine.accountNoun.toLowerCase()}`}
          onRefresh={onRefresh}
          refreshing={refreshing}
        />
      </div>

      {error ? (
        <ErrorBlock
          title="The database list could not be loaded"
          message={error}
          onRetry={onRefresh}
        />
      ) : loading ? (
        <TableSkeleton columns={totalColumns} />
      ) : entries.length === 0 ? (
        <EmptyState
          title={`No ${engine.label} database yet`}
          description={
            canCreate
              ? `Create one and the panel will make the database, the ${engine.accountNoun.toLowerCase()} and the grant in a single step.`
              : `Register a ${engine.label} instance above first. A database needs somewhere to live.`
          }
          action={
            canCreate ? (
              <Button type="button" onClick={onCreate} className={PRIMARY_BUTTON_CLASS}>
                <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
                Create database
              </Button>
            ) : undefined
          }
        />
      ) : visible.length === 0 ? (
        <EmptyState
          title="Nothing matches that search"
          description={`No ${engine.label} database has a name or ${engine.accountNoun.toLowerCase()} containing "${search}". Clear the search to see all ${entries.length}.`}
          action={
            <Button
              type="button"
              variant="outline"
              onClick={() => onSearchChange('')}
              className="border border-gray-300 bg-white text-gray-700 hover:bg-gray-50"
            >
              Clear search
            </Button>
          }
        />
      ) : (
        <TableScroller>
          <table className="w-full min-w-[860px] border-collapse">
            <thead className="bg-gray-50">
              <tr className="border-b border-gray-200">
                <th className={cn(TH_CLASS, 'w-10')}>
                  <span className="sr-only">Expand</span>
                </th>
                <th className={TH_CLASS}>Name</th>
                <th className={TH_CLASS}>{engine.accountNoun}</th>
                {showCharsetColumns && <th className={TH_CLASS}>Character set</th>}
                {showCharsetColumns && <th className={TH_CLASS}>Collation</th>}
                {extraColumnHeader && <th className={TH_CLASS}>{extraColumnHeader}</th>}
                <th className={TH_CLASS}>Size</th>
                <th className={TH_CLASS}>Instance</th>
                <th className={TH_CLASS}>Status</th>
                <th className={TH_CLASS}>Created</th>
                <th className={cn(TH_CLASS, 'text-right')}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((entry) => {
                const server = serverById[String(entry.database_server_id)] || null;
                const host = instanceHost(server, nodes);
                const port = instancePort(server, engine);
                const open = expanded === entry.id;
                return (
                  <Fragment key={entry.id}>
                    <tr className="border-b border-gray-200">
                      <td className={cn(TD_CLASS, 'pr-0')}>
                        <button
                          type="button"
                          onClick={() => setExpanded(open ? null : entry.id)}
                          aria-expanded={open}
                          aria-label={
                            open
                              ? `Hide connection details for ${entry.name}`
                              : `Show connection details for ${entry.name}`
                          }
                          className="rounded-md p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                        >
                          {open ? (
                            <ChevronDown className="h-4 w-4" aria-hidden="true" />
                          ) : (
                            <ChevronRight className="h-4 w-4" aria-hidden="true" />
                          )}
                        </button>
                      </td>
                      <td className={cn(TD_CLASS, 'font-mono font-medium text-gray-900')}>
                        {entry.name}
                      </td>
                      <td className={cn(TD_CLASS, 'font-mono')}>{entry.username}</td>
                      {showCharsetColumns && (
                        <td className={TD_CLASS}>{entry.charset || DASH}</td>
                      )}
                      {showCharsetColumns && (
                        <td className={TD_CLASS}>{entry.collation || DASH}</td>
                      )}
                      {extraColumnHeader && (
                        <td className={TD_CLASS}>{extraColumnCell?.(entry)}</td>
                      )}
                      <td className={TD_CLASS}>
                        <Unavailable reason={SIZE_REASON} />
                      </td>
                      <td className={TD_CLASS}>
                        {server ? nodeName(server, nodes) : DASH}
                      </td>
                      <td className={TD_CLASS}>
                        <Badge variant={statusBadgeVariant(entry.status)}>
                          {entry.status || 'unknown'}
                        </Badge>
                      </td>
                      <td className={TD_CLASS}>{formatDate(entry.created_at)}</td>
                      <td className={cn(TD_CLASS, 'text-right')}>
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            type="button"
                            variant="ghost"
                            onClick={() => onSetPassword(entry)}
                            aria-label={`Set a new password for ${entry.name}`}
                            title="Set a new password"
                            className="h-8 w-8 p-0 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:ring-brand-500"
                          >
                            <KeyRound className="h-4 w-4" aria-hidden="true" />
                          </Button>
                          <Button
                            type="button"
                            variant="ghost"
                            onClick={() => onDelete(entry)}
                            aria-label={`Drop ${entry.name}`}
                            title="Drop this database"
                            className="h-8 w-8 p-0 text-red-600 hover:bg-red-50 hover:text-red-700 focus-visible:ring-red-500"
                          >
                            <Trash2 className="h-4 w-4" aria-hidden="true" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                    {open && (
                      <tr className="border-b border-gray-200">
                        <td colSpan={totalColumns} className="bg-gray-50 px-4 py-4">
                          <div className="max-w-xl">
                            <ConnectionDetails
                              host={host}
                              port={String(port)}
                              database={entry.name}
                              accountLabel={engine.accountNoun}
                              account={entry.username}
                              sessionSecret={sessionSecrets[entry.id]}
                              connectionString={connectionString(
                                engine.id,
                                host,
                                port,
                                entry.name,
                                entry.username
                              )}
                              onSetPassword={() => onSetPassword(entry)}
                            />
                          </div>
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
  );
}

export default ManagedDatabasesPanel;
