'use client';

/**
 * The accounts the panel created, and exactly what it knows about them.
 *
 * There is no account endpoint. Every row here is derived from a database entry,
 * because the service creates one account per database and nothing else:
 *
 *   MySQL       CREATE USER <name>@'localhost'; GRANT ALL PRIVILEGES ON <db>.*
 *   PostgreSQL  CREATE USER <name> WITH PASSWORD ...; GRANT ALL PRIVILEGES ON DATABASE <db>
 *
 * So "user@localhost with ALL PRIVILEGES on one database" is not a guess - it is
 * the only thing createMySQLDatabase can produce. What the panel genuinely does
 * not know is whether someone has since changed a grant by hand, and the note
 * under the table says so rather than implying the list is authoritative.
 */

import { useMemo } from 'react';
import { KeyRound, ShieldCheck } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import type { DBEntry, DBServer, DatabaseEngine } from '@/types/databases';
import type { ManagedServer } from '@/types/server';

import {
  EmptyState,
  Panel,
  SectionHeader,
  TableScroller,
  TableSkeleton,
  TD_CLASS,
  TH_CLASS,
} from './PaneChrome';
import { formatDate, matchesSearch, nodeName } from './helpers';

export interface AccountsPanelProps {
  engine: DatabaseEngine;
  entries: DBEntry[];
  servers: DBServer[];
  nodes: ManagedServer[];
  loading: boolean;
  search: string;
  /** MySQL says "user@host"; PostgreSQL says "role with attributes". */
  scopeHeader: string;
  scopeCell: (entry: DBEntry) => string;
  grantHeader: string;
  grantCell: (entry: DBEntry) => string;
  /** The sentence that says how far this list can be trusted. */
  caveat: string;
  onSetPassword: (entry: DBEntry) => void;
}

export function AccountsPanel({
  engine,
  entries,
  servers,
  nodes,
  loading,
  search,
  scopeHeader,
  scopeCell,
  grantHeader,
  grantCell,
  caveat,
  onSetPassword,
}: AccountsPanelProps) {
  const serverById = useMemo(() => {
    const map: Record<string, DBServer> = {};
    servers.forEach((s) => {
      map[String(s.id)] = s;
    });
    return map;
  }, [servers]);

  const visible = useMemo(
    () => entries.filter((entry) => matchesSearch(search, entry.username, entry.name)),
    [entries, search]
  );

  return (
    <Panel>
      <SectionHeader
        title={`${engine.accountNoun}s`}
        description={`One ${engine.accountNoun.toLowerCase()} per database, as the panel created it.`}
      />

      {loading ? (
        <TableSkeleton columns={5} rows={2} />
      ) : entries.length === 0 ? (
        <EmptyState
          title={`No ${engine.accountNoun.toLowerCase()} yet`}
          description={`A ${engine.accountNoun.toLowerCase()} is created together with its database. Create a database and it will appear here.`}
        />
      ) : visible.length === 0 ? (
        <EmptyState
          title="Nothing matches that search"
          description={`No ${engine.accountNoun.toLowerCase()} name or database name contains "${search}".`}
        />
      ) : (
        <>
          <TableScroller>
            <table className="w-full min-w-[720px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH_CLASS}>{engine.accountNoun}</th>
                  <th className={TH_CLASS}>{scopeHeader}</th>
                  <th className={TH_CLASS}>Database</th>
                  <th className={TH_CLASS}>{grantHeader}</th>
                  <th className={TH_CLASS}>Instance</th>
                  <th className={TH_CLASS}>Created</th>
                  <th className={cn(TH_CLASS, 'text-right')}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {visible.map((entry) => {
                  const server = serverById[String(entry.database_server_id)] || null;
                  return (
                    <tr key={entry.id} className="border-b border-gray-200 last:border-b-0">
                      <td className={cn(TD_CLASS, 'font-mono font-medium text-gray-900')}>
                        {entry.username}
                      </td>
                      <td className={cn(TD_CLASS, 'font-mono')}>{scopeCell(entry)}</td>
                      <td className={cn(TD_CLASS, 'font-mono')}>{entry.name}</td>
                      <td className={TD_CLASS}>
                        <span className="inline-flex items-center gap-1.5">
                          <ShieldCheck
                            className="h-4 w-4 text-gray-400"
                            aria-hidden="true"
                          />
                          {grantCell(entry)}
                        </span>
                      </td>
                      <td className={TD_CLASS}>{nodeName(server, nodes)}</td>
                      <td className={TD_CLASS}>{formatDate(entry.created_at)}</td>
                      <td className={cn(TD_CLASS, 'text-right')}>
                        <Button
                          type="button"
                          variant="ghost"
                          onClick={() => onSetPassword(entry)}
                          aria-label={`Set a new password for ${entry.username}`}
                          title="Set a new password"
                          className="h-8 w-8 p-0 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:ring-brand-500"
                        >
                          <KeyRound className="h-4 w-4" aria-hidden="true" />
                        </Button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </TableScroller>
          <p className="border-t border-gray-200 px-4 py-3 text-xs text-gray-500">
            {caveat}
          </p>
        </>
      )}
    </Panel>
  );
}

export default AccountsPanel;
