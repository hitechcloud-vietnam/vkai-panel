'use client';

/**
 * How to connect to one database, and the one thing the panel honestly cannot
 * tell you.
 *
 * models.DatabaseEntry declares the password as `Password string \`json:"-"\``.
 * The API therefore never sends it, and no amount of interface can conjure it
 * back. So the password row does not pretend: it offers a reveal only for a
 * secret this browser session actually holds - one the operator just typed when
 * creating the database or setting a new password - and otherwise says where
 * the secret went and how to get a new one.
 *
 * The revealed value is held in React state and written to the clipboard from a
 * closure. Until the operator explicitly reveals it, it is not in the document,
 * so it is not in the page source, not in the accessibility tree, and not in a
 * screenshot of the page.
 */

import { useCallback, useEffect, useState } from 'react';
import { Check, Copy, Eye, EyeOff, KeyRound } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { SECONDARY_BUTTON_CLASS } from './PaneChrome';

/** Seconds a revealed secret stays on screen before it hides itself again. */
const REVEAL_SECONDS = 30;

function CopyButton({
  label,
  getValue,
}: {
  label: string;
  getValue: () => string;
}) {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), 2000);
    return () => clearTimeout(timer);
  }, [copied]);

  const copy = useCallback(async () => {
    const value = getValue();
    if (!value) return;
    try {
      // navigator.clipboard is undefined on an insecure origin, which a panel
      // reached by bare IP often is. The textarea path keeps copy working there
      // instead of failing silently.
      if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(value);
      } else if (typeof document !== 'undefined') {
        const scratch = document.createElement('textarea');
        scratch.value = value;
        scratch.setAttribute('readonly', '');
        scratch.style.position = 'fixed';
        scratch.style.opacity = '0';
        document.body.appendChild(scratch);
        scratch.select();
        document.execCommand('copy');
        document.body.removeChild(scratch);
      }
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }, [getValue]);

  return (
    <button
      type="button"
      onClick={copy}
      aria-label={copied ? `${label} copied` : `Copy ${label.toLowerCase()}`}
      title={copied ? 'Copied' : `Copy ${label.toLowerCase()}`}
      className="rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
    >
      {copied ? (
        <Check className="h-4 w-4 text-emerald-600" aria-hidden="true" />
      ) : (
        <Copy className="h-4 w-4" aria-hidden="true" />
      )}
    </button>
  );
}

function DetailRow({
  label,
  value,
  mono = true,
  copyable = true,
}: {
  label: string;
  value: string;
  mono?: boolean;
  copyable?: boolean;
}) {
  return (
    <div className="flex items-start justify-between gap-3 border-b border-gray-100 py-2 last:border-b-0">
      <dt className="w-28 shrink-0 text-xs font-medium uppercase tracking-wide text-gray-500">
        {label}
      </dt>
      <dd className="flex min-w-0 flex-1 items-center justify-end gap-1">
        <span className={cn('truncate text-sm text-gray-900', mono && 'font-mono')}>
          {value}
        </span>
        {copyable && value ? <CopyButton label={label} getValue={() => value} /> : null}
      </dd>
    </div>
  );
}

export interface ConnectionDetailsProps {
  host: string;
  port: string;
  database: string;
  /** "User" on MySQL, "Role" on PostgreSQL - the engine decides the word. */
  accountLabel: string;
  account: string;
  /**
   * The password, when THIS browser session knows it because the operator just
   * set it. Undefined means the panel does not have it and will say so.
   */
  sessionSecret?: string;
  /** A ready-to-paste connection string, when the engine has a standard one. */
  connectionString?: string;
  /** Opens the change-password dialog. */
  onSetPassword?: () => void;
}

export function ConnectionDetails({
  host,
  port,
  database,
  accountLabel,
  account,
  sessionSecret,
  connectionString,
  onSetPassword,
}: ConnectionDetailsProps) {
  const [revealed, setRevealed] = useState(false);

  // A secret left on screen is a secret on a colleague's photograph of the
  // screen. It hides itself whether or not anyone remembers to.
  useEffect(() => {
    if (!revealed) return;
    const timer = setTimeout(() => setRevealed(false), REVEAL_SECONDS * 1000);
    return () => clearTimeout(timer);
  }, [revealed]);

  // A new row means a different secret; never carry a reveal across.
  useEffect(() => {
    setRevealed(false);
  }, [database, account, sessionSecret]);

  return (
    <div className="rounded-md border border-gray-200 bg-gray-50 p-3">
      <dl className="m-0">
        <DetailRow label="Host" value={host} />
        <DetailRow label="Port" value={port} />
        <DetailRow label="Database" value={database} />
        <DetailRow label={accountLabel} value={account} />

        <div className="flex items-start justify-between gap-3 border-b border-gray-100 py-2 last:border-b-0">
          <dt className="w-28 shrink-0 text-xs font-medium uppercase tracking-wide text-gray-500">
            Password
          </dt>
          <dd className="flex min-w-0 flex-1 items-center justify-end gap-1">
            {sessionSecret ? (
              <>
                <span className="truncate font-mono text-sm text-gray-900">
                  {revealed ? sessionSecret : '••••••••••••'}
                </span>
                <button
                  type="button"
                  onClick={() => setRevealed((v) => !v)}
                  aria-label={revealed ? 'Hide password' : 'Reveal password'}
                  title={
                    revealed
                      ? 'Hide password'
                      : `Reveal password for ${REVEAL_SECONDS} seconds`
                  }
                  className="rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                >
                  {revealed ? (
                    <EyeOff className="h-4 w-4" aria-hidden="true" />
                  ) : (
                    <Eye className="h-4 w-4" aria-hidden="true" />
                  )}
                </button>
                <CopyButton label="Password" getValue={() => sessionSecret} />
              </>
            ) : (
              <span className="text-right text-sm text-gray-500">
                Stored encrypted and never returned by the API. Set a new one to learn it.
              </span>
            )}
          </dd>
        </div>
      </dl>

      {connectionString && (
        <div className="mt-3 flex items-center gap-2 rounded-md border border-gray-200 bg-white px-3 py-2">
          <span className="min-w-0 flex-1 truncate font-mono text-xs text-gray-700">
            {connectionString}
          </span>
          <CopyButton label="Connection string" getValue={() => connectionString} />
        </div>
      )}

      {sessionSecret && (
        <p className="mt-2 text-xs text-amber-700">
          This password is held in this browser tab only. Close the tab and the panel
          cannot show it again.
        </p>
      )}

      {onSetPassword && (
        <Button
          type="button"
          variant="outline"
          onClick={onSetPassword}
          className={cn(SECONDARY_BUTTON_CLASS, 'mt-3 w-full')}
        >
          <KeyRound className="mr-2 h-4 w-4" aria-hidden="true" />
          Set a new password
        </Button>
      )}
    </div>
  );
}

export default ConnectionDetails;
