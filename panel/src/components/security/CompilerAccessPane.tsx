'use client';

/**
 * Compiler access from web processes.
 *
 * On shared hosting this is a short, well understood hardening step: a compiler
 * that the web user can execute turns an uploaded C file into a running binary,
 * and most of the local privilege escalation exploits that circulate are
 * distributed as source precisely because a compiler is usually there.
 *
 * The panel cannot see any of it. Nothing reads which compilers are installed,
 * nothing checks the mode or ownership of the binaries, and nothing can change
 * them. So the pane says which binaries matter and what the change would be,
 * and offers no switch.
 */

import { BackendGap, Panel, PaneHeading, SectionHeader, TableScroller, TD_CLASS, TH_CLASS } from './chrome';
import { cn } from '@/lib/utils';

const COMPILERS: { binary: string; why: string }[] = [
  { binary: 'cc / gcc', why: 'The C compiler most exploit source expects to find.' },
  { binary: 'c++ / g++', why: 'The C++ compiler, for the same reason.' },
  { binary: 'cc1 / cc1plus', why: 'The compiler proper. Restricting only the driver leaves these reachable.' },
  { binary: 'as', why: 'The assembler. Enough on its own for a hand-written payload.' },
  { binary: 'ld', why: 'The linker. Needed to turn objects into something that runs.' },
  { binary: 'make', why: 'Not a compiler, but what most published exploits are actually run with.' },
];

export function CompilerAccessPane() {
  return (
    <div className="space-y-4">
      <PaneHeading
        title="Compiler Access"
        description="Whether a web process on this host can reach a compiler. The panel cannot check this and cannot change it, so this section reports what would be checked rather than a state."
      />

      <Panel>
        <SectionHeader title="Not available" description="What has to exist before this becomes a switch." />
        <BackendGap
          title="The panel cannot see whether compilers are reachable"
          summary="No route reports which compiler binaries are installed, what their mode and group are, or whether the account the web server runs as can execute them. The Overview reports compiler access as not verifiable rather than showing a tick."
          controls={[
            'Which of the compiler binaries below exist on this host, with their mode, owner and group',
            'Whether the web server and PHP-FPM accounts can execute each one',
            'Restrict: chgrp to a compile group and chmod 0750, so root and that group keep access and nothing else does',
            'Release: restore the recorded previous mode, owner and group exactly',
            'A warning before restricting when a site on this host builds native extensions, because that build will start failing',
          ]}
          endpoints={[
            'GET  /api/v1/security/compilers',
            'POST /api/v1/security/compilers/restrict',
            'POST /api/v1/security/compilers/release',
          ]}
          note="Recording the previous mode and owner is what makes this reversible. A restrict action that cannot say what the permissions were before it ran is a change nobody can safely undo on a production node."
        />
      </Panel>

      <Panel>
        <SectionHeader
          title="What would be checked"
          description="Reference only. Restricting the driver alone is not enough - the second row is the one people forget."
        />
        <TableScroller>
          <table className="w-full min-w-[600px] border-collapse">
            <thead className="bg-gray-50">
              <tr className="border-b border-gray-200">
                <th className={TH_CLASS}>Binary</th>
                <th className={TH_CLASS}>Why it counts</th>
              </tr>
            </thead>
            <tbody>
              {COMPILERS.map((entry) => (
                <tr key={entry.binary} className="border-b border-gray-200 last:border-b-0">
                  <td className={cn(TD_CLASS, 'font-mono font-medium text-gray-900')}>{entry.binary}</td>
                  <td className={TD_CLASS}>{entry.why}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableScroller>
      </Panel>
    </div>
  );
}

export default CompilerAccessPane;
