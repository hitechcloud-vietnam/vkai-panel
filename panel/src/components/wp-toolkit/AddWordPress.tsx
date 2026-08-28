'use client';

/**
 * Add WordPress.
 *
 * Two things happen here and the screen never lets them blur together:
 *
 *   1. POST /api/v1/wordpress writes the panel's RECORD of an installation.
 *      This route is mounted and works on every panel.
 *   2. POST /api/v1/wordpress/:id/install downloads WordPress, writes
 *      wp-config.php, runs the installer as a non-root user and fixes
 *      ownership. This is the route that puts a site on disk, and it only
 *      exists when the WP-CLI routes are mounted.
 *
 * When step 2 is unavailable the form still submits step 1, but it is relabelled
 * and says plainly that it registers an installation rather than creating one.
 * The alternative - a button called "Install WordPress" that writes a database
 * row and stops - is the exact defect this section exists to avoid.
 *
 * The admin password is generated in the browser from crypto.getRandomValues,
 * shown once on the confirmation panel, and dropped from state when the operator
 * dismisses it. Nothing in this component can bring it back.
 */

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { RefreshCw } from 'lucide-react';
import { errorMessage, records, runtime as runtimeApi, supporting } from './api';
import { useFormOptions, useRuntimeAvailability } from './hooks';
import { RuntimeBanner } from './RuntimeBanner';
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  ErrorNote,
  Field,
  Notice,
  PageHeader,
  SelectInput,
  ShownOnce,
  TextInput,
  generatePassword,
} from './ui';
import type { CreateWordPressSiteRequest, InstallResult } from '@/types/wordpress';

/**
 * The locales WordPress ships translations for that this panel offers.
 *
 * Not the full list of 200-odd: an operator picking from a list that long
 * scrolls past the one they want. Anything absent can be installed afterwards
 * from wp-admin, and the value is passed straight through to `wp core install
 * --locale=`, so a wrong entry here is a WP-CLI error rather than a silent
 * default.
 */
const LOCALES: { value: string; label: string }[] = [
  { value: 'en_US', label: 'English (United States)' },
  { value: 'en_GB', label: 'English (United Kingdom)' },
  { value: 'vi', label: 'Vietnamese' },
  { value: 'fr_FR', label: 'French' },
  { value: 'de_DE', label: 'German' },
  { value: 'es_ES', label: 'Spanish' },
  { value: 'pt_BR', label: 'Portuguese (Brazil)' },
  { value: 'it_IT', label: 'Italian' },
  { value: 'nl_NL', label: 'Dutch' },
  { value: 'ru_RU', label: 'Russian' },
  { value: 'ja', label: 'Japanese' },
  { value: 'ko_KR', label: 'Korean' },
  { value: 'zh_CN', label: 'Chinese (Simplified)' },
  { value: 'th', label: 'Thai' },
  { value: 'id_ID', label: 'Indonesian' },
];

const WEB_SERVERS = ['nginx', 'apache', 'caddy', 'litespeed', 'openlitespeed', 'traefik'];

type SiteMode = 'existing' | 'create';

interface Outcome {
  siteId: string;
  domain: string;
  adminUser: string;
  adminPassword: string;
  loginUrl: string;
  installed: InstallResult | null;
  /** Steps that were skipped, and why. Rendered next to the credentials. */
  notes: string[];
}

