'use client';

/**
 * Data Copy - clone a site to staging, and push staging back.
 *
 * The push is the whole reason this screen is written carefully. Pushing a
 * staging database over production discards every order, comment and post made
 * on production since the clone, and the way that happens is a form with a
 * checkbox that was already ticked.
 *
 * So, in this order:
 *
 *   - The database decision is a radio group with NO option selected. Its state
 *     starts as the empty string, which is not one of the three values the API
 *     accepts. The push button is disabled until the operator picks one.
 *   - The two destructive choices additionally require typing the production
 *     domain. Not a confirm dialog - a dialog is dismissed by the same reflex
 *     that pressed the button.
 *   - The three options are described by what they do to production, not by
 *     what they are for.
 *   - After a push, the two backup paths the API returns are shown, because a
 *     wrong choice needs a way back that does not involve a support ticket.
 *
 * The server enforces the same rule independently: PushStaging rejects an
 * absent or unrecognised value with ErrDatabaseChoiceRequired. This screen
 * never sends a value the operator did not choose.
 */

import { useCallback, useEffect, useState } from 'react';
import { errorMessage, runtime as runtimeApi } from './api';
import { useRuntimeAvailability, useSites } from './hooks';
import { RuntimeBanner } from './RuntimeBanner';
import { SitePicker } from './SitePicker';
import {
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  EmptyState,
  ErrorNote,
  Field,
  Loading,
  Notice,
  PageHeader,
  TextInput,
  generatePassword,
} from './ui';
import type { StagingDatabaseAction, StagingView, WordPressSite } from '@/types/wordpress';

interface DatabaseChoice {
  value: StagingDatabaseAction;
  label: string;
  /** What it does to production. Written from production's point of view. */
  consequence: string;
  destructive: boolean;
}

const DATABASE_CHOICES: DatabaseChoice[] = [
  {
    value: 'keep_production',
    label: 'Files only. Leave the production database untouched.',
    consequence:
      'Themes, plugins and uploads are copied to production. Every order, comment, post and setting in the production database stays exactly as it is. This is the right choice for a theme or plugin change.',
    destructive: false,
  },
  {
    value: 'overwrite_production',
    label: 'Files and database. Replace the production database with the staging one.',
    consequence:
      'The production database is replaced. Everything customers did on production since the clone was taken - orders, comments, registrations, posts - is gone from the live site. It survives only in the backup this push takes first.',
    destructive: true,
  },
  {
    value: 'database_only',
    label: 'Database only. Replace the production database and leave production files alone.',
    consequence:
      'Production files are not touched. The production database is replaced, with the same loss of everything customers did on production since the clone. For a content-only staging round.',
    destructive: true,
  },
];

function StagingSummary({ staging }: { staging: StagingView }) {
  const history = staging.history ?? {};
  return (
    <dl className="grid grid-cols-1 gap-4 text-sm sm:grid-cols-2">
      <div>
        <dt className="text-gray-500">Staging URL</dt>
        <dd className="break-all text-gray-900">
          {staging.staging_url ? (
            <a
              href={staging.staging_url}
              target="_blank"
              rel="noreferrer"
              className="text-brand-600 hover:text-brand-700"
            >
              {staging.staging_url}
            </a>
          ) : (
            staging.staging_domain || '-'
          )}
        </dd>
      </div>
      <div>
        <dt className="text-gray-500">Status</dt>
        <dd>
          <Badge tone={staging.status === 'ready' ? 'emerald' : staging.status === 'error' ? 'red' : 'sky'}>
            {staging.status || 'unknown'}
          </Badge>
        </dd>
      </div>
      <div>
        <dt className="text-gray-500">Path</dt>
        <dd className="break-all font-mono text-xs text-gray-900">{staging.staging_path || '-'}</dd>
      </div>
      <div>
        <dt className="text-gray-500">Runs as</dt>
        <dd className="font-mono text-xs text-gray-900">{staging.ran_as || '-'}</dd>
      </div>
      <div>
        <dt className="text-gray-500">Last clone</dt>
        <dd className="text-gray-900">{history.last_clone_at || 'never'}</dd>
      </div>
      <div>
        <dt className="text-gray-500">Last push</dt>
        <dd className="text-gray-900">
          {history.last_push_at ? (
            <>
              {history.last_push_at}
              {history.last_push_database ? (
                <span className="ml-2">
                  <Badge tone={history.last_push_database === 'keep_production' ? 'gray' : 'amber'}>
                    {history.last_push_database}
                  </Badge>
                </span>
              ) : null}
            </>
          ) : (
            'never'
          )}
        </dd>
      </div>
      {history.last_push_files_backup ? (
        <div className="sm:col-span-2">
          <dt className="text-gray-500">Backup of production files taken before the last push</dt>
          <dd className="break-all font-mono text-xs text-gray-900">{history.last_push_files_backup}</dd>
        </div>
      ) : null}
      {history.last_push_database_backup ? (
        <div className="sm:col-span-2">
          <dt className="text-gray-500">Backup of the production database taken before the last push</dt>
          <dd className="break-all font-mono text-xs text-gray-900">
            {history.last_push_database_backup}
          </dd>
        </div>
      ) : null}
      {history.last_error ? (
        <div className="sm:col-span-2">
          <dt className="text-gray-500">Last error</dt>
          <dd className="text-red-700">{history.last_error}</dd>
        </div>
      ) : null}
    </dl>
  );
}

