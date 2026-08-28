'use client';

/**
 * Removal, asked one question at a time.
 *
 * Deleting a vhost is recoverable in an afternoon. Deleting a customer's
 * document root is not recoverable at all, and deleting their database is
 * worse. So the two destructive halves are separate questions with their own
 * checkboxes, both off by default, and the file deletion is gated behind typing
 * the domain.
 *
 * Each dialog also states what the backend really does, because the API's
 * delete is narrower than an operator would assume:
 *
 *   DELETE /api/v1/websites/:id   disables and removes the vhost, reloads the
 *                                 web server, soft-deletes the row. It does NOT
 *                                 touch the document root and does not know
 *                                 which database belongs to the site.
 *   DELETE /api/v1/node-apps/:id  removes the row only. It does not stop the
 *                                 service and does not remove the systemd unit.
 *   DELETE /api/v1/reverse-proxy/:id  removes the row only.
 */

import { useState } from 'react';
import Link from 'next/link';
import { AlertTriangle, Loader2, Trash2 } from 'lucide-react';

import { errorMessage } from '@/lib/apiError';
import type { ManagedWebsite } from '@/types/server';
import type { NodeApp, ReverseProxy } from '@/types/website';

import { nodeAppApi, reverseProxyApi, siteFilesApi } from './api';
import { websiteApi } from '@/services/api';
import { BTN_DANGER, BTN_SECONDARY, FormError, INPUT_CLASS, Modal, Notice } from './ui';

function Checkbox({
  id,
  checked,
  onChange,
  disabled = false,
  label,
  description,
}: {
  id: string;
  checked: boolean;
  onChange: (value: boolean) => void;
  disabled?: boolean;
  label: string;
  description: React.ReactNode;
}) {
  return (
    <div
      className={`rounded-md border px-4 py-3 ${
        disabled ? 'border-gray-200 bg-gray-50' : 'border-gray-200 bg-white'
      }`}
    >
      <div className="flex items-start gap-3">
        <input
          id={id}
          type="checkbox"
          checked={checked}
          disabled={disabled}
          onChange={(e) => onChange(e.target.checked)}
          className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500 disabled:cursor-not-allowed"
        />
        <div className="min-w-0">
          <label
            htmlFor={id}
            className={`block text-sm font-medium ${disabled ? 'text-gray-500' : 'text-gray-900'}`}
          >
            {label}
          </label>
          <div className="mt-1 text-sm text-gray-600">{description}</div>
        </div>
      </div>
    </div>
  );
}

export interface RemoveWebsiteDialogProps {
  site: ManagedWebsite;
  onClose: () => void;
  onRemoved: () => void;
}

