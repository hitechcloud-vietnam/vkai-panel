'use client';

/**
 * The confirmation that has to earn its place.
 *
 * A delete on this screen removes a customer's website, so the dialog does not
 * ask "are you sure": it names every path that is about to go, and for each
 * directory it counts what is inside before the operator commits. The count
 * comes from GET /files/search with the pattern "*", which is `find -maxdepth
 * 5`, so it is honest about being a count to five levels rather than a total.
 *
 * DELETE is recursive on the server - service.Delete calls os.RemoveAll - and
 * there is no undo, no trash and no batch endpoint, so a multi-row delete is
 * one request per row and each failure is reported against its own name.
 */

import { useCallback, useEffect, useState } from 'react';
import { AlertTriangle } from 'lucide-react';

import { Button } from '@/components/ui/button';
import type { FileEntry } from '@/types/files';
import Modal from './Modal';
import { filesApi, fileErrorMessage } from './api';
import { formatBytes } from './format';

export interface DeleteDialogProps {
  open: boolean;
  items: FileEntry[];
  onClose: () => void;
  /** One request per item; resolves with a message per failure. */
  onConfirm: (items: FileEntry[]) => Promise<string[]>;
}

type CountState = { status: 'counting' } | { status: 'ok'; count: number } | { status: 'failed'; reason: string };

export default function DeleteDialog({ open, items, onClose, onConfirm }: DeleteDialogProps) {
  const [counts, setCounts] = useState<Record<string, CountState>>({});
  const [failures, setFailures] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const directories = items.filter((entry) => entry.is_dir);

  const countDirectories = useCallback(async (dirs: FileEntry[]) => {
    for (const dir of dirs) {
      setCounts((prev) => ({ ...prev, [dir.path]: { status: 'counting' } }));
      try {
        const found = await filesApi.search(dir.path, '*');
        // `find` prints the starting directory itself; it is not "inside".
        const inside = found.filter((entry) => entry.path !== dir.path).length;
        setCounts((prev) => ({ ...prev, [dir.path]: { status: 'ok', count: inside } }));
      } catch (err) {
        setCounts((prev) => ({
          ...prev,
          [dir.path]: {
            status: 'failed',
            reason: fileErrorMessage(err, 'the server could not count it'),
          },
        }));
      }
    }
  }, []);

  useEffect(() => {
    if (!open) return;
    setCounts({});
    setFailures([]);
    setError(null);
    setBusy(false);
    if (directories.length > 0) void countDirectories(directories);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, items]);

  const stillCounting = Object.values(counts).some((state) => state.status === 'counting');

  const confirm = async () => {
    setBusy(true);
    setError(null);
    setFailures([]);
    try {
      const problems = await onConfirm(items);
      if (problems.length > 0) {
        setFailures(problems);
        return;
      }
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const describe = (entry: FileEntry) => {
    if (!entry.is_dir) return formatBytes(entry.size);
    const state = counts[entry.path];
    if (!state || state.status === 'counting') return 'counting what is inside...';
    if (state.status === 'failed') return `contents could not be counted: ${state.reason}`;
    if (state.count === 0) return 'empty directory';
    return `${state.count} file${state.count === 1 ? '' : 's'} and directories inside go with it`;
  };

  return (
    <Modal
      open={open}
      title={
        items.length === 1
          ? `Delete ${items[0]?.name || ''}`
          : `Delete ${items.length} items`
      }
      description="Deleting is immediate and permanent. There is no trash and no undo in this API."
      onClose={onClose}
      footer={
        <>
          <Button type="button" variant="secondary" onClick={onClose} disabled={busy}>
            Keep them
          </Button>
          <Button
            type="button"
            variant="danger"
            onClick={confirm}
            disabled={busy || items.length === 0 || stillCounting}
          >
            {busy
              ? 'Deleting...'
              : items.length === 1
                ? 'Delete permanently'
                : `Delete ${items.length} permanently`}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <div className="flex items-start gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2">
          <AlertTriangle size={16} className="mt-0.5 shrink-0 text-red-700" aria-hidden="true" />
          <p className="text-sm text-red-700">
            {directories.length > 0
              ? 'A directory is removed with everything inside it. If any of these are a live site’s document root, that site stops serving the moment this runs.'
              : 'If any of these files belong to a live site, that site changes the moment this runs.'}
          </p>
        </div>

        <ul className="max-h-64 space-y-2 overflow-auto">
          {items.map((entry) => (
            <li key={entry.path} className="rounded-md border border-gray-200 px-3 py-2">
              <p className="truncate font-mono text-sm text-gray-900" title={entry.path}>
                {entry.path}
              </p>
              <p className="mt-0.5 text-xs text-gray-600">
                {entry.is_dir ? 'Directory — ' : 'File — '}
                {describe(entry)}
              </p>
            </li>
          ))}
        </ul>

        {directories.length > 0 ? (
          <p className="text-xs text-gray-500">
            Counts come from the server’s search, which looks five levels deep. Anything deeper than
            that is deleted too but is not in the number above.
          </p>
        ) : null}

        {items.length > 1 ? (
          <p className="text-xs text-gray-500">
            There is no batch delete endpoint, so this sends {items.length} separate requests.
          </p>
        ) : null}

        {error ? (
          <p className="text-sm text-red-700" role="alert">
            {error}
          </p>
        ) : null}

        {failures.length > 0 ? (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2" role="alert">
            <p className="text-sm font-medium text-red-700">
              {failures.length} of {items.length} were not deleted.
            </p>
            <ul className="mt-1 space-y-0.5 text-sm text-red-700">
              {failures.map((message) => (
                <li key={message}>{message}</li>
              ))}
            </ul>
          </div>
        ) : null}
      </div>
    </Modal>
  );
}