export function DataCopy() {
  const { availability, ready, recheck } = useRuntimeAvailability();
  const { sites, loading: sitesLoading } = useSites();

  const [siteId, setSiteId] = useState('');
  const [staging, setStaging] = useState<StagingView | null>(null);
  const [stagingLoading, setStagingLoading] = useState(false);
  const [stagingMissing, setStagingMissing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  // Clone form
  const [subdomain, setSubdomain] = useState('staging');
  const [dbName, setDbName] = useState('');
  const [dbUser, setDbUser] = useState('');
  const [dbPassword, setDbPassword] = useState('');
  const [dbHost, setDbHost] = useState('localhost');
  const [blockIndexing, setBlockIndexing] = useState(true);

  // Push form. '' is not a valid API value; that is the point.
  const [databaseChoice, setDatabaseChoice] = useState<StagingDatabaseAction | ''>('');
  const [confirmDomain, setConfirmDomain] = useState('');

  const site: WordPressSite | undefined = sites.find((s) => s.id === siteId);

  const loadStaging = useCallback(async (id: string) => {
    setStagingLoading(true);
    setStaging(null);
    setStagingMissing(false);
    setError(null);
    try {
      const view = await runtimeApi.staging(id);
      setStaging(view);
      setStagingMissing(!view);
    } catch (err) {
      const status = (err as { response?: { status?: number } })?.response?.status;
      if (status === 404) {
        // No staging copy for this installation. Not an error.
        setStagingMissing(true);
      } else {
        setError(errorMessage(err, 'The staging environment could not be read.'));
      }
    } finally {
      setStagingLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!ready || !siteId) {
      setStaging(null);
      setStagingMissing(false);
      return;
    }
    void loadStaging(siteId);
  }, [ready, siteId, loadStaging]);

  useEffect(() => {
    // A new installation means a new decision. Never carry a database choice
    // across sites.
    setDatabaseChoice('');
    setConfirmDomain('');
  }, [siteId]);

  const createStaging = async () => {
    setBusy('clone');
    setError(null);
    try {
      const view = await runtimeApi.createStaging(siteId, {
        subdomain: subdomain.trim() || undefined,
        db_name: dbName.trim(),
        db_user: dbUser.trim(),
        db_password: dbPassword,
        db_host: dbHost.trim() || undefined,
        block_indexing: blockIndexing,
      });
      setStaging(view);
      setStagingMissing(!view);
      setDbPassword('');
    } catch (err) {
      setError(errorMessage(err, 'The staging copy could not be created.'));
    } finally {
      setBusy(null);
    }
  };

  const push = async () => {
    if (databaseChoice === '') return; // unreachable; the button is disabled
    setBusy('push');
    setError(null);
    try {
      const view = await runtimeApi.pushStaging(siteId, databaseChoice);
      setStaging(view);
      setDatabaseChoice('');
      setConfirmDomain('');
    } catch (err) {
      setError(errorMessage(err, 'The push to production failed.'));
    } finally {
      setBusy(null);
    }
  };

  const removeStaging = async () => {
    setBusy('delete');
    setError(null);
    try {
      await runtimeApi.deleteStaging(siteId);
      setStaging(null);
      setStagingMissing(true);
    } catch (err) {
      setError(errorMessage(err, 'The staging record could not be removed.'));
    } finally {
      setBusy(null);
    }
  };

  const chosen = DATABASE_CHOICES.find((c) => c.value === databaseChoice) || null;
  const productionDomain = site?.domain || '';
  const confirmationNeeded = Boolean(chosen?.destructive);
  const confirmationMet =
    !confirmationNeeded || (productionDomain !== '' && confirmDomain.trim() === productionDomain);

  const pushReason = !ready
    ? 'This needs the WP-CLI routes, which are not mounted on this panel.'
    : !staging
      ? 'There is no staging copy to push.'
      : databaseChoice === ''
        ? 'Choose what happens to the production database first. There is no default.'
        : !confirmationMet
          ? `Type ${productionDomain || 'the production domain'} to confirm that the production database may be replaced.`
          : null;

  const cloneReason = !ready
    ? 'This needs the WP-CLI routes, which are not mounted on this panel.'
    : !siteId
      ? 'Choose an installation first.'
      : !dbName.trim() || !dbUser.trim() || !dbPassword
        ? 'The staging database needs its own name, user and password. They must differ from production.'
        : null;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Data Copy"
        description="Clone a live site into a staging copy, work on it there, and push the result back with an explicit decision about the production database."
      />

      <RuntimeBanner availability={availability} onRecheck={recheck} />

      {error ? <ErrorNote>{error}</ErrorNote> : null}

      <Card>
        <CardHeader title="Installation" />
        <CardBody>
          {sitesLoading ? (
            <Loading label="Loading installations" />
          ) : (
            <SitePicker
              sites={sites}
              value={siteId}
              onChange={setSiteId}
              label="Production installation"
              hint="Staging is always a copy of one production installation and shares its unix user."
            />
          )}
        </CardBody>
      </Card>

      {!siteId ? null : stagingLoading ? (
        <Card>
          <Loading label="Reading the staging environment" />
        </Card>
      ) : staging ? (
        <>
          <Card>
            <CardHeader
              title="Staging copy"
              description="A second directory and a second database, running as the same unix user as production."
              actions={
                <Button
                  tone="secondary"
                  busy={busy === 'delete'}
                  unavailableReason={ready ? null : 'This needs the WP-CLI routes.'}
                  onClick={removeStaging}
                >
                  Forget this staging copy
                </Button>
              }
            />
            <CardBody className="space-y-4">
              <StagingSummary staging={staging} />
              <p className="text-xs text-gray-500">
                Forgetting a staging copy removes the panel&apos;s record of it. The staging files and
                the staging database are left on the server, because &quot;remove staging&quot; from a
                list is not a request to delete a directory, and that difference is not recoverable.
              </p>
            </CardBody>
          </Card>

          <Card className="border-amber-200">
            <CardHeader
              title="Push staging back to production"
              description="Production is backed up before anything is overwritten, and the path to that backup is shown afterwards."
            />
            <CardBody className="space-y-4">
              <fieldset>
                <legend className="mb-2 text-sm font-medium text-gray-700">
                  What happens to the production database
                </legend>
                <p className="mb-3 text-sm text-gray-600">
                  Nothing is selected. This choice has no default, here or in the API, because
                  choosing for you is how a week of orders is lost.
                </p>
                <div className="space-y-2">
                  {DATABASE_CHOICES.map((choice) => (
                    <label
                      key={choice.value}
                      className={
                        'flex items-start gap-3 rounded-md border p-3 ' +
                        (databaseChoice === choice.value
                          ? choice.destructive
                            ? 'border-red-300 bg-red-50'
                            : 'border-brand-300 bg-brand-50'
                          : 'border-gray-200')
                      }
                    >
                      <input
                        type="radio"
                        name="staging-database"
                        className="mt-1"
                        checked={databaseChoice === choice.value}
                        onChange={() => {
                          setDatabaseChoice(choice.value);
                          setConfirmDomain('');
                        }}
                      />
                      <span className="text-sm">
                        <span className="font-medium text-gray-900">{choice.label}</span>
                        <span className="mt-0.5 block text-gray-600">{choice.consequence}</span>
                        <code className="mt-1 block font-mono text-xs text-gray-500">
                          database: &quot;{choice.value}&quot;
                        </code>
                      </span>
                    </label>
                  ))}
                </div>
              </fieldset>

              {confirmationNeeded ? (
                <Notice tone="red" title="This replaces the live database">
                  <p>
                    Type the production domain to confirm. A dialog would be dismissed by the same
                    reflex that pressed the button.
                  </p>
                  <div className="mt-3 max-w-sm">
                    <Field label={`Type ${productionDomain || 'the production domain'}`} htmlFor="confirm-domain">
                      <TextInput
                        id="confirm-domain"
                        value={confirmDomain}
                        autoComplete="off"
                        onChange={(e) => setConfirmDomain(e.target.value)}
                      />
                    </Field>
                  </div>
                </Notice>
              ) : null}

              {staging.push ? (
                <div className="rounded-md border border-gray-200 bg-gray-50 p-4 text-sm">
                  <p className="font-medium text-gray-900">The last push</p>
                  <ul className="mt-2 space-y-1 text-gray-700">
                    <li>Decision: {staging.push.database_action || 'unknown'}</li>
                    <li>Files copied: {staging.push.files_copied ? 'yes' : 'no'}</li>
                    <li>Database copied: {staging.push.database_copied ? 'yes' : 'no'}</li>
                    {staging.push.backup_path ? (
                      <li className="break-all">
                        Production files backed up to{' '}
                        <code className="font-mono text-xs">{staging.push.backup_path}</code>
                      </li>
                    ) : null}
                    {staging.push.database_backup_path ? (
                      <li className="break-all">
                        Production database backed up to{' '}
                        <code className="font-mono text-xs">{staging.push.database_backup_path}</code>
                      </li>
                    ) : null}
                  </ul>
                </div>
              ) : null}

              <div className="flex justify-end">
                <Button
                  tone={chosen?.destructive ? 'danger' : 'primary'}
                  busy={busy === 'push'}
                  unavailableReason={pushReason}
                  onClick={push}
                >
                  {chosen?.destructive ? 'Push and replace the production database' : 'Push files to production'}
                </Button>
              </div>
            </CardBody>
          </Card>
        </>
      ) : stagingMissing ? (
        <Card>
          <CardHeader
            title="Create a staging copy"
            description="Copies the production files and database into a second site on the same server, rewrites the URLs, and blocks search engines from indexing it."
          />
          <CardBody className="space-y-4">
            {!ready ? (
              <EmptyState title="Staging is not available on this panel">
                Cloning and pushing are implemented in Go and are not reachable: the routes under
                /api/v1/wordpress/:id/staging are not mounted. No form is shown, because it could not
                do anything.
              </EmptyState>
            ) : (
              <>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <Field
                    label="Subdomain"
                    htmlFor="stg-sub"
                    hint={
                      site?.domain
                        ? `The staging site will be ${subdomain || 'staging'}.${site.domain}.`
                        : 'Prepended to the production domain.'
                    }
                  >
                    <TextInput id="stg-sub" value={subdomain} onChange={(e) => setSubdomain(e.target.value)} />
                  </Field>
                  <Field label="Staging database host" htmlFor="stg-host">
                    <TextInput id="stg-host" value={dbHost} onChange={(e) => setDbHost(e.target.value)} />
                  </Field>
                  <Field
                    label="Staging database name"
                    required
                    htmlFor="stg-db"
                    hint="Must differ from the production database. The API refuses it otherwise."
                  >
                    <TextInput id="stg-db" value={dbName} onChange={(e) => setDbName(e.target.value)} />
                  </Field>
                  <Field label="Staging database user" required htmlFor="stg-user">
                    <TextInput id="stg-user" value={dbUser} onChange={(e) => setDbUser(e.target.value)} />
                  </Field>
                  <Field label="Staging database password" required htmlFor="stg-pass">
                    <div className="flex gap-2">
                      <TextInput
                        id="stg-pass"
                        type="text"
                        className="font-mono"
                        value={dbPassword}
                        onChange={(e) => setDbPassword(e.target.value)}
                      />
                      <Button tone="secondary" onClick={() => setDbPassword(generatePassword(24))}>
                        Generate
                      </Button>
                    </div>
                  </Field>
                </div>

                <label className="flex items-start gap-3 rounded-md border border-gray-200 p-3 text-sm">
                  <input
                    type="checkbox"
                    className="mt-1"
                    checked={blockIndexing}
                    onChange={(e) => setBlockIndexing(e.target.checked)}
                  />
                  <span>
                    <span className="font-medium text-gray-900">
                      Block search engines from indexing the staging copy
                    </span>
                    <span className="mt-0.5 block text-gray-600">
                      Sets blog_public to 0. Leaving this off puts two copies of the same site in the
                      index, which costs the customer their ranking and is the most common complaint
                      about staging.
                    </span>
                  </span>
                </label>

                <div className="flex justify-end">
                  <Button tone="primary" busy={busy === 'clone'} unavailableReason={cloneReason} onClick={createStaging}>
                    Clone to staging
                  </Button>
                </div>
              </>
            )}
          </CardBody>
        </Card>
      ) : null}
    </div>
  );
}

export default DataCopy;
