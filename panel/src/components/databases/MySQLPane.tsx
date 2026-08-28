'use client';

/**
 * MySQL and MariaDB.
 *
 * The one engine the service drives most completely: it creates the database
 * with a character set and a collation from a fixed allowlist, creates the user
 * on localhost, grants it everything on that one database, and can re-password
 * it later. Those four things are real, so they are controls. The character set
 * and collation columns appear here and nowhere else, because this is the only
 * engine where the stored values were actually applied.
 */

import type { DBEntry } from '@/types/databases';

import ManagedEnginePane, { type ManagedEnginePaneProps } from './ManagedEnginePane';
import { MYSQL_GAPS } from './gaps';

export type MySQLPaneProps = Pick<
  ManagedEnginePaneProps,
  | 'engine'
  | 'servers'
  | 'entries'
  | 'nodes'
  | 'loading'
  | 'refreshing'
  | 'error'
  | 'onRefresh'
  | 'search'
  | 'onSearchChange'
  | 'sessionSecrets'
  | 'onRegisterInstance'
  | 'onDeleteInstance'
  | 'onCreateDatabase'
  | 'onChangePassword'
  | 'onDropDatabase'
>;

export function MySQLPane(props: MySQLPaneProps) {
  return (
    <ManagedEnginePane
      {...props}
      showCharsetColumns
      accountScopeHeader="Access host"
      // createMySQLDatabase hard-codes @'localhost'. It is not a stored field,
      // and nothing in the panel can change it, so the column states the fact
      // rather than reading a value that does not exist.
      accountScopeCell={() => 'localhost'}
      accountGrantHeader="Grant"
      accountGrantCell={(entry: DBEntry) => `ALL PRIVILEGES ON ${entry.name}.*`}
      accountCaveat="Derived from what the panel issued at creation: one user on localhost with ALL PRIVILEGES on its own database. The panel cannot read the live grant table, so a privilege changed by hand on the server will not show here."
      gaps={MYSQL_GAPS}
    />
  );
}

export default MySQLPane;
