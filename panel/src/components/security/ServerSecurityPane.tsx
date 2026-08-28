'use client';

/**
 * Server security: the kernel and service hardening the panel can apply.
 *
 * The panel can apply none of it today. There is no sysctl reader, no service
 * account audit, no hardening profile, and no endpoint for any of them. So this
 * pane lists the measures with what each one changes and how it is undone -
 * which is the part that has to be settled before anything is built, because a
 * hardening switch without a documented reversal is a switch nobody dares press
 * on a machine with paying customers on it.
 */

import { Undo2 } from 'lucide-react';

import { BackendGap, Panel, PaneHeading, SectionHeader, TableScroller, TD_CLASS, TH_CLASS } from './chrome';
import { cn } from '@/lib/utils';

interface Measure {
  name: string;
  changes: string;
  undo: string;
  risk: string;
}

/**
 * Each row is a measure with a named, reversible change. Nothing here is
 * applied by the panel; the table is the specification for the endpoint, and it
 * is shown to the operator so they can see exactly what is not being done.
 */
const MEASURES: Measure[] = [
  {
    name: 'Ignore ICMP broadcasts',
    changes: 'net.ipv4.icmp_echo_ignore_broadcasts = 1 in a panel-owned file under /etc/sysctl.d.',
    undo: 'Delete the panel-owned file and run sysctl --system. The kernel returns to the distribution default.',
    risk: 'None in normal hosting. Breaks a smurf-amplification test and nothing else.',
  },
  {
    name: 'Reverse path filtering',
    changes: 'net.ipv4.conf.all.rp_filter = 1, which drops packets whose source address could not have arrived on that interface.',
    undo: 'Remove the setting and run sysctl --system.',
    risk: 'Breaks asymmetric routing. A node with two uplinks may lose traffic.',
  },
  {
    name: 'Refuse source-routed packets',
    changes: 'net.ipv4.conf.all.accept_source_route = 0.',
    undo: 'Remove the setting and run sysctl --system.',
    risk: 'None in normal hosting.',
  },
  {
    name: 'SYN cookies',
    changes: 'net.ipv4.tcp_syncookies = 1, so a SYN flood cannot exhaust the backlog.',
    undo: 'Remove the setting and run sysctl --system.',
    risk: 'None. Cookies only engage when the backlog is already full.',
  },
  {
    name: 'Disable core dumps for setuid programs',
    changes: 'fs.suid_dumpable = 0, plus a hard core limit of 0 in a panel-owned limits.d file.',
    undo: 'Remove both panel-owned files.',
    risk: 'Removes a debugging aid. Crash analysis on this host becomes harder.',
  },
  {
    name: 'Restrict kernel pointers and dmesg',
    changes: 'kernel.kptr_restrict = 2 and kernel.dmesg_restrict = 1.',
    undo: 'Remove the setting and run sysctl --system.',
    risk: 'Some monitoring agents read dmesg and will need to run as root afterwards.',
  },
  {
    name: 'Shared memory hardening',
    changes: 'Mounts /dev/shm with noexec, nosuid and nodev by adding a panel-owned fstab entry.',
    undo: 'Remove the fstab entry and remount /dev/shm.',
    risk: 'A few database and browser workloads expect exec on /dev/shm and will fail to start.',
  },
  {
    name: 'Service accounts without shells',
    changes: 'Sets the login shell of non-interactive service accounts to /usr/sbin/nologin.',
    undo: 'Restore the recorded previous shell for each account the panel changed.',
    risk: 'An account that was genuinely being used for a cron shell will stop working.',
  },
];

export function ServerSecurityPane() {
  return (
    <div className="space-y-4">
      <PaneHeading
        title="Server security"
        description="Kernel parameters and service settings the panel would apply on request, each with the exact change it makes and the exact way back. None of them is applied today."
      />

      <Panel>
        <SectionHeader
          title="Not available"
          description="What has to exist before any of these become switches."
        />
        <BackendGap
          title="The panel cannot read or apply any host hardening"
          summary="Nothing in core/internal reads a sysctl value, writes a file under /etc/sysctl.d, or inspects a service account. Every measure below is a specification, not a control, and the Overview reports kernel and service hardening as not verifiable rather than showing a state for it."
          controls={[
            'Read the current value of every measure from the running kernel, not from a stored intention',
            'Apply one measure, writing only to a panel-owned file so nothing hand-edited is overwritten',
            'Revert one measure, restoring exactly what was there before',
            'Apply a named profile of measures in one action, with a preview of every change first',
          ]}
          endpoints={[
            'GET  /api/v1/security/hardening/measures',
            'POST /api/v1/security/hardening/measures/:id/apply',
            'POST /api/v1/security/hardening/measures/:id/revert',
            'GET  /api/v1/security/hardening/profiles',
          ]}
          note="Two properties matter more than the list itself: the read must come from the live kernel so the panel never reports an intention as a fact, and every write must go to a file the panel owns so reverting is a deletion rather than a guess at what was there before."
        />
      </Panel>

      <Panel>
        <SectionHeader
          title="Measures, and how each is undone"
          description="Reference only. Nothing on this table is applied by the panel."
        />
        <TableScroller>
          <table className="w-full min-w-[900px] border-collapse">
            <thead className="bg-gray-50">
              <tr className="border-b border-gray-200">
                <th className={TH_CLASS}>Measure</th>
                <th className={TH_CLASS}>What it changes</th>
                <th className={TH_CLASS}>How to undo it</th>
                <th className={TH_CLASS}>What it can break</th>
              </tr>
            </thead>
            <tbody>
              {MEASURES.map((measure) => (
                <tr key={measure.name} className="border-b border-gray-200 last:border-b-0">
                  <td className={cn(TD_CLASS, 'font-medium text-gray-900')}>{measure.name}</td>
                  <td className={TD_CLASS}>{measure.changes}</td>
                  <td className={TD_CLASS}>
                    <span className="inline-flex items-start gap-1.5">
                      <Undo2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-gray-400" aria-hidden="true" />
                      <span>{measure.undo}</span>
                    </span>
                  </td>
                  <td className={TD_CLASS}>{measure.risk}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableScroller>
      </Panel>
    </div>
  );
}

export default ServerSecurityPane;