export function AddWordPress() {
  const { availability, ready, recheck } = useRuntimeAvailability();
  const options = useFormOptions();

  const [mode, setMode] = useState<SiteMode>('existing');
  const [websiteId, setWebsiteId] = useState('');
  const [serverId, setServerId] = useState('');
  const [webServer, setWebServer] = useState('nginx');

  const [domain, setDomain] = useState('');
  const [title, setTitle] = useState('');
  const [path, setPath] = useState('');
  const [locale, setLocale] = useState('en_US');
  const [phpVersion, setPhpVersion] = useState('');

  const [adminUser, setAdminUser] = useState('admin');
  const [adminEmail, setAdminEmail] = useState('');
  const [adminPassword, setAdminPassword] = useState('');

  const [dbName, setDbName] = useState('');
  const [dbUser, setDbUser] = useState('');
  const [dbPassword, setDbPassword] = useState('');
  const [dbHost, setDbHost] = useState('localhost');
  const [dbPrefix, setDbPrefix] = useState('wp_');

  const [runAsUser, setRunAsUser] = useState('');
  const [runAsGroup, setRunAsGroup] = useState('');
  const [autoUpdate, setAutoUpdate] = useState(true);

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [outcome, setOutcome] = useState<Outcome | null>(null);

  // The password is generated on mount rather than during render:
  // crypto.getRandomValues does not exist while Next renders on the server, and
  // a value that differs between server and client is a hydration mismatch.
  useEffect(() => {
    setAdminPassword(generatePassword());
  }, []);

  // Picking an existing website fills in what the panel already knows about it.
  useEffect(() => {
    if (mode !== 'existing' || !websiteId) return;
    const site = options.websites.find((w) => w.id === websiteId);
    if (!site) return;
    setDomain((current) => current || site.domain);
    setPath((current) => current || site.root_dir);
    setServerId((current) => current || site.server_id);
    setPhpVersion((current) => current || '');
  }, [mode, websiteId, options.websites]);

  const selectedServer = useMemo(
    () => options.servers.find((s) => s.id === serverId) || null,
    [options.servers, serverId],
  );

  const missingRequired = useMemo(() => {
    const missing: string[] = [];
    if (!serverId) missing.push('server');
    if (mode === 'existing' && !websiteId) missing.push('website');
    if (!domain.trim()) missing.push('domain');
    if (!title.trim()) missing.push('site title');
    if (!path.trim()) missing.push('install path');
    if (!adminUser.trim()) missing.push('admin username');
    if (!adminEmail.trim()) missing.push('admin email');
    if (!adminPassword) missing.push('admin password');
    if (!dbName.trim()) missing.push('database name');
    if (!dbUser.trim()) missing.push('database user');
    if (!dbPassword) missing.push('database password');
    if (ready && !runAsUser.trim()) missing.push('run-as user');
    return missing;
  }, [
    serverId, mode, websiteId, domain, title, path, adminUser, adminEmail,
    adminPassword, dbName, dbUser, dbPassword, ready, runAsUser,
  ]);

  const submit = async () => {
    setSubmitting(true);
    setError(null);
    const notes: string[] = [];
    try {
      // 1. The website, when the operator asked for one to be created.
      let linkedWebsiteId = mode === 'existing' ? websiteId : '';
      if (mode === 'create') {
        const created = await supporting.createWebsite({
          domain: domain.trim(),
          server_id: serverId,
          web_server_type: webServer,
          root_dir: path.trim(),
          php_version: phpVersion || undefined,
          site_type: 'wordpress',
        });
        linkedWebsiteId = typeof created?.id === 'string' ? created.id : '';
        if (!linkedWebsiteId) {
          notes.push(
            'The website was created but the API did not return its id, so the WordPress record is not linked to it.',
          );
        }
      }

      // 2. The panel's record of the installation.
      const body: CreateWordPressSiteRequest = {
        server_id: serverId,
        website_id: linkedWebsiteId || undefined,
        name: title.trim(),
        domain: domain.trim(),
        path: path.trim(),
        db_name: dbName.trim(),
        db_user: dbUser.trim(),
        db_password: dbPassword,
        db_host: dbHost.trim() || 'localhost',
        db_prefix: dbPrefix.trim() || 'wp_',
        admin_user: adminUser.trim(),
        admin_password: adminPassword,
        admin_email: adminEmail.trim(),
        auto_update: autoUpdate,
      };
      const record = await records.create(body);
      if (!record?.id) {
        throw new Error('The panel accepted the request but returned no installation id.');
      }

      // 3. The real installation, when the route for it exists.
      let installed: InstallResult | null = null;
      if (ready) {
        installed = await runtimeApi.install(record.id, {
          run_as_user: runAsUser.trim(),
          run_as_group: runAsGroup.trim() || undefined,
          site_title: title.trim(),
          locale,
        });
        if (phpVersion) {
          try {
            await runtimeApi.setView(record.id, {
              run_as_user: runAsUser.trim(),
              run_as_group: runAsGroup.trim() || undefined,
              php_version: phpVersion,
            });
          } catch (err) {
            notes.push(
              `WordPress was installed, but the PHP version could not be recorded against it: ${errorMessage(err, 'unknown error')}`,
            );
          }
        }
      } else {
        notes.push(
          'WordPress was NOT installed on disk. Only the panel record was written, because ' +
            'POST /api/v1/wordpress/:id/install is not mounted on this panel.',
        );
      }

      setOutcome({
        siteId: record.id,
        domain: domain.trim(),
        adminUser: adminUser.trim(),
        adminPassword,
        loginUrl: `http://${domain.trim()}/wp-admin/`,
        installed,
        notes,
      });
      // The generated password now exists in exactly one place: `outcome`.
      // Clearing the field means a re-render of the form cannot leak it.
      setAdminPassword('');
    } catch (err) {
      setError(errorMessage(err, 'The installation could not be created.'));
    } finally {
      setSubmitting(false);
    }
  };

  if (outcome) {
    return (
      <div className="space-y-5">
        <PageHeader
          title={outcome.installed ? 'WordPress installed' : 'Installation registered'}
          description={outcome.domain}
        />
        <ShownOnce
          title="Administrator credentials"
          description="This password is shown once. It is not displayed again anywhere in the panel, so copy it now."
          entries={[
            { label: 'Login URL', value: outcome.loginUrl },
            { label: 'Username', value: outcome.adminUser },
            { label: 'Password', value: outcome.adminPassword },
          ]}
          onDismiss={() => {
            setOutcome(null);
            setAdminPassword(generatePassword());
          }}
        />
        {outcome.installed ? (
          <Card>
            <CardHeader title="What the installer did" />
            <CardBody>
              <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
                <div>
                  <dt className="text-gray-500">Installed version</dt>
                  <dd className="text-gray-900">{outcome.installed.installed_version || 'unknown'}</dd>
                </div>
                <div>
                  <dt className="text-gray-500">Path</dt>
                  <dd className="break-all font-mono text-xs text-gray-900">{outcome.installed.path}</dd>
                </div>
                <div>
                  <dt className="text-gray-500">Ran as</dt>
                  <dd className="font-mono text-xs text-gray-900">{outcome.installed.ran_as}</dd>
                </div>
                <div>
                  <dt className="text-gray-500">Ownership</dt>
                  <dd className="break-all font-mono text-xs text-gray-900">{outcome.installed.ownership}</dd>
                </div>
              </dl>
            </CardBody>
          </Card>
        ) : null}
        {outcome.notes.length > 0 ? (
          <Notice tone="amber" title="Not everything was done">
            <ul className="list-disc space-y-1 pl-5">
              {outcome.notes.map((note) => (
                <li key={note}>{note}</li>
              ))}
            </ul>
          </Notice>
        ) : null}
        <div className="flex gap-2">
          <Link
            href="/wp-toolkit"
            className="inline-flex items-center gap-2 rounded-md border border-transparent bg-brand-600 px-3 py-2 text-sm font-medium text-white hover:bg-brand-700"
          >
            Back to installations
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Add WordPress"
        description="Install WordPress into a website this panel already serves, or create the website at the same time."
      />

      <RuntimeBanner availability={availability} onRecheck={recheck} />

      {!ready && availability !== 'checking' ? (
        <Notice tone="amber" title="This form will not install WordPress on this panel">
          Submitting registers the installation so the rest of the toolkit can see it, and writes the
          database, admin and path details you enter. It does not download WordPress, write
          wp-config.php or run the installer, because the route that does that is not mounted. The
          button below is labelled for what it will actually do.
        </Notice>
      ) : null}

      {error ? <ErrorNote>{error}</ErrorNote> : null}

      <Card>
        <CardHeader title="Where it goes" />
        <CardBody className="space-y-4">
          <fieldset>
            <legend className="mb-2 text-sm font-medium text-gray-700">Website</legend>
            <div className="space-y-2">
              <label className="flex items-start gap-3 rounded-md border border-gray-200 p-3">
                <input
                  type="radio"
                  name="site-mode"
                  className="mt-1"
                  checked={mode === 'existing'}
                  onChange={() => setMode('existing')}
                />
                <span className="text-sm">
                  <span className="font-medium text-gray-900">Install into an existing website</span>
                  <span className="mt-0.5 block text-gray-600">
                    The website, its document root and its virtual host already exist in this panel.
                  </span>
                </span>
              </label>
              <label className="flex items-start gap-3 rounded-md border border-gray-200 p-3">
                <input
                  type="radio"
                  name="site-mode"
                  className="mt-1"
                  checked={mode === 'create'}
                  onChange={() => setMode('create')}
                />
                <span className="text-sm">
                  <span className="font-medium text-gray-900">Create the website too</span>
                  <span className="mt-0.5 block text-gray-600">
                    Creates the website through POST /api/v1/websites first, then installs into it.
                  </span>
                </span>
              </label>
            </div>
          </fieldset>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Server" required htmlFor="wp-server">
              <SelectInput
                id="wp-server"
                value={serverId}
                onChange={(e) => setServerId(e.target.value)}
              >
                <option value="">
                  {options.servers.length === 0 ? 'No servers are registered' : 'Select a server'}
                </option>
                {options.servers.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.label}
                  </option>
                ))}
              </SelectInput>
            </Field>

            {mode === 'existing' ? (
              <Field
                label="Website"
                required
                htmlFor="wp-website"
                hint={options.errors.websites || 'Selecting one fills in the domain and document root.'}
              >
                <SelectInput
                  id="wp-website"
                  value={websiteId}
                  onChange={(e) => setWebsiteId(e.target.value)}
                >
                  <option value="">
                    {options.websites.length === 0 ? 'No websites are registered' : 'Select a website'}
                  </option>
                  {options.websites.map((w) => (
                    <option key={w.id} value={w.id}>
                      {w.domain || w.id}
                    </option>
                  ))}
                </SelectInput>
              </Field>
            ) : (
              <Field label="Web server" required htmlFor="wp-webserver">
                <SelectInput
                  id="wp-webserver"
                  value={webServer}
                  onChange={(e) => setWebServer(e.target.value)}
                >
                  {WEB_SERVERS.map((w) => (
                    <option key={w} value={w}>
                      {w}
                    </option>
                  ))}
                </SelectInput>
              </Field>
            )}

            <Field label="Domain" required htmlFor="wp-domain">
              <TextInput
                id="wp-domain"
                value={domain}
                placeholder="example.com"
                onChange={(e) => setDomain(e.target.value)}
              />
            </Field>

            <Field
              label="Install path"
              required
              htmlFor="wp-path"
              hint="The WordPress root on the server. It has to sit inside the panel web root; the API refuses anything outside it."
            >
              <TextInput
                id="wp-path"
                value={path}
                placeholder="/vkai-panel/www/domains/example.com"
                onChange={(e) => setPath(e.target.value)}
              />
            </Field>

            <Field label="Site title" required htmlFor="wp-title">
              <TextInput
                id="wp-title"
                value={title}
                placeholder="My company site"
                onChange={(e) => setTitle(e.target.value)}
              />
            </Field>

            <Field label="Site language" htmlFor="wp-locale" hint="Passed to wp core install as --locale.">
              <SelectInput id="wp-locale" value={locale} onChange={(e) => setLocale(e.target.value)}>
                {LOCALES.map((l) => (
                  <option key={l.value} value={l.value}>
                    {l.label} ({l.value})
                  </option>
                ))}
              </SelectInput>
            </Field>

            <Field
              label="PHP version"
              htmlFor="wp-php"
              hint={
                options.errors.php
                  ? options.errors.php
                  : options.phpVersions.length === 0
                    ? 'No PHP versions are registered on this panel, so none can be chosen.'
                    : mode === 'create'
                      ? 'Applied to the website that is created, and recorded against the installation.'
                      : ready
                        ? 'Recorded against the installation through PUT /api/v1/wordpress/:id/runtime.'
                        : 'Nothing can be done with this on this panel: the website already exists and the runtime route that records a PHP version is not mounted.'
              }
            >
              <SelectInput
                id="wp-php"
                value={phpVersion}
                disabled={options.phpVersions.length === 0 || (mode === 'existing' && !ready)}
                onChange={(e) => setPhpVersion(e.target.value)}
              >
                <option value="">Panel default</option>
                {options.phpVersions.map((v) => (
                  <option key={v} value={v}>
                    {v}
                  </option>
                ))}
              </SelectInput>
            </Field>
          </div>

          {selectedServer ? (
            <p className="text-xs text-gray-500">Installing on {selectedServer.label}.</p>
          ) : null}
        </CardBody>
      </Card>

      <Card>
        <CardHeader
          title="Administrator account"
          description="The password is generated here and shown once, on the next screen."
        />
        <CardBody>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label="Username" required htmlFor="wp-admin-user">
              <TextInput
                id="wp-admin-user"
                value={adminUser}
                onChange={(e) => setAdminUser(e.target.value)}
              />
            </Field>
            <Field label="Email" required htmlFor="wp-admin-email">
              <TextInput
                id="wp-admin-email"
                type="email"
                value={adminEmail}
                placeholder="you@example.com"
                onChange={(e) => setAdminEmail(e.target.value)}
              />
            </Field>
            <Field
              label="Password"
              required
              htmlFor="wp-admin-pass"
              hint="Generated in your browser. Shown once after the installation and never again."
            >
              <div className="flex gap-2">
                <TextInput
                  id="wp-admin-pass"
                  value={adminPassword}
                  onChange={(e) => setAdminPassword(e.target.value)}
                  className="font-mono"
                />
                <Button
                  tone="secondary"
                  title="Generate a new password"
                  onClick={() => setAdminPassword(generatePassword())}
                >
                  <RefreshCw className="h-4 w-4" aria-hidden />
                </Button>
              </div>
            </Field>
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardHeader
          title="Database"
          description="The database WordPress will use. It has to exist and be reachable from the server."
        />
        <CardBody className="space-y-4">
          {options.databases.length > 0 ? (
            <Field
              label="Use a database this panel already manages"
              htmlFor="wp-db-pick"
              hint="Fills in the name and user. The panel never returns stored database passwords, so that still has to be typed."
            >
              <SelectInput
                id="wp-db-pick"
                value=""
                onChange={(e) => {
                  const picked = options.databases.find((d) => d.id === e.target.value);
                  if (picked) {
                    setDbName(picked.name);
                    setDbUser(picked.username);
                  }
                }}
              >
                <option value="">Enter the details manually</option>
                {options.databases.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name} ({d.username})
                  </option>
                ))}
              </SelectInput>
            </Field>
          ) : null}

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Field label="Database name" required htmlFor="wp-db-name">
              <TextInput id="wp-db-name" value={dbName} onChange={(e) => setDbName(e.target.value)} />
            </Field>
            <Field label="Database user" required htmlFor="wp-db-user">
              <TextInput id="wp-db-user" value={dbUser} onChange={(e) => setDbUser(e.target.value)} />
            </Field>
            <Field label="Database password" required htmlFor="wp-db-pass">
              <TextInput
                id="wp-db-pass"
                type="password"
                value={dbPassword}
                onChange={(e) => setDbPassword(e.target.value)}
              />
            </Field>
            <Field label="Database host" htmlFor="wp-db-host">
              <TextInput id="wp-db-host" value={dbHost} onChange={(e) => setDbHost(e.target.value)} />
            </Field>
            <Field
              label="Table prefix"
              htmlFor="wp-db-prefix"
              hint="Two installations can share one database only if their prefixes differ."
            >
              <TextInput id="wp-db-prefix" value={dbPrefix} onChange={(e) => setDbPrefix(e.target.value)} />
            </Field>
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardHeader
          title="Ownership"
          description="Which unix user the installation belongs to. WP-CLI runs as this user and never as root."
        />
        <CardBody>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field
              label="Run as user"
              required={ready}
              htmlFor="wp-run-user"
              hint={
                ready
                  ? 'Required. The API refuses root, and refuses a user that does not exist on the server.'
                  : 'Only used by the installer, which is not mounted on this panel, so it is not collected.'
              }
            >
              <TextInput
                id="wp-run-user"
                value={runAsUser}
                disabled={!ready}
                placeholder="www-example"
                onChange={(e) => setRunAsUser(e.target.value)}
              />
            </Field>
            <Field label="Run as group" htmlFor="wp-run-group" hint="Defaults to the user's primary group.">
              <TextInput
                id="wp-run-group"
                value={runAsGroup}
                disabled={!ready}
                onChange={(e) => setRunAsGroup(e.target.value)}
              />
            </Field>
            <Field label="Automatic updates" htmlFor="wp-auto">
              <label className="mt-2 flex items-center gap-2 text-sm text-gray-700">
                <input
                  id="wp-auto"
                  type="checkbox"
                  checked={autoUpdate}
                  onChange={(e) => setAutoUpdate(e.target.checked)}
                />
                Record this installation as opting in to automatic updates
              </label>
            </Field>
          </div>
        </CardBody>
      </Card>

      <div className="flex flex-wrap items-center justify-end gap-3">
        {missingRequired.length > 0 ? (
          <p className="text-sm text-gray-500">Still needed: {missingRequired.join(', ')}.</p>
        ) : null}
        <Button
          tone="primary"
          busy={submitting}
          disabled={missingRequired.length > 0}
          onClick={submit}
        >
          {ready ? 'Install WordPress' : 'Register installation record'}
        </Button>
      </div>
    </div>
  );
}

export default AddWordPress;
