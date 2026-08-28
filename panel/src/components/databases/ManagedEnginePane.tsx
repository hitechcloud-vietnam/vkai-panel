'use client';

/**
 * The shape of a pane for an engine the panel can actually drive.
 *
 * MySQL and PostgreSQL both get: the instances, the databases, the accounts,
 * whatever the engine adds on top, and the list of what is still missing. The
 * two engines fill that shape differently - a MySQL operator wants a character
 * set and a collation, a PostgreSQL operator wants roles and extensions - so
 * the differences arrive as props and as `children`, and the wiring to the four
 * real endpoints is written once.
 */

import { useState } from 'react';

import type { ManagedServer } from '@/types/server';
import type {
  CapabilityGap,
  CreateDatabasePayload,
  DBEntry,
  DBServer,
  DatabaseEngine,
} from '@/types/databases';

import AccountsPanel from './AccountsPanel';
import { CapabilityGaps } from './CapabilityNotice';
import ConfirmByName from './ConfirmByName';
import CreateDatabaseDialog from './CreateDatabaseDialog';
import InstancesPanel, { type InstancesPanelProps } from './InstancesPanel';
import ManagedDatabasesPanel from './ManagedDatabasesPanel';
import SetPasswordDialog from './SetPasswordDialog';

export interface ManagedEnginePaneProps {
  engine: DatabaseEngine;
  servers: DBServer[];
  entries: DBEntry[];
  nodes: ManagedServer[];
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  onRefresh: () => void;
  search: string;
  onSearchChange: (value: string) => void;
  sessionSecrets: Record<string, string>;

  onRegisterInstance: InstancesPanelProps['onRegister'];
  onDeleteInstance: InstancesPanelProps['onDelete'];
  onCreateDatabase: (payload: CreateDatabasePayload) => Promise<void>;
  onChangePassword: (entry: DBEntry, password: string) => Promise<void>;
  onDropDatabase: (entry: DBEntry) => Promise<void>;

  showCharsetColumns: boolean;
  extraColumnHeader?: string;
  extraColumnCell?: (entry: DBEntry) => React.ReactNode;

  accountScopeHeader: string;
  accountScopeCell: (entry: DBEntry) => string;
  accountGrantHeader: string;
  accountGrantCell: (entry: DBEntry) => string;
  accountCaveat: string;

  gaps: CapabilityGap[];
  /** Sections the engine adds between the accounts table and the gap list. */
  children?: React.ReactNode;
}

export function ManagedEnginePane({
  engine,
  servers,
  entries,
  nodes,
  loading,
  refreshing,
  error,
  onRefresh,
  search,
  onSearchChange,
  sessionSecrets,
  onRegisterInstance,
  onDeleteInstance,
  onCreateDatabase,
  onChangePassword,
  onDropDatabase,
  showCharsetColumns,
  extraColumnHeader,
  extraColumnCell,
  accountScopeHeader,
  accountScopeCell,
  accountGrantHeader,
  accountGrantCell,
  accountCaveat,
  gaps,
  children,
}: ManagedEnginePaneProps) {
  const [showCreate, setShowCreate] = useState(false);
  const [passwordTarget, setPasswordTarget] = useState<DBEntry | null>(null);
  const [dropTarget, setDropTarget] = useState<DBEntry | null>(null);
  const [dropping, setDropping] = useState(false);
  const [dropError, setDropError] = useState<string | null>(null);

  const databaseCounts: Record<string, number> = {};
  entries.forEach((entry) => {
    const key = String(entry.database_server_id || '');
    databaseCounts[key] = (databaseCounts[key] || 0) + 1;
  });

  const confirmDrop = async () => {
    if (!dropTarget) return;
    setDropping(true);
    setDropError(null);
    try {
      await onDropDatabase(dropTarget);
      setDropTarget(null);
    } catch (err) {
      setDropError(err instanceof Error ? err.message : String(err));
    } finally {
      setDropping(false);
    }
  };

  return (
    <div className="space-y-4">
      <InstancesPanel
        engine={engine}
        servers={servers}
        nodes={nodes}
        loading={loading}
        databaseCounts={databaseCounts}
        search={search}
        onSearchChange={onSearchChange}
        onRefresh={onRefresh}
        refreshing={refreshing}
        onRegister={onRegisterInstance}
        onDelete={onDeleteInstance}
      />

      <ManagedDatabasesPanel
        engine={engine}
        entries={entries}
        servers={servers}
        nodes={nodes}
        loading={loading}
        refreshing={refreshing}
        error={error}
        onRefresh={onRefresh}
        search={search}
        onSearchChange={onSearchChange}
        sessionSecrets={sessionSecrets}
        onCreate={() => setShowCreate(true)}
        onSetPassword={setPasswordTarget}
        onDelete={(entry) => {
          setDropError(null);
          setDropTarget(entry);
        }}
        showCharsetColumns={showCharsetColumns}
        extraColumnHeader={extraColumnHeader}
        extraColumnCell={extraColumnCell}
      />

      <AccountsPanel
        engine={engine}
        entries={entries}
        servers={servers}
        nodes={nodes}
        loading={loading}
        search={search}
        scopeHeader={accountScopeHeader}
        scopeCell={accountScopeCell}
        grantHeader={accountGrantHeader}
        grantCell={accountGrantCell}
        caveat={accountCaveat}
        onSetPassword={setPasswordTarget}
      />

      {children}

      <CapabilityGaps
        title="What this pane cannot do yet"
        intro="These are the controls an operator expects on this engine and the reason each one is absent. Nothing here is drawn as a button, because a button that answers 500 is worse than a gap that is written down."
        gaps={gaps}
      />

      <CreateDatabaseDialog
        open={showCreate}
        engine={engine}
        servers={servers}
        nodes={nodes}
        onClose={() => setShowCreate(false)}
        onCreate={onCreateDatabase}
      />

      <SetPasswordDialog
        open={Boolean(passwordTarget)}
        databaseName={passwordTarget?.name || ''}
        accountLabel={engine.accountNoun}
        accountName={passwordTarget?.username || ''}
        onClose={() => setPasswordTarget(null)}
        onSubmit={async (password) => {
          if (passwordTarget) await onChangePassword(passwordTarget, password);
        }}
      />

      <ConfirmByName
        open={Boolean(dropTarget)}
        title={`Drop ${dropTarget?.name || 'this database'}`}
        expected={dropTarget?.name || ''}
        consequence={`The database and its ${engine.accountNoun.toLowerCase()} are dropped on the server. There is no restore endpoint in this panel, so nothing here can bring the data back.`}
        confirmLabel="Drop database"
        busy={dropping}
        error={dropError}
        onCancel={() => setDropTarget(null)}
        onConfirm={confirmDrop}
      />
    </div>
  );
}

export default ManagedEnginePane;
