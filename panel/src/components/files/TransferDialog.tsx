'use client';

/**
 * Copy and Move.
 *
 * Both are one endpoint each - POST /files/copy takes {src, dst} and POST
 * /files/rename takes {old_path, new_path} - and neither accepts a batch, so a
 * multi-row transfer is one request per row. The dialog says how many requests
 * that is, and reports each failure against the name it belongs to instead of
 * stopping at the first one.
 */

import { useEffect, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import type { FileEntry } from '@/types/files';
import Modal from './Modal';
import { joinPath, validateDirectoryPath, validateName } from './paths';

export interface TransferDialogProps {
  open: boolean;
  mode: 'copy' | 'move';
  items: FileEntry[];
  /** Where the operator is standing; the destination starts here. */
  currentPath: string;
  onClose: () => void;
  /** Runs one request per item. Resolves with a message per failure. */
  onSubmit: (destinations: { entry: FileEntry; destination: string }[]) => Promise<string[]>;
}

export default function TransferDialog({
  open,
  mode,
  items,
  currentPath,
  onClose,
  onSubmit,
}: TransferDialogProps) {
  const single = items.length === 1;
  const [destination, setDestination] = useState(currentPath);
  const [name, setName] = useState(single ? items[0]?.name || '' : '');
  const [error, setError] = useState<string | null>(null);
  const [failures, setFailures] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setDestination(currentPath);
    setName(items.length === 1 ? items[0]?.name || '' : '');
    setError(null);
    setFailures([]);
    setBusy(false);
  }, [open, currentPath, items]);

  const verb = mode === 'copy' ? 'Copy' : 'Move';

  const submit = async () => {
    const invalidDir = validateDirectoryPath(destination);
    if (invalidDir) {
      setError(invalidDir);
      return;
    }
    if (single) {
      const invalidName = validateName(name);
      if (invalidName) {
        setError(invalidName);
        return;
      }
    }

    const targets = items.map((entry) => ({
      entry,
      destination: joinPath(destination, single ? name.trim() : entry.name),
    }));

    const clash = targets.find((target) => target.destination === target.entry.path);
    if (clash) {
      setError(`${clash.entry.name} is already at that path. Choose a different destination or name.`);
      return;
    }

    setBusy(true);
    setError(null);
    setFailures([]);
    try {
      const problems = await onSubmit(targets);
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

  return (
    <Modal
      open={open}
      title={`${verb} ${items.length === 1 ? items[0]?.name || '' : `${items.length} items`}`}
      description={
        mode === 'copy'
          ? 'The originals stay where they are. A directory is copied with everything inside it.'
          : 'The originals are moved. Anything already at the destination path is replaced.'
      }
      onClose={onClose}
      footer={
        <>
          <Button type="button" variant="secondary" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button type="button" onClick={submit} disabled={busy || items.length === 0}>
            {busy ? `${verb}ing...` : verb}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <div>
          <label
            htmlFor="transfer-destination"
            className="mb-1.5 block text-sm font-medium text-gray-700"
          >
            Destination directory
          </label>
          <Input
            id="transfer-destination"
            value={destination}
            onChange={(event) => setDestination(event.target.value)}
            spellCheck={false}
            autoComplete="off"
          />
          <p className="mt-1 text-xs text-gray-500">
            An absolute path inside the file manager root. The server refuses anything outside it.
          </p>
        </div>

        {single ? (
          <div>
            <label
              htmlFor="transfer-name"
              className="mb-1.5 block text-sm font-medium text-gray-700"
            >
              Name at the destination
            </label>
            <Input
              id="transfer-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              spellCheck={false}
              autoComplete="off"
            />
          </div>
        ) : (
          <div className="rounded-md border border-gray-200 bg-gray-50 px-3 py-2">
            <p className="text-sm text-gray-700">
              {items.length} items keep their names and go to that directory. There is no batch
              endpoint, so this sends {items.length} separate requests.
            </p>
            <ul className="mt-2 max-h-40 space-y-0.5 overflow-auto text-xs text-gray-600">
              {items.map((entry) => (
                <li key={entry.path} className="truncate font-mono">
                  {entry.name}
                </li>
              ))}
            </ul>
          </div>
        )}

        {error ? (
          <p className="text-sm text-red-700" role="alert">
            {error}
          </p>
        ) : null}

        {failures.length > 0 ? (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2" role="alert">
            <p className="text-sm font-medium text-red-700">
              {failures.length} of {items.length} did not go through.
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
