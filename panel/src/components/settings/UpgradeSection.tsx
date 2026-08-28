'use client';

/**
 * Panel upgrades.
 *
 * The hard part of this screen is not the upgrade, it is the restart. The
 * upgrade replaces the API this page is talking to and restarts it, so for a
 * minute or two every request from here fails. That is the normal middle of a
 * successful upgrade, and treating it as an error - a red banner, a redirect to
 * the login page, a spinner that never resolves - is the failure mode this
 * component exists to avoid.
 *
 * So the watch loop is built on two endpoints rather than one:
 *
 *   /api/v1/upgrade/progress/:id  the step and the percentage, while the API
 *                                 is up. A connection error here is expected
 *                                 and is shown as "restarting", not "failed".
 *   /api/v1/version               needs no token and answers as soon as the new
 *                                 API accepts connections. When it reports the
 *                                 version we were upgrading to, the upgrade
 *                                 landed - that is the proof, not the progress
 *                                 endpoint.
 *
 * The server keeps the same two facts on disk and reconciles them the same way,
 * so an operator who closes this page and comes back still gets the outcome.
 * When neither endpoint can prove an outcome, this page says so rather than
 * guessing.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  AlertTriangle,
  ArrowUpCircle,
  Check,
  CheckCircle,
  Circle,
  GitCommitHorizontal,
  Loader2,
  RefreshCw,
  RotateCcw,
  Tag,
  XCircle,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { unwrap, upgradeApi } from '@/services/api';
import UpgradeConfirmDialog from './UpgradeConfirmDialog';
import {
  FALLBACK_UPGRADE_STEPS,
  UPGRADE_CONFIRM_GRACE_MS,
  UPGRADE_POLL_MS,
  UPGRADE_WATCH_LIMIT_MS,
  type BuildInfo,
  type UpgradeJob,
  type UpgradePhase,
  type UpgradeStartResult,
  type UpgradeStatus,
  type UpgradeStepInfo,
} from './upgrade-types';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function errorMessage(err: unknown, fallback: string): string {
  const response = (err as { response?: { data?: { error?: { message?: string } } } })?.response;
  return response?.data?.error?.message || fallback;
}

function httpStatus(err: unknown): number | null {
  const status = (err as { response?: { status?: number } })?.response?.status;
  return typeof status === 'number' ? status : null;
}

/** "v1.2.0" and "1.2.0" are one version. Empty never equals anything. */
function sameVersion(a: string | undefined, b: string | undefined): boolean {
  const na = (a ?? '').trim().replace(/^v/, '');
  const nb = (b ?? '').trim().replace(/^v/, '');
  return na !== '' && na === nb;
}

function formatTimestamp(value: string | null | undefined): string {
  if (!value) return 'Never';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return 'Unknown';
  return parsed.toLocaleString();
}

/**
 * True for a job that finished within the last day. The outcome banner is worth
 * showing on arrival for an upgrade that just happened - the operator may have
 * been watching from a browser that lost the connection - but a success from
 * last month is history, not news.
 */
function recentlyFinished(finishedAt: string | null | undefined): boolean {
  if (!finishedAt) return false;
  const parsed = new Date(finishedAt);
  if (Number.isNaN(parsed.getTime())) return false;
  return Date.now() - parsed.getTime() < 24 * 60 * 60 * 1000;
}

