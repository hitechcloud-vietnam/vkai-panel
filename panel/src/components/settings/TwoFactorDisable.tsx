'use client';

/**
 * Turning two-factor off, and reissuing recovery codes.
 *
 * Both cost the current password AND a current code, because both hand back
 * exactly what an attacker with a stolen session wants: an account with one
 * factor. The form says what will happen before it happens, and the panel
 * writes the result to the audit log either way.
 */

import { useState } from 'react';
import { Loader2, ShieldOff, RefreshCw } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { errorMessage, twoFactorApi, type RecoveryCodeSet } from './TwoFactorTypes';
import { unwrap } from '@/services/api';

const LABEL_CLASS = 'mb-1.5 block text-sm font-medium text-gray-700';
const HELP_CLASS = 'mt-1 text-xs text-gray-500';

export type TwoFactorCredentialAction = 'disable' | 'regenerate';

export interface TwoFactorDisableProps {
  action: TwoFactorCredentialAction;
  digits: number;
  onCancel: () => void;
  onDisabled: () => void;
  onRegenerated: (set: RecoveryCodeSet) => void;
}

export default function TwoFactorDisable({
  action,
  digits,
  onCancel,
  onDisabled,
  onRegenerated,
}: TwoFactorDisableProps) {
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const disabling = action === 'disable';

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      if (disabling) {
        await twoFactorApi.disable(password, code.trim());
        onDisabled();
      } else {
        const response = await twoFactorApi.regenerateRecoveryCodes(password, code.trim());
        const set = unwrap<RecoveryCodeSet>(response, null);
        if (!set) throw new Error('empty response');
        onRegenerated(set);
      }
    } catch (err) {
      setError(
        errorMessage(
          err,
          disabling
            ? 'Two-factor was not turned off. Check your password and code.'
            : 'New recovery codes were not issued. Check your password and code.'
        )
      );
      setBusy(false);
      return;
    }
    setBusy(false);
  };

  return (
    <form onSubmit={submit} className="max-w-md space-y-4">
      <div
        className={`rounded-md border p-3 text-sm ${
          disabling ? 'border-red-200 bg-red-50 text-red-800' : 'border-gray-200 bg-gray-50 text-gray-700'
        }`}
      >
        {disabling ? (
          <>
            Turning two-factor off leaves your password as the only thing between an attacker and
            every server this panel manages. Your recovery codes are destroyed with it.
          </>
        ) : (
          <>
            A new set replaces the old one immediately. Any code you have written down or printed
            stops working as soon as the new codes are issued.
          </>
        )}
      </div>

      <div>
        <label htmlFor="two-factor-current-password" className={LABEL_CLASS}>
          Current password
        </label>
        <Input
          id="two-factor-current-password"
          type="password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          autoComplete="current-password"
          autoFocus
        />
      </div>

      <div>
        <label htmlFor="two-factor-current-code" className={LABEL_CLASS}>
          Current code
        </label>
        <Input
          id="two-factor-current-code"
          value={code}
          onChange={(event) => setCode(event.target.value)}
          inputMode="text"
          autoComplete="one-time-code"
          placeholder={'0'.repeat(digits)}
          className="max-w-[16rem] font-mono tracking-[0.2em]"
        />
        <p className={HELP_CLASS}>
          A code from your authenticator app, or one of your unused recovery codes.
        </p>
      </div>

      {error ? (
        <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">
          {error}
        </p>
      ) : null}

      <div className="flex items-center gap-2">
        <Button
          type="submit"
          variant={disabling ? 'destructive' : 'default'}
          disabled={busy || password.length === 0 || code.trim().length === 0}
        >
          {busy ? (
            <Loader2 size={16} className="animate-spin" />
          ) : disabling ? (
            <ShieldOff size={16} />
          ) : (
            <RefreshCw size={16} />
          )}
          {disabling ? 'Turn off two-factor' : 'Issue new recovery codes'}
        </Button>
        <Button type="button" variant="secondary" onClick={onCancel} disabled={busy}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
