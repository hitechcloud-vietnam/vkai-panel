'use client';

/**
 * The accounts pane.
 *
 * aaPanel's FTP page is one table of accounts with a toolbar above it. This
 * pane does not draw that table, because the API has no route that could fill
 * it and no model that could store a row: an empty accounts table would read as
 * "no accounts on this server", which is a different statement from "the panel
 * cannot ask". So the intended table is described, the missing routes are
 * named, and the pane spends its space on the one thing it can answer truthfully
 * today - where an FTP account would be confined, and which of those directories
 * would be a bad place to confine one.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Folder, ShieldCheck } from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import { errorMessage } from '@/lib/apiError';
import { cn } from '@/lib/utils';

import { listWebsites } from './api';
import { CopyButton } from './CopyButton';
import { FeatureUnavailableNotice } from './CapabilityNotice';
import {
  EmptyState,
  ErrorBlock,
  Panel,
  PaneToolbar,
  SectionHeader,
  TableScroller,
  TableSkeleton,
  TD_CLASS,
  TH_CLASS,
} from './PaneChrome';
import { assessHomeDirectory, normalizePath } from './paths';
import { ACCOUNT_GAPS, type WebsiteRow } from './types';

/** The columns aaPanel's table carries, kept here so the intent is not lost. */
const INTENDED_COLUMNS = [
  'Username, and the account it maps to on the machine',
  'Home directory, with the effective root after symlinks are resolved',
  'Quota and current use, so a full account is visible before the customer calls',
  'Status - enabled or disabled - switched from the row',
  'Last login, and the address it came from',
  'Create, change password, change home directory, enable, disable, delete',
];

export function AccountsPane() {
  const [sites, setSites] = useState<WebsiteRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    else setLoading(true);
    setError(null);
    try {
      setSites(await listWebsites());
    } catch (err) {
      setError(errorMessage(err, 'Could not load the sites on this panel.'));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const roots = useMemo(() => sites.map((site) => site.root_dir ?? ''), [sites]);

  const rows = useMemo(
    () =>
      sites.map((site) => ({
        site,
        root: normalizePath(site.root_dir ?? ''),
        problem: assessHomeDirectory(site.root_dir ?? '', roots),
      })),
    [sites, roots]
  );

  const unsafe = rows.filter((row) => row.problem !== null).length;

  return (
    <div className="space-y-4">
      <FeatureUnavailableNotice
        title="FTP accounts cannot be managed from the panel yet"
        summary={
          <>
            The API has no FTP routes at all. Nothing under{' '}
            <span className="font-mono text-xs">/api/v1/ftp</span> is mounted, no handler
            mentions FTP and no table stores an account, so this pane will not draw a
            table it cannot fill or a create form that would post into nothing. Accounts
            have to be created on the machine itself - <span className="font-mono text-xs">pure-pw</span>{' '}
            or <span className="font-mono text-xs">useradd</span> plus the daemon&apos;s own
            user database - until the routes below exist.
          </>
        }
        wouldShow={INTENDED_COLUMNS}
        gaps={ACCOUNT_GAPS}
      />

      <Panel>
        <SectionHeader
          title="Where an FTP account may be confined"
          description="An FTP account must not be able to leave its directory. These are the site roots this panel reports, which are the only directories an account should ever be given."
          actions={<PaneToolbar onRefresh={() => void load(true)} refreshing={refreshing} />}
        />

        {loading ? (
          <TableSkeleton columns={4} />
        ) : error ? (
          <ErrorBlock
            title="Could not load site roots"
            message={error}
            onRetry={() => void load(true)}
          />
        ) : rows.length === 0 ? (
          <EmptyState
            title="No sites on this panel"
            description="An FTP account is confined to a site directory, so add a website first. Websites, then Add site - the root the panel creates for it becomes the home directory an account can be given."
          />
        ) : (
          <>
            <TableScroller>
              <table className="w-full min-w-[720px] border-collapse">
                <thead className="bg-gray-50">
                  <tr className="border-b border-gray-200">
                    <th className={TH_CLASS}>Site</th>
                    <th className={TH_CLASS}>Effective root</th>
                    <th className={TH_CLASS}>Confinement</th>
                    <th className={TH_CLASS}>Site status</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map(({ site, root, problem }) => (
                    <tr key={site.id} className="border-b border-gray-200 last:border-b-0">
                      <td className={cn(TD_CLASS, 'font-medium text-gray-900')}>
                        {site.domain || '—'}
                      </td>
                      <td className={TD_CLASS}>
                        <div className="flex items-center gap-1">
                          <Folder className="h-4 w-4 shrink-0 text-gray-400" aria-hidden="true" />
                          <span className="font-mono text-xs text-gray-900">
                            {root || '—'}
                          </span>
                          {root ? <CopyButton label="Root" getValue={() => root} /> : null}
                        </div>
                        {site.root_dir && site.root_dir !== root && (
                          <p className="mt-1 text-xs text-gray-500">
                            Recorded as{' '}
                            <span className="font-mono">{site.root_dir}</span>
                          </p>
                        )}
                      </td>
                      <td className={TD_CLASS}>
                        {problem ? (
                          <div className="flex items-start gap-2">
                            <Badge variant={problem.severity === 'danger' ? 'danger' : 'warning'}>
                              <AlertTriangle className="h-3 w-3" aria-hidden="true" />
                              {problem.label}
                            </Badge>
                            <span className="max-w-md text-xs text-gray-600">
                              {problem.detail}
                            </span>
                          </div>
                        ) : (
                          <Badge variant="success">
                            <ShieldCheck className="h-3 w-3" aria-hidden="true" />
                            Safe to confine
                          </Badge>
                        )}
                      </td>
                      <td className={TD_CLASS}>
                        <Badge variant={site.status === 'active' ? 'success' : 'neutral'}>
                          {site.status || 'unknown'}
                        </Badge>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </TableScroller>

            <div className="border-t border-gray-200 px-4 py-3 text-xs text-gray-500">
              {unsafe === 0
                ? `${rows.length} site ${rows.length === 1 ? 'root' : 'roots'}, none of which would give an FTP account more than its own site.`
                : `${unsafe} of ${rows.length} site roots would give an FTP account more than its own site.`}{' '}
              These checks run in this browser against the roots the API reports. They are
              how the interface explains the rule; they are not the security boundary. The
              boundary is the daemon&apos;s chroot plus a server-side check that resolves
              symlinks, and that check has to be written with the create and change-home
              endpoints.
            </div>
          </>
        )}
      </Panel>
    </div>
  );
}

export default AccountsPane;
