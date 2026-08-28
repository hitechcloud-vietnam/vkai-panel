'use client';

/**
 * Other Panel Migrate - importing accounts from cPanel, Plesk, DirectAdmin or
 * aaPanel.
 *
 * Nothing behind it. There is no detection endpoint, no inventory endpoint and
 * no import endpoint anywhere in the Go source, and each of the four sources
 * needs different code: cPanel has cpmove archives and a WHM API, Plesk has its
 * XML API and a different backup format, DirectAdmin has its own tarballs,
 * aaPanel has a panel database and its own backup layout. There is no single
 * "import" that covers them.
 *
 * So this screen is a specification rather than a form. It lists what would have
 * to exist, which is a backlog somebody can work from, and it collects no
 * credentials it cannot use.
 */

import { Card, CardBody, CardHeader, NotBuiltYet, PageHeader, Table, Td, Th } from './ui';

const SOURCES: { panel: string; whatIsThere: string; whatItTakes: string }[] = [
  {
    panel: 'cPanel / WHM',
    whatIsThere: 'Accounts, addon and subdomains, MySQL databases and users, mail, DNS zones, cron.',
    whatItTakes:
      'The WHM API on port 2087 with an API token, or a cpmove-*.tar.gz archive uploaded to the panel. The archive form is the one that works when the source server is already gone.',
  },
  {
    panel: 'Plesk',
    whatIsThere: 'Subscriptions, domains, databases, mail, DNS, scheduled tasks.',
    whatItTakes:
      'The Plesk XML API on port 8443 with an administrator key, or a Plesk backup (.xml plus its data directory).',
  },
  {
    panel: 'DirectAdmin',
    whatIsThere: 'Users, domains, databases, mail, DNS.',
    whatItTakes: 'The DirectAdmin API on port 2222 with a login key, or a user backup tarball.',
  },
  {
    panel: 'aaPanel / BT Panel',
    whatIsThere: 'Sites, databases, FTP accounts, SSL certificates, cron.',
    whatItTakes:
      'The panel API with its key, or its SQLite panel database plus the site directories and database dumps.',
  },
];

export function OtherPanelMigrate() {
  return (
    <div className="space-y-5">
      <PageHeader
        title="Other Panel Migrate"
        description="Import websites, databases and WordPress installations from another control panel."
      />

      <NotBuiltYet
        what="Importing from another control panel"
        because={
          <>
            The panel has no endpoint that talks to cPanel, Plesk, DirectAdmin or aaPanel, and no
            reader for any of their backup formats. Detection, inventory and import would all be new
            work.
          </>
        }
        endpoints={[
          {
            endpoint: 'POST /api/v1/migrations/panels/detect',
            purpose:
              'Given a host and credentials, or an uploaded backup archive, say which panel it is and which version. Detection has to come first, because the four sources have nothing in common past this point.',
          },
          {
            endpoint: 'POST /api/v1/migrations/panels/inventory',
            purpose:
              'List what the source holds - domains, databases, mail accounts, certificates, cron entries - with a size for each, so an operator can decide what to bring and how long it will take.',
          },
          {
            endpoint: 'POST /api/v1/migrations/panels/import',
            purpose:
              'Import the selected subset. Returns a job id; the same per-step, resumable state the Migrate Site screen needs applies here, because a panel import is many transfers rather than one.',
          },
          {
            endpoint: 'GET /api/v1/migrations/panels/:id',
            purpose: 'Per-item progress and per-item failure, so one domain failing does not tell the operator the whole import failed.',
          },
        ]}
      />

      <Card>
        <CardHeader
          title="What each source would need"
          description="The four panels are named as one feature and are four pieces of work. This is what each one involves."
        />
        <Table>
          <thead>
            <tr>
              <Th>Source panel</Th>
              <Th>What it holds</Th>
              <Th>How it would be read</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {SOURCES.map((source) => (
              <tr key={source.panel}>
                <Td className="font-medium text-gray-900">{source.panel}</Td>
                <Td>{source.whatIsThere}</Td>
                <Td>{source.whatItTakes}</Td>
              </tr>
            ))}
          </tbody>
        </Table>
        <CardBody>
          <p className="text-sm text-gray-500">
            Until those endpoints exist, moving from one of these panels means moving the files and
            the database by hand and then using the URL rewrite on the Migrate Site screen, which is
            implemented.
          </p>
        </CardBody>
      </Card>
    </div>
  );
}

export default OtherPanelMigrate;
