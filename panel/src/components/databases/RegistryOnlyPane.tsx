'use client';

/**
 * The pane for an engine the panel can inventory but cannot manage.
 *
 * DatabaseService.CreateDatabase switches on the instance type and has exactly
 * two arms, "mysql" and "postgresql". Everything else falls to:
 *
 *     default:
 *         return nil, fmt.Errorf("unsupported database type: %s", dbServer.Type)
 *
 * So the tab exists - hiding it would leave an operator guessing whether the
 * panel forgot SQL Server or never had it - and it offers the one thing that is
 * genuinely wired: registering the instance against a node. Every other control
 * an operator would expect is listed as a gap with the reason, instead of being
 * drawn as a button that answers 500.
 */

import { AlertTriangle } from 'lucide-react';

import type { ManagedServer } from '@/types/server';
import type { CapabilityGap, DBEntry, DBServer, DatabaseEngine } from '@/types/databases';

import { EngineUnsupportedNotice } from './CapabilityNotice';
import InstancesPanel, { type InstancesPanelProps } from './InstancesPanel';

export interface RegistryOnlyPaneProps {
  engine: DatabaseEngine;
  servers: DBServer[];
  /** Rows recorded against these instances. Normally none; see the warning below. */
  entries: DBEntry[];
  nodes: ManagedServer[];
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  onRefresh: () => void;
  search: string;
  onSearchChange: (value: string) => void;
  wouldShow: string[];
  gaps: CapabilityGap[];
  onRegister: InstancesPanelProps['onRegister'];
  onDeleteInstance: InstancesPanelProps['onDelete'];
}

export function RegistryOnlyPane({
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
  wouldShow,
  gaps,
  onRegister,
  onDeleteInstance,
}: RegistryOnlyPaneProps) {
  const databaseCounts: Record<string, number> = {};
  entries.forEach((entry) => {
    const key = String(entry.database_server_id || '');
    databaseCounts[key] = (databaseCounts[key] || 0) + 1;
  });

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
        error={error}
        onRegister={onRegister}
        onDelete={onDeleteInstance}
      />

      {entries.length > 0 && (
        <div
          role="alert"
          className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 p-4"
        >
          <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" aria-hidden="true" />
          <div>
            <p className="text-sm font-semibold text-amber-900">
              {entries.length} database row(s) are recorded against a {engine.label}{' '}
              instance
            </p>
            <p className="mt-1 text-sm text-amber-800">
              The panel cannot have created these, and deleting one removes only the
              panel&apos;s record - nothing is dropped on the server. Treat them as
              bookkeeping until {engine.label} management lands:{' '}
              <span className="font-mono text-xs">
                {entries.map((e) => e.name).join(', ')}
              </span>
              .
            </p>
          </div>
        </div>
      )}

      <EngineUnsupportedNotice
        engineLabel={engine.label}
        wouldShow={wouldShow}
        gaps={gaps}
      />
    </div>
  );
}

export default RegistryOnlyPane;
