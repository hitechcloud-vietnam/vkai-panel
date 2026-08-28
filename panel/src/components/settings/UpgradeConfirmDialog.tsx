'use client';

/**
 * The dialog that stands between an operator and a panel that is briefly gone.
 *
 * An upgrade replaces every binary on the server and restarts the three panel
 * services, so this dialog says exactly that, names the versions it is moving
 * between, and refuses to submit until the operator has ticked the box. It also
 * repeats any unsaved settings warning here rather than only on the page
 * behind it: this is the last moment those edits can still be saved.
 */

import { useEffect, useRef, useState } from 'react';
import { AlertTriangle, X } from 'lucide-react';
import { Button } from '@/components/ui/button';

interface UpgradeConfirmDialogProps {
  open: boolean;
  fromVersion: string;
  toVersion: string;
  /** Release notes for the version being installed, if the server sent any. */
  changelog: string;
  /** True when another settings tab is holding edits that have not been saved. */
  unsavedSettings: boolean;
  submitting: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}

export default function UpgradeConfirmDialog({
  open,
  fromVersion,
  toVersion,
  changelog,
  unsavedSettings,
  submitting,
  onCancel,
  onConfirm,
}: UpgradeConfirmDialogProps) {
  const [acknowledged, setAcknowledged] = useState(false);
  const cancelRef = useRef<HTMLButtonElement | null>(null);

  // Reopening the dialog must never inherit the previous acknowledgement: a
  // tick left over from an upgrade the operator abandoned is not consent.
  useEffect(() => {
    if (open) {
      setAcknowledged(false);
      cancelRef.current?.focus();
    }
  }, [open]);

  useEffect(() => {
    if (!open) return undefined;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !submitting) onCancel();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [open, submitting, onCancel]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40 p-4">
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="upgrade-confirm-title"
        className="max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg"
      >
        <div className="flex items-start justify-between gap-4 border-b border-gray-200 px-5 py-4">
          <div className="flex items-start gap-3">
            <span className="mt-0.5 text-amber-600">
              <AlertTriangle size={18} />
            </span>
            <div>
              <h2 id="upgrade-confirm-title" className="text-sm font-semibold text-gray-900">
                Upgrade the panel to {toVersion}?
              </h2>
              <p className="mt-1 text-sm text-gray-600">
                The panel will restart during the upgrade and will be unreachable for a minute or
                two. Websites, databases and mail on this server keep running.
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onCancel}
            disabled={submitting}
            aria-label="Close"
            className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 disabled:opacity-50"
          >
            <X size={16} />
          </button>
        </div>

        <div className="space-y-4 px-5 py-4">
          <div className="flex items-center gap-3 rounded-md border border-gray-200 bg-gray-50 px-3 py-2.5">
            <div>
              <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">From</p>
              <p className="mt-0.5 font-mono text-sm text-gray-900">{fromVersion || 'unknown'}</p>
            </div>
            <span aria-hidden="true" className="text-gray-400">
              &rarr;
            </span>
            <div>
              <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">To</p>
              <p className="mt-0.5 font-mono text-sm text-gray-900">{toVersion || 'unknown'}</p>
            </div>
          </div>

          {unsavedSettings ? (
            <div className="flex items-start gap-2.5 rounded-md border border-amber-200 bg-amber-50 px-3 py-2.5">
              <span className="mt-0.5 shrink-0 text-amber-600">
                <AlertTriangle size={16} />
              </span>
              <p className="text-sm text-amber-800">
                Another settings tab has changes that have not been saved. They will be lost when
                the panel restarts. Cancel, save them, then come back.
              </p>
            </div>
          ) : null}

          {changelog ? (
            <div>
              <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                What is in {toVersion}
              </p>
              <pre className="mt-1.5 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-md border border-gray-200 bg-gray-50 px-3 py-2.5 font-sans text-sm text-gray-700">
                {changelog}
              </pre>
            </div>
          ) : null}

          <ul className="list-inside list-disc space-y-1 text-sm text-gray-600">
            <li>The current release is kept so the panel can roll back if the new one fails.</li>
            <li>Do not stop the server or close this page while the upgrade is running.</li>
            <li>This page keeps watching and will tell you the outcome once the panel is back.</li>
          </ul>

          <label className="flex items-start gap-2.5 text-sm text-gray-700">
            <input
              type="checkbox"
              checked={acknowledged}
              onChange={(event) => setAcknowledged(event.target.checked)}
              className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
            />
            <span>I understand the panel will be briefly unavailable.</span>
          </label>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-gray-200 px-5 py-4">
          <Button
            ref={cancelRef}
            type="button"
            variant="secondary"
            onClick={onCancel}
            disabled={submitting}
          >
            Cancel
          </Button>
          <Button type="button" onClick={onConfirm} disabled={!acknowledged || submitting}>
            {submitting ? 'Starting…' : `Upgrade to ${toVersion}`}
          </Button>
        </div>
      </div>
    </div>
  );
}
