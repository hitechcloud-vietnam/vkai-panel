'use client';

/**
 * "Type the name to confirm."
 *
 * A dropped production database is not undone by an undo button, because there
 * is no undo button and no restore endpoint behind one either (see
 * BackupService.RestoreBackup, which returns "restore not yet implemented").
 * The only protection this panel can honestly offer is the half-second in which
 * an operator has to read the name and type it back.
 *
 * The button stays disabled until the typed text matches exactly, including
 * case, because a database called `orders` and one called `Orders` are two
 * databases on a case-sensitive filesystem.
 */

import { useEffect, useState } from 'react';
import { AlertTriangle, Loader2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import Modal from './Modal';
import { DANGER_BUTTON_CLASS, PANE_INPUT_CLASS, SECONDARY_BUTTON_CLASS } from './PaneChrome';

export function ConfirmByName({
  open,
  title,
  /** The exact text the operator must retype - normally the database name. */
  expected,
  /** What happens when they confirm, in one plain sentence. */
  consequence,
  confirmLabel,
  busy,
  error,
  onCancel,
  onConfirm,
}: {
  open: boolean;
  title: string;
  expected: string;
  consequence: string;
  confirmLabel: string;
  busy?: boolean;
  error?: string | null;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const [typed, setTyped] = useState('');

  useEffect(() => {
    if (open) setTyped('');
  }, [open, expected]);

  const matches = typed === expected && expected.length > 0;

  return (
    <Modal
      open={open}
      title={title}
      onClose={busy ? () => undefined : onCancel}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={onCancel}
            disabled={busy}
            className={SECONDARY_BUTTON_CLASS}
          >
            Cancel
          </Button>
          <Button
            type="button"
            onClick={onConfirm}
            disabled={!matches || busy}
            className={DANGER_BUTTON_CLASS}
          >
            {busy && <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />}
            {confirmLabel}
          </Button>
        </>
      }
    >
      <div className="flex items-start gap-3 rounded-md border border-red-200 bg-red-50 p-3">
        <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-red-600" aria-hidden="true" />
        <p className="text-sm text-red-800">{consequence}</p>
      </div>

      <label
        htmlFor="confirm-by-name"
        className="mt-4 block text-sm font-medium text-gray-900"
      >
        Type <span className="font-mono font-semibold">{expected}</span> to confirm
      </label>
      <Input
        id="confirm-by-name"
        value={typed}
        autoComplete="off"
        autoCapitalize="off"
        spellCheck={false}
        onChange={(e) => setTyped(e.target.value)}
        placeholder={expected}
        className={cn(PANE_INPUT_CLASS, 'mt-2 font-mono')}
      />
      {typed.length > 0 && !matches && (
        <p className="mt-2 text-xs text-gray-500">
          The name does not match yet. It is compared exactly, including capitals.
        </p>
      )}

      {error && (
        <p className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </p>
      )}
    </Modal>
  );
}

export default ConfirmByName;
