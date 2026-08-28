'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  Database,
  Plus,
  Trash2,
  Key,
  Server,
  Users,
  User,
  Shield,
  RefreshCw,
  X,
  Search,
  Loader2,
  AlertTriangle,
  HardDrive,
} from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { api, databaseApi, serverApi, unwrapList } from '@/services/api';
import ServerScopeField, {
  SERVER_SCOPE_COPY_EN,
} from '@/components/servers/ServerScopeField';
import { defaultServerId, isLocalNode, serverLabel } from '@/lib/servers';
import type { ManagedServer } from '@/types/server';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface DBServer {
  id: string;
  tenant_id: string;
  server_id: string;
  type: string;
  version: string;
  port: number;
  status: string;
  created_at: string;
  updated_at: string;
}

interface DBEntry {
  id: string;
  tenant_id: string;
  database_server_id: string;
  name: string;
  username: string;
  charset: string;
  collation: string;
  size: number;
  status: string;
  created_at: string;
  updated_at: string;
}

interface DBUser {
  id: string;
  username: string;
  database_server_id: string;
  database_name: string;
  database_id: string;
  created_at: string;
}

interface ServerFormData {
  server_id: string;
  type: string;
  version: string;
  port: string;
}

interface DatabaseFormData {
  database_server_id: string;
  name: string;
  username: string;
  password: string;
  charset: string;
  collation: string;
}

interface PasswordFormData {
  password: string;
  confirmPassword: string;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const EMPTY_SERVER_FORM: ServerFormData = {
  server_id: '',
  type: 'mysql',
  version: '',
  port: '3306',
};

const EMPTY_DATABASE_FORM: DatabaseFormData = {
  database_server_id: '',
  name: '',
  username: '',
  password: '',
  charset: 'utf8mb4',
  collation: 'utf8mb4_unicode_ci',
};

const EMPTY_PASSWORD_FORM: PasswordFormData = {
  password: '',
  confirmPassword: '',
};

const DB_TYPES = [
  { value: 'mysql', label: 'MySQL', defaultPort: '3306' },
  { value: 'postgresql', label: 'PostgreSQL', defaultPort: '5432' },
  { value: 'redis', label: 'Redis', defaultPort: '6379' },
];

const INPUT_CLASS =
  'border-gray-300 bg-white text-gray-900 placeholder:text-gray-400 focus-visible:ring-1 focus-visible:ring-brand-500 focus-visible:ring-offset-0';
const SELECT_TRIGGER_CLASS =
  'border-gray-300 bg-white text-gray-900 focus:ring-1 focus:ring-brand-500 focus:ring-offset-0';
const PRIMARY_BUTTON_CLASS =
  'bg-brand-600 text-white hover:bg-brand-700 focus-visible:ring-brand-500';
const SECONDARY_BUTTON_CLASS =
  'border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus-visible:ring-brand-500';
const ICON_BUTTON_CLASS =
  'h-8 w-8 p-0 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:ring-brand-500';
const DANGER_ICON_BUTTON_CLASS =
  'h-8 w-8 p-0 text-red-600 hover:bg-red-50 hover:text-red-700 focus-visible:ring-red-500';
const TH_CLASS =
  'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
const TABS_TRIGGER_CLASS =
  'text-gray-600 data-[state=active]:bg-brand-50 data-[state=active]:text-brand-700 data-[state=active]:shadow-none focus-visible:ring-brand-500';

/**
 * How a database server reads in a picker: the engine, the node it runs on, and
 * its port.
 */
function dbServerOptionLabel(
  server: DBServer,
  infraName: (serverId: string) => string
): string {
  return `${(server.type || '').toUpperCase()} – ${infraName(server.server_id)} (${server.port})`;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function DatabasesPage() {
  // Data
  const [servers, setServers] = useState<DBServer[]>([]);
  const [databases, setDatabases] = useState<DBEntry[]>([]);
  const [infraServers, setInfraServers] = useState<ManagedServer[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Search
  const [serverSearch, setServerSearch] = useState('');
  const [databaseSearch, setDatabaseSearch] = useState('');
  const [userSearch, setUserSearch] = useState('');

  // Modal state
  const [showServerForm, setShowServerForm] = useState(false);
  const [showDatabaseForm, setShowDatabaseForm] = useState(false);
  const [showUserForm, setShowUserForm] = useState(false);
  const [showPasswordForm, setShowPasswordForm] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  // Form state
  const [serverForm, setServerForm] = useState<ServerFormData>(EMPTY_SERVER_FORM);
  const [databaseForm, setDatabaseForm] = useState<DatabaseFormData>(EMPTY_DATABASE_FORM);
  const [passwordForm, setPasswordForm] = useState<PasswordFormData>(EMPTY_PASSWORD_FORM);
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  // Delete state
  const [deleteTarget, setDeleteTarget] = useState<{
    type: 'server' | 'database';
    id: string;
    name: string;
  } | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Password change target
  const [passwordTarget, setPasswordTarget] = useState<{ id: string; name: string } | null>(null);

  // Toast
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // -------------------------------------------------------------------------
  // Data fetching
  // -------------------------------------------------------------------------

  const fetchAll = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const [serversRes, databasesRes, infraRes] = await Promise.all([
        api.get('/api/v1/databases/servers'),
        databaseApi.list(),
        serverApi.list({ page: 1, per_page: 200 }),
      ]);
      setServers(unwrapList<DBServer>(serversRes));
      setDatabases(unwrapList<DBEntry>(databasesRes));
      // GET /api/v1/servers is paginated: read it through unwrapList or the
      // machine the panel runs on never reaches the picker below.
      setInfraServers(unwrapList<ManagedServer>(infraRes));
    } catch (err: any) {
      console.error('Failed to load database data:', err);
      setServers([]);
      setDatabases([]);
      setInfraServers([]);
      setError(err?.response?.data?.message || 'Failed to load data');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  // Auto-dismiss toast
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 4000);
    return () => clearTimeout(t);
  }, [toast]);

