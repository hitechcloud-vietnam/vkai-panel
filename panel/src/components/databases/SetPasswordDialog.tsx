'use client';

/**
 * Setting a new password on an existing database account.
 *
 * POST /api/v1/databases/{id}/change-password is real: the service runs ALTER
 * USER on the host and re-encrypts the stored copy. It is also the ONLY way an
 * operator can ever learn a password again, because models.DatabaseEntry hides
 * the field from JSON. The dialog says so, rather than leaving someone hunting
 * for a reveal button that cannot exist.
 */

import { useEffect, useState } from 'react';
import { Loader2, RefreshCw } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';

import Modal from './Modal';
import { PANE_INPUT_CLASS, PRIMARY_BUTTON_CLASS, SECONDARY_BUTTON_CLASS } from './PaneChrome';
import { generatePassword, passwordProblem } from './passwordRules';

export function SetPasswordDialog({
  open,
  databaseName,
  accountLabel,
  accountName,
  onClose,
  onSubmit,
}: {
  open: boolean;
  databaseName: string;
  accountLabel: string;
  accountName: string;
  onClose: () => void;
  onSubmit: (password: string) => Promise<void>;
}) {
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [show, setShow] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setPassword(generatePassword());
    setConfirm('');
    setShow(false);
    setSaving(false);
    setError(null);
  }, [open, databaseName]);

  const problem = passwordProblem(password);
  const mismatch = confirm.length > 0 && confirm !== password;
  const canSubmit = !problem && confirm === password && !saving;

  const submit = async () => {
    if (!canSubmit) return;
    setSaving(true);
    setError(null);
    try {
      await onSubmit(password);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open={open}
      title={`Set a new password for ${databaseName}`}
      description={`Changes the password of ${accountLabel.toLowerCase()} ${accountName} on the server and in the panel.`}
      onClose={saving ? () => undefined : onClose}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={onClose}
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
            Set password
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <p className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
          Anything still connecting with the old password stops working the moment this
          is applied. Update your application configuration in the same maintenance
          window.
        </p>

        <div>
          <label
            htmlFor="db-new-password"
            className="mb-1.5 block text-sm font-medium text-gray-700"
          >
            New password
          </label>
          <div className="flex gap-2">
            <Input
              id="db-new-password"
              type={show ? 'text' : 'password'}
              value={password}
              autoComplete="new-password"
              onChange={(e) => setPassword(e.target.value)}
              className={cn(PANE_INPUT_CLASS, 'font-mono')}
            />
            <Button
              type="button"
              variant="outline"
              onClick={() => setShow((v) => !v)}
              className={SECONDARY_BUTTON_CLASS}
            >
              {show ? 'Hide' : 'Show'}
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => setPassword(generatePassword())}
              aria-label="Generate a new password"
              className={SECONDARY_BUTTON_CLASS}
            >
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
          {problem && <p className="mt-1 text-xs text-red-600">{problem}</p>}
        </div>

        <div>
          <label
            htmlFor="db-confirm-password"
            className="mb-1.5 block text-sm font-medium text-gray-700"
          >
            Repeat the new password
          </label>
          <Input
            id="db-confirm-password"
            type={show ? 'text' : 'password'}
            value={confirm}
            autoComplete="new-password"
            onChange={(e) => setConfirm(e.target.value)}
            className={cn(PANE_INPUT_CLASS, 'font-mono')}
          />
          {mismatch && (
            <p className="mt-1 text-xs text-red-600">The two entries do not match.</p>
          )}
        </div>

        {error && (
          <p className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
            {error}
          </p>
        )}
      </div>
    </Modal>
  );
}

export default SetPasswordDialog;
