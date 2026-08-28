/**
 * Shapes returned by /api/v1/version and /api/v1/upgrade/*.
 *
 * They mirror the Go types in core/internal/service/upgrade.go. Keeping them in
 * one place means a field renamed on the server fails to compile here rather
 * than rendering as "undefined" in front of an operator deciding whether to
 * replace every binary on their server.
 */

/** GET /api/v1/version. Three fields, deliberately: this route needs no token. */
export interface BuildInfo {
  version: string;
  commit: string;
  build_date: string;
}

/** One entry in the step list drawn during an upgrade. */
export interface UpgradeStepInfo {
  key: string;
  label: string;
}

export type UpgradeJobState = 'running' | 'succeeded' | 'failed' | 'rolled_back';

export interface UpgradeJob {
  job_id: string;
  state: UpgradeJobState;
  running: boolean;
  from_version: string;
  to_version: string;
  /** The step name as the release engine reported it. */
  step: string;
  /** That step mapped onto one of `steps`, or "" when it is not one of them. */
  step_key: string;
  /** Its position in `steps`, or -1. */
  step_index: number;
  percent: number;
  message: string;
  error?: string;
  started_at: string;
  updated_at: string;
  finished_at?: string | null;
  /**
   * True when the API process that started this job is gone - which is the
   * normal outcome of a successful upgrade, not a fault.
   */
  detached: boolean;
  steps: UpgradeStepInfo[] | null;
}

export interface UpgradeStatus {
  current: string;
  latest: string;
  update_available: boolean;
  checked_at: string | null;
  changelog: string;
  commit: string;
  build_date: string;
  /** False when this build has no release engine wired in. */
  available: boolean;
  unavailable_reason?: string;
  job: UpgradeJob | null;
  steps: UpgradeStepInfo[] | null;
}

export interface UpgradeStartResult {
  job_id: string;
  from_version: string;
  to_version: string;
  started_at: string;
  steps: UpgradeStepInfo[] | null;
}

/**
 * What the section is showing. The two that do not come from the server are the
 * ones an upgrade makes necessary:
 *
 *   restarting  - the API stopped answering. Expected, not an error.
 *   unconfirmed - the API is answering again but neither proved the upgrade
 *                 landed nor proved it was rolled back. Said plainly rather
 *                 than guessed at.
 */
export type UpgradePhase =
  | 'idle'
  | 'running'
  | 'restarting'
  | 'succeeded'
  | 'rolled_back'
  | 'failed'
  | 'unconfirmed';

/**
 * The step list used before the server has sent one of its own - on the very
 * first render, and if a poll comes back without it. It mirrors UpgradeSteps in
 * core/internal/service/upgrade.go; the server's copy always wins when present,
 * so this going stale degrades the labels, never the behaviour.
 */
export const FALLBACK_UPGRADE_STEPS: UpgradeStepInfo[] = [
  { key: 'lock', label: 'Take the upgrade lock' },
  { key: 'check', label: 'Check the release feed' },
  { key: 'download', label: 'Download the release' },
  { key: 'verify', label: 'Verify the package' },
  { key: 'stage', label: 'Unpack into a staging directory' },
  { key: 'preflight', label: 'Run the pre-flight checks' },
  { key: 'backup', label: 'Back up the database' },
  { key: 'switch', label: 'Switch to the new release' },
  { key: 'restart', label: 'Restart the panel services' },
  { key: 'health', label: 'Check the panel came back' },
  { key: 'cleanup', label: 'Clean up old releases' },
];

/** How often the watcher polls while an upgrade is running. */
export const UPGRADE_POLL_MS = 2500;

/**
 * How long the panel may answer on the old version, with no job to poll, before
 * the outcome is reported as unconfirmed. Long enough for a service restart and
 * a health check; short enough that an operator is not left watching a spinner.
 */
export const UPGRADE_CONFIRM_GRACE_MS = 3 * 60 * 1000;

/** The longest an upgrade is watched before the outcome is called unconfirmed. */
export const UPGRADE_WATCH_LIMIT_MS = 30 * 60 * 1000;
