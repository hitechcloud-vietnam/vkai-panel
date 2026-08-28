'use client';

/**
 * Migrate Site.
 *
 * Two different jobs live under this heading, and only one of them has an API.
 *
 * 1. ADOPTING a WordPress site this panel already serves. aaPanel's menu entry
 *    of the same name does exactly this: it lists the WordPress sites the panel
 *    hosts but the toolkit has no record of, and brings them under toolkit
 *    management. Every route that needs is mounted here, so it is a working
 *    feature on this screen.
 *
 * 2. PULLING a site off another server over SSH or FTP, resumably. There is no
 *    API for it at all - nothing in router.go accepts remote credentials,
 *    nothing copies a remote document root, and there is no migrations table
 *    for a transfer to record progress in. So no credentials form is drawn.
 *    Asking an operator for the root password of the server they are leaving,
 *    and then failing, is worse than saying up front that it cannot be done.
 *
 * The third card is the step people get wrong when they move a site by hand:
 * rewriting the old domain to the new one across every table. That one is real,
 * and it defaults to a dry run.
 */

import { useMemo, useState } from 'react';
import { errorMessage, records, runtime as runtimeApi } from './api';
import { useFormOptions, useRuntimeAvailability, useSites } from './hooks';
import { RuntimeBanner } from './RuntimeBanner';
import { SitePicker } from './SitePicker';
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  EmptyState,
  ErrorNote,
  Field,
  Loading,
  NotBuiltYet,
  Notice,
  PageHeader,
  Table,
  Td,
  Th,
  TextInput,
  generatePassword,
} from './ui';
import type { SearchReplaceReport } from '@/types/wordpress';

interface AdoptDraft {
  websiteId: string;
  domain: string;
  path: string;
  serverId: string;
  name: string;
  dbName: string;
  dbUser: string;
  dbPassword: string;
  dbHost: string;
  dbPrefix: string;
  adminUser: string;
  adminEmail: string;
  adminPassword: string;
}

function emptyDraft(website: {
  id: string;
  domain: string;
  root_dir: string;
  server_id: string;
}): AdoptDraft {
  return {
    websiteId: website.id,
    domain: website.domain,
    path: website.root_dir,
    serverId: website.server_id,
    name: website.domain,
    dbName: '',
    dbUser: '',
    dbPassword: '',
    dbHost: 'localhost',
    dbPrefix: 'wp_',
    adminUser: 'admin',
    adminEmail: '',
    adminPassword: '',
  };
}

