'use client';

/**
 * Enrolment: password, then scan, then prove a code.
 *
 * The middle step is the point of the whole screen. Two-factor is not switched
 * on when the secret is generated - only when the user has typed a code that
 * the panel accepts. Enabling on generation is how people lock themselves out
 * of their own panel when an authenticator app quietly fails to take the
 * secret.
 */

import { useState } from 'react';
import { Check, Copy, KeyRound, Loader2, ShieldCheck, Smartphone } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import TwoFactorQRCode from './TwoFactorQRCode';
import TwoFactorRecoveryCodes from './TwoFactorRecoveryCodes';
import {
  errorMessage,
  twoFactorApi,
  type RecoveryCodeSet,
  type TwoFactorEnrolment,
} from './TwoFactorTypes';
import { unwrap } from '@/services/api';

const LABEL_CLASS = 'mb-1.5 block text-sm font-medium text-gray-700';
const HELP_CLASS = 'mt-1 text-xs text-gray-500';

type Step = 'password' | 'scan' | 'codes';

export interface TwoFactorEnrollProps {
  onCancel: () => void;
  onEnabled: () => void;
}

function CopyField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      /* Clipboard blocked: the value is on screen and selectable. */
    }
  };

  return (
    <div>
      <span className={LABEL_CLASS}>{label}</span>
      <div className="flex items-start gap-2">
        <code
          className={`min-w-0 flex-1 select-all break-all rounded-md border border-gray-200 bg-gray-50 px-3 py-2 text-xs text-gray-900 ${
            mono ? 'font-mono tracking-wider' : ''
          }`}
        >
          {value}
        </code>
        <Button type="button" variant="secondary" size="sm" onClick={copy} aria-label={`Copy ${label}`}>
          {copied ? <Check size={14} /> : <Copy size={14} />}
          {copied ? 'Copied' : 'Copy'}
        </Button>
      </div>
    </div>
  );
}

export default function TwoFactorEnroll({ onCancel, onEnabled }: TwoFactorEnrollProps) {
  const [step, setStep] = useState<Step>('password');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [enrolment, setEnrolment] = useState<TwoFactorEnrolment | null>(null);
  const [codes, setCodes] = useState<RecoveryCodeSet | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const start = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const response = await twoFactorApi.enroll(password);
      const data = unwrap<TwoFactorEnrolment>(response, null);
      if (!data) throw new Error('empty response');
      setEnrolment(data);
      setPassword('');
      setStep('scan');
    } catch (err) {
      setError(errorMessage(err, 'Could not start enrolment. Check your password and try again.'));
    } finally {
      setBusy(false);
    }
  };

  const confirm = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const response = await twoFactorApi.confirm(code.trim());
      const data = unwrap<RecoveryCodeSet>(response, null);
      if (!data) throw new Error('empty response');
      setCodes(data);
      setCode('');
      setStep('codes');
    } catch (err) {
      setError(errorMessage(err, 'That code was not accepted. Wait for the next one and try again.'));
    } finally {
      setBusy(false);
    }
  };

  if (step === 'codes' && codes) {
    return (
      <TwoFactorRecoveryCodes
        set={codes}
        account={enrolment?.account}
        onAcknowledge={onEnabled}
      />
    );
  }

  if (step === 'scan' && enrolment) {
    return (
      <form onSubmit={confirm} className="space-y-5">
        <ol className="space-y-5">
          <li className="flex gap-3">
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-gray-100 text-xs font-semibold text-gray-700">
              1
            </span>
            <div className="min-w-0 flex-1 space-y-3">
              <p className="text-sm font-medium text-gray-900">
                <Smartphone size={14} className="mr-1.5 inline text-gray-400" />
                Scan this with your authenticator app
              </p>
              <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
                <TwoFactorQRCode value={enrolment.otpauth_uri} size={200} />
                <div className="min-w-0 flex-1 space-y-3">
                  <CopyField label="Setup key (if you cannot scan)" value={enrolment.secret} mono />
                  <CopyField label="Setup link" value={enrolment.otpauth_uri} />
                  <p className={HELP_CLASS}>
                    {enrolment.algorithm}, {enrolment.digits} digits, a new code every{' '}
                    {enrolment.period_seconds} seconds.
                  </p>
                </div>
              </div>
            </div>
          </li>

          <li className="flex gap-3">
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-gray-100 text-xs font-semibold text-gray-700">
              2
            </span>
            <div className="min-w-0 flex-1">
              <label htmlFor="two-factor-code" className={LABEL_CLASS}>
                Enter the code your app is showing now
              </label>
              <Input
                id="two-factor-code"
                value={code}
                onChange={(event) => setCode(event.target.value)}
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={enrolment.digits}
                placeholder={'0'.repeat(enrolment.digits)}
                className="max-w-[12rem] font-mono tracking-[0.35em]"
                autoFocus
              />
              <p className={HELP_CLASS}>
                Two-factor authentication is not switched on until this code is accepted, so a phone
                that failed to take the key cannot lock you out.
              </p>
            </div>
          </li>
        </ol>

        {error ? (
          <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">
            {error}
          </p>
        ) : null}

        <div className="flex items-center gap-2">
          <Button type="submit" disabled={busy || code.trim().length !== enrolment.digits}>
            {busy ? <Loader2 size={16} className="animate-spin" /> : <ShieldCheck size={16} />}
            Turn on two-factor
          </Button>
          <Button type="button" variant="secondary" onClick={onCancel} disabled={busy}>
            Cancel
          </Button>
        </div>
      </form>
    );
  }

  return (
    <form onSubmit={start} className="max-w-md space-y-4">
      <div>
        <label htmlFor="two-factor-password" className={LABEL_CLASS}>
          Confirm your password
        </label>
        <Input
          id="two-factor-password"
          type="password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          autoComplete="current-password"
          autoFocus
        />
        <p className={HELP_CLASS}>
          Asked here so that a session left open on an unattended machine cannot move your second
          factor to somebody else&apos;s phone.
        </p>
      </div>

      {error ? (
        <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">
          {error}
        </p>
      ) : null}

      <div className="flex items-center gap-2">
        <Button type="submit" disabled={busy || password.length === 0}>
          {busy ? <Loader2 size={16} className="animate-spin" /> : <KeyRound size={16} />}
          Continue
        </Button>
        <Button type="button" variant="secondary" onClick={onCancel} disabled={busy}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
