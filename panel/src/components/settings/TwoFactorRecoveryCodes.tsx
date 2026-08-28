'use client';

/**
 * The one and only showing of a set of recovery codes.
 *
 * The panel stores hashes, so this list cannot be produced again. The screen is
 * built around that fact: the codes are large and selectable, copy and download
 * are one click, and the way onward is blocked behind an explicit "I have saved
 * these" acknowledgement.
 */

import { useState } from 'react';
import { AlertTriangle, Check, Copy, Download } from 'lucide-react';
import { Button } from '@/components/ui/button';
import type { RecoveryCodeSet } from './TwoFactorTypes';

export interface TwoFactorRecoveryCodesProps {
  set: RecoveryCodeSet;
  /** Account name used in the downloaded file, for people with several panels. */
  account?: string;
  onAcknowledge: () => void;
  acknowledgeLabel?: string;
}

export default function TwoFactorRecoveryCodes({
  set,
  account,
  onAcknowledge,
  acknowledgeLabel = 'I have saved these codes',
}: TwoFactorRecoveryCodesProps) {
  const [copied, setCopied] = useState(false);
  const [saved, setSaved] = useState(false);

  const asText = [
    'VKAI Panel two-factor recovery codes',
    account ? `Account: ${account}` : null,
    `Generated: ${new Date(set.generated_at).toLocaleString()}`,
    '',
    'Each code works once. Keep them somewhere you can reach without your phone.',
    '',
    ...set.codes,
    '',
  ]
    .filter((line) => line !== null)
    .join('\n');

  const copyAll = async () => {
    try {
      await navigator.clipboard.writeText(set.codes.join('\n'));
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      /* Clipboard blocked: the codes are on screen and selectable. */
    }
  };

  const download = () => {
    const blob = new Blob([asText], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'vkai-panel-recovery-codes.txt';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  };

  return (
    <div className="space-y-4">
      <div className="flex items-start gap-3 rounded-md border border-amber-200 bg-amber-50 p-3">
        <AlertTriangle size={18} className="mt-0.5 shrink-0 text-amber-600" />
        <div className="text-sm text-amber-900">
          <p className="font-medium">These codes are shown once.</p>
          <p className="mt-1 text-amber-800">
            The panel keeps only their hashes and cannot show them again. Each code works a single
            time, and any one of them signs you in if you lose your phone.
          </p>
        </div>
      </div>

      <ul className="grid grid-cols-1 gap-2 rounded-md border border-gray-200 bg-gray-50 p-4 sm:grid-cols-2">
        {set.codes.map((code) => (
          <li
            key={code}
            className="select-all rounded border border-gray-200 bg-white px-3 py-2 text-center font-mono text-sm tracking-wider text-gray-900"
          >
            {code}
          </li>
        ))}
      </ul>

      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" variant="secondary" size="sm" onClick={copyAll}>
          {copied ? <Check size={14} /> : <Copy size={14} />}
          {copied ? 'Copied' : 'Copy all'}
        </Button>
        <Button type="button" variant="secondary" size="sm" onClick={download}>
          <Download size={14} />
          Download as text file
        </Button>
      </div>

      <label className="flex items-start gap-2 text-sm text-gray-700">
        <input
          type="checkbox"
          checked={saved}
          onChange={(event) => setSaved(event.target.checked)}
          className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
        />
        <span>{acknowledgeLabel}</span>
      </label>

      <Button type="button" disabled={!saved} onClick={onAcknowledge}>
        Done
      </Button>
    </div>
  );
}