function formatBuildDate(value: string | undefined): string {
  if (!value || value === 'unknown') return 'Unknown';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

// ---------------------------------------------------------------------------
// Small presentational pieces
// ---------------------------------------------------------------------------

function Row({
  icon,
  label,
  children,
}: {
  icon?: React.ReactNode;
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-1 gap-1 border-b border-gray-100 py-3 last:border-b-0 sm:grid-cols-[200px_minmax(0,1fr)] sm:gap-4">
      <div className="flex items-center gap-2 text-sm font-medium text-gray-700">
        {icon ? <span className="text-gray-400">{icon}</span> : null}
        {label}
      </div>
      <div className="min-w-0 text-sm text-gray-900">{children}</div>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div className="animate-pulse space-y-3" aria-hidden="true">
      {Array.from({ length: 4 }).map((_, index) => (
        <div
          key={index}
          className="grid grid-cols-1 gap-2 sm:grid-cols-[200px_minmax(0,1fr)] sm:gap-4"
        >
          <div className="h-4 w-40 rounded bg-gray-100" />
          <div className="h-4 w-full max-w-md rounded bg-gray-100" />
        </div>
      ))}
    </div>
  );
}

/** One row of the step list. */
function StepRow({
  label,
  state,
}: {
  label: string;
  state: 'done' | 'active' | 'pending';
}) {
  const icon =
    state === 'done' ? (
      <Check size={16} className="text-emerald-600" />
    ) : state === 'active' ? (
      <Loader2 size={16} className="animate-spin text-brand-600" />
    ) : (
      <Circle size={16} className="text-gray-300" />
    );

  return (
    <li className="flex items-center gap-2.5 py-1.5">
      <span className="shrink-0">{icon}</span>
      <span
        className={
          state === 'active'
            ? 'text-sm font-medium text-gray-900'
            : state === 'done'
              ? 'text-sm text-gray-600'
              : 'text-sm text-gray-400'
        }
      >
        {label}
      </span>
      {state === 'active' ? (
        <Badge variant="brand" className="ml-1">
          In progress
        </Badge>
      ) : null}
    </li>
  );
}

// ---------------------------------------------------------------------------
// Section
// ---------------------------------------------------------------------------

interface UpgradeSectionProps {
  /**
   * True when another settings tab is holding edits that have not been saved.
   * An upgrade restarts the panel and those edits would be lost, so this is
   * surfaced both on the page and again inside the confirmation dialog.
   */
  unsavedSettings?: boolean;
}

/** Everything the watch loop needs that must not trigger a re-render. */
interface WatchState {
  jobId: string | null;
  fromVersion: string;
  toVersion: string;
  startedAt: number;
  reachableSince: number | null;
}

export default function UpgradeSection({ unsavedSettings = false }: UpgradeSectionProps) {
  const [status, setStatus] = useState<UpgradeStatus | null>(null);
  const [build, setBuild] = useState<BuildInfo | null>(null);
  const [job, setJob] = useState<UpgradeJob | null>(null);
  const [phase, setPhase] = useState<UpgradePhase>('idle');

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);
  const [starting, setStarting] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [dismissedJobID, setDismissedJobID] = useState<string | null>(null);
  /** The job this page watched to its end, so its outcome is always worth showing. */
  const [watchedJobID, setWatchedJobID] = useState<string | null>(null);

  const watch = useRef<WatchState>({
    jobId: null,
    fromVersion: '',
    toVersion: '',
    startedAt: 0,
    reachableSince: null,
  });

  // -------------------------------------------------------------------------
  // Loading
  // -------------------------------------------------------------------------

  const beginWatch = useCallback((jobId: string, fromVersion: string, toVersion: string) => {
    watch.current = {
      jobId,
      fromVersion,
      toVersion,
      startedAt: Date.now(),
      reachableSince: Date.now(),
    };
    setWatchedJobID(jobId);
  }, []);

  const endWatch = useCallback(() => {
    watch.current.jobId = null;
  }, []);

  /**
   * Re-reads the status without touching the phase. Used after an upgrade has
   * settled: the outcome has already been decided from the version endpoint,
   * and letting a status response reopen a phase this component has closed
   * would restart a watch loop with no job left to poll.
   */
  const refreshStatus = useCallback(async (): Promise<UpgradeStatus | null> => {
    try {
      const data = unwrap<UpgradeStatus>(await upgradeApi.status(), null);
      if (!data) return null;
      setStatus(data);
      setBuild({ version: data.current, commit: data.commit, build_date: data.build_date });
      setJob(data.job);
      return data;
    } catch {
      // The caller is a background refresh; the phase it just decided is still
      // what the operator needs to see, so a failure here changes nothing.
      return null;
    }
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await upgradeApi.status();
      const data = unwrap<UpgradeStatus>(response, null);
      if (!data) {
        setStatus(null);
        setError('The panel returned no version information.');
        return;
      }

      setStatus(data);
      setBuild({ version: data.current, commit: data.commit, build_date: data.build_date });
      setJob(data.job);

      // An upgrade started before this page was opened - by this operator in
      // another tab, or by another administrator - is picked up and watched
      // rather than ignored.
      if (data.job && data.job.running) {
        beginWatch(data.job.job_id, data.job.from_version, data.job.to_version);
        setPhase('running');
      } else if (data.job) {
        setPhase(data.job.state);
      } else {
        setPhase('idle');
      }
    } catch (err) {
      setStatus(null);
      setError(errorMessage(err, 'Could not read the panel version.'));
    } finally {
      setLoading(false);
    }
  }, [beginWatch]);

  useEffect(() => {
    void load();
  }, [load]);

  // -------------------------------------------------------------------------
  // The watch loop
  // -------------------------------------------------------------------------

  const settle = useCallback(
    (finalPhase: UpgradePhase) => {
      endWatch();
      setPhase(finalPhase);
      // Re-read the status so the version card, the changelog and the
      // "up to date" line all reflect the release that is now running.
      void refreshStatus();
    },
    [endWatch, refreshStatus],
  );

  const tick = useCallback(async () => {
    const current = watch.current;
    if (!current.jobId) return;

    // 1. The version endpoint. It needs no token and comes back as soon as the
    //    new API accepts connections, which makes it both the liveness probe
    //    and the proof of what is running.
    let liveBuild: BuildInfo | null = null;
    try {
      liveBuild = unwrap<BuildInfo>(await upgradeApi.version(), null);
      if (liveBuild) setBuild(liveBuild);
    } catch {
      // Expected while the API is restarting.
      liveBuild = null;
    }
    const reachable = liveBuild !== null;

    // 2. The job. Only worth asking when the API is answering at all.
    let liveJob: UpgradeJob | null = null;
    let jobForgotten = false;
    if (reachable) {
      try {
        liveJob = unwrap<UpgradeJob>(await upgradeApi.progress(current.jobId), null);
        if (liveJob) setJob(liveJob);
      } catch (err) {
        // 404 means this API has no record of the job. That happens when the
        // job was started by an install whose state file did not survive; it
        // is not a failed upgrade, so the version endpoint decides instead.
        if (httpStatus(err) === 404) jobForgotten = true;
      }
    }

    // 3. Decide. The server's own verdict wins when it has one.
    if (liveJob && !liveJob.running) {
      settle(liveJob.state);
      return;
    }

    if (reachable && sameVersion(liveBuild?.version, current.toVersion)) {
      settle('succeeded');
      return;
    }

    if (!reachable) {
      watch.current.reachableSince = null;
      setPhase('restarting');
      return;
    }

    if (watch.current.reachableSince === null) {
      watch.current.reachableSince = Date.now();
    }

    const now = Date.now();
    const answeringFor = now - (watch.current.reachableSince ?? now);

    // The API is up, it does not know the job, and it is not on the new
    // version. Nothing left can prove an outcome, so say that plainly.
    if (jobForgotten && answeringFor > UPGRADE_CONFIRM_GRACE_MS) {
      settle('unconfirmed');
      return;
    }

    if (now - current.startedAt > UPGRADE_WATCH_LIMIT_MS) {
      settle('unconfirmed');
      return;
    }

    setPhase('running');
  }, [settle]);

  useEffect(() => {
    const watching = phase === 'running' || phase === 'restarting';
    if (!watching || !watch.current.jobId) return undefined;

    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    // A self-scheduling timeout rather than an interval: a poll that takes
    // longer than the interval - which is exactly what happens against an API
    // that is going away - must not stack up behind itself.
    const schedule = () => {
      timer = setTimeout(async () => {
        if (cancelled) return;
        await tick();
        if (!cancelled) schedule();
      }, UPGRADE_POLL_MS);
    };
    schedule();

    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [phase, tick]);

  // -------------------------------------------------------------------------
  // Actions
  // -------------------------------------------------------------------------

  const runCheck = async () => {
    setChecking(true);
    setActionError(null);
    try {
      const response = await upgradeApi.check();
      const data = unwrap<UpgradeStatus>(response, null);
      if (data) {
        setStatus(data);
        setBuild({ version: data.current, commit: data.commit, build_date: data.build_date });
        setJob(data.job);
      }
    } catch (err) {
      setActionError(errorMessage(err, 'Could not reach the release source.'));
    } finally {
      setChecking(false);
    }
  };

  const startUpgrade = async () => {
    if (!status) return;
    setStarting(true);
    setActionError(null);
    try {
      const response = await upgradeApi.start(status.latest);
      const result = unwrap<UpgradeStartResult>(response, null);
      if (!result?.job_id) {
        setActionError('The panel accepted the upgrade but did not return a job to follow.');
        return;
      }

      beginWatch(result.job_id, result.from_version, result.to_version);
      setDismissedJobID(null);
      setJob({
        job_id: result.job_id,
        state: 'running',
        running: true,
        from_version: result.from_version,
        to_version: result.to_version,
        step: '',
        step_key: '',
        step_index: 0,
        percent: 0,
        message: 'The upgrade has been queued.',
        started_at: result.started_at,
        updated_at: result.started_at,
        detached: false,
        steps: result.steps,
      });
      setConfirmOpen(false);
      setPhase('running');
    } catch (err) {
      // 409 is the guard against a second upgrade. Say which one, rather than
      // reporting a generic failure the operator cannot act on.
      if (httpStatus(err) === 409) {
        setActionError('An upgrade is already running. Wait for it to finish.');
        void load();
      } else {
        setActionError(errorMessage(err, 'Could not start the upgrade.'));
      }
      setConfirmOpen(false);
    } finally {
      setStarting(false);
    }
  };

  // -------------------------------------------------------------------------
  // Derived
  // -------------------------------------------------------------------------

  const steps: UpgradeStepInfo[] = useMemo(
    () => job?.steps ?? status?.steps ?? FALLBACK_UPGRADE_STEPS,
    [job?.steps, status?.steps],
  );

  const inFlight = phase === 'running' || phase === 'restarting';
  const settled =
    phase === 'succeeded' || phase === 'failed' || phase === 'rolled_back' || phase === 'unconfirmed';
  const showOutcome =
    settled &&
    job !== null &&
    job.job_id !== dismissedJobID &&
    // Either this page watched the upgrade happen, or it finished recently
    // enough that the operator has not been told yet.
    (job.job_id === watchedJobID || recentlyFinished(job.finished_at));

  const currentVersion = build?.version ?? status?.current ?? '';
  const latestVersion = status?.latest ?? '';
  const canUpgrade = Boolean(status?.available) && Boolean(status?.update_available) && !inFlight;

  // The active step: what the engine reported, mapped onto the list. A step the
  // panel does not recognise is still shown, as its own row, rather than
  // silently freezing the list on the previous step.
  const activeIndex = job?.step_index ?? -1;
  const unknownStep = inFlight && activeIndex < 0 && Boolean(job?.step);

  const percent = phase === 'succeeded' ? 100 : (job?.percent ?? 0);

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  if (loading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Panel version</CardTitle>
        </CardHeader>
        <CardContent>
          <LoadingSkeleton />
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      {error ? (
        <div className="flex items-start gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3">
          <span className="mt-0.5 text-red-600">
            <XCircle size={18} />
          </span>
          <div className="min-w-0 text-sm text-red-700">
            <p className="font-medium">Version information unavailable</p>
            <p className="mt-0.5">{error}</p>
          </div>
        </div>
      ) : null}

      {/* Outcome of the last upgrade. Shown until it is dismissed, so an
          operator who was away while the panel restarted still sees it. */}
      {showOutcome && job ? (
        <div
          role="status"
          className={`flex items-start gap-3 rounded-lg border px-4 py-3 ${
            phase === 'succeeded'
              ? 'border-emerald-200 bg-emerald-50'
              : phase === 'rolled_back'
                ? 'border-amber-200 bg-amber-50'
                : phase === 'unconfirmed'
                  ? 'border-gray-200 bg-gray-50'
                  : 'border-red-200 bg-red-50'
          }`}
        >
          <span
            className={`mt-0.5 shrink-0 ${
              phase === 'succeeded'
                ? 'text-emerald-600'
                : phase === 'rolled_back'
                  ? 'text-amber-600'
                  : phase === 'unconfirmed'
                    ? 'text-gray-500'
                    : 'text-red-600'
            }`}
          >
            {phase === 'succeeded' ? (
              <CheckCircle size={18} />
            ) : phase === 'rolled_back' ? (
              <RotateCcw size={18} />
            ) : phase === 'unconfirmed' ? (
              <AlertTriangle size={18} />
            ) : (
              <XCircle size={18} />
            )}
          </span>
          <div
            className={`min-w-0 flex-1 text-sm ${
              phase === 'succeeded'
                ? 'text-emerald-700'
                : phase === 'rolled_back'
                  ? 'text-amber-800'
                  : phase === 'unconfirmed'
                    ? 'text-gray-700'
                    : 'text-red-700'
            }`}
          >
            {phase === 'succeeded' ? (
              <>
                <p className="font-medium">
                  Upgraded to {currentVersion || job.to_version}
                </p>
                <p className="mt-0.5">
                  The panel restarted and is running version {currentVersion || job.to_version}.
                  Reload the page if any screen still looks like the old version.
                </p>
              </>
            ) : phase === 'rolled_back' ? (
              <>
                <p className="font-medium">
                  The upgrade to {job.to_version} failed and was rolled back
                </p>
                <p className="mt-0.5">
                  The panel was restored to version {job.from_version} and is running normally.
                  Nothing was left half-installed. {job.message}
                </p>
              </>
            ) : phase === 'unconfirmed' ? (
              <>
                <p className="font-medium">The outcome of the upgrade could not be confirmed</p>
                <p className="mt-0.5">
                  The panel is answering on version {currentVersion || 'unknown'}, but it no longer
                  has a record of job {job.job_id}. Check the upgrade log on the server before
                  trying again.
                </p>
              </>
            ) : (
              <>
                <p className="font-medium">The upgrade to {job.to_version} failed</p>
                <p className="mt-0.5">
                  {job.message || 'The upgrade stopped before it finished.'}
                  {job.error ? ` (${job.error})` : ''} The panel is running version{' '}
                  {currentVersion || 'unknown'}. Check the upgrade log on the server.
                </p>
              </>
            )}
          </div>
          <button
            type="button"
            onClick={() => setDismissedJobID(job.job_id)}
            className="shrink-0 rounded-md px-2 py-1 text-xs font-medium text-gray-600 hover:bg-white/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            Dismiss
          </button>
        </div>
      ) : null}

      {/* Version card */}
      <Card>
        <CardHeader className="flex-row items-start justify-between gap-4 space-y-0">
          <div>
            <CardTitle>Panel version</CardTitle>
            <p className="mt-1 text-sm text-gray-600">
              The release this server is running, and where it came from.
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => void runCheck()}
              disabled={checking || inFlight || !status?.available}
            >
              <RefreshCw size={14} className={checking ? 'animate-spin' : undefined} />
              {checking ? 'Checking…' : 'Check for updates'}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="py-1">
          <Row icon={<Tag size={16} />} label="Current version">
            <span className="font-mono text-sm text-gray-900">{currentVersion || 'Unknown'}</span>
          </Row>
          <Row icon={<GitCommitHorizontal size={16} />} label="Build commit">
            <span className="font-mono text-sm text-gray-900">{build?.commit || 'Unknown'}</span>
          </Row>
          <Row label="Build date">{formatBuildDate(build?.build_date)}</Row>
          <Row label="Last checked">{formatTimestamp(status?.checked_at)}</Row>
        </CardContent>
      </Card>

      {/* Update card */}
      <Card>
        <CardHeader>
          <CardTitle>Available update</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {actionError ? (
            <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2.5 text-sm text-red-700">
              {actionError}
            </div>
          ) : null}

          {!status?.available ? (
            <div className="flex items-start gap-3 rounded-md border border-gray-200 bg-gray-50 px-3 py-2.5">
              <span className="mt-0.5 shrink-0 text-gray-500">
                <AlertTriangle size={16} />
              </span>
              <div className="text-sm text-gray-700">
                <p className="font-medium">This panel cannot upgrade itself</p>
                <p className="mt-0.5">
                  No release source is configured on this install, so updates have to be applied
                  from the server with <code className="font-mono">vkai upgrade</code>.
                </p>
              </div>
            </div>
          ) : inFlight ? null : status.update_available ? (
            <>
              <div className="flex flex-wrap items-center gap-3">
                <Badge variant="success">Version {latestVersion} available</Badge>
                <span className="text-sm text-gray-600">
                  You are on {currentVersion || 'an unknown version'}.
                </span>
              </div>

              {status.changelog ? (
                <div>
                  <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                    What is in {latestVersion}
                  </p>
                  <pre className="mt-1.5 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md border border-gray-200 bg-gray-50 px-3 py-2.5 font-sans text-sm text-gray-700">
                    {status.changelog}
                  </pre>
                </div>
              ) : (
                <p className="text-sm text-gray-600">
                  The release source published no notes for this version.
                </p>
              )}

              {unsavedSettings ? (
                <div className="flex items-start gap-2.5 rounded-md border border-amber-200 bg-amber-50 px-3 py-2.5">
                  <span className="mt-0.5 shrink-0 text-amber-600">
                    <AlertTriangle size={16} />
                  </span>
                  <p className="text-sm text-amber-800">
                    Another settings tab has unsaved changes. Save them first - the panel restarts
                    during an upgrade and anything not saved is lost.
                  </p>
                </div>
              ) : null}

              <div className="flex items-center gap-3">
                <Button
                  type="button"
                  onClick={() => {
                    setActionError(null);
                    setConfirmOpen(true);
                  }}
                  disabled={!canUpgrade || starting}
                >
                  <ArrowUpCircle size={16} />
                  Upgrade to {latestVersion}
                </Button>
                <p className="text-sm text-gray-600">
                  The panel restarts and is briefly unavailable.
                </p>
              </div>
            </>
          ) : (
            <div className="flex items-start gap-3 rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2.5">
              <span className="mt-0.5 shrink-0 text-emerald-600">
                <CheckCircle size={16} />
              </span>
              <div className="text-sm text-emerald-700">
                <p className="font-medium">The panel is up to date</p>
                <p className="mt-0.5">
                  Version {currentVersion || 'unknown'} is the newest release available.
                </p>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Progress card */}
      {inFlight ? (
        <Card>
          <CardHeader>
            <CardTitle>
              Upgrading to {job?.to_version || latestVersion}
            </CardTitle>
            <p className="mt-1 text-sm text-gray-600">
              Started from version {job?.from_version || currentVersion || 'unknown'}. Leave this
              page open - it keeps watching and will report the outcome.
            </p>
          </CardHeader>
          <CardContent className="space-y-4">
            {phase === 'restarting' ? (
              <div
                role="status"
                className="flex items-start gap-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2.5"
              >
                <span className="mt-0.5 shrink-0 text-amber-600">
                  <Loader2 size={16} className="animate-spin" />
                </span>
                <div className="text-sm text-amber-800">
                  <p className="font-medium">The panel is restarting</p>
                  <p className="mt-0.5">
                    The API is not answering right now. This is the expected middle of an upgrade,
                    not a failure. This page reconnects on its own - do not reload it and do not
                    restart the server.
                  </p>
                </div>
              </div>
            ) : (
              <div className="flex items-start gap-3 rounded-md border border-gray-200 bg-gray-50 px-3 py-2.5">
                <span className="mt-0.5 shrink-0 text-brand-600">
                  <Loader2 size={16} className="animate-spin" />
                </span>
                <div className="text-sm text-gray-700">
                  <p className="font-medium">{job?.message || 'The upgrade is running.'}</p>
                  <p className="mt-0.5">
                    The panel will restart during this upgrade and will be briefly unavailable.
                  </p>
                </div>
              </div>
            )}

            <div>
              <div className="mb-1.5 flex items-center justify-between text-xs font-medium text-gray-600">
                <span>Progress</span>
                <span>{percent}%</span>
              </div>
              <div
                role="progressbar"
                aria-valuenow={percent}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-label="Upgrade progress"
                className="h-2 w-full overflow-hidden rounded-full bg-gray-100"
              >
                <div
                  className="h-full rounded-full bg-brand-600 transition-all duration-500"
                  style={{ width: `${percent}%` }}
                />
              </div>
            </div>

            <ol className="divide-y divide-gray-100">
              {steps.map((step, index) => (
                <StepRow
                  key={step.key}
                  label={step.label}
                  state={
                    activeIndex < 0
                      ? 'pending'
                      : index < activeIndex
                        ? 'done'
                        : index === activeIndex
                          ? 'active'
                          : 'pending'
                  }
                />
              ))}
              {unknownStep ? (
                <StepRow label={job?.step ?? 'Working'} state="active" />
              ) : null}
            </ol>
          </CardContent>
        </Card>
      ) : null}

      <UpgradeConfirmDialog
        open={confirmOpen}
        fromVersion={currentVersion}
        toVersion={latestVersion}
        changelog={status?.changelog ?? ''}
        unsavedSettings={unsavedSettings}
        submitting={starting}
        onCancel={() => setConfirmOpen(false)}
        onConfirm={() => void startUpgrade()}
      />
    </div>
  );
}