  // -------------------------------------------------------------------------
  // Derived data
  // -------------------------------------------------------------------------

  const safeServers: DBServer[] = Array.isArray(servers) ? servers : [];
  const safeDatabases: DBEntry[] = Array.isArray(databases) ? databases : [];
  const safeInfraServers: ManagedServer[] = Array.isArray(infraServers)
    ? infraServers
    : [];

  const users: DBUser[] = safeDatabases.map((db) => ({
    id: db?.id,
    username: db?.username,
    database_server_id: db?.database_server_id,
    database_name: db?.name,
    database_id: db?.id,
    created_at: db?.created_at,
  }));

  /** Database servers a new database can actually be created on. */
  const activeDBServers = safeServers.filter((s) => s?.status === 'active');

  /** The machine this panel runs on, when it is among the managed nodes. */
  const localInfraNode = safeInfraServers.find((s) => isLocalNode(s)) || null;

  const getInfraServerName = (serverId: string) => {
    if (!serverId) return '—';
    const server = safeInfraServers.find((s) => s?.id === serverId);
    // Nodes have no display name - the hostname is the name. See lib/servers.
    return server ? serverLabel(server) : String(serverId).slice(0, 8) + '…';
  };

  const getServerDisplayName = (dbServerId: string) => {
    if (!dbServerId) return '—';
    const dbServer = safeServers.find((s) => s?.id === dbServerId);
    if (!dbServer) return String(dbServerId).slice(0, 8) + '…';
    const infra = safeInfraServers.find((s) => s?.id === dbServer.server_id);
    return infra
      ? serverLabel(infra)
      : (dbServer.type || 'db') + ' (' + String(dbServerId).slice(0, 8) + ')';
  };

  const formatBytes = (bytes: number) => {
    if (!bytes || bytes <= 0 || !Number.isFinite(bytes)) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const idx = Math.floor(Math.log(bytes) / Math.log(k));
    const i = Math.min(Math.max(idx, 0), sizes.length - 1);
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  };

