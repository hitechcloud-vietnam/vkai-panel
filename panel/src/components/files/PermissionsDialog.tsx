'use client';

/**
 * Changing a mode, in both notations at once.
 *
 * Operators think in octal ("chmod 644") and read in symbolic ("-rw-r--r--"),
 * and which one they reach for depends entirely on where they learned it, so
 * both are on screen and both update as the checkboxes move.
 *
 * The warning about world-writable is not decoration. 0777 on a web root is the
 * single most common way a shared host gets a web shell written into it, and
 * the operator setting it is usually trying to fix an upload that failed for a
 * different reason. The dialog states the consequence and still lets them do
 * it, because sometimes it really is what is needed.
 *
 * What this dialog cannot do: setuid, setgid and the sticky bit. The service
 * refuses any mode outside 0000-0777, so there is no control for them.
 */

import { useEffect, useMemo, useState } from 'react';
import { AlertTriangle } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import type { FileEntry } from '@/types/files';
import Modal from './Modal';
import {
  isGroupOrWorldExecutable,
  isWorldWritable,
  octalFromMode,
  symbolicFromOctal,
  validateOctalMode,
} from './format';

const CLASSES: { key: 'owner' | 'group' | 'other'; label: string }[] = [
  { key: 'owner', label: 'Owner' },
  { key: 'group', label: 'Group' },
  { key: 'other', label: 'Everyone else' },
];

const BITS: { key: 'read' | 'write' | 'execute'; label: string; value: number }[] = [
  { key: 'read', label: 'Read', value: 4 },
  { key: 'write', label: 'Write', value: 2 },
  { key: 'execute', label: 'Execute', value: 1 },
];

export interface PermissionsDialogProps {
  open: boolean;
  items: FileEntry[];
  onClose: () => void;
  /** One request per item; resolves with a message per failure. */
  onSubmit: (mode: string, items: FileEntry[]) => Promise<string[]>;
}

function digitsOf(octal: string): [number, number, number] {
  const padded = String(octal || '').replace(/^0+/, '').padStart(3, '0');
  if (!/^[0-7]{3}$/.test(padded)) return [0, 0, 0];
  return [Number(padded[0]), Number(padded[1]), Number(padded[2])];
}

export default function PermissionsDialog({
  open,
  items,
  onClose,
  onSubmit,
}: PermissionsDialogProps) {
  const startingMode = useMemo(() => {
    if (items.length !== 1) return '644';
    return octalFromMode(items[0]?.mode) || '644';
  }, [items]);

  const [octal, setOctal] = useState(startingMode);
  const [error, setError] = useState<string | null>(null);
  const [failures, setFailures] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setOctal(startingMode);
    setError(null);
    setFailures([]);
    setBusy(false);
  }, [open, startingMode]);

  const normalized = octal.replace(/^0+/, '').padStart(3, '0');
  const symbolic = symbolicFromOctal(normalized);
  const [ownerDigit, groupDigit, otherDigit] = digitsOf(normalized);
  const digitByClass = { owner: ownerDigit, group: groupDigit, other: otherDigit };
  const worldWritable = isWorldWritable(normalized);
  const broadlyExecutable = isGroupOrWorldExecutable(normalized);

  const toggle = (classKey: 'owner' | 'group' | 'other', bit: number) => {
    const next = { ...digitByClass };
    next[classKey] = next[classKey] & bit ? next[classKey] & ~bit : next[classKey] | bit;
    setOctal(`${next.owner}${next.group}${next.other}`);
    setError(null);
  };

  const submit = async () => {
    const invalid = validateOctalMode(octal);
    if (invalid) {
      setError(invalid);
      return;
    }
    setBusy(true);
    setError(null);
    setFailures([]);
    try {
      const problems = await onSubmit(octal.trim(), items);
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

  const target =
    items.length === 1 ? items[0]?.name || '' : `${items.length} selected items`;

  return (
    <Modal
      open={open}
      title={`Permissions on ${target}`}
      description={
        items.length === 1
          ? 'The server accepts permission bits only. Setuid, setgid and the sticky bit are refused.'
          : `The same mode is applied to all ${items.length} items, one request each. There is no batch endpoint.`
      }
      onClose={onClose}
      footer={
        <>
          <Button type="button" variant="secondary" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button
            type="button"
            variant={worldWritable ? 'danger' : 'default'}
            onClick={submit}
            disabled={busy || items.length === 0}
          >
            {busy ? 'Applying...' : worldWritable ? 'Apply anyway' : 'Apply'}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-[1fr_auto]">
          <div className="overflow-hidden rounded-md border border-gray-200">
            <table className="w-full">
              <thead className="bg-gray-50">
                <tr className="[&_th]:border-b [&_th]:border-gray-200">
                  <th className="px-3 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">
                    Who
                  </th>
                  {BITS.map((bit) => (
                    <th
                      key={bit.key}
                      className="px-3 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gray-500"
                    >
                      {bit.label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                {CLASSES.map((klass) => (
                  <tr key={klass.key}>
                    <td className="px-3 py-2 text-sm text-gray-700">{klass.label}</td>
                    {BITS.map((bit) => (
                      <td key={bit.key} className="px-3 py-2">
                        <input
                          type="checkbox"
                          checked={(digitByClass[klass.key] & bit.value) === bit.value}
                          onChange={() => toggle(klass.key, bit.value)}
                          aria-label={`${klass.label} may ${bit.label.toLowerCase()}`}
                          className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                        />
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="sm:w-44">
            <label htmlFor="mode-octal" className="mb-1.5 block text-sm font-medium text-gray-700">
              Octal
            </label>
            <Input
              id="mode-octal"
              value={octal}
              onChange={(event) => {
                setOctal(event.target.value);
                setError(null);
              }}
              inputMode="numeric"
              spellCheck={false}
              autoComplete="off"
              className="font-mono"
            />
            <p className="mt-2 text-xs text-gray-500">Symbolic</p>
            <p className="font-mono text-sm text-gray-900">{symbolic || '—'}</p>
          </div>
        </div>

        {worldWritable ? (
          <div
            className="flex items-start gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2"
            role="alert"
          >
            <AlertTriangle size={16} className="mt-0.5 shrink-0 text-red-700" aria-hidden="true" />
            <div className="text-sm text-red-700">
              <p className="font-medium">This makes it writable by every user on the server.</p>
              <p className="mt-0.5">
                Any account on this machine, including one a compromised site is running as, can
                rewrite {items.length === 1 ? 'this file' : 'these files'}
                {broadlyExecutable ? ' and it is also executable by them' : ''}. If an upload is
                failing, changing the owner is almost always the fix instead — though this API has
                no endpoint for that, so it has to be done over SSH.
              </p>
            </div>
          </div>
        ) : null}

        {error ? (
          <p className="text-sm text-red-700" role="alert">
            {error}
          </p>
        ) : null}

        {failures.length > 0 ? (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2" role="alert">
            <p className="text-sm font-medium text-red-700">
              {failures.length} of {items.length} were not changed.
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
