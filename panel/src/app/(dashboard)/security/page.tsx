'use client';

/**
 * Security.
 *
 * One page per area, a horizontal strip of sections above one content pane per
 * section - the shape aaPanel uses, so an operator moving across knows where
 * everything is. The selection lives in the URL (`/security?tab=firewall`), so
 * a tab can be bookmarked and a reload does not drop the operator back on the
 * Overview.
 *
 * What each pane may render is decided by what the API can actually do:
 *
 *   Firewall          wired to /api/v1/firewall - list, create, edit, delete,
 *                     persist, plus the live iptables table
 *   Brute force       wired to the audit log, which is the only record the
 *                     credential limiter leaves that HTTP can read
 *   Anti Intrusion    wired to /api/v1/tamper-proof - the file integrity module
 *   Overview /
 *   System Hardening  wired to panel settings, two-factor, firewall,
 *                     tamper-proof, WAF and the audit log
 *
 *   SSH, Server security, Website Security, Compiler Access
 *                     no endpoint exists. Each renders what is missing and no
 *                     control at all.
 *
 * The security scan endpoints (/api/v1/security/scans) are deliberately NOT on
 * this page. SecurityService.runScan sleeps and then writes a fixed score of 85
 * over 50 checks that were never performed; putting that number on a posture
 * screen would be the exact defect this screen exists to avoid.
 */

import { Suspense, useCallback, useMemo } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';

import AntiIntrusionPane from '@/components/security/AntiIntrusionPane';
import BruteForcePane from '@/components/security/BruteForcePane';
import CompilerAccessPane from '@/components/security/CompilerAccessPane';
import FirewallPane from '@/components/security/FirewallPane';
import OverviewPane from '@/components/security/OverviewPane';
import SecurityTabs from '@/components/security/SecurityTabs';
import ServerSecurityPane from '@/components/security/ServerSecurityPane';
import SshPane from '@/components/security/SshPane';
import SystemHardeningPane from '@/components/security/SystemHardeningPane';
import WebsiteSecurityPane from '@/components/security/WebsiteSecurityPane';
import { BlockSkeleton, Panel, SectionHeader } from '@/components/security/chrome';
import { toSecurityTab, type SecurityTabId } from '@/components/security/types';
import { usePosture } from '@/components/security/usePosture';

/** The sections with no endpoint behind them, marked on the tab strip. */
const NO_BACKEND: SecurityTabId[] = ['ssh', 'server-security', 'website-security', 'compiler-access'];

function SecurityWorkspace() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const active = toSecurityTab(searchParams.get('tab'));

  const posture = usePosture();

  // The tab goes in the URL rather than in state: a reload keeps the operator
  // where they were, and a link to one section is something a colleague can be
  // sent. `replace` rather than `push` so the back button leaves the Security
  // screen instead of walking backwards through its tabs.
  const select = useCallback(
    (tab: SecurityTabId) => {
      const params = new URLSearchParams(searchParams.toString());
      if (tab === 'overview') params.delete('tab');
      else params.set('tab', tab);
      const query = params.toString();
      router.replace(query ? `/security?${query}` : '/security', { scroll: false });
    },
    [router, searchParams]
  );

  const counts = useMemo(
    () => ({
      firewall: posture.firewall.status === 'ok' ? (posture.firewall.data?.length ?? 0) : null,
      'anti-intrusion': posture.tamper.status === 'ok' ? (posture.tamper.data?.active_alerts ?? 0) : null,
      'brute-force': posture.failures.status === 'ok' ? (posture.failures.data?.length ?? 0) : null,
    }),
    [posture.firewall, posture.tamper, posture.failures]
  );

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-lg font-semibold text-gray-900">Security</h1>
        <p className="mt-1 max-w-3xl text-sm text-gray-600">
          Everything this panel can see and change about the security of this machine. Where the
          panel cannot do something, the section says so rather than showing a control that would
          save nothing.
        </p>
      </div>

      <SecurityTabs active={active} onSelect={select} counts={counts} unavailable={NO_BACKEND} />

      <div
        role="tabpanel"
        id={`security-panel-${active}`}
        aria-labelledby={`security-tab-${active}`}
        tabIndex={-1}
      >
        {active === 'overview' && <OverviewPane posture={posture} onNavigate={select} />}
        {active === 'firewall' && <FirewallPane />}
        {active === 'ssh' && <SshPane />}
        {active === 'server-security' && <ServerSecurityPane />}
        {active === 'website-security' && <WebsiteSecurityPane />}
        {active === 'brute-force' && <BruteForcePane />}
        {active === 'compiler-access' && <CompilerAccessPane />}
        {active === 'anti-intrusion' && <AntiIntrusionPane />}
        {active === 'system-hardening' && (
          <SystemHardeningPane posture={posture} onNavigate={select} />
        )}
      </div>
    </div>
  );
}

/**
 * useSearchParams suspends during prerender, so the boundary is required and
 * its fallback is the loading state for the whole screen rather than a blank.
 */
export default function SecurityPage() {
  return (
    <Suspense
      fallback={
        <div className="space-y-4">
          <div>
            <h1 className="text-lg font-semibold text-gray-900">Security</h1>
          </div>
          <Panel>
            <SectionHeader title="Loading the security screen" />
            <BlockSkeleton rows={4} />
          </Panel>
        </div>
      }
    >
      <SecurityWorkspace />
    </Suspense>
  );
}
