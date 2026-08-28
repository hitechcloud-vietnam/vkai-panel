/**
 * The database screen's vocabulary, and - more importantly - the record of what
 * the backend can and cannot actually do.
 *
 * Everything here was read out of the Go source, not assumed:
 *
 *   core/internal/handler/router.go      lines 362-373, the eight routes that exist
 *   core/internal/handler/database.go    the handlers behind them
 *   core/internal/service/database.go    CreateDatabase's switch on server type
 *   core/internal/models/models.go       DatabaseServer / DatabaseEntry
 *
 * The switch in DatabaseService.CreateDatabase has exactly two arms, "mysql"
 * and "postgresql", and a default that returns `unsupported database type`.
 * That single fact decides which tabs on this screen may offer a control and
 * which must say so plainly. A tab for an engine the service cannot manage,
 * wired to a button that returns a 500, is the defect this file exists to stop
 * being repeated.
 */

/** The engines the screen has a tab for, in aaPanel's own order. */
export type DatabaseEngineId =
  | 'mysql'
  | 'sqlserver'
  | 'mongodb'
  | 'redis'
  | 'postgresql';

/**
 * How much of an engine the backend can actually drive.
 *
 * `managed`    the service creates, deletes and re-passwords real databases.
 * `registry`   database SERVER rows can be registered and deleted, and nothing
 *              more - POST /api/v1/databases against such a server returns
 *              "unsupported database type".
 */
export type EngineSupport = 'managed' | 'registry';

/** One capability the interface would like to offer, and the reason it cannot. */
export interface CapabilityGap {
  /** What an operator would call it. */
  label: string;
  /** What is missing, named precisely enough to become a backlog item. */
  missing: string;
}

export interface DatabaseEngine {
  id: DatabaseEngineId;
  /** Tab label. */
  label: string;
  /** One line under the pane heading. */
  blurb: string;
  /**
   * Values of DatabaseServer.type that belong to this tab. The Go model's
   * comment lists "mariadb, mysql, postgresql"; MariaDB shares the MySQL pane
   * because the service drives both through the `mysql` client.
   */
  serverTypes: string[];
  /** The type written when registering a new server from this pane. */
  defaultServerType: string;
  defaultPort: number;
  support: EngineSupport;
  /** What the operator's own word for a database account is on this engine. */
  accountNoun: string;
}

/**
 * MySQL and PostgreSQL are managed. The other three are registry-only, and
 * every pane below says so in its own words rather than pretending otherwise.
 */
export const DATABASE_ENGINES: readonly DatabaseEngine[] = [
  {
    id: 'mysql',
    label: 'MySQL / MariaDB',
    blurb: 'Databases created through the panel, each with its own user and grant.',
    serverTypes: ['mysql', 'mariadb'],
    defaultServerType: 'mysql',
    defaultPort: 3306,
    support: 'managed',
    accountNoun: 'User',
  },
  {
    id: 'sqlserver',
    label: 'SQL Server',
    blurb: 'Registered instances only. The panel cannot create databases here yet.',
    serverTypes: ['sqlserver', 'mssql'],
    defaultServerType: 'sqlserver',
    defaultPort: 1433,
    support: 'registry',
    accountNoun: 'Login',
  },
  {
    id: 'mongodb',
    label: 'MongoDB',
    blurb: 'Registered instances only. The panel cannot create databases here yet.',
    serverTypes: ['mongodb', 'mongo'],
    defaultServerType: 'mongodb',
    defaultPort: 27017,
    support: 'registry',
    accountNoun: 'User',
  },
  {
    id: 'redis',
    label: 'Redis',
    blurb: 'A key-value store, not a set of SQL databases. Registered instances only.',
    serverTypes: ['redis'],
    defaultServerType: 'redis',
    defaultPort: 6379,
    support: 'registry',
    accountNoun: 'ACL user',
  },
  {
    id: 'postgresql',
    label: 'PostgreSQL',
    blurb: 'Databases created through the panel, each owned by a role of the same name.',
    serverTypes: ['postgresql', 'postgres', 'pgsql'],
    defaultServerType: 'postgresql',
    defaultPort: 5432,
    support: 'managed',
    accountNoun: 'Role',
  },
];

export const DEFAULT_ENGINE: DatabaseEngineId = 'mysql';

/** Reads an engine id out of a URL value, falling back to MySQL. */
export function parseEngineId(value: string | null | undefined): DatabaseEngineId {
  const wanted = String(value || '').trim().toLowerCase();
  const found = DATABASE_ENGINES.find(
    (engine) => engine.id === wanted || engine.serverTypes.includes(wanted)
  );
  return found ? found.id : DEFAULT_ENGINE;
}

export function engineById(id: DatabaseEngineId): DatabaseEngine {
  return DATABASE_ENGINES.find((engine) => engine.id === id) || DATABASE_ENGINES[0];
}

/** Which tab a DatabaseServer row belongs to; null when its type matches none. */
export function engineForServerType(type: string | null | undefined): DatabaseEngine | null {
  const wanted = String(type || '').trim().toLowerCase();
  if (!wanted) return null;
  return DATABASE_ENGINES.find((engine) => engine.serverTypes.includes(wanted)) || null;
}

/**
 * A database server as GET /api/v1/databases/servers returns it.
 * Mirrors models.DatabaseServer.
 */
export interface DBServer {
  id: string;
  tenant_id?: string;
  /** The node this instance runs on - a row in GET /api/v1/servers. */
  server_id: string;
  type: string;
  version?: string;
  port?: number;
  status?: string;
  created_at?: string;
  updated_at?: string;
}

/**
 * A database as GET /api/v1/databases returns it. Mirrors models.DatabaseEntry.
 *
 * There is no `password` field, and its absence is deliberate on the Go side:
 * the struct tag is `json:"-"`. Nothing the API returns can reveal a stored
 * password, so nothing in this UI claims to.
 *
 * `size` is on the struct and in the table, but no code path ever writes it -
 * there is no UpdateEntrySize anywhere in the repository. It is always 0, which
 * is why the size column renders as unavailable rather than as "0 B".
 */
export interface DBEntry {
  id: string;
  tenant_id?: string;
  database_server_id: string;
  name: string;
  username: string;
  charset?: string;
  collation?: string;
  size?: number;
  status?: string;
  created_at?: string;
  updated_at?: string;
}

/** The form POST /api/v1/databases accepts. */
export interface CreateDatabasePayload {
  database_server_id: string;
  name: string;
  username: string;
  password: string;
  charset?: string;
  collation?: string;
}

/**
 * Charsets and collations the service will accept. Anything outside these two
 * allowlists is refused in validateDBRequest, so the form offers exactly them
 * and no free text.
 */
export const ALLOWED_CHARSETS = [
  'utf8mb4',
  'utf8',
  'utf8mb3',
  'latin1',
  'ascii',
] as const;

export const ALLOWED_COLLATIONS = [
  'utf8mb4_unicode_ci',
  'utf8mb4_general_ci',
  'utf8mb4_0900_ai_ci',
  'utf8mb4_bin',
  'utf8_general_ci',
  'utf8_unicode_ci',
  'latin1_swedish_ci',
  'ascii_general_ci',
] as const;

/** The password rule from CreateDBEntryRequest: `min=12,max=128`. */
export const PASSWORD_MIN_LENGTH = 12;
export const PASSWORD_MAX_LENGTH = 128;

/** The identifier rule from CreateDBEntryRequest: `max=63`, plus the service's charset. */
export const IDENTIFIER_MAX_LENGTH = 63;
export const IDENTIFIER_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/;
