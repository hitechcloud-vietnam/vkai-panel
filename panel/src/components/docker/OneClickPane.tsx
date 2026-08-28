'use client';

/**
 * One-Click Install: a catalogue of applications, each with the fields it needs
 * - a port, a root password, a data directory - filled in once and turned into
 * a running container.
 *
 * There is no catalogue. Not an empty one: no route, no table, no seed data
 * anywhere in core/. Rendering a hand-written list of MySQL, Redis, WordPress
 * and Nextcloud here would be the worst version of this screen, because every
 * entry would be an install button that installs nothing, and an operator would
 * only find out after choosing a root password for a database that was never
 * created.
 *
 * So the pane says what a catalogue entry has to carry, which is the useful
 * thing to hand whoever builds it.
 */

import { PaneUnavailable } from './CapabilityNotice';
import type { CapabilityGap } from '@/types/docker';

const GAPS: CapabilityGap[] = [
  {
    label: 'The catalogue',
    missing:
      'No route serves a list of installable applications. There is no /api/v1/docker/apps, no app-store handler in core/internal/handler/, and no seeded table for one.',
  },
  {
    label: 'Per-application fields',
    missing:
      'Each entry needs its own form definition - published port, admin password, data directory, version choice - and a place to store it. Nothing in the API describes an application’s inputs.',
  },
  {
    label: 'Install',
    missing:
      'An install is a compose file or a container spec rendered from those inputs and then run. Both underlying routes (POST /docker/compose/deploy and container creation) are stubs, and there is no container-create route at all.',
  },
  {
    label: 'Installed-application state',
    missing:
      'Nothing records that an application was installed from the catalogue, so a second visit could not show what is already running or offer an upgrade.',
  },
];

const WOULD_SHOW = [
  'A grid of applications with an icon, a one-line description and the version on offer',
  'The fields each one needs before it can start - port, admin password, data directory',
  'A port conflict check against what is already published on this host',
  'What is already installed, and where its data lives',
];

export function OneClickPane() {
  return (
    <PaneUnavailable
      title="There is no application catalogue yet"
      summary={
        <>
          This tab is where an operator picks WordPress or MySQL and gets a running
          container without writing a compose file. The panel has no catalogue to pick
          from - not an empty one, but no endpoint, no table and no seed data. A list is
          not invented here on the front end, because every entry would be an install
          button that installs nothing.
        </>
      }
      wouldShow={WOULD_SHOW}
      gaps={GAPS}
    />
  );
}

export default OneClickPane;
