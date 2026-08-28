'use client';

/**
 * Two-factor authentication settings.
 *
 * One card, one state at a time: what the account has now, and the single
 * action that changes it. The status half always says three things a user
 * actually needs - whether the second factor is on, when it was last used, and
 * how many recovery codes are left - because a silent run down to zero codes
 * leaves the account one lost phone away from an out-of-band recovery process.
 */

import { useCallback, useEffect, useState } from 'react';
import {
  AlertTriangle,
  CheckCircle,
  Clock,
  KeyRound,
  Loader2,
  RefreshCw,
  ShieldCheck,
  ShieldOff,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { unwrap } from '@/services/api';
import TwoFactorDisable, { type TwoFactorCredentialAction } from './TwoFactorDisable';
import TwoFactorEnroll from './TwoFactorEnroll';
import TwoFactorRecoveryCodes from './TwoFactorRecoveryCodes';
import {
  errorMessage,
  formatDateTime,
  twoFactorApi,
  type RecoveryCodeSet,
  type TwoFactorStatus,
} from './TwoFactorTypes';

type Mode = 'idle' | 'enrolling' | 'disabling' | 'regenerating' | 'showing-codes';

function Row({ icon, label, children }: { icon?: React.ReactNode; label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-1 gap-1 border-b border-gray-100 py-3 last:border-b-0 sm:grid-cols-[220px_minmax(0,1fr)] sm:gap-4">
      <div className="flex items-center gap-2 text-sm font-medium text-gray-700">
        {icon ? <span className="text-gray-400">{icon}</span> : null}
        {label}
      </div>
      <div className="min-w-0 text-sm text-gray-900">{children}</div>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div className="animate-pulse space-y-3" aria-hidden="true">
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className="grid grid-cols-1 gap-2 sm:grid-cols-[220px_minmax(0,1fr)] sm:gap-4">
          <div className="h-4 w-40 rounded bg-gray-100" />
          <div className="h-4 w-full max-w-md rounded bg-gray-100" />
        </div>
      ))}
    </div>
  );
}

