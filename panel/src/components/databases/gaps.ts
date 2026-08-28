/**
 * The backlog, written where the operator can read it.
 *
 * Every entry below was checked against the Go source before it was written
 * here, and each names the specific thing that is absent - a route that is not
 * in router.go, a switch arm that is not in the service, a function that
 * returns an error instead of doing the work. That precision is the point: an
 * operator learns whether to wait or to reach for a shell, and whoever picks
 * the item up gets a ticket that is already written.
 *
 * Two findings are worth reading before the lists, because they explain why
 * "backup" is a gap on an engine that appears to have a backup feature:
 *
 *   1. BackupService.backupDatabase (core/internal/service/backup.go) always
 *      shells out to `pg_dump`, and passes `job.ResourceID.String()` - a UUID -
 *      as the database NAME. It never resolves that id to a database, and it
 *      has no mysqldump path at all. A backup job of type "database" therefore
 *      dumps a database named after a UUID, which does not exist.
 *
 *   2. BackupService.RestoreBackup returns the literal error
 *      "restore not yet implemented", and no route in router.go reaches it.
 *
 * So neither engine has a working backup or restore, and this screen does not
 * draw a button for one.
 */

import type { CapabilityGap } from '@/types/databases';

const BACKUP_GAP: CapabilityGap = {
  label: 'Backup',
  missing:
    'BackupService.backupDatabase always runs pg_dump and passes the job’s resource UUID as the database name, so it dumps a database that does not exist. There is no mysqldump path and no route that backs up one named database on demand.',
};

const RESTORE_GAP: CapabilityGap = {
  label: 'Restore',
  missing:
    'BackupService.RestoreBackup returns "restore not yet implemented", and no route in router.go reaches it.',
};

const SIZE_GAP: CapabilityGap = {
  label: 'Size on disk',
  missing:
    'models.DatabaseEntry.size is written as 0 at creation and never updated - there is no UpdateEntrySize in the repository and no collector that measures a database.',
};

const REMOTE_INSTANCE_GAP: CapabilityGap = {
  label: 'Remote instances',
  missing:
    'models.DatabaseServer records only a node id, a type, a version and a port. It has no host, user or password field, so an instance outside the managed nodes cannot be described, let alone reached.',
};

export const MYSQL_GAPS: CapabilityGap[] = [
  BACKUP_GAP,
  RESTORE_GAP,
  {
    label: 'Import a .sql file',
    missing:
      'No upload or import route exists. The only SQL the service ever executes is the CREATE, GRANT, ALTER and DROP it composes itself.',
  },
  {
    label: 'Per-user grants',
    missing:
      'createMySQLDatabase issues GRANT ALL PRIVILEGES on the new database and nothing can narrow it afterwards. There is no route to list, add or revoke a grant, and no way to add a second user to an existing database.',
  },
  {
    label: 'Access host',
    missing:
      'The user is always created as user@’localhost’. There is no field for an access host, so a database cannot be opened to another machine from the panel.',
  },
  {
    label: 'Root password',
    missing:
      'No route changes the MySQL root password. The service runs `mysql -u root` with no credentials, which means it depends on socket authentication - changing root would break it.',
  },
  SIZE_GAP,
  {
    label: 'SQL console and table tools',
    missing:
      'No route runs arbitrary SQL, lists tables, or repairs and optimises them. aaPanel offers phpMyAdmin and a per-table tool box; the panel has no equivalent.',
  },
  REMOTE_INSTANCE_GAP,
];

