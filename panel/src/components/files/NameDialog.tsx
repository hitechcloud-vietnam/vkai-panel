'use client';

/**
 * "What should it be called?" - used for New folder, New file and Rename.
 *
 * The name is checked here against the one rule the API does not enforce
 * loudly: a name containing "/" is not refused by the server, it is resolved,
 * so the file quietly lands in a different directory. Refusing it in the dialog
 * keeps what the operator typed and what the server does in agreement.
 */

import { useEffect, useState, type ReactNode } from 'react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import Modal from './Modal';
import { validateName } from './paths';

export interface NameDialogProps {
  open: boolean;
  title: string;
  description?: ReactNode;
  label: string;
  initialValue: string;
  confirmLabel: string;
  onClose: () => void;
  onSubmit: (name: string) => Promise<void>;
}

export default function NameDialog({
  open,
  title,
  description,
  label,
  initialValue,
  confirmLabel,
  onClose,
  onSubmit,
}: NameDialogProps) {
  const [value, setValue] = useState(initialValue);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (open) {
      setValue(initialValue);
      setError(null);
      setBusy(false);
    }
  }, [open, initialValue]);

  const submit = async () => {
    const invalid = validateName(value);
    if (invalid) {
      setError(invalid);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await onSubmit(value.trim());
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      title={title}
      description={description}
      onClose={onClose}
      footer={
        <>
          <Button type="button" variant="secondary" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button type="button" onClick={submit} disabled={busy}>
            {busy ? 'Working...' : confirmLabel}
          </Button>
        </>
      }
    >
      <label htmlFor="file-name-input" className="mb-1.5 block text-sm font-medium text-gray-700">
        {label}
      </label>
      <Input
        id="file-name-input"
        value={value}
        onChange={(event) => setValue(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') void submit();
        }}
        autoComplete="off"
        spellCheck={false}
      />
      {error ? (
        <p className="mt-2 text-sm text-red-700" role="alert">
          {error}
        </p>
      ) : null}
    </Modal>
  );
}
