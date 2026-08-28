'use client';

/**
 * Shared types and API calls for the two-factor screens.
 *
 * The calls live here rather than in services/api.ts so that this feature is
 * self-contained: one directory holds the whole flow, and the shared axios
 * instance (with its bearer token and refresh handling) is still what talks to
 * the panel.
 */

import { api } from '@/services/api';

export interface TwoFactorStatus {
  enabled: boolean;
  pending_enrolment: boolean;
  confirmed_at: string | null;
  last_used_at: string | null;
  locked_until: string | null;
  recovery_codes_remaining: number;
  recovery_codes_total: number;
  recovery_codes_low: boolean;
  algorithm: string;
  digits: number;
  period_seconds: number;
}

export interface TwoFactorEnrolment {
  secret: string;
  otpauth_uri: string;
  issuer: string;
  account: string;
  algorithm: string;
  digits: number;
  period_seconds: number;
  expires_at: string;
}

export interface RecoveryCodeSet {
  codes: string[];
  count: number;
  generated_at: string;
}

export const twoFactorApi = {
  status: () => api.get('/api/v1/two-factor/status'),
  /** Starts enrolment. Returns a secret; does NOT turn two-factor on. */
  enroll: (password: string) => api.post('/api/v1/two-factor/enroll', { password }),
  /** The only call that enables two-factor, and only on a proven code. */
  confirm: (code: string) => api.post('/api/v1/two-factor/enroll/verify', { code }),
  verify: (code: string) => api.post('/api/v1/two-factor/verify', { code }),
  regenerateRecoveryCodes: (password: string, code: string) =>
    api.post('/api/v1/two-factor/recovery-codes', { password, code }),
  disable: (password: string, code: string) =>
    api.post('/api/v1/two-factor/disable', { password, code }),
};

/** errorMessage pulls the panel's error text out of an axios failure. */
export function errorMessage(err: unknown, fallback: string): string {
  const response = (err as { response?: { data?: { error?: { message?: string } } } })?.response;
  return response?.data?.error?.message || fallback;
}

/** formatDateTime renders a timestamp for display, or a dash when absent. */
export function formatDateTime(value: string | null | undefined): string {
  if (!value) return '--';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return '--';
  return parsed.toLocaleString();
}