export const POSTGRES_GAPS: CapabilityGap[] = [
  {
    label: 'Roles beyond the per-database owner',
    missing:
      'The only role the panel ever creates is the one it makes alongside a database. No route runs CREATE ROLE, DROP ROLE or \\du, so a login role shared by several databases cannot be managed here.',
  },
  {
    label: 'Role attributes',
    missing:
      'No route reads or sets SUPERUSER, CREATEDB, CREATEROLE, LOGIN, CONNECTION LIMIT or VALID UNTIL. Every role the panel creates is a plain LOGIN role with no attributes.',
  },
  {
    label: 'Extensions',
    missing:
      'No route runs CREATE EXTENSION or reads pg_extension, so the panel cannot say whether postgis, pgcrypto or uuid-ossp is installed in a database, let alone install one.',
  },
  {
    label: 'Schemas',
    missing:
      'No route reads information_schema.schemata or runs CREATE SCHEMA. Every database created here has only the default public schema.',
  },
  {
    label: 'Encoding and collation',
    missing:
      'createPostgresDatabase calls `createdb` with no --encoding, --lc-collate or --lc-ctype, so a database inherits the template. Worse, CreateDatabase then stores the MySQL defaults utf8mb4 / utf8mb4_unicode_ci on the row regardless of engine, which is why this pane does not show them as if they were PostgreSQL settings.',
  },
  BACKUP_GAP,
  RESTORE_GAP,
  SIZE_GAP,
  REMOTE_INSTANCE_GAP,
];

export const SQLSERVER_WOULD_SHOW = [
  'Databases with their owner, recovery model and size',
  'Server logins and the database users mapped to them',
  'Recovery model per database - FULL, SIMPLE or BULK_LOGGED - and the log growth that follows from it',
  'Native .bak backups, the format SQL Server operators actually restore from',
  'Collation per database, which SQL Server sets per database rather than per server',
];

export const SQLSERVER_GAPS: CapabilityGap[] = [
  {
    label: 'Create or drop a database',
    missing:
      'DatabaseService.CreateDatabase has arms for "mysql" and "postgresql" only; a sqlserver instance falls to the default and returns "unsupported database type".',
  },
  {
    label: 'Logins and database users',
    missing:
      'No route runs CREATE LOGIN or CREATE USER. SQL Server separates a server login from a database user, and the panel models neither.',
  },
  {
    label: 'Recovery model',
    missing:
      'No route reads or sets ALTER DATABASE ... SET RECOVERY. Nothing in the panel can tell an operator that a database is in FULL recovery with a log nobody is truncating.',
  },
  {
    label: 'Native .bak backup and restore',
    missing:
      'BackupService knows only pg_dump. It has no BACKUP DATABASE ... TO DISK path, so the format SQL Server restores from is not produced anywhere.',
  },
  {
    label: 'A client to talk to it',
    missing:
      'The service shells out to `mysql` and `psql`. There is no sqlcmd or go-mssqldb path, so even with a route there is nothing to execute a statement with.',
  },
  REMOTE_INSTANCE_GAP,
];

export const MONGODB_WOULD_SHOW = [
  'Databases with their collection count and storage size',
  'Collections inside a database, with document counts and index sizes',
  'Users with the roles they hold, per database - readWrite, dbAdmin, clusterMonitor',
  'A connection string carrying the right authSource, which is the detail that breaks most first connections',
  'Whether the instance is standalone or a replica set member',
];

export const MONGODB_GAPS: CapabilityGap[] = [
  {
    label: 'Create or drop a database',
    missing:
      'DatabaseService.CreateDatabase has arms for "mysql" and "postgresql" only; a mongodb instance falls to the default and returns "unsupported database type".',
  },
  {
    label: 'Collections',
    missing:
      'No route lists, creates or drops a collection. MongoDB has no schema to inspect from SQL, and the panel has no driver to ask.',
  },
  {
    label: 'Users and their roles',
    missing:
      'No route runs db.createUser or db.getUsers. A MongoDB user is a set of role grants scoped to a database, which the panel’s single username-and-password model cannot represent.',
  },
  {
    label: 'Connection string with credentials',
    missing:
      'The address below is derived from the node record. A usable string also needs the authSource and the user, which depend on the user management that does not exist yet.',
  },
  {
    label: 'A client to talk to it',
    missing:
      'The service shells out to `mysql` and `psql`. There is no mongosh or driver path, so even with a route there is nothing to execute a command with.',
  },
  REMOTE_INSTANCE_GAP,
];