export function RemoveWebsiteDialog({ site, onClose, onRemoved }: RemoveWebsiteDialogProps) {
  const [deleteFiles, setDeleteFiles] = useState(false);
  const [confirmation, setConfirmation] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const domain = site.domain || '';
  const rootDir = site.root_dir || '';
  const filesConfirmed = !deleteFiles || confirmation.trim() === domain;

  const submit = async () => {
    setError('');
    if (!filesConfirmed) {
      setError('Type the domain exactly to confirm that the files should be deleted.');
      return;
    }
    setBusy(true);
    try {
      /*
       * Files first, and only then the site row. If the file deletion fails the
       * site is still there to try again from; the other order leaves an
       * orphaned document root and no row pointing at it.
       */
      if (deleteFiles && rootDir) {
        await siteFilesApi.remove(rootDir);
      }
      await websiteApi.delete(site.id);
      onRemoved();
      onClose();
    } catch (err) {
      setError(errorMessage(err, 'Failed to remove the website'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title={`Remove ${domain || 'this website'}`} onClose={onClose}>
      <div className="space-y-4">
        <FormError message={error} />

        <p className="text-sm text-gray-600">
          The panel will disable and delete this site&apos;s web server configuration and reload the
          web server. Everything below is a separate decision.
        </p>

        <Checkbox
          id="remove-site-files"
          checked={deleteFiles}
          onChange={setDeleteFiles}
          disabled={!rootDir}
          label="Also delete the website files"
          description={
            rootDir ? (
              <>
                Deletes <span className="break-all font-mono text-xs">{rootDir}</span> and
                everything under it. This cannot be undone and there is no trash to recover it
                from.
              </>
            ) : (
              'Unavailable: this site has no document root recorded, so there is no directory to delete.'
            )
          }
        />

        {deleteFiles && (
          <div>
            <label htmlFor="remove-site-confirm" className="mb-1.5 block text-sm font-medium text-gray-700">
              Type <span className="font-mono">{domain}</span> to confirm
            </label>
            <input
              id="remove-site-confirm"
              type="text"
              value={confirmation}
              onChange={(e) => setConfirmation(e.target.value)}
              className={INPUT_CLASS}
              autoComplete="off"
              placeholder={domain}
            />
          </div>
        )}

        <Checkbox
          id="remove-site-database"
          checked={false}
          onChange={() => undefined}
          disabled
          label="Also drop the database"
          description={
            <>
              Unavailable: nothing records which database belongs to which site. The databases
              table has no website column, and no endpoint answers &ldquo;which database does this
              site use&rdquo;, so the panel cannot pick one without guessing. Drop it by name from{' '}
              <Link
                href="/databases"
                className="font-medium text-brand-700 underline underline-offset-2 hover:text-brand-800"
              >
                Databases
              </Link>
              .
            </>
          }
        />

        <div className="flex justify-end gap-3 border-t border-gray-200 pt-4">
          <button type="button" onClick={onClose} className={BTN_SECONDARY} disabled={busy}>
            Cancel
          </button>
          <button type="button" onClick={submit} className={BTN_DANGER} disabled={busy || !filesConfirmed}>
            {busy ? (
              <Loader2 size={16} className="animate-spin" aria-hidden="true" />
            ) : (
              <Trash2 size={16} aria-hidden="true" />
            )}
            {busy ? 'Removing...' : deleteFiles ? 'Remove site and files' : 'Remove site'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

export interface RemoveNodeAppDialogProps {
  app: NodeApp;
  onClose: () => void;
  onRemoved: () => void;
}

export function RemoveNodeAppDialog({ app, onClose, onRemoved }: RemoveNodeAppDialogProps) {
  const [stopFirst, setStopFirst] = useState(true);
  const [deleteFiles, setDeleteFiles] = useState(false);
  const [confirmation, setConfirmation] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const name = app.name || 'this project';
  const path = app.path || '';
  const filesConfirmed = !deleteFiles || confirmation.trim() === name;

  const submit = async () => {
    setError('');
    if (!filesConfirmed) {
      setError('Type the project name exactly to confirm that the files should be deleted.');
      return;
    }
    setBusy(true);
    try {
      if (stopFirst) {
        await nodeAppApi.stop(app.id);
      }
      if (deleteFiles && path) {
        await siteFilesApi.remove(path);
      }
      await nodeAppApi.remove(app.id);
      onRemoved();
      onClose();
    } catch (err) {
      setError(errorMessage(err, 'Failed to remove the Node.js project'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title={`Remove ${name}`} onClose={onClose}>
      <div className="space-y-4">
        <FormError message={error} />

        <Notice tone="amber" title="The backend removes the record, not the unit">
          <p>
            DELETE /api/v1/node-apps/:id deletes the database row only. It does not stop the
            service and does not remove the systemd unit it installed, so a running process
            survives the removal unless it is stopped first.
          </p>
        </Notice>

        <Checkbox
          id="remove-node-stop"
          checked={stopFirst}
          onChange={setStopFirst}
          label="Stop the service first"
          description="Calls POST /api/v1/node-apps/:id/stop before the record is removed, so no orphaned process keeps holding the port."
        />

        <Checkbox
          id="remove-node-files"
          checked={deleteFiles}
          onChange={setDeleteFiles}
          disabled={!path}
          label="Also delete the project files"
          description={
            path ? (
              <>
                Deletes <span className="break-all font-mono text-xs">{path}</span> and everything
                under it. This cannot be undone.
              </>
            ) : (
              'Unavailable: this project has no working directory recorded.'
            )
          }
        />

        {deleteFiles && (
          <div>
            <label htmlFor="remove-node-confirm" className="mb-1.5 block text-sm font-medium text-gray-700">
              Type <span className="font-mono">{name}</span> to confirm
            </label>
            <input
              id="remove-node-confirm"
              type="text"
              value={confirmation}
              onChange={(e) => setConfirmation(e.target.value)}
              className={INPUT_CLASS}
              autoComplete="off"
              placeholder={name}
            />
          </div>
        )}

        <div className="flex justify-end gap-3 border-t border-gray-200 pt-4">
          <button type="button" onClick={onClose} className={BTN_SECONDARY} disabled={busy}>
            Cancel
          </button>
          <button type="button" onClick={submit} className={BTN_DANGER} disabled={busy || !filesConfirmed}>
            {busy ? (
              <Loader2 size={16} className="animate-spin" aria-hidden="true" />
            ) : (
              <Trash2 size={16} aria-hidden="true" />
            )}
            {busy ? 'Removing...' : 'Remove project'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

export interface RemoveProxyDialogProps {
  proxy: ReverseProxy;
  onClose: () => void;
  onRemoved: () => void;
}

export function RemoveProxyDialog({ proxy, onClose, onRemoved }: RemoveProxyDialogProps) {
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const label = proxy.domain || proxy.name || 'this proxy';

  const submit = async () => {
    setError('');
    setBusy(true);
    try {
      await reverseProxyApi.remove(proxy.id);
      onRemoved();
      onClose();
    } catch (err) {
      setError(errorMessage(err, 'Failed to remove the proxy project'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title={`Remove ${label}`} onClose={onClose}>
      <div className="space-y-4">
        <FormError message={error} />
        <p className="flex gap-3 text-sm text-gray-600">
          <AlertTriangle size={16} className="mt-0.5 shrink-0 text-amber-500" aria-hidden="true" />
          <span>
            A proxy project has no document root and no database of its own, so there is nothing
            else to decide. Removing it deletes the panel&apos;s record of the forwarding rule; any
            web server configuration written by hand for this hostname stays where it is.
          </span>
        </p>
        <div className="flex justify-end gap-3 border-t border-gray-200 pt-4">
          <button type="button" onClick={onClose} className={BTN_SECONDARY} disabled={busy}>
            Cancel
          </button>
          <button type="button" onClick={submit} className={BTN_DANGER} disabled={busy}>
            {busy ? (
              <Loader2 size={16} className="animate-spin" aria-hidden="true" />
            ) : (
              <Trash2 size={16} aria-hidden="true" />
            )}
            {busy ? 'Removing...' : 'Remove proxy'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
