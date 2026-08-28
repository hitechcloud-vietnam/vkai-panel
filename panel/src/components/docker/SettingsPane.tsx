'use client';

/**
 * Settings: what this panel is allowed to change about the daemon, and what
 * taking effect costs.
 *
 * The honest answer today is: nothing. There is no route that reads or writes
 * /etc/docker/daemon.json, and there is nothing in core/ that touches daemon
 * configuration at all.
 *
 * There is a second fact that compounds the first and is easy to miss, so it is
 * stated plainly here rather than discovered later: almost every daemon setting
 * needs a daemon restart to take effect, and this panel cannot restart the
 * Docker daemon. `checkServiceName` in
 * core/internal/service/service_manager.go carries an allow list of units the
 * API may act on - nginx, mysql, redis, postfix and a dozen more - and `docker`
 * is not on it. So even a finished settings form would end with "now go and
 * restart it over SSH". Both halves have to be built, or neither is worth
 * building.
 */

import { PaneUnavailable } from './CapabilityNotice';
import type { CapabilityGap } from '@/types/docker';

import { DaemonStatusCard, useDaemonStatus } from './DaemonStatus';
import { Panel, SectionHeader } from './PaneChrome';

const GAPS: CapabilityGap[] = [
  {
    label: 'Read the daemon configuration',
    missing:
      'No route reads /etc/docker/daemon.json. Nothing in core/ opens that file, so the panel cannot show the current registry mirrors, storage driver, log driver, default address pools or live-restore setting.',
  },
  {
    label: 'Write the daemon configuration',
    missing:
      'No route writes it either, and no validation exists for it. A daemon.json with one bad key stops Docker from starting, so a write path needs to parse and validate before saving, and keep the previous file to roll back to.',
  },
  {
    label: 'Restart the daemon so a change takes effect',
    missing:
      'POST /api/v1/services/docker/restart is mounted but refused: `docker` is absent from the alwaysManageable allow list in core/internal/service/service_manager.go, so the route answers 500 with `service "docker" is not managed by this panel`. Adding it to that list is a one-line change and is the prerequisite for this whole tab.',
  },
  {
    label: 'Container monitoring settings',
    missing:
      'aaPanel keeps a monitoring switch and a retention period here. The panel collects no per-container metrics at all, so there is nothing yet to switch on or retain.',
  },
];

const WOULD_SHOW = [
  'Registry mirrors, and a note that changing them needs a daemon restart',
  'The storage driver and log driver currently in use, read-only',
  'Default address pools, so a new network does not collide with the host LAN',
  'Live restore, and whether it is safe to restart the daemon without stopping containers',
];

export function SettingsPane() {
  const daemon = useDaemonStatus();

  return (
    <div className="space-y-4">
      <DaemonStatusCard resource={daemon} verbose />

      <PaneUnavailable
        title="The panel cannot change any daemon setting yet"
        summary={
          <>
            Nothing in the API reads or writes{' '}
            <span className="font-mono text-xs">/etc/docker/daemon.json</span>, so there is
            no setting to offer here. No form is shown rather than a form that saves
            nowhere.
          </>
        }
        wouldShow={WOULD_SHOW}
        gaps={GAPS}
      />

      <Panel>
        <SectionHeader
          title="What a daemon restart costs"
          description="Worth knowing before this tab exists, because it constrains how it should behave."
        />
        <div className="space-y-2 px-4 py-4 text-sm text-gray-600">
          <p>
            Registry mirrors, the storage driver, the log driver, default address pools and
            live restore all take effect only when the Docker daemon restarts. Saving one is
            not the same as applying it, and this tab must say which of the two just
            happened.
          </p>
          <p>
            Unless <span className="font-mono text-xs">live-restore</span> is enabled, a
            daemon restart stops every running container on the host. On a shared hosting box
            that is every customer&rsquo;s workload at once, so the restart belongs behind an
            explicit confirmation that names how many containers are running — a count this
            panel also does not have yet.
          </p>
          <p>
            Changing the storage driver is the exception that is not a restart at all:
            existing images and containers are not migrated, and the host effectively starts
            empty. It should never be a dropdown on this page.
          </p>
        </div>
      </Panel>
    </div>
  );
}

export default SettingsPane;
