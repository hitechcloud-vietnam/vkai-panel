'use client';

/**
 * The service pane: the daemon, then the ports a client has to get through.
 *
 * The two are one pane because they are one question. "FTP does not work" is
 * answered by looking at whether the daemon is running and whether 21 and the
 * passive range are open, in that order, and splitting them across two screens
 * only makes an operator click twice.
 */

import { useState } from 'react';

import { DaemonCard } from './DaemonCard';
import { PortsCard } from './PortsCard';

export function ServicePane() {
  // Which unit the daemon card found, so the ports card can name it.
  // setState is referentially stable, so passing it straight down does not
  // re-trigger the child's load effect.
  const [unit, setUnit] = useState<string | null>(null);

  return (
    <div className="space-y-4">
      <DaemonCard onUnitChange={setUnit} />
      <PortsCard unit={unit} />
    </div>
  );
}

export default ServicePane;
