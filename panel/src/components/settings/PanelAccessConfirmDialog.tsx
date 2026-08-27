'use client';

/**
 * The dialog that stands between an operator and a locked-out panel.
 *
 * It exists because every change on this page can be the last one an operator
 * ever makes from this browser. It therefore does three things and nothing
 * else: it spells out the exact URL to use afterwards, it lists what is
 * changing and why the current session is at risk, and it refuses to submit
 * until the operator has ticked the box saying the URL has been written down.
 */

import { useEffect, useRef, useState } from 'react';
import { AlertTriangle, Check, Copy, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  PANEL_FIELD_LABELS,
  PANEL_SECRET_FIELDS,
  type PanelConfirmationReason,
  type PanelSettingChange,
} from './panel-access-types';

interface PanelAccessConfirmDialogProps {
  open: boolean;
  title: string;
  /** The URL the operator has to use once the change is applied. */
  newUrl: string;
  reasons: PanelConfirmationReason[];
  changes: PanelSettingChange[];
  submitting: boolean;
  confirmLabel: string;
  onCancel: () => void;
  onConfirm: () => void;
}

function displayValue(field: string, value: string): string {
  if (PANEL_SECRET_FIELDS.has(field)) return '••••••••';
  if (value === '') return '(empty)';
  return value;
}

export default function PanelAccessConfirmDialog({
  open,
  title,
  newUrl,
  reasons,
  changes,
  submitting,
  confirmLabel,
  onCancel,
  onConfirm,
}: PanelAccessConfirmDialogProps) {
  const [acknowledged, setAcknowledged] = useState(false);
  const [copied, setCopied] = useState(false);
  const cancelRef = useRef<HTMLButtonElement | null>(null);

  // Reopening the dialog must never inherit the previous acknowledgement:
  // a tick left over from a change the operator abandoned is not consent.
  useEffect(() => {
    if (open) {
      setAcknowledged(false);
      setCopied(false);
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

  const copyUrl = async () => {
    try {
      await navigator.clipboard.writeText(newUrl);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      /* Clipboard blocked: the URL is on screen and selectable anyway. */
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40 p-4">
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="panel-access-confirm-title"
        className="max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg"
      >
        <div className="flex items-start justify-between gap-4 border-b border-gray-200 px-5 py-4">
          <div className="flex items-start gap-3">
            <span className="mt-0.5 text-amber-600">
              <AlertTriangle size={18} />
            </span>
            <div>
              <h2 id="panel-access-confirm-title" className="text-sm font-semibold text-gray-900">
                {title}
              </h2>
              <p className="mt-1 text-sm text-gray-600">
                Save the new address before you continue. Nothing else on this machine will show it
                to you again.
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
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">
              New panel URL
            </p>
            <div className="mt-1.5 flex items-center gap-2 rounded-md border border-gray-200 bg-gray-50 px-3 py-2">
              <code className="min-w-0 flex-1 break-all font-mono text-sm text-gray-900">
                {newUrl || '(unchanged)'}
              </code>
              {newUrl ? (
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  onClick={copyUrl}
                  aria-label="Copy the new panel URL"
                >
                  {copied ? <Check size={14} /> : <Copy size={14} />}
                  {copied ? 'Copied' : 'Copy'}
                </Button>
              ) : null}
            </div>
          </div>

          {reasons.length > 0 ? (
            <ul className="space-y-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2.5">
              {reasons.map((reason) => (
                <li key={reason.code} className="text-sm text-amber-800">
                  {reason.message}
                </li>
              ))}
            </ul>
          ) : null}

          {changes.length > 0 ? (
            <div>
              <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                Changes
              </p>
              <div className="mt-1.5 overflow-x-auto rounded-md border border-gray-200">
                <table className="w-full min-w-[420px] text-sm">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-3 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">
                        Setting
                      </th>
                      <th className="px-3 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">
                        Current
                      </th>
                      <th className="px-3 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">
                        New
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {changes.map((change) => (
                      <tr key={change.field}>
                        <td className="px-3 py-2 text-gray-700">
                          {PANEL_FIELD_LABELS[change.field] ?? change.field}
                        </td>
                        <td className="px-3 py-2 font-mono text-xs text-gray-500">
                          {displayValue(change.field, change.old)}
                        </td>
                        <td className="px-3 py-2 font-mono text-xs text-gray-900">
                          {displayValue(change.field, change.new)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ) : null}

          <label className="flex items-start gap-2.5 text-sm text-gray-700">
            <input
              type="checkbox"
              checked={acknowledged}
              onChange={(event) => setAcknowledged(event.target.checked)}
              className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
            />
            <span>I have saved the new panel URL and understand the current one may stop working.</span>
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
            {submitting ? 'Applying…' : confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}
