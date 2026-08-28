'use client';

/**
 * PostgreSQL.
 *
 * A Postgres operator does not look for "users". They look for roles, for the
 * attributes on those roles, for the extensions installed in a database and for
 * the schemas inside it. This pane is arranged around those four words even
 * where the backend can only answer the first one, because arranging it around
 * MySQL's vocabulary and then relabelling the columns is how a panel ends up
 * feeling like it was written for a different product.
 *
 * Two deliberate absences:
 *
 *  - No character set or collation columns. createPostgresDatabase runs
 *    `createdb` with no encoding flags, and CreateDatabase then stores the
 *    MySQL defaults utf8mb4 / utf8mb4_unicode_ci on the row whatever the engine
 *    was. Printing those as if they were PostgreSQL settings would be printing
 *    a value the server never saw.
 *
 *  - The Extensions and Schemas sections carry no data, and say why. Rendering
 *    an empty list would read as "this database has no extensions", which is
 *    false: every PostgreSQL database has at least plpgsql.
 */

import { Blocks, FolderTree } from 'lucide-react';

import type { DBEntry } from '@/types/databases';

import ManagedEnginePane, { type ManagedEnginePaneProps } from './ManagedEnginePane';
import { Panel, SectionHeader } from './PaneChrome';
import { POSTGRES_GAPS } from './gaps';

export type PostgresPaneProps = Pick<
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

/** A section that exists so the reader knows it is missing, not empty. */
function UnknownSection({
  title,
  description,
  icon,
  body,
}: {
  title: string;
  description: string;
  icon: React.ReactNode;
  body: string;
}) {
  return (
    <Panel>
      <SectionHeader title={title} description={description} />
      <div className="flex items-start gap-3 px-4 py-6">
        <span className="mt-0.5 text-gray-300" aria-hidden="true">
          {icon}
        </span>
        <p className="max-w-2xl text-sm text-gray-500">{body}</p>
      </div>
    </Panel>
  );
}

export function PostgresPane(props: PostgresPaneProps) {
  return (
    <ManagedEnginePane
      {...props}
      showCharsetColumns={false}
      extraColumnHeader="Owner"
      // createPostgresDatabase grants the new role ALL PRIVILEGES on the new
      // database, so the role the panel made is the one an operator will treat
      // as the owner. The database is created by the `postgres` superuser, so
      // this is the effective owner rather than pg_database.datdba.
      extraColumnCell={(entry: DBEntry) => (
        <span className="font-mono">{entry.username}</span>
      )}
      accountScopeHeader="Attributes"
      accountScopeCell={() => 'LOGIN'}
      accountGrantHeader="Privileges"
      accountGrantCell={(entry: DBEntry) => `ALL PRIVILEGES ON DATABASE ${entry.name}`}
      accountCaveat="Derived from the CREATE USER and GRANT the panel issued. Every role it creates is a plain LOGIN role - it never sets SUPERUSER, CREATEDB, CREATEROLE, a connection limit or an expiry, and it cannot read pg_roles to report one set by hand."
      gaps={POSTGRES_GAPS}
    >
      <UnknownSection
        title="Extensions"
        description="What is installed in each database."
        icon={<Blocks className="h-5 w-5" />}
        body="The panel cannot list extensions. No route reads pg_extension or runs CREATE EXTENSION, so it can neither confirm that postgis is installed nor deny it. An empty list would say the wrong thing: every PostgreSQL database has at least plpgsql. Use psql and \dx until this is wired."
      />

      <UnknownSection
        title="Schemas"
        description="The namespaces inside each database."
        icon={<FolderTree className="h-5 w-5" />}
        body="The panel cannot list schemas. No route reads information_schema.schemata or runs CREATE SCHEMA. A database created here has whatever the template gave it - normally just public - but the panel has no way to confirm that, or to show a schema someone added later."
      />
    </ManagedEnginePane>
  );
}

export default PostgresPane;
