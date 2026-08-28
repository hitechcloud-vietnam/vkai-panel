'use client';

/**
 * Loading the security posture, one source at a time.
 *
 * Six endpoints feed this screen and they carry six different permissions:
 * /panel/settings and /firewall are administrator-only, /audit/search needs the
 * audit permission, /tamper-proof and /waf need settings, /two-factor needs
 * nothing. An operator with a partial role will get a 403 from some of them,
 * and a single Promise.all would turn one 403 into a blank page.
 *
 * So each source is loaded and kept independently, with its own status and its
 * own message. A source that failed does not contribute a green tick and does
 * not contribute a red cross either - it contributes an item whose state is
 * "not verifiable", carrying the reason the request gave. That distinction is
 * the whole point of this screen.
 */

import { useCallback, useEffect, useRef, useState } from 'react';

import { errorMessage, httpStatus } from '@/lib/apiError';

import * as securityApi from './api';
import type {
  AuditLog,
  FirewallRule,
  PanelSettingsView,
  TamperStats,
  TwoFactorStatus,
  WafRule,
} from './types';

export type SourceStatus = 'loading' | 'ok' | 'error';

export interface Source<T> {
  data: T | null;
  status: SourceStatus;
  /** The message the API sent. Empty unless status is 'error'. */
  error: string;
  /** 0 when the request never reached the server. */
  code: number;
}

function idle<T>(): Source<T> {
  return { data: null, status: 'loading', error: '', code: 0 };
}

/**
 * Runs one request and records how it went.
 *
 * A resolved-but-empty payload is treated as a failure rather than as data.
 * An endpoint that answers 200 with no body is not telling this screen that
 * everything is fine - it is telling it nothing, and a nothing rendered as a
 * green tick is the whole class of defect this screen was written to avoid.
 */
async function load<T>(fn: () => Promise<T | null>, what: string): Promise<Source<T>> {
  try {
    const data = await fn();
    if (data === null || data === undefined) {
      return {
        data: null,
        status: 'error',
        error: `The panel answered without ${what}.`,
        code: 0,
      };
    }
    return { data, status: 'ok', error: '', code: 0 };
  } catch (err) {
    return {
      data: null,
      status: 'error',
      error: errorMessage(err, 'The panel did not answer this request.'),
      code: httpStatus(err),
    };
  }
}

/** Turns a failed source into the sentence a posture item will carry. */
export function sourceReason(source: Source<unknown>, what: string): string {
  if (source.status === 'loading') return `Still reading ${what}.`;
  if (source.code === 403) {
    return `Your account is not allowed to read ${what}, so this screen cannot confirm it.`;
  }
  if (source.code === 404) {
    return `The panel has no endpoint for ${what} on this build.`;
  }
  if (source.code === 503) {
    return `The panel cannot serve ${what} on this installation: ${source.error}`;
  }
  // The empty-payload case already reads as a sentence; wrapping it again
  // produces "the audit log could not be read: The panel answered without the
  // audit log", which says the same thing twice.
  if (source.code === 0 && source.error.startsWith('The panel answered without')) {
    return source.error;
  }
  return `${what} could not be read: ${source.error}`;
}

export interface PostureSources {
  panel: Source<PanelSettingsView>;
  twoFactor: Source<TwoFactorStatus>;
  firewall: Source<FirewallRule[]>;
  tamper: Source<TamperStats>;
  waf: Source<WafRule[]>;
  failures: Source<AuditLog[]>;
}

export interface UsePostureResult extends PostureSources {
  /** True until every source has settled once. */
  loading: boolean;
  /** True while a manual refresh is in flight over already-loaded data. */
  refreshing: boolean;
  reload: () => void;
}

export function usePosture(): UsePostureResult {
  const [sources, setSources] = useState<PostureSources>({
    panel: idle<PanelSettingsView>(),
    twoFactor: idle<TwoFactorStatus>(),
    firewall: idle<FirewallRule[]>(),
    tamper: idle<TamperStats>(),
    waf: idle<WafRule[]>(),
    failures: idle<AuditLog[]>(),
  });
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  // Six requests land after the operator may already have navigated away.
  const mounted = useRef(true);

  // Six requests that must not be able to cancel each other. Promise.all is
  // safe here only because every element is already a resolved Source rather
  // than a rejection: `load` never throws, so one 403 cannot take the other
  // five down with it.
  const run = useCallback(async (initial: boolean) => {
    if (initial) setLoading(true);
    else setRefreshing(true);

    const [panel, twoFactor, firewall, tamper, waf, failures] = await Promise.all([
      load(() => securityApi.panelAccess.get(), 'the panel access settings'),
      load(() => securityApi.twoFactor.status(), 'a two-factor status'),
      load(() => securityApi.firewall.list(), 'the firewall rules'),
      load(() => securityApi.tamperProof.stats(), 'the file integrity statistics'),
      load(() => securityApi.waf.rules(), 'the WAF rules'),
      load(() => securityApi.audit.signInFailures(200), 'the audit log'),
    ]);

    if (!mounted.current) return;
    setSources({ panel, twoFactor, firewall, tamper, waf, failures });
    setLoading(false);
    setRefreshing(false);
  }, []);

  useEffect(() => {
    mounted.current = true;
    void run(true);
    return () => {
      mounted.current = false;
    };
  }, [run]);

  const reload = useCallback(() => {
    void run(false);
  }, [run]);

  return { ...sources, loading, refreshing, reload };
}
