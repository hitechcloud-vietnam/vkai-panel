/**
 * The five project types the Websites screen splits into, and the truth about
 * what the backend can do for each one.
 *
 * The order matches the product owner's: PHP, Node.js, Proxy, Go, Python.
 *
 * `backend` is not decoration. It is read by the screen to decide whether a
 * tab gets a working panel or an honest gap, so that a tab can never grow a
 * create button for a capability that does not exist. Changing it to 'available'
 * without the routes existing is the bug this field is here to prevent.
 */

export type ProjectTypeId = 'php' | 'nodejs' | 'proxy' | 'go' | 'python';

export interface ProjectTypeDef {
  id: ProjectTypeId;
  /** Tab label. */
  label: string;
  /** Value of ?type= in the URL. Same as the id, kept explicit so it is stable. */
  slug: string;
  /**
   * 'available'  - the backend manages this type and the panel wires it.
   * 'partial'    - the backend records this type but does not apply it to the host.
   * 'missing'    - no backend at all; the tab shows what would have to be built.
   */
  backend: 'available' | 'partial' | 'missing';
  /** One line under the tab strip saying what this type is. */
  summary: string;
}

export const PROJECT_TYPES: ProjectTypeDef[] = [
  {
    id: 'php',
    label: 'PHP Project',
    slug: 'php',
    backend: 'available',
    summary:
      'Sites served from a document root by a PHP runtime. The panel writes the vhost and can attach a certificate.',
  },
  {
    id: 'nodejs',
    label: 'Node.js Project',
    slug: 'nodejs',
    backend: 'available',
    summary:
      'Long-running Node processes supervised by systemd. The panel installs the unit, starts it and reads its journal.',
  },
  {
    id: 'proxy',
    label: 'Proxy Project',
    slug: 'proxy',
    backend: 'partial',
    summary:
      'A hostname forwarded to an upstream. A proxy has no document root, so this list does not pretend it has one.',
  },
  {
    id: 'go',
    label: 'Go Project',
    slug: 'go',
    backend: 'missing',
    summary: 'A compiled binary run as a service unit.',
  },
  {
    id: 'python',
    label: 'Python Project',
    slug: 'python',
    backend: 'missing',
    summary: 'A WSGI or ASGI application served from a virtual environment.',
  },
];

export const DEFAULT_PROJECT_TYPE: ProjectTypeId = 'php';

/** Reads ?type= into a project type, falling back rather than rendering nothing. */
export function projectTypeFromSlug(value: string | null | undefined): ProjectTypeId {
  const match = PROJECT_TYPES.find((t) => t.slug === String(value || '').toLowerCase());
  return match ? match.id : DEFAULT_PROJECT_TYPE;
}

export function projectType(id: ProjectTypeId): ProjectTypeDef {
  return PROJECT_TYPES.find((t) => t.id === id) || PROJECT_TYPES[0];
}

/**
 * Which values of a website row's `site_type` belong on the PHP tab.
 *
 * `site_type` is a free-form varchar the API stores and never interprets, so
 * rows written before this screen existed carry anything or nothing at all. A
 * row with no type is shown on the PHP tab rather than hidden: a site that
 * exists and appears on no tab is worse than one on the wrong tab.
 */
export const PHP_TAB_SITE_TYPES = ['', 'php', 'static', 'html', 'wordpress'];

/**
 * What each type's backend cannot do today, in the operator's words.
 *
 * Every line was checked against the Go source. They are rendered on the tab
 * they belong to, and they are the backlog this screen hands to whoever picks
 * the work up next.
 */
export const PROJECT_TYPE_GAPS: Record<ProjectTypeId, string[]> = {
  php: [
    'Rewrite rules cannot be edited from the panel. The web server adapters carry a RewriteRule type (core/internal/webserver/adapter.go) but no route exposes it, so there is no endpoint to read or write a site’s rules.',
    'Changing a pool limit records it and does not reach the host. PUT /api/v1/php/pools/:id updates the database row only; the route that rewrites the pool file and reloads PHP-FPM, PUT /api/v1/php/pools/:id/settings, exists in core/internal/handler/php_wordpress_runtime.go but is not mounted in router.go.',
    'Setting a PHP version per site through the dedicated route is unavailable for the same reason: PUT /api/v1/php/sites/:website_id/version is written but not mounted. The panel writes php_version through PUT /api/v1/websites/:id instead, which stores it and regenerates the vhost only on the next SSL change.',
    'The WordPress toolkit is a record, not an installer. POST /api/v1/wordpress creates a row; the route that downloads WordPress and writes wp-config.php, POST /api/v1/wordpress/:id/install, is not mounted, and neither are the live WP-CLI plugin, theme, core, search-replace and staging routes.',
  ],
  nodejs: [
    'There is no entry-file field. The backend stores a working directory and a start command (models.NodeApp.path, .start_script) and nothing between them, so the entry file is whatever the start command names.',
    'There is no package-manager field. npm, pnpm and yarn are not distinguished by the model; whichever one the start command invokes is the one that runs.',
    'The process manager is not a choice. core/internal/nodeapp/systemd.go installs a systemd unit, and that is the only supervisor the backend implements.',
  ],
  proxy: [
    'Creating a proxy records it and does not serve it. ReverseProxyService.Create writes a database row and never touches the web server: no vhost is generated, nothing is reloaded. Until that lands, a proxy created here is a plan, not live traffic.',
    'There are no cache settings. models.ReverseProxy has no cache fields at all, so cache lifetime, cache keys and purge have nowhere to be stored.',
  ],
  go: [
    'There is no Go project resource. No /api/v1/go-apps route group is mounted, there is no GoApp model in core/internal/models, and no service manages one.',
    'The nearest existing piece is the systemd API, /api/v1/services, which can create, start, stop and read the logs of a unit but is restricted to administrators and knows nothing about builds, binaries or working directories.',
  ],
  python: [
    'There is no Python project resource. No /api/v1/python-apps route group is mounted, there is no PythonApp model, and nothing in core knows about interpreters, virtual environments, WSGI or ASGI.',
    'The nearest existing piece is the systemd API, /api/v1/services, which can run a Gunicorn or Uvicorn unit somebody wrote by hand but is restricted to administrators and stores none of the fields this tab would show.',
  ],
};

/** What the Go tab would need built, named so it can become tickets. */
export const GO_REQUIREMENTS = [
  'A go_apps table and model holding the built binary path, the working directory, the listen port and the service unit name.',
  'A /api/v1/go-apps route group with create, list, update, delete, start, stop, restart, status and logs, in the shape /api/v1/node-apps already has.',
  'A build step, or an explicit decision that the panel manages an already-built binary and never compiles.',
];

/** What the Python tab would need built. */
export const PYTHON_REQUIREMENTS = [
  'A python_apps table and model holding the interpreter version, the virtual environment path, the server (Gunicorn, uWSGI, Uvicorn, Hypercorn), and the entry module.',
  'A /api/v1/python-apps route group with the same lifecycle routes /api/v1/node-apps has.',
  'Virtual environment creation and dependency installation, which nothing in core does today.',
];