  // Deterministic formatting (avoids server/client hydration drift)
  const formatDate = (dateStr?: string | null) => {
    if (!dateStr) return '—';
    const d = new Date(dateStr);
    if (Number.isNaN(d.getTime())) return '—';
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${pad(d.getDate())}/${pad(d.getMonth() + 1)}/${d.getFullYear()}`;
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'active':
        return (
          <Badge
            variant="success"
            className="rounded-md border-emerald-200 bg-emerald-50 px-2 py-0.5 font-medium text-emerald-700 hover:bg-emerald-50"
          >
            {status}
          </Badge>
        );
      case 'inactive':
      case 'error':
        return (
          <Badge
            variant="destructive"
            className="rounded-md border-red-200 bg-red-50 px-2 py-0.5 font-medium text-red-700 hover:bg-red-50"
          >
            {status}
          </Badge>
        );
      default:
        return (
          <Badge
            variant="warning"
            className="rounded-md border-amber-200 bg-amber-50 px-2 py-0.5 font-medium text-amber-700 hover:bg-amber-50"
          >
            {status || 'unknown'}
          </Badge>
        );
    }
  };

  const getDbTypeIcon = (type: string) => {
    switch (type) {
      case 'mysql':
      case 'mariadb':
        return <Database className="text-brand-600" size={16} />;
      case 'postgresql':
        return <Database className="text-sky-600" size={16} />;
      case 'redis':
        return <HardDrive className="text-red-600" size={16} />;
      default:
        return <Database className="text-gray-400" size={16} />;
    }
  };

  // -------------------------------------------------------------------------
  // Filtered data
  // -------------------------------------------------------------------------

  const filteredServers = safeServers.filter((s) => {
    if (!s) return false;
    if (!serverSearch) return true;
    const q = serverSearch.toLowerCase();
    return (
      (s.type || '').toLowerCase().includes(q) ||
      (s.version || '').toLowerCase().includes(q) ||
      (s.status || '').toLowerCase().includes(q) ||
      (s.id || '').toLowerCase().includes(q)
    );
  });

  const filteredDatabases = safeDatabases.filter((d) => {
    if (!d) return false;
    if (!databaseSearch) return true;
    const q = databaseSearch.toLowerCase();
    return (
      (d.name || '').toLowerCase().includes(q) ||
      (d.username || '').toLowerCase().includes(q) ||
      (d.charset || '').toLowerCase().includes(q)
    );
  });

  const filteredUsers = users.filter((u) => {
    if (!u) return false;
    if (!userSearch) return true;
    const q = userSearch.toLowerCase();
    return (
      (u.username || '').toLowerCase().includes(q) ||
      (u.database_name || '').toLowerCase().includes(q)
    );
  });

  // -------------------------------------------------------------------------
  // Server CRUD
  // -------------------------------------------------------------------------

  const handleCreateServer = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);
    if (!serverForm.server_id) {
      setFormError('Infrastructure server is required');
      return;
    }
    setSubmitting(true);
    try {
      await api.post('/api/v1/databases/servers', {
        server_id: serverForm.server_id,
        type: serverForm.type,
        version: serverForm.version,
        port: parseInt(serverForm.port) || 0,
      });
      setToast({ type: 'success', message: 'Database server added successfully' });
      setShowServerForm(false);
      setServerForm(EMPTY_SERVER_FORM);
      fetchAll();
    } catch (err: any) {
      setFormError(err?.response?.data?.message || 'Failed to add server');
    } finally {
      setSubmitting(false);
    }
  };

  // -------------------------------------------------------------------------
  // Database CRUD
  // -------------------------------------------------------------------------

  const handleCreateDatabase = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);
    if (!databaseForm.database_server_id || !databaseForm.name || !databaseForm.username || !databaseForm.password) {
      setFormError('Server, name, username, and password are required');
      return;
    }
    setSubmitting(true);
    try {
      await databaseApi.create({
        database_server_id: databaseForm.database_server_id,
        name: databaseForm.name,
        username: databaseForm.username,
        password: databaseForm.password,
        charset: databaseForm.charset || 'utf8mb4',
        collation: databaseForm.collation || 'utf8mb4_unicode_ci',
      });
      setToast({ type: 'success', message: 'Database created successfully' });
      setShowDatabaseForm(false);
      setDatabaseForm(EMPTY_DATABASE_FORM);
      fetchAll();
    } catch (err: any) {
      setFormError(err?.response?.data?.message || 'Failed to create database');
    } finally {
      setSubmitting(false);
    }
  };

  // -------------------------------------------------------------------------
  // User CRUD (creates a database entry which also creates the DB user)
  // -------------------------------------------------------------------------

  const handleCreateUser = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);
    if (!databaseForm.database_server_id || !databaseForm.name || !databaseForm.username || !databaseForm.password) {
      setFormError('Server, database name, username, and password are required');
      return;
    }
    setSubmitting(true);
    try {
      await databaseApi.create({
        database_server_id: databaseForm.database_server_id,
        name: databaseForm.name,
        username: databaseForm.username,
        password: databaseForm.password,
        charset: databaseForm.charset || 'utf8mb4',
        collation: databaseForm.collation || 'utf8mb4_unicode_ci',
      });
      setToast({ type: 'success', message: 'Database user created successfully' });
      setShowUserForm(false);
      setDatabaseForm(EMPTY_DATABASE_FORM);
      fetchAll();
    } catch (err: any) {
      setFormError(err?.response?.data?.message || 'Failed to create user');
    } finally {
      setSubmitting(false);
    }
  };

  // -------------------------------------------------------------------------
  // Password change
  // -------------------------------------------------------------------------

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);
    if (!passwordForm.password) {
      setFormError('Password is required');
      return;
    }
    if (passwordForm.password !== passwordForm.confirmPassword) {
      setFormError('Passwords do not match');
      return;
    }
    setSubmitting(true);
    try {
      await api.post(`/api/v1/databases/${passwordTarget?.id}/change-password`, {
        password: passwordForm.password,
      });
      setToast({ type: 'success', message: 'Password changed successfully' });
      setShowPasswordForm(false);
      setPasswordTarget(null);
      setPasswordForm(EMPTY_PASSWORD_FORM);
    } catch (err: any) {
      setFormError(err?.response?.data?.message || 'Failed to change password');
    } finally {
      setSubmitting(false);
    }
  };

  // -------------------------------------------------------------------------
  // Delete
  // -------------------------------------------------------------------------

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      if (deleteTarget.type === 'server') {
        await api.delete(`/api/v1/databases/servers/${deleteTarget.id}`);
      } else {
        await databaseApi.delete(deleteTarget.id);
      }
      setToast({ type: 'success', message: `${deleteTarget.type === 'server' ? 'Server' : 'Database'} deleted` });
      setShowDeleteConfirm(false);
      setDeleteTarget(null);
      fetchAll();
    } catch (err: any) {
      setToast({
        type: 'error',
        message: err?.response?.data?.message || `Failed to delete ${deleteTarget.type}`,
      });
    } finally {
      setDeleting(false);
    }
  };

  const openDeleteConfirm = (type: 'server' | 'database', id: string, name: string) => {
    setDeleteTarget({ type, id, name });
    setShowDeleteConfirm(true);
  };

  const openPasswordForm = (id: string, name: string) => {
    setPasswordTarget({ id, name });
    setPasswordForm(EMPTY_PASSWORD_FORM);
    setFormError(null);
    setShowPasswordForm(true);
  };

  // -------------------------------------------------------------------------
  // Loading / Error states
  // -------------------------------------------------------------------------

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="animate-spin text-brand-600" size={24} aria-hidden="true" />
        <span className="sr-only">Loading databases</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Databases</h1>
          <p className="mt-1 text-sm text-gray-600">
            Manage database servers, databases, and users
          </p>
        </div>
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-4 text-red-700">
          <div className="flex items-start gap-3">
            <AlertTriangle size={18} className="mt-0.5" />
            <div>
              <p className="text-sm font-medium">{error}</p>
              <Button
                onClick={fetchAll}
                variant="outline"
                size="sm"
                className="mt-3 border-red-300 bg-white text-red-700 hover:bg-red-50 focus-visible:ring-red-500"
              >
                <RefreshCw size={16} className="mr-2" />
                Retry
              </Button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Databases</h1>
          <p className="mt-1 text-sm text-gray-600">
            Manage database servers, databases, and users
          </p>
        </div>
        <Button
          onClick={fetchAll}
          variant="outline"
          size="sm"
          className={SECONDARY_BUTTON_CLASS}
        >
          <RefreshCw size={16} className="mr-2" />
          Refresh
        </Button>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <Card className="border-gray-200 bg-white shadow-sm">
          <CardContent className="p-5">
            <div className="flex items-center gap-3">
              <div className="rounded-md border border-brand-200 bg-brand-50 p-2">
                <Server className="text-brand-600" size={18} />
              </div>
              <div>
                <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                  Database Servers
                </p>
                <p className="mt-1 text-2xl font-semibold text-gray-900">
                  {safeServers.length}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="border-gray-200 bg-white shadow-sm">
          <CardContent className="p-5">
            <div className="flex items-center gap-3">
              <div className="rounded-md border border-emerald-200 bg-emerald-50 p-2">
                <Database className="text-emerald-600" size={18} />
              </div>
              <div>
                <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                  Databases
                </p>
                <p className="mt-1 text-2xl font-semibold text-gray-900">
                  {safeDatabases.length}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="border-gray-200 bg-white shadow-sm">
          <CardContent className="p-5">
            <div className="flex items-center gap-3">
              <div className="rounded-md border border-sky-200 bg-sky-50 p-2">
                <Users className="text-sky-600" size={18} />
              </div>
              <div>
                <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                  Database Users
                </p>
                <p className="mt-1 text-2xl font-semibold text-gray-900">{users.length}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="servers">
        <TabsList className="border border-gray-200 bg-white p-1">
          <TabsTrigger value="servers" className={TABS_TRIGGER_CLASS}>
            <Server size={16} className="mr-2" />
            Database Servers
          </TabsTrigger>
          <TabsTrigger value="databases" className={TABS_TRIGGER_CLASS}>
            <Database size={16} className="mr-2" />
            Databases
          </TabsTrigger>
          <TabsTrigger value="users" className={TABS_TRIGGER_CLASS}>
            <Users size={16} className="mr-2" />
            Users
          </TabsTrigger>
        </TabsList>

        {/* ================================================================= */}
        {/* Database Servers Tab                                               */}
        {/* ================================================================= */}
        <TabsContent value="servers" className="mt-4">
          <Card className="border-gray-200 bg-white shadow-sm">
            <CardHeader className="flex flex-row items-center justify-between space-y-0 border-b border-gray-200 px-5 py-4">
              <CardTitle className="text-sm font-semibold text-gray-900">
                Database Servers
              </CardTitle>
              <div className="flex items-center gap-3">
                <div className="relative">
                  <Search
                    className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
                    size={16}
                  />
                  <Input
                    aria-label="Search database servers"
                    placeholder="Search servers..."
                    value={serverSearch}
                    onChange={(e) => setServerSearch(e.target.value)}
                    className={`w-64 pl-10 ${INPUT_CLASS}`}
                  />
                </div>
                <Button
                  onClick={() => {
                    // The node is answered before the dialog opens: on a
                    // single-node panel it is the machine the panel runs on.
                    setServerForm({
                      ...EMPTY_SERVER_FORM,
                      server_id: defaultServerId(safeInfraServers),
                    });
                    setFormError(null);
                    setShowServerForm(true);
                  }}
                  className={PRIMARY_BUTTON_CLASS}
                >
                  <Plus size={16} className="mr-2" />
                  Add Server
                </Button>
              </div>
            </CardHeader>
            <CardContent className="p-0">
              {filteredServers.length === 0 ? (
                <div className="px-5 py-12 text-center">
                  <Server className="mx-auto text-gray-300" size={40} />
                  <h3 className="mt-3 text-sm font-semibold text-gray-900">
                    No database servers
                  </h3>
                  {/*
                    A fresh install has a node - the machine the panel runs on -
                    so the useful thing to say is that MySQL or PostgreSQL can
                    be registered on it right here, not that the list is empty.
                  */}
                  <p className="mx-auto mt-1 max-w-lg text-sm text-gray-600">
                    {serverSearch ? (
                      'No servers match your search'
                    ) : localInfraNode ? (
                      <>
                        Register the MySQL or PostgreSQL running on{' '}
                        <span className="font-mono">{serverLabel(localInfraNode)}</span>, the
                        machine this panel runs on, and your websites can start using it.
                      </>
                    ) : (
                      'Register the database engine running on one of your machines to get started.'
                    )}
                  </p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead className="border-b border-gray-200 bg-gray-50">
                      <tr>
                        <th className={TH_CLASS}>Name</th>
                        <th className={TH_CLASS}>Type</th>
                        <th className={TH_CLASS}>Host</th>
                        <th className={TH_CLASS}>Port</th>
                        <th className={TH_CLASS}>Status</th>
                        <th className={TH_CLASS}>Version</th>
                        <th className={TH_CLASS}>Created</th>
                        <th className={`${TH_CLASS} text-right`}>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredServers.map((server) => {
                        const infra = safeInfraServers.find(
                          (s) => s?.id === server.server_id,
                        );
                        return (
                          <tr
                            key={server.id}
                            className="border-b border-gray-100 hover:bg-gray-50"
                          >
                            <td className="px-4 py-3">
                              <div className="flex items-center gap-2">
                                {getDbTypeIcon(server.type)}
                                <span className="flex flex-wrap items-center gap-2 text-sm font-medium text-gray-900">
                                  {infra ? serverLabel(infra) : 'Unknown'}
                                  {isLocalNode(infra) && (
                                    <span
                                      title="The machine this panel runs on."
                                      className="rounded-md border border-brand-200 bg-brand-50 px-1.5 py-0.5 text-[10px] font-medium text-brand-700"
                                    >
                                      Panel host
                                    </span>
                                  )}
                                </span>
                              </div>
                            </td>
                            <td className="px-4 py-3">
                              <span className="inline-flex items-center rounded-md border border-gray-200 bg-gray-100 px-2 py-0.5 text-xs font-medium capitalize text-gray-700">
                                {server.type || 'unknown'}
                              </span>
                            </td>
                            <td className="px-4 py-3 font-mono text-sm text-gray-700">
                              {infra?.ip_address || infra?.hostname || '—'}
                            </td>
                            <td className="px-4 py-3 font-mono text-sm text-gray-700">
                              {server.port || '—'}
                            </td>
                            <td className="px-4 py-3">{getStatusBadge(server.status)}</td>
                            <td className="px-4 py-3 text-sm text-gray-700">
                              {server.version || '—'}
                            </td>
                            <td className="px-4 py-3 text-sm text-gray-600">
                              {formatDate(server.created_at)}
                            </td>
                            <td className="px-4 py-3 text-right">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() =>
                                  openDeleteConfirm(
                                    'server',
                                    server.id,
                                    infra ? serverLabel(infra) : server.type,
                                  )
                                }
                                className={DANGER_ICON_BUTTON_CLASS}
                                aria-label={`Delete server ${infra ? serverLabel(infra) : server.type}`}
                                title="Delete server"
                              >
                                <Trash2 size={16} />
                              </Button>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* ================================================================= */}
        {/* Databases Tab                                                      */}
        {/* ================================================================= */}
        <TabsContent value="databases" className="mt-4">
          <Card className="border-gray-200 bg-white shadow-sm">
            <CardHeader className="flex flex-row items-center justify-between space-y-0 border-b border-gray-200 px-5 py-4">
              <CardTitle className="text-sm font-semibold text-gray-900">Databases</CardTitle>
              <div className="flex items-center gap-3">
                <div className="relative">
                  <Search
                    className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
                    size={16}
                  />
                  <Input
                    aria-label="Search databases"
                    placeholder="Search databases..."
                    value={databaseSearch}
                    onChange={(e) => setDatabaseSearch(e.target.value)}
                    className={`w-64 pl-10 ${INPUT_CLASS}`}
                  />
                </div>
                <Button
                  onClick={() => {
                    // One active database server means one answer; the picker
                    // in the dialog only appears when there is a choice.
                    const active = safeServers.filter((srv) => srv?.status === 'active');
                    setDatabaseForm({
                      ...EMPTY_DATABASE_FORM,
                      database_server_id: active.length === 1 ? active[0].id : '',
                    });
                    setFormError(null);
                    setShowDatabaseForm(true);
                  }}
                  className={PRIMARY_BUTTON_CLASS}
                >
                  <Plus size={16} className="mr-2" />
                  Create Database
                </Button>
              </div>
            </CardHeader>
            <CardContent className="p-0">
              {filteredDatabases.length === 0 ? (
                <div className="px-5 py-12 text-center">
                  <Database className="mx-auto text-gray-300" size={40} />
                  <h3 className="mt-3 text-sm font-semibold text-gray-900">No databases</h3>
                  <p className="mt-1 text-sm text-gray-600">
                    {databaseSearch
                      ? 'No databases match your search'
                      : 'Create your first database to get started'}
                  </p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead className="border-b border-gray-200 bg-gray-50">
                      <tr>
                        <th className={TH_CLASS}>Name</th>
                        <th className={TH_CLASS}>Server</th>
                        <th className={TH_CLASS}>Size</th>
                        <th className={TH_CLASS}>Charset</th>
                        <th className={TH_CLASS}>Status</th>
                        <th className={TH_CLASS}>Created</th>
                        <th className={`${TH_CLASS} text-right`}>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredDatabases.map((db) => (
                        <tr
                          key={db.id}
                          className="border-b border-gray-100 hover:bg-gray-50"
                        >
                          <td className="px-4 py-3">
                            <div className="flex items-center gap-2">
                              <Database className="text-brand-600" size={16} />
                              <span className="text-sm font-medium text-gray-900">
                                {db.name}
                              </span>
                            </div>
                          </td>
                          <td className="px-4 py-3 text-sm text-gray-700">
                            {getServerDisplayName(db.database_server_id)}
                          </td>
                          <td className="px-4 py-3 text-sm text-gray-700">
                            {formatBytes(db.size)}
                          </td>
                          <td className="px-4 py-3 font-mono text-sm text-gray-700">
                            {db.charset || '—'}
                          </td>
                          <td className="px-4 py-3">{getStatusBadge(db.status)}</td>
                          <td className="px-4 py-3 text-sm text-gray-600">
                            {formatDate(db.created_at)}
                          </td>
                          <td className="px-4 py-3 text-right">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => openPasswordForm(db.id, db.name)}
                                className={ICON_BUTTON_CLASS}
                                aria-label={`Change password for ${db.name}`}
                                title="Change password"
                              >
                                <Key size={16} />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() =>
                                  openDeleteConfirm('database', db.id, db.name)
                                }
                                className={DANGER_ICON_BUTTON_CLASS}
                                aria-label={`Delete database ${db.name}`}
                                title="Delete database"
                              >
                                <Trash2 size={16} />
                              </Button>
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* ================================================================= */}
        {/* Users Tab                                                          */}
        {/* ================================================================= */}
        <TabsContent value="users" className="mt-4">
          <Card className="border-gray-200 bg-white shadow-sm">
            <CardHeader className="flex flex-row items-center justify-between space-y-0 border-b border-gray-200 px-5 py-4">
              <CardTitle className="text-sm font-semibold text-gray-900">
                Database Users
              </CardTitle>
              <div className="flex items-center gap-3">
                <div className="relative">
                  <Search
                    className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
                    size={16}
                  />
                  <Input
                    aria-label="Search database users"
                    placeholder="Search users..."
                    value={userSearch}
                    onChange={(e) => setUserSearch(e.target.value)}
                    className={`w-64 pl-10 ${INPUT_CLASS}`}
                  />
                </div>
                <Button
                  onClick={() => {
                    setDatabaseForm(EMPTY_DATABASE_FORM);
                    setFormError(null);
                    setShowUserForm(true);
                  }}
                  className={PRIMARY_BUTTON_CLASS}
                >
                  <Plus size={16} className="mr-2" />
                  Create User
                </Button>
              </div>
            </CardHeader>
            <CardContent className="p-0">
              {filteredUsers.length === 0 ? (
                <div className="px-5 py-12 text-center">
                  <Users className="mx-auto text-gray-300" size={40} />
                  <h3 className="mt-3 text-sm font-semibold text-gray-900">
                    No database users
                  </h3>
                  <p className="mt-1 text-sm text-gray-600">
                    {userSearch
                      ? 'No users match your search'
                      : 'Create your first database user to get started'}
                  </p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead className="border-b border-gray-200 bg-gray-50">
                      <tr>
                        <th className={TH_CLASS}>Username</th>
                        <th className={TH_CLASS}>Database</th>
                        <th className={TH_CLASS}>Server</th>
                        <th className={TH_CLASS}>Permissions</th>
                        <th className={TH_CLASS}>Created</th>
                        <th className={`${TH_CLASS} text-right`}>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredUsers.map((user) => (
                        <tr
                          key={user.id}
                          className="border-b border-gray-100 hover:bg-gray-50"
                        >
                          <td className="px-4 py-3">
                            <div className="flex items-center gap-2">
                              <User className="text-gray-400" size={16} />
                              <span className="font-mono text-sm font-medium text-gray-900">
                                {user.username}
                              </span>
                            </div>
                          </td>
                          <td className="px-4 py-3 text-sm text-gray-700">
                            {user.database_name}
                          </td>
                          <td className="px-4 py-3 text-sm text-gray-700">
                            {getServerDisplayName(user.database_server_id)}
                          </td>
                          <td className="px-4 py-3">
                            <span className="inline-flex items-center rounded-md border border-gray-200 bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-700">
                              <Shield size={12} className="mr-1" />
                              ALL PRIVILEGES
                            </span>
                          </td>
                          <td className="px-4 py-3 text-sm text-gray-600">
                            {formatDate(user.created_at)}
                          </td>
                          <td className="px-4 py-3 text-right">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() =>
                                  openPasswordForm(user.database_id, user.username)
                                }
                                className={ICON_BUTTON_CLASS}
                                aria-label={`Change password for ${user.username}`}
                                title="Change password"
                              >
                                <Key size={16} />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() =>
                                  openDeleteConfirm(
                                    'database',
                                    user.database_id,
                                    user.database_name,
                                  )
                                }
                                className={DANGER_ICON_BUTTON_CLASS}
                                aria-label={`Delete user ${user.username} and its database`}
                                title="Delete user (drops database)"
                              >
                                <Trash2 size={16} />
                              </Button>
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* ================================================================== */}
      {/* Add Server Modal                                                    */}
      {/* ================================================================== */}
      {showServerForm && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40"
          onClick={() => setShowServerForm(false)}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Add Database Server"
            className="mx-4 w-full max-w-md rounded-lg border border-gray-200 bg-white p-6 shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-6 flex items-center justify-between">
              <h2 className="text-sm font-semibold text-gray-900">Add Database Server</h2>
              <button
                type="button"
                onClick={() => setShowServerForm(false)}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X size={16} />
              </button>
            </div>
            <form onSubmit={handleCreateServer} className="space-y-4">
              {formError && (
                <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
                  {formError}
                </div>
              )}
              {/* One node means one answer; the picker appears only with a real choice. */}
              <ServerScopeField
                id="server-infra"
                servers={safeInfraServers}
                value={serverForm.server_id}
                onChange={(id) => setServerForm((prev) => ({ ...prev, server_id: id }))}
                copy={SERVER_SCOPE_COPY_EN}
              />
              <div>
                <label
                  htmlFor="server-type"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Database Type <span className="text-red-600">*</span>
                </label>
                <Select
                  value={serverForm.type}
                  onValueChange={(v) => {
                    const dbType = DB_TYPES.find((t) => t.value === v);
                    setServerForm((prev) => ({
                      ...prev,
                      type: v,
                      port: dbType?.defaultPort || prev.port,
                    }));
                  }}
                >
                  <SelectTrigger
                    id="server-type"
                    aria-label="Database type"
                    className={SELECT_TRIGGER_CLASS}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className="border-gray-200 bg-white">
                    {DB_TYPES.map((t) => (
                      <SelectItem key={t.value} value={t.value}>
                        {t.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label
                    htmlFor="server-version"
                    className="mb-1.5 block text-sm font-medium text-gray-700"
                  >
                    Version
                  </label>
                  <Input
                    id="server-version"
                    value={serverForm.version}
                    onChange={(e) =>
                      setServerForm((prev) => ({
                        ...prev,
                        version: e.target.value,
                      }))
                    }
                    placeholder="e.g. 8.0"
                    className={INPUT_CLASS}
                  />
                </div>
                <div>
                  <label
                    htmlFor="server-port"
                    className="mb-1.5 block text-sm font-medium text-gray-700"
                  >
                    Port
                  </label>
                  <Input
                    id="server-port"
                    type="number"
                    value={serverForm.port}
                    onChange={(e) =>
                      setServerForm((prev) => ({
                        ...prev,
                        port: e.target.value,
                      }))
                    }
                    placeholder="3306"
                    className={INPUT_CLASS}
                  />
                </div>
              </div>
              <div className="flex justify-end gap-3 border-t border-gray-200 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowServerForm(false)}
                  className={SECONDARY_BUTTON_CLASS}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={submitting}
                  className={`${PRIMARY_BUTTON_CLASS} disabled:opacity-50`}
                >
                  {submitting ? (
                    <Loader2 size={16} className="mr-2 animate-spin" />
                  ) : (
                    <Plus size={16} className="mr-2" />
                  )}
                  Add Server
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* Create Database Modal                                               */}
      {/* ================================================================== */}
      {showDatabaseForm && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40"
          onClick={() => setShowDatabaseForm(false)}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Create Database"
            className="mx-4 w-full max-w-lg rounded-lg border border-gray-200 bg-white p-6 shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-6 flex items-center justify-between">
              <h2 className="text-sm font-semibold text-gray-900">Create Database</h2>
              <button
                type="button"
                onClick={() => setShowDatabaseForm(false)}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X size={16} />
              </button>
            </div>
            <form onSubmit={handleCreateDatabase} className="space-y-4">
              {formError && (
                <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
                  {formError}
                </div>
              )}
              <div>
                <label
                  htmlFor="db-server"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Database Server <span className="text-red-600">*</span>
                </label>
                {/*
                  With one database engine running there is nothing to choose:
                  the field states where the database will be created instead of
                  making the operator answer a question with one answer.
                */}
                {activeDBServers.length === 1 ? (
                  <div className="rounded-md border border-gray-200 bg-gray-50 px-3 py-2">
                    <p className="truncate text-sm font-medium text-gray-900">
                      {dbServerOptionLabel(activeDBServers[0], getInfraServerName)}
                    </p>
                    <p className="mt-0.5 text-xs text-gray-500">
                      The only database server running, so the database is created there.
                    </p>
                  </div>
                ) : (
                  <Select
                    value={databaseForm.database_server_id}
                    onValueChange={(v) =>
                      setDatabaseForm((prev) => ({
                        ...prev,
                        database_server_id: v,
                      }))
                    }
                  >
                    <SelectTrigger
                      id="db-server"
                      aria-label="Database server"
                      className={SELECT_TRIGGER_CLASS}
                    >
                      <SelectValue placeholder="Select a database server" />
                    </SelectTrigger>
                    <SelectContent className="border-gray-200 bg-white">
                      {activeDBServers.map((s) => (
                        <SelectItem key={s.id} value={s.id}>
                          {dbServerOptionLabel(s, getInfraServerName)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              </div>
              <div>
                <label
                  htmlFor="db-name"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Database Name <span className="text-red-600">*</span>
                </label>
                <Input
                  id="db-name"
                  value={databaseForm.name}
                  onChange={(e) =>
                    setDatabaseForm((prev) => ({ ...prev, name: e.target.value }))
                  }
                  placeholder="my_database"
                  className={INPUT_CLASS}
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label
                    htmlFor="db-username"
                    className="mb-1.5 block text-sm font-medium text-gray-700"
                  >
                    Username <span className="text-red-600">*</span>
                  </label>
                  <Input
                    id="db-username"
                    value={databaseForm.username}
                    onChange={(e) =>
                      setDatabaseForm((prev) => ({
                        ...prev,
                        username: e.target.value,
                      }))
                    }
                    placeholder="db_user"
                    className={INPUT_CLASS}
                  />
                </div>
                <div>
                  <label
                    htmlFor="db-password"
                    className="mb-1.5 block text-sm font-medium text-gray-700"
                  >
                    Password <span className="text-red-600">*</span>
                  </label>
                  <Input
                    id="db-password"
                    type="password"
                    value={databaseForm.password}
                    onChange={(e) =>
                      setDatabaseForm((prev) => ({
                        ...prev,
                        password: e.target.value,
                      }))
                    }
                    placeholder="••••••••"
                    className={INPUT_CLASS}
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label
                    htmlFor="db-charset"
                    className="mb-1.5 block text-sm font-medium text-gray-700"
                  >
                    Charset
                  </label>
                  <Input
                    id="db-charset"
                    value={databaseForm.charset}
                    onChange={(e) =>
                      setDatabaseForm((prev) => ({
                        ...prev,
                        charset: e.target.value,
                      }))
                    }
                    placeholder="utf8mb4"
                    className={INPUT_CLASS}
                  />
                </div>
                <div>
                  <label
                    htmlFor="db-collation"
                    className="mb-1.5 block text-sm font-medium text-gray-700"
                  >
                    Collation
                  </label>
                  <Input
                    id="db-collation"
                    value={databaseForm.collation}
                    onChange={(e) =>
                      setDatabaseForm((prev) => ({
                        ...prev,
                        collation: e.target.value,
                      }))
                    }
                    placeholder="utf8mb4_unicode_ci"
                    className={INPUT_CLASS}
                  />
                </div>
              </div>
              <div className="flex justify-end gap-3 border-t border-gray-200 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowDatabaseForm(false)}
                  className={SECONDARY_BUTTON_CLASS}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={submitting}
                  className={`${PRIMARY_BUTTON_CLASS} disabled:opacity-50`}
                >
                  {submitting ? (
                    <Loader2 size={16} className="mr-2 animate-spin" />
                  ) : (
                    <Database size={16} className="mr-2" />
                  )}
                  Create Database
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* Create User Modal                                                   */}
      {/* ================================================================== */}
      {showUserForm && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40"
          onClick={() => setShowUserForm(false)}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Create Database User"
            className="mx-4 w-full max-w-md rounded-lg border border-gray-200 bg-white p-6 shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-6 flex items-center justify-between">
              <h2 className="text-sm font-semibold text-gray-900">Create Database User</h2>
              <button
                type="button"
                onClick={() => setShowUserForm(false)}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X size={16} />
              </button>
            </div>
            <form onSubmit={handleCreateUser} className="space-y-4">
              {formError && (
                <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
                  {formError}
                </div>
              )}
              <div>
                <label
                  htmlFor="user-db-server"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Database Server <span className="text-red-600">*</span>
                </label>
                <Select
                  value={databaseForm.database_server_id}
                  onValueChange={(v) =>
                    setDatabaseForm((prev) => ({
                      ...prev,
                      database_server_id: v,
                    }))
                  }
                >
                  <SelectTrigger
                    id="user-db-server"
                    aria-label="Database server"
                    className={SELECT_TRIGGER_CLASS}
                  >
                    <SelectValue placeholder="Select a database server" />
                  </SelectTrigger>
                  <SelectContent className="border-gray-200 bg-white">
                    {safeServers
                      .filter((s) => s?.status === 'active')
                      .map((s) => (
                        <SelectItem key={s.id} value={s.id}>
                          {(s.type || '').toUpperCase()} –{' '}
                          {getInfraServerName(s.server_id)} ({s.port})
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              </div>
              <div>
                <label
                  htmlFor="user-db-name"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Database Name <span className="text-red-600">*</span>
                </label>
                <Input
                  id="user-db-name"
                  value={databaseForm.name}
                  onChange={(e) =>
                    setDatabaseForm((prev) => ({ ...prev, name: e.target.value }))
                  }
                  placeholder="my_database"
                  className={INPUT_CLASS}
                />
              </div>
              <div>
                <label
                  htmlFor="user-username"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Username <span className="text-red-600">*</span>
                </label>
                <Input
                  id="user-username"
                  value={databaseForm.username}
                  onChange={(e) =>
                    setDatabaseForm((prev) => ({
                      ...prev,
                      username: e.target.value,
                    }))
                  }
                  placeholder="db_user"
                  className={INPUT_CLASS}
                />
              </div>
              <div>
                <label
                  htmlFor="user-password"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Password <span className="text-red-600">*</span>
                </label>
                <Input
                  id="user-password"
                  type="password"
                  value={databaseForm.password}
                  onChange={(e) =>
                    setDatabaseForm((prev) => ({
                      ...prev,
                      password: e.target.value,
                    }))
                  }
                  placeholder="••••••••"
                  className={INPUT_CLASS}
                />
              </div>
              <div className="flex justify-end gap-3 border-t border-gray-200 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowUserForm(false)}
                  className={SECONDARY_BUTTON_CLASS}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={submitting}
                  className={`${PRIMARY_BUTTON_CLASS} disabled:opacity-50`}
                >
                  {submitting ? (
                    <Loader2 size={16} className="mr-2 animate-spin" />
                  ) : (
                    <User size={16} className="mr-2" />
                  )}
                  Create User
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* Change Password Modal                                               */}
      {/* ================================================================== */}
      {showPasswordForm && passwordTarget && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40"
          onClick={() => setShowPasswordForm(false)}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Change Password"
            className="mx-4 w-full max-w-md rounded-lg border border-gray-200 bg-white p-6 shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-4 flex items-center justify-between">
              <h2 className="text-sm font-semibold text-gray-900">Change Password</h2>
              <button
                type="button"
                onClick={() => setShowPasswordForm(false)}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X size={16} />
              </button>
            </div>
            <p className="mb-4 text-sm text-gray-600">
              Change password for{' '}
              <span className="font-medium text-gray-900">{passwordTarget.name}</span>
            </p>
            <form onSubmit={handleChangePassword} className="space-y-4">
              {formError && (
                <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
                  {formError}
                </div>
              )}
              <div>
                <label
                  htmlFor="new-password"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  New Password <span className="text-red-600">*</span>
                </label>
                <Input
                  id="new-password"
                  type="password"
                  value={passwordForm.password}
                  onChange={(e) =>
                    setPasswordForm((prev) => ({
                      ...prev,
                      password: e.target.value,
                    }))
                  }
                  placeholder="••••••••"
                  className={INPUT_CLASS}
                />
              </div>
              <div>
                <label
                  htmlFor="confirm-password"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Confirm Password <span className="text-red-600">*</span>
                </label>
                <Input
                  id="confirm-password"
                  type="password"
                  value={passwordForm.confirmPassword}
                  onChange={(e) =>
                    setPasswordForm((prev) => ({
                      ...prev,
                      confirmPassword: e.target.value,
                    }))
                  }
                  placeholder="••••••••"
                  className={INPUT_CLASS}
                />
              </div>
              <div className="flex justify-end gap-3 border-t border-gray-200 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowPasswordForm(false)}
                  className={SECONDARY_BUTTON_CLASS}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={submitting}
                  className={`${PRIMARY_BUTTON_CLASS} disabled:opacity-50`}
                >
                  {submitting ? (
                    <Loader2 size={16} className="mr-2 animate-spin" />
                  ) : (
                    <Key size={16} className="mr-2" />
                  )}
                  Change Password
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* Delete Confirmation Modal                                           */}
      {/* ================================================================== */}
      {showDeleteConfirm && deleteTarget && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40"
          onClick={() => setShowDeleteConfirm(false)}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Confirm Deletion"
            className="mx-4 w-full max-w-md rounded-lg border border-gray-200 bg-white p-6 shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-4 flex items-center gap-3">
              <div className="rounded-md border border-red-200 bg-red-50 p-2">
                <AlertTriangle className="text-red-600" size={20} />
              </div>
              <h2 className="text-sm font-semibold text-gray-900">Confirm Deletion</h2>
            </div>
            <div className="mb-6 rounded-md border border-gray-200 bg-gray-50 p-3">
              <p className="text-sm text-gray-700">
                Are you sure you want to delete{' '}
                <span className="font-medium text-gray-900">{deleteTarget.name}</span>?
                {deleteTarget.type === 'database' && (
                  <span className="mt-1 block text-sm text-red-700">
                    This will also drop the associated database user.
                  </span>
                )}
                <span className="mt-1 block text-sm text-gray-500">
                  This action cannot be undone.
                </span>
              </p>
            </div>
            <div className="flex justify-end gap-3">
              <Button
                variant="outline"
                onClick={() => setShowDeleteConfirm(false)}
                className={SECONDARY_BUTTON_CLASS}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={handleDelete}
                disabled={deleting}
                className="bg-red-600 text-white hover:bg-red-700 focus-visible:ring-red-500 disabled:opacity-50"
              >
                {deleting ? (
                  <Loader2 size={16} className="mr-2 animate-spin" />
                ) : (
                  <Trash2 size={16} className="mr-2" />
                )}
                Delete
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* Toast                                                               */}
      {/* ================================================================== */}
      {toast && (
        <div className="fixed bottom-6 right-6 z-50" role="status">
          <div
            className={`rounded-md border px-4 py-3 text-sm font-medium shadow-lg ${
              toast.type === 'success'
                ? 'border-emerald-200 bg-emerald-50 text-emerald-700'
                : 'border-red-200 bg-red-50 text-red-700'
            }`}
          >
            {toast.message}
          </div>
        </div>
      )}
    </div>
  );
}
