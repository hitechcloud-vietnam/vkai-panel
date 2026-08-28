'use client';

/**
 * The instances of one engine that the panel knows about.
 *
 * Registering an instance is real for EVERY engine: POST
 * /api/v1/databases/servers stores whatever `type` it is given and never checks
 * it. That is why a Redis or a SQL Server tab still has something honest to
 * offer - the inventory - even though nothing can be created inside those
 * instances. The distinction is stated in the pane, not hidden.
 */

import { useEffect, useMemo, useState } from 'react';
import { Loader2, Plus, Server, Trash2 } from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import ServerScopeField, {
  SERVER_SCOPE_COPY_EN,
} from '@/components/servers/ServerScopeField';
import { defaultServerId } from '@/lib/servers';
import { cn } from '@/lib/utils';
import type { ManagedServer } from '@/types/server';
import type { DBServer, DatabaseEngine } from '@/types/databases';

import ConfirmByName from './ConfirmByName';
import Modal from './Modal';
import {
  EmptyState,
  ErrorBlock,
  PANE_INPUT_CLASS,
  Panel,
  PRIMARY_BUTTON_CLASS,
  PaneToolbar,
  SECONDARY_BUTTON_CLASS,
  SectionHeader,
  TableScroller,
  TableSkeleton,
  TD_CLASS,
  TH_CLASS,
} from './PaneChrome';
import {
  DASH,
  formatDate,
  instanceHost,
  instancePort,
  matchesSearch,
  nodeName,
  statusBadgeVariant,
} from './helpers';

export interface InstancesPanelProps {
  engine: DatabaseEngine;
  servers: DBServer[];
  nodes: ManagedServer[];
  loading: boolean;
  /** Databases per instance id, so a delete can warn about what it takes with it. */
  databaseCounts: Record<string, number>;
  /**
   * The four states every pane owes an operator. On the engines the backend
   * cannot manage, this table IS the pane, so the search box, the refresh and
   * the failure message live here rather than in a wrapper none of them share.
   */
  search: string;
  onSearchChange: (value: string) => void;
  onRefresh: () => void;
  refreshing?: boolean;
  error?: string | null;
  onRegister: (payload: {
    server_id: string;
    type: string;
    version: string;
    port: number;
  }) => Promise<void>;
  onDelete: (server: DBServer) => Promise<void>;
}