export default function TwoFactorSettings() {
  const [status, setStatus] = useState<TwoFactorStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [mode, setMode] = useState<Mode>('idle');
  const [freshCodes, setFreshCodes] = useState<RecoveryCodeSet | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const response = await twoFactorApi.status();
      const data = unwrap<TwoFactorStatus>(response, null);
      if (!data) throw new Error('empty response');
      setStatus(data);
      setError(null);
    } catch (err) {
      setError(errorMessage(err, 'Could not read two-factor status.'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const finish = useCallback(
    (message: string | null) => {
      setMode('idle');
      setFreshCodes(null);
      setNotice(message);
      void load();
    },
    [load]
  );

  const startCredentialAction = (action: TwoFactorCredentialAction) => {
    setNotice(null);
    setMode(action === 'disable' ? 'disabling' : 'regenerating');
  };

  const digits = status?.digits ?? 6;

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-4 space-y-0">
        <div className="min-w-0">
          <CardTitle>Two-factor authentication</CardTitle>
          <p className="mt-1 text-sm text-gray-600">
            A time-based code from your phone, on top of your password.
          </p>
        </div>
        {status ? (
          <Badge variant={status.enabled ? 'success' : 'warning'}>
            {status.enabled ? <CheckCircle size={12} /> : <AlertTriangle size={12} />}
            {status.enabled ? 'On' : 'Off'}
          </Badge>
        ) : null}
      </CardHeader>

      <CardContent className="space-y-5">
        {loading ? <LoadingSkeleton /> : null}

        {!loading && error ? (
          <div className="flex items-start justify-between gap-3 rounded-md border border-red-200 bg-red-50 p-3">
            <p className="text-sm text-red-700">{error}</p>
            <Button type="button" variant="secondary" size="sm" onClick={() => void load()}>
              <RefreshCw size={14} />
              Retry
            </Button>
          </div>
        ) : null}

        {!loading && notice ? (
          <p className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
            {notice}
          </p>
        ) : null}

        {!loading && status && mode === 'idle' ? (
          <>
            <div>
              <Row icon={<ShieldCheck size={16} />} label="Status">
                {status.enabled ? (
                  <span className="text-gray-900">
                    On since {formatDateTime(status.confirmed_at)}
                  </span>
                ) : status.pending_enrolment ? (
                  <span className="text-amber-700">
                    Started but not finished. Two-factor stays off until a code is proved.
                  </span>
                ) : (
                  <span className="text-gray-600">
                    Off. Your password is the only thing protecting this account.
                  </span>
                )}
              </Row>

              {status.enabled ? (
                <>
                  <Row icon={<Clock size={16} />} label="Last used">
                    {formatDateTime(status.last_used_at)}
                  </Row>

                  <Row icon={<KeyRound size={16} />} label="Recovery codes">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className={status.recovery_codes_low ? 'text-amber-700' : 'text-gray-900'}>
                        {status.recovery_codes_remaining} of {status.recovery_codes_total} unused
                      </span>
                      {status.recovery_codes_low ? (
                        <Badge variant="warning">
                          <AlertTriangle size={12} />
                          Running low
                        </Badge>
                      ) : null}
                    </div>
                    {status.recovery_codes_low ? (
                      <p className="mt-1 text-xs text-amber-700">
                        Issue a new set before you run out. With no codes and no phone, only an
                        administrator can get you back in.
                      </p>
                    ) : null}
                  </Row>

                  <Row icon={<ShieldCheck size={16} />} label="Algorithm">
                    {status.algorithm}, {status.digits} digits, new code every {status.period_seconds}s
                  </Row>
                </>
              ) : null}

              {status.locked_until ? (
                <Row icon={<AlertTriangle size={16} />} label="Locked">
                  <span className="text-red-700">
                    Too many failed codes. Verification reopens at {formatDateTime(status.locked_until)}.
                  </span>
                </Row>
              ) : null}
            </div>

            <div className="flex flex-wrap items-center gap-2">
              {status.enabled ? (
                <>
                  <Button type="button" variant="secondary" onClick={() => startCredentialAction('regenerate')}>
                    <RefreshCw size={16} />
                    Issue new recovery codes
                  </Button>
                  <Button type="button" variant="danger-outline" onClick={() => startCredentialAction('disable')}>
                    <ShieldOff size={16} />
                    Turn off
                  </Button>
                </>
              ) : (
                <Button
                  type="button"
                  onClick={() => {
                    setNotice(null);
                    setMode('enrolling');
                  }}
                >
                  <ShieldCheck size={16} />
                  {status.pending_enrolment ? 'Finish setting up' : 'Set up two-factor'}
                </Button>
              )}
            </div>
          </>
        ) : null}

        {mode === 'enrolling' ? (
          <TwoFactorEnroll
            onCancel={() => finish(null)}
            onEnabled={() => finish('Two-factor authentication is on for this account.')}
          />
        ) : null}

        {mode === 'disabling' || mode === 'regenerating' ? (
          <TwoFactorDisable
            action={mode === 'disabling' ? 'disable' : 'regenerate'}
            digits={digits}
            onCancel={() => finish(null)}
            onDisabled={() => finish('Two-factor authentication has been turned off.')}
            onRegenerated={(set) => {
              setFreshCodes(set);
              setMode('showing-codes');
            }}
          />
        ) : null}

        {mode === 'showing-codes' && freshCodes ? (
          <TwoFactorRecoveryCodes
            set={freshCodes}
            onAcknowledge={() => finish('A new set of recovery codes has been issued.')}
          />
        ) : null}

        {loading ? (
          <p className="flex items-center gap-2 text-xs text-gray-500">
            <Loader2 size={12} className="animate-spin" />
            Loading
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}
