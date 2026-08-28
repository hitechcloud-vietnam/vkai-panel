/**
 * Docker: the shapes the panel expects, and an honest record of which of them
 * the backend can actually fill today.
 *
 * `core/internal/handler/docker.go` mounts twenty routes. Every one of them is
 * a stub: the list handlers return a hardcoded empty slice, the mutating
 * handlers write a log line and answer `{"message": "Container started"}`
 * without touching a Docker daemon. `core/internal/docker/` is an empty
 * directory and neither `core/go.mod` nor `agent/go.mod` depends on a Docker
 * client library.
 *
 * That matters for how this screen is built. An empty container list from a
 * stub does not mean "you have no containers", it means "the panel cannot see
 * your containers", and those two are not allowed to look the same. So each
 * pane names the handler that is empty and renders the truth for that state
 * rather than a generic empty table.
 *
 * The shapes below are what the panes already read. A handler that starts
 * returning these fields lights its pane up with no change on this side.
 */

// ---------------------------------------------------------------------------
// Tabs
// ---------------------------------------------------------------------------

export type DockerTabId =
  | 'overview'
  | 'container'
  | 'one-click'
  | 'cloud-image'
  | 'local-image'
  | 'compose'
  | 'network'
  | 'volume'
  | 'repository'
  | 'settings';

export interface DockerTab {
  id: DockerTabId;
  label: string;
}

/**
 * The strip across the top, in aaPanel's order so an operator moving across
 * finds each area where they expect it.
 */
export const DOCKER_TABS: DockerTab[] = [
  { id: 'overview', label: 'Overview' },
  { id: 'container', label: 'Container' },
  { id: 'one-click', label: 'One-Click Install' },
  { id: 'cloud-image', label: 'Cloud image' },
  { id: 'local-image', label: 'Local image' },
  { id: 'compose', label: 'Docker Compose' },
  { id: 'network', label: 'Network' },
  { id: 'volume', label: 'Volume' },
  { id: 'repository', label: 'Repository' },
  { id: 'settings', label: 'Settings' },
];

const TAB_IDS = new Set<string>(DOCKER_TABS.map((tab) => tab.id));

export const DEFAULT_DOCKER_TAB: DockerTabId = 'overview';

/** Narrows an arbitrary `?tab=` value, so a stale bookmark lands on Overview. */
export function parseDockerTab(value: string | null | undefined): DockerTabId {
  if (value && TAB_IDS.has(value)) return value as DockerTabId;
  return DEFAULT_DOCKER_TAB;
}

// ---------------------------------------------------------------------------
// Backend capability
// ---------------------------------------------------------------------------

/** One thing this screen would offer if the API could perform it. */
export interface CapabilityGap {
  /** The control an operator was looking for. */
  label: string;
  /** What is absent, named precisely enough to open a ticket from. */
  missing: string;
}

// ---------------------------------------------------------------------------
// Resources
// ---------------------------------------------------------------------------

/** A container row, in the columns aaPanel's Container tab carries. */
export interface DockerContainer {
  id: string;
  name: string;
  image: string;
  /** running | exited | paused | created | restarting | dead */
  state: string;
  status: string;
  ports: string;
  ip: string;
  cpu_percent: number | null;
  memory_bytes: number | null;
  memory_limit_bytes: number | null;
  started_at: string;
  created_at: string;
}

export interface DockerImage {
  id: string;
  repository: string;
  tag: string;
  size: number | null;
  /** Names of the containers currently built on this image. */
  containers: string[];
  created_at: string;
}

export interface DockerNetwork {
  id: string;
  name: string;
  driver: string;
  scope: string;
  subnet: string;
  gateway: string;
  containers: string[];
  created_at: string;
}

export interface DockerVolume {
  id: string;
  name: string;
  driver: string;
  mountpoint: string;
  size: number | null;
  /** Containers with this volume mounted. Removing while non-empty loses data. */
  containers: string[];
  created_at: string;
}

export interface DockerComposeProject {
  name: string;
  path: string;
  status: string;
  service_count: number | null;
  services: string[];
  created_at: string;
}

export interface DockerSummary {
  running_containers: number | null;
  stopped_containers: number | null;
  total_images: number | null;
  total_networks: number | null;
  total_volumes: number | null;
  disk_used_bytes: number | null;
  reclaimable_bytes: number | null;
  server_version: string | null;
}

/**
 * The systemd view of the daemon, from `GET /api/v1/services/docker`. This is
 * the one Docker figure on the screen that is genuinely measured: the handler
 * shells out to `systemctl show docker`.
 */
export interface DockerDaemonStatus {
  name: string;
  status: string;
  active_state: string;
  sub_state: string;
  description: string;
  pid: number;
  memory: number;
}