export function MigrateSite() {
  const { availability, ready, recheck } = useRuntimeAvailability();
  const { sites, loading: sitesLoading, reload: reloadSites } = useSites();
  const options = useFormOptions();

  // --- adopt -------------------------------------------------------------
  const [draft, setDraft] = useState<AdoptDraft | null>(null);
  const [adopting, setAdopting] = useState(false);
  const [adoptError, setAdoptError] = useState<string | null>(null);
  const [adopted, setAdopted] = useState<string | null>(null);

  const unregistered = useMemo(() => {
    const claimed = new Set(sites.map((s) => s.website_id).filter(Boolean) as string[]);
    const byDomain = new Set(sites.map((s) => (s.domain || '').toLowerCase()).filter(Boolean));
    return options.websites.filter(
      (w) => !claimed.has(w.id) && !byDomain.has((w.domain || '').toLowerCase()),
    );
  }, [sites, options.websites]);

  const adopt = async () => {
    if (!draft) return;
    setAdopting(true);
    setAdoptError(null);
    try {
      await records.create({
        server_id: draft.serverId,
        website_id: draft.websiteId,
        name: draft.name.trim() || draft.domain,
        domain: draft.domain,
        path: draft.path.trim(),
        db_name: draft.dbName.trim(),
        db_user: draft.dbUser.trim(),
        db_password: draft.dbPassword,
        db_host: draft.dbHost.trim() || 'localhost',
        db_prefix: draft.dbPrefix.trim() || 'wp_',
        admin_user: draft.adminUser.trim(),
        admin_password: draft.adminPassword,
        admin_email: draft.adminEmail.trim(),
        auto_update: false,
      });
      setAdopted(draft.domain);
      setDraft(null);
      reloadSites();
    } catch (err) {
      setAdoptError(errorMessage(err, 'The site could not be brought into the toolkit.'));
    } finally {
      setAdopting(false);
    }
  };

  const adoptReason = !draft
    ? null
    : !draft.serverId
      ? 'This website has no server recorded, so the installation cannot be attached to one.'
      : !draft.path.trim()
        ? 'The WordPress root path is required.'
        : !draft.dbName.trim() || !draft.dbUser.trim() || !draft.dbPassword
          ? 'The database name, user and password are required by POST /api/v1/wordpress.'
          : !draft.adminUser.trim() || !draft.adminEmail.trim() || !draft.adminPassword
            ? 'The administrator username, email and password are required by POST /api/v1/wordpress.'
            : null;

  // --- URL rewrite -------------------------------------------------------
  const [siteId, setSiteId] = useState('');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [dryRun, setDryRun] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [report, setReport] = useState<{ report: SearchReplaceReport | null; ran_as?: string } | null>(
    null,
  );

  const runRewrite = async () => {
    setBusy(true);
    setError(null);
    setReport(null);
    try {
      setReport(
        await runtimeApi.searchReplace(siteId, { from: from.trim(), to: to.trim(), dry_run: dryRun }),
      );
    } catch (err) {
      setError(errorMessage(err, 'The URL rewrite could not be run.'));
    } finally {
      setBusy(false);
    }
  };

  const rewriteReason = !ready
    ? 'This needs the WP-CLI routes, which are not mounted on this panel.'
    : !siteId
      ? 'Choose the installation to rewrite first.'
      : !from.trim() || !to.trim()
        ? 'Both the old and the new value are required.'
        : null;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Migrate Site"
        description="Bring an existing WordPress site under toolkit management, and finish a move that was done by hand."
      />

      <RuntimeBanner availability={availability} onRecheck={recheck} />

      <Card>
        <CardHeader
          title="Sites this panel serves that the toolkit has no record of"
          description="Every website registered on this panel that is not already a WordPress installation here. Adopting one gives it the update, statistics and staging screens."
        />
        {options.loading || sitesLoading ? (
          <Loading label="Comparing websites against WordPress installations" />
        ) : unregistered.length === 0 ? (
          <EmptyState title="Every website on this panel is already in the toolkit">
            Nothing here needs adopting.
          </EmptyState>
        ) : (
          <>
            <Notice tone="sky">
              The panel cannot tell which of these actually contain WordPress: detecting that means
              reading the document root, which needs the WP-CLI routes. Everything registered as a
              website is listed, and choosing one that is not WordPress produces a record for a site
              that has none.
            </Notice>
            <Table>
              <thead>
                <tr>
                  <Th>Domain</Th>
                  <Th>Document root</Th>
                  <Th className="text-right">Action</Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {unregistered.map((website) => (
                  <tr key={website.id} className="hover:bg-gray-50">
                    <Td className="font-medium text-gray-900">{website.domain || website.id}</Td>
                    <Td className="font-mono text-xs">{website.root_dir || '-'}</Td>
                    <Td className="text-right">
                      <Button
                        tone="secondary"
                        onClick={() => {
                          setAdopted(null);
                          setAdoptError(null);
                          setDraft(emptyDraft(website));
                        }}
                      >
                        Bring into the toolkit
                      </Button>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </>
        )}
        {adopted ? (
          <CardBody>
            <Notice tone="emerald">{adopted} is now managed by the toolkit.</Notice>
          </CardBody>
        ) : null}
      </Card>

      {draft ? (
        <Card>
          <CardHeader
            title={`Bring ${draft.domain} into the toolkit`}
            description="These are the fields POST /api/v1/wordpress requires. Nothing is written to the site; this records what the panel needs to know in order to manage it."
          />
          <CardBody className="space-y-4">
            {adoptError ? <ErrorNote>{adoptError}</ErrorNote> : null}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
              <Field label="Name" htmlFor="ad-name">
                <TextInput
                  id="ad-name"
                  value={draft.name}
                  onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                />
              </Field>
              <Field label="WordPress root" required htmlFor="ad-path">
                <TextInput
                  id="ad-path"
                  value={draft.path}
                  onChange={(e) => setDraft({ ...draft, path: e.target.value })}
                />
              </Field>
              <Field
                label="Table prefix"
                htmlFor="ad-prefix"
                hint="Read it from wp-config.php. A wrong prefix makes every later operation address the wrong tables."
              >
                <TextInput
                  id="ad-prefix"
                  value={draft.dbPrefix}
                  onChange={(e) => setDraft({ ...draft, dbPrefix: e.target.value })}
                />
              </Field>
              <Field label="Database name" required htmlFor="ad-db">
                <TextInput
                  id="ad-db"
                  value={draft.dbName}
                  onChange={(e) => setDraft({ ...draft, dbName: e.target.value })}
                />
              </Field>
              <Field label="Database user" required htmlFor="ad-dbu">
                <TextInput
                  id="ad-dbu"
                  value={draft.dbUser}
                  onChange={(e) => setDraft({ ...draft, dbUser: e.target.value })}
                />
              </Field>
              <Field label="Database password" required htmlFor="ad-dbp">
                <TextInput
                  id="ad-dbp"
                  type="password"
                  value={draft.dbPassword}
                  onChange={(e) => setDraft({ ...draft, dbPassword: e.target.value })}
                />
              </Field>
              <Field label="Database host" htmlFor="ad-dbh">
                <TextInput
                  id="ad-dbh"
                  value={draft.dbHost}
                  onChange={(e) => setDraft({ ...draft, dbHost: e.target.value })}
                />
              </Field>
              <Field label="Administrator username" required htmlFor="ad-au">
                <TextInput
                  id="ad-au"
                  value={draft.adminUser}
                  onChange={(e) => setDraft({ ...draft, adminUser: e.target.value })}
                />
              </Field>
              <Field label="Administrator email" required htmlFor="ad-ae">
                <TextInput
                  id="ad-ae"
                  type="email"
                  value={draft.adminEmail}
                  onChange={(e) => setDraft({ ...draft, adminEmail: e.target.value })}
                />
              </Field>
              <Field
                label="Administrator password"
                required
                htmlFor="ad-ap"
                hint="The API requires this field. Nothing checks it against the site, so a guess here is stored as if it were true. Enter the real one, or set a new one with a password reset afterwards."
              >
                <div className="flex gap-2">
                  <TextInput
                    id="ad-ap"
                    value={draft.adminPassword}
                    className="font-mono"
                    onChange={(e) => setDraft({ ...draft, adminPassword: e.target.value })}
                  />
                  <Button
                    tone="secondary"
                    onClick={() => setDraft({ ...draft, adminPassword: generatePassword() })}
                  >
                    Generate
                  </Button>
                </div>
              </Field>
            </div>
            <div className="flex justify-end gap-2">
              <Button tone="secondary" onClick={() => setDraft(null)}>
                Cancel
              </Button>
              <Button tone="primary" busy={adopting} unavailableReason={adoptReason} onClick={adopt}>
                Bring into the toolkit
              </Button>
            </div>
          </CardBody>
        </Card>
      ) : null}

      <NotBuiltYet
        what="Pulling a site off another server"
        because={
          <>
            The panel has no endpoint that connects to a remote host over SSH or FTP, copies a
            WordPress document root and its database, and resumes where it left off. Nothing in
            router.go accepts a set of remote credentials.
          </>
        }
        endpoints={[
          {
            endpoint: 'POST /api/v1/wordpress/migrations',
            purpose:
              'Start a migration from remote credentials (host, port, SSH key or password, or FTP user, remote document root, remote database). Returns a migration id immediately; the transfer runs as a job, because a 40 GB uploads directory does not fit in an HTTP request.',
          },
          {
            endpoint: 'GET /api/v1/wordpress/migrations/:id',
            purpose:
              'The state of one migration: the named step it is on (connect, inventory, files, database, rewrite URLs, verify), byte counts, and the error if it stopped.',
          },
          {
            endpoint: 'POST /api/v1/wordpress/migrations/:id/resume',
            purpose:
              'Continue from the last completed step rather than from the beginning. This is the requirement that makes the feature worth having: a transfer that fails at 90 percent and can only start again gets used once and never again. It needs the migration row to persist per-step state and a file manifest, so the file step can skip what already arrived - a resume without those is a restart with a friendlier name.',
          },
          {
            endpoint: 'POST /api/v1/wordpress/migrations/:id/cancel',
            purpose: 'Stop a running migration and report what it left behind on disk.',
          },
          {
            endpoint: 'GET /api/v1/wordpress/migrations',
            purpose:
              'The migrations for this tenant, so a half-finished one can be found again after a page reload. Progress that only exists in a browser tab is lost with the tab.',
          },
        ]}
      />

      <Card>
        <CardHeader
          title="Rewrite URLs after a move"
          description="After a site has been moved onto this server by hand, this replaces the old domain with the new one across every table, serialised values included."
        />
        <CardBody className="space-y-4">
          {sitesLoading ? (
            <Loading label="Loading installations" />
          ) : (
            <>
              <SitePicker
                sites={sites}
                value={siteId}
                onChange={setSiteId}
                hint="The installation that has already been moved onto this server."
              />

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <Field label="Replace" htmlFor="sr-from" hint="The old value, usually the previous domain or full URL.">
                  <TextInput
                    id="sr-from"
                    value={from}
                    placeholder="https://old-domain.example"
                    onChange={(e) => setFrom(e.target.value)}
                  />
                </Field>
                <Field label="With" htmlFor="sr-to" hint="The new value on this server.">
                  <TextInput
                    id="sr-to"
                    value={to}
                    placeholder="https://new-domain.example"
                    onChange={(e) => setTo(e.target.value)}
                  />
                </Field>
              </div>

              <label className="flex items-start gap-3 rounded-md border border-gray-200 p-3 text-sm">
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={dryRun}
                  onChange={(e) => setDryRun(e.target.checked)}
                />
                <span>
                  <span className="font-medium text-gray-900">Dry run</span>
                  <span className="mt-0.5 block text-gray-600">
                    Report what would change without writing anything. Leave this on until the row
                    count looks like the site you expect. Turning it off rewrites every matching row
                    in the database, and there is no undo on this screen.
                  </span>
                </span>
              </label>

              {!dryRun ? (
                <Notice tone="red" title="This will write to the production database">
                  Every matching row in every table is rewritten in place. Take a backup first if the
                  dry run reported more rows than you expected.
                </Notice>
              ) : null}

              {error ? <ErrorNote>{error}</ErrorNote> : null}

              {report ? (
                <div className="rounded-md border border-gray-200">
                  <div className="border-b border-gray-200 bg-gray-50 px-4 py-2 text-xs font-semibold uppercase tracking-wide text-gray-500">
                    {dryRun ? 'Dry run report' : 'Rewrite report'}
                    {report.ran_as ? ` - ran as ${report.ran_as}` : ''}
                  </div>
                  <pre className="overflow-x-auto px-4 py-3 font-mono text-xs text-gray-700">
                    {JSON.stringify(report.report ?? {}, null, 2)}
                  </pre>
                </div>
              ) : null}

              <div className="flex justify-end">
                <Button
                  tone={dryRun ? 'primary' : 'danger'}
                  busy={busy}
                  unavailableReason={rewriteReason}
                  onClick={runRewrite}
                >
                  {dryRun ? 'Run dry run' : 'Rewrite URLs for real'}
                </Button>
              </div>
            </>
          )}
        </CardBody>
      </Card>
    </div>
  );
}

export default MigrateSite;