export function InstancesPanel({
  engine,
  servers,
  nodes,
  loading,
  databaseCounts,
  search,
  onSearchChange,
  onRefresh,
  refreshing,
  error,
  onRegister,
  onDelete,
}: InstancesPanelProps) {
  const [showForm, setShowForm] = useState(false);
  const [nodeId, setNodeId] = useState('');
  const [version, setVersion] = useState('');
  const [port, setPort] = useState(String(engine.defaultPort));
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const [deleteTarget, setDeleteTarget] = useState<DBServer | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  // A one-node panel never renders a picker, so the default has to be set here
  // or the form submits an empty server_id and the API answers 400.
  useEffect(() => {
    if (showForm) {
      setNodeId(defaultServerId(nodes));
      setVersion('');
      setPort(String(engine.defaultPort));
      setFormError(null);
    }
  }, [showForm, nodes, engine.defaultPort]);

  const portNumber = Number(port);
  const portValid = Number.isInteger(portNumber) && portNumber > 0 && portNumber <= 65535;
  const canSubmit = Boolean(nodeId) && portValid && !saving;

  const submit = async () => {
    if (!canSubmit) return;
    setSaving(true);
    setFormError(null);
    try {
      await onRegister({
        server_id: nodeId,
        type: engine.defaultServerType,
        version: version.trim(),
        port: portNumber,
      });
      setShowForm(false);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const visible = useMemo(
    () =>
      servers.filter((server) =>
        matchesSearch(
          search,
          nodeName(server, nodes),
          server.type,
          server.version,
          instanceHost(server, nodes),
          String(server.port || '')
        )
      ),
    [servers, nodes, search]
  );

  const deleteName = useMemo(() => {
    if (!deleteTarget) return '';
    return `${nodeName(deleteTarget, nodes)}:${instancePort(deleteTarget, engine)}`;
  }, [deleteTarget, nodes, engine]);

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await onDelete(deleteTarget);
      setDeleteTarget(null);
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : String(err));
    } finally {
      setDeleting(false);
    }
  };

  return (
    <>
      <Panel>
        <SectionHeader
          title={`${engine.label} instances`}
          description="Where the panel believes this engine is listening, and on which node."
          actions={
            <Button
              type="button"
              onClick={() => setShowForm(true)}
              className={PRIMARY_BUTTON_CLASS}
            >
              <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
              Register instance
            </Button>
          }
        />

        <div className="border-b border-gray-200 px-4 py-3">
          <PaneToolbar
            search={search}
            onSearchChange={onSearchChange}
            searchPlaceholder={`Search ${engine.label} instances by node, address or version`}
            onRefresh={onRefresh}
            refreshing={refreshing}
          />
        </div>

        {error ? (
          <ErrorBlock
            title="The instance list could not be loaded"
            message={error}
            onRetry={onRefresh}
          />
        ) : loading ? (
          <TableSkeleton columns={6} rows={2} />
        ) : servers.length === 0 ? (
          <EmptyState
            title={`No ${engine.label} instance is registered`}
            description={`Register the instance the panel should talk to. Until then this tab has nothing to list, and no ${engine.label} database can be created.`}
            action={
              <Button
                type="button"
                onClick={() => setShowForm(true)}
                className={PRIMARY_BUTTON_CLASS}
              >
                <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
                Register instance
              </Button>
            }
          />
        ) : visible.length === 0 ? (
          <EmptyState
            title="Nothing matches that search"
            description={`No registered ${engine.label} instance matches "${search}". Clear the search to see all ${servers.length}.`}
            action={
              <Button
                type="button"
                variant="outline"
                onClick={() => onSearchChange('')}
                className={SECONDARY_BUTTON_CLASS}
              >
                Clear search
              </Button>
            }
          />
        ) : (
          <TableScroller>
            <table className="w-full min-w-[720px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH_CLASS}>Node</th>
                  <th className={TH_CLASS}>Address</th>
                  <th className={TH_CLASS}>Version</th>
                  <th className={TH_CLASS}>Databases</th>
                  <th className={TH_CLASS}>Status</th>
                  <th className={TH_CLASS}>Registered</th>
                  <th className={cn(TH_CLASS, 'text-right')}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {visible.map((server) => (
                  <tr key={server.id} className="border-b border-gray-200 last:border-b-0">
                    <td className={cn(TD_CLASS, 'font-medium text-gray-900')}>
                      <span className="flex items-center gap-2">
                        <Server className="h-4 w-4 text-gray-400" aria-hidden="true" />
                        {nodeName(server, nodes)}
                      </span>
                    </td>
                    <td className={cn(TD_CLASS, 'font-mono')}>
                      {instanceHost(server, nodes)}:{instancePort(server, engine)}
                    </td>
                    <td className={TD_CLASS}>{server.version?.trim() || DASH}</td>
                    <td className={TD_CLASS}>{databaseCounts[server.id] ?? 0}</td>
                    <td className={TD_CLASS}>
                      <Badge variant={statusBadgeVariant(server.status)}>
                        {server.status || 'unknown'}
                      </Badge>
                    </td>
                    <td className={TD_CLASS}>{formatDate(server.created_at)}</td>
                    <td className={cn(TD_CLASS, 'text-right')}>
                      <Button
                        type="button"
                        variant="ghost"
                        onClick={() => {
                          setDeleteError(null);
                          setDeleteTarget(server);
                        }}
                        aria-label={`Remove ${nodeName(server, nodes)} instance`}
                        className="h-8 w-8 p-0 text-red-600 hover:bg-red-50 hover:text-red-700 focus-visible:ring-red-500"
                      >
                        <Trash2 className="h-4 w-4" aria-hidden="true" />
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>

      <Modal
        open={showForm}
        title={`Register a ${engine.label} instance`}
        description="The panel records where the instance runs. It does not install or start it."
        onClose={() => setShowForm(false)}
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setShowForm(false)}
              disabled={saving}
              className={SECONDARY_BUTTON_CLASS}
            >
              Cancel
            </Button>
            <Button
              type="button"
              onClick={submit}
              disabled={!canSubmit}
              className={PRIMARY_BUTTON_CLASS}
            >
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />}
              Register
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <ServerScopeField
            id="db-instance-node"
            servers={nodes}
            value={nodeId}
            onChange={setNodeId}
            copy={SERVER_SCOPE_COPY_EN}
          />

          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <label
                htmlFor="db-instance-port"
                className="mb-1.5 block text-sm font-medium text-gray-700"
              >
                Port
              </label>
              <Input
                id="db-instance-port"
                inputMode="numeric"
                value={port}
                onChange={(e) => setPort(e.target.value)}
                className={PANE_INPUT_CLASS}
              />
              {!portValid && port.length > 0 && (
                <p className="mt-1 text-xs text-red-600">
                  A port must be a whole number between 1 and 65535.
                </p>
              )}
            </div>
            <div>
              <label
                htmlFor="db-instance-version"
                className="mb-1.5 block text-sm font-medium text-gray-700"
              >
                Version <span className="font-normal text-gray-400">(optional)</span>
              </label>
              <Input
                id="db-instance-version"
                value={version}
                onChange={(e) => setVersion(e.target.value)}
                placeholder="For example 8.0.36"
                className={PANE_INPUT_CLASS}
              />
            </div>
          </div>

          <p className="rounded-md border border-gray-200 bg-gray-50 p-3 text-xs text-gray-600">
            The instance is recorded with type{' '}
            <span className="font-mono">{engine.defaultServerType}</span>.
            {engine.support === 'managed'
              ? ' Databases can be created on it from this tab.'
              : ' The panel cannot create databases on this engine yet, so the instance is inventory only.'}
          </p>

          {formError && (
            <p className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
              {formError}
            </p>
          )}
        </div>
      </Modal>

      <ConfirmByName
        open={Boolean(deleteTarget)}
        title="Remove this instance from the panel"
        expected={deleteName}
        consequence={`The panel stops tracking this instance. ${
          databaseCounts[deleteTarget?.id || ''] || 0
        } database row(s) recorded against it are orphaned in the panel, and nothing is dropped on the server itself.`}
        confirmLabel="Remove instance"
        busy={deleting}
        error={deleteError}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={confirmDelete}
      />
    </>
  );
}

export default InstancesPanel;
