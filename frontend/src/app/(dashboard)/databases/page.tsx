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
import { api, databaseApi } from '@/services/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface InfrastructureServer {
  id: string;
  name: string;
  hostname: string;
  ip_address: string;
  status: string;
}

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

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function DatabasesPage() {
  // Data
  const [servers, setServers] = useState<DBServer[]>([]);
  const [databases, setDatabases] = useState<DBEntry[]>([]);
  const [infraServers, setInfraServers] = useState<InfrastructureServer[]>([]);
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
        api.get('/api/v1/servers'),
      ]);
      setServers(serversRes.data.data || []);
      setDatabases(databasesRes.data.data || []);
      setInfraServers(infraRes.data.data || []);
    } catch (err: any) {
      console.error('Failed to load database data:', err);
      setError(err.response?.data?.message || 'Failed to load data');
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

  const users: DBUser[] = databases.map((db) => ({
    id: db.id,
    username: db.username,
    database_server_id: db.database_server_id,
    database_name: db.name,
    database_id: db.id,
    created_at: db.created_at,
  }));

  const getInfraServerName = (serverId: string) => {
    const server = infraServers.find((s) => s.id === serverId);
    return server?.name || serverId.slice(0, 8) + '…';
  };

  const getServerDisplayName = (dbServerId: string) => {
    const dbServer = servers.find((s) => s.id === dbServerId);
    if (!dbServer) return dbServerId.slice(0, 8) + '…';
    const infra = infraServers.find((s) => s.id === dbServer.server_id);
    return infra?.name || dbServer.type + ' (' + dbServerId.slice(0, 8) + ')';
  };

  const formatBytes = (bytes: number) => {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'active':
        return <Badge variant="success">{status}</Badge>;
      case 'inactive':
      case 'error':
        return <Badge variant="destructive">{status}</Badge>;
      default:
        return <Badge variant="warning">{status}</Badge>;
    }
  };

  const getDbTypeIcon = (type: string) => {
    switch (type) {
      case 'mysql':
      case 'mariadb':
        return <Database className="text-blue-400" size={16} />;
      case 'postgresql':
        return <Database className="text-indigo-400" size={16} />;
      case 'redis':
        return <HardDrive className="text-red-400" size={16} />;
      default:
        return <Database className="text-gray-400" size={16} />;
    }
  };

  // -------------------------------------------------------------------------
  // Filtered data
  // -------------------------------------------------------------------------

  const filteredServers = servers.filter((s) => {
    if (!serverSearch) return true;
    const q = serverSearch.toLowerCase();
    return (
      s.type.toLowerCase().includes(q) ||
      s.version.toLowerCase().includes(q) ||
      s.status.toLowerCase().includes(q) ||
      s.id.toLowerCase().includes(q)
    );
  });

  const filteredDatabases = databases.filter((d) => {
    if (!databaseSearch) return true;
    const q = databaseSearch.toLowerCase();
    return (
      d.name.toLowerCase().includes(q) ||
      d.username.toLowerCase().includes(q) ||
      d.charset.toLowerCase().includes(q)
    );
  });

  const filteredUsers = users.filter((u) => {
    if (!userSearch) return true;
    const q = userSearch.toLowerCase();
    return (
      u.username.toLowerCase().includes(q) ||
      u.database_name.toLowerCase().includes(q)
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
      setFormError(err.response?.data?.message || 'Failed to add server');
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
      setFormError(err.response?.data?.message || 'Failed to create database');
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
      setFormError(err.response?.data?.message || 'Failed to create user');
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
      setFormError(err.response?.data?.message || 'Failed to change password');
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
        message: err.response?.data?.message || `Failed to delete ${deleteTarget.type}`,
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
      <div className="flex items-center justify-center h-64">
        <Loader2 className="animate-spin text-primary-500" size={32} />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <AlertTriangle className="text-red-400" size={48} />
        <p className="text-dark-300 text-lg">{error}</p>
        <Button onClick={fetchAll} variant="outline">
          <RefreshCw size={16} className="mr-2" />
          Retry
        </Button>
      </div>
    );
  }

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Databases</h1>
          <p className="text-dark-400 mt-1">
            Manage database servers, databases, and users
          </p>
        </div>
        <Button onClick={fetchAll} variant="outline" size="sm">
          <RefreshCw size={16} className="mr-2" />
          Refresh
        </Button>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card className="bg-dark-900 border-dark-700">
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-blue-500/10 rounded-lg">
                <Server className="text-blue-400" size={20} />
              </div>
              <div>
                <p className="text-sm text-dark-400">Database Servers</p>
                <p className="text-2xl font-bold text-dark-50">{servers.length}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-dark-900 border-dark-700">
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-green-500/10 rounded-lg">
                <Database className="text-green-400" size={20} />
              </div>
              <div>
                <p className="text-sm text-dark-400">Databases</p>
                <p className="text-2xl font-bold text-dark-50">{databases.length}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-dark-900 border-dark-700">
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-purple-500/10 rounded-lg">
                <Users className="text-purple-400" size={20} />
              </div>
              <div>
                <p className="text-sm text-dark-400">Database Users</p>
                <p className="text-2xl font-bold text-dark-50">{users.length}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="servers">
        <TabsList className="bg-dark-800 border border-dark-700">
          <TabsTrigger
            value="servers"
            className="data-[state=active]:bg-dark-700 data-[state=active]:text-dark-50 text-dark-400"
          >
            <Server size={16} className="mr-2" />
            Database Servers
          </TabsTrigger>
          <TabsTrigger
            value="databases"
            className="data-[state=active]:bg-dark-700 data-[state=active]:text-dark-50 text-dark-400"
          >
            <Database size={16} className="mr-2" />
            Databases
          </TabsTrigger>
          <TabsTrigger
            value="users"
            className="data-[state=active]:bg-dark-700 data-[state=active]:text-dark-50 text-dark-400"
          >
            <Users size={16} className="mr-2" />
            Users
          </TabsTrigger>
        </TabsList>

        {/* ================================================================= */}
        {/* Database Servers Tab                                               */}
        {/* ================================================================= */}
        <TabsContent value="servers">
          <Card className="bg-dark-900 border-dark-700">
            <CardHeader className="flex flex-row items-center justify-between space-y-0">
              <CardTitle className="text-dark-50 text-lg">Database Servers</CardTitle>
              <div className="flex items-center gap-3">
                <div className="relative">
                  <Search
                    className="absolute left-3 top-1/2 -translate-y-1/2 text-dark-400"
                    size={16}
                  />
                  <Input
                    placeholder="Search servers..."
                    value={serverSearch}
                    onChange={(e) => setServerSearch(e.target.value)}
                    className="pl-10 bg-dark-800 border-dark-600 text-dark-100 w-64"
                  />
                </div>
                <Button
                  onClick={() => {
                    setServerForm(EMPTY_SERVER_FORM);
                    setFormError(null);
                    setShowServerForm(true);
                  }}
                >
                  <Plus size={16} className="mr-2" />
                  Add Server
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {filteredServers.length === 0 ? (
                <div className="text-center py-12">
                  <Server className="mx-auto text-dark-600" size={48} />
                  <h3 className="mt-4 text-lg font-medium text-dark-300">
                    No database servers
                  </h3>
                  <p className="mt-2 text-dark-500">
                    {serverSearch
                      ? 'No servers match your search'
                      : 'Add your first database server to get started'}
                  </p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-dark-700">
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Name
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Type
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Host
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Port
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Status
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Version
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Created
                        </th>
                        <th className="text-right py-3 px-4 text-dark-400 font-medium text-sm">
                          Actions
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredServers.map((server) => {
                        const infra = infraServers.find(
                          (s) => s.id === server.server_id,
                        );
                        return (
                          <tr
                            key={server.id}
                            className="border-b border-dark-800 hover:bg-dark-800/50 transition-colors"
                          >
                            <td className="py-3 px-4">
                              <div className="flex items-center gap-2">
                                {getDbTypeIcon(server.type)}
                                <span className="text-dark-100 font-medium">
                                  {infra?.name || 'Unknown'}
                                </span>
                              </div>
                            </td>
                            <td className="py-3 px-4">
                              <Badge variant="secondary" className="capitalize">
                                {server.type}
                              </Badge>
                            </td>
                            <td className="py-3 px-4 text-dark-200 font-mono text-sm">
                              {infra?.ip_address || infra?.hostname || '—'}
                            </td>
                            <td className="py-3 px-4 text-dark-300 font-mono text-sm">
                              {server.port || '—'}
                            </td>
                            <td className="py-3 px-4">{getStatusBadge(server.status)}</td>
                            <td className="py-3 px-4 text-dark-300 text-sm">
                              {server.version || '—'}
                            </td>
                            <td className="py-3 px-4 text-dark-400 text-sm">
                              {new Date(server.created_at).toLocaleDateString()}
                            </td>
                            <td className="py-3 px-4 text-right">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() =>
                                  openDeleteConfirm(
                                    'server',
                                    server.id,
                                    infra?.name || server.type,
                                  )
                                }
                                className="text-red-400 hover:text-red-300 hover:bg-red-500/10"
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
        <TabsContent value="databases">
          <Card className="bg-dark-900 border-dark-700">
            <CardHeader className="flex flex-row items-center justify-between space-y-0">
              <CardTitle className="text-dark-50 text-lg">Databases</CardTitle>
              <div className="flex items-center gap-3">
                <div className="relative">
                  <Search
                    className="absolute left-3 top-1/2 -translate-y-1/2 text-dark-400"
                    size={16}
                  />
                  <Input
                    placeholder="Search databases..."
                    value={databaseSearch}
                    onChange={(e) => setDatabaseSearch(e.target.value)}
                    className="pl-10 bg-dark-800 border-dark-600 text-dark-100 w-64"
                  />
                </div>
                <Button
                  onClick={() => {
                    setDatabaseForm(EMPTY_DATABASE_FORM);
                    setFormError(null);
                    setShowDatabaseForm(true);
                  }}
                >
                  <Plus size={16} className="mr-2" />
                  Create Database
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {filteredDatabases.length === 0 ? (
                <div className="text-center py-12">
                  <Database className="mx-auto text-dark-600" size={48} />
                  <h3 className="mt-4 text-lg font-medium text-dark-300">
                    No databases
                  </h3>
                  <p className="mt-2 text-dark-500">
                    {databaseSearch
                      ? 'No databases match your search'
                      : 'Create your first database to get started'}
                  </p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-dark-700">
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Name
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Server
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Size
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Charset
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Status
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Created
                        </th>
                        <th className="text-right py-3 px-4 text-dark-400 font-medium text-sm">
                          Actions
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredDatabases.map((db) => (
                        <tr
                          key={db.id}
                          className="border-b border-dark-800 hover:bg-dark-800/50 transition-colors"
                        >
                          <td className="py-3 px-4">
                            <div className="flex items-center gap-2">
                              <Database className="text-blue-400" size={16} />
                              <span className="text-dark-100 font-medium">
                                {db.name}
                              </span>
                            </div>
                          </td>
                          <td className="py-3 px-4 text-dark-200 text-sm">
                            {getServerDisplayName(db.database_server_id)}
                          </td>
                          <td className="py-3 px-4 text-dark-300 text-sm">
                            {formatBytes(db.size)}
                          </td>
                          <td className="py-3 px-4 text-dark-300 text-sm font-mono">
                            {db.charset}
                          </td>
                          <td className="py-3 px-4">{getStatusBadge(db.status)}</td>
                          <td className="py-3 px-4 text-dark-400 text-sm">
                            {new Date(db.created_at).toLocaleDateString()}
                          </td>
                          <td className="py-3 px-4 text-right">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => openPasswordForm(db.id, db.name)}
                                className="text-yellow-400 hover:text-yellow-300 hover:bg-yellow-500/10"
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
                                className="text-red-400 hover:text-red-300 hover:bg-red-500/10"
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
        <TabsContent value="users">
          <Card className="bg-dark-900 border-dark-700">
            <CardHeader className="flex flex-row items-center justify-between space-y-0">
              <CardTitle className="text-dark-50 text-lg">Database Users</CardTitle>
              <div className="flex items-center gap-3">
                <div className="relative">
                  <Search
                    className="absolute left-3 top-1/2 -translate-y-1/2 text-dark-400"
                    size={16}
                  />
                  <Input
                    placeholder="Search users..."
                    value={userSearch}
                    onChange={(e) => setUserSearch(e.target.value)}
                    className="pl-10 bg-dark-800 border-dark-600 text-dark-100 w-64"
                  />
                </div>
                <Button
                  onClick={() => {
                    setDatabaseForm(EMPTY_DATABASE_FORM);
                    setFormError(null);
                    setShowUserForm(true);
                  }}
                >
                  <Plus size={16} className="mr-2" />
                  Create User
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {filteredUsers.length === 0 ? (
                <div className="text-center py-12">
                  <Users className="mx-auto text-dark-600" size={48} />
                  <h3 className="mt-4 text-lg font-medium text-dark-300">
                    No database users
                  </h3>
                  <p className="mt-2 text-dark-500">
                    {userSearch
                      ? 'No users match your search'
                      : 'Create your first database user to get started'}
                  </p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-dark-700">
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Username
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Database
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Server
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Permissions
                        </th>
                        <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                          Created
                        </th>
                        <th className="text-right py-3 px-4 text-dark-400 font-medium text-sm">
                          Actions
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredUsers.map((user) => (
                        <tr
                          key={user.id}
                          className="border-b border-dark-800 hover:bg-dark-800/50 transition-colors"
                        >
                          <td className="py-3 px-4">
                            <div className="flex items-center gap-2">
                              <User className="text-purple-400" size={16} />
                              <span className="text-dark-100 font-medium font-mono">
                                {user.username}
                              </span>
                            </div>
                          </td>
                          <td className="py-3 px-4 text-dark-200 text-sm">
                            {user.database_name}
                          </td>
                          <td className="py-3 px-4 text-dark-300 text-sm">
                            {getServerDisplayName(user.database_server_id)}
                          </td>
                          <td className="py-3 px-4">
                            <Badge variant="secondary">
                              <Shield size={12} className="mr-1" />
                              ALL PRIVILEGES
                            </Badge>
                          </td>
                          <td className="py-3 px-4 text-dark-400 text-sm">
                            {new Date(user.created_at).toLocaleDateString()}
                          </td>
                          <td className="py-3 px-4 text-right">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() =>
                                  openPasswordForm(user.database_id, user.username)
                                }
                                className="text-yellow-400 hover:text-yellow-300 hover:bg-yellow-500/10"
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
                                className="text-red-400 hover:text-red-300 hover:bg-red-500/10"
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
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
          onClick={() => setShowServerForm(false)}
        >
          <div
            className="bg-dark-900 border border-dark-700 rounded-lg w-full max-w-md p-6 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-lg font-semibold text-dark-50">
                Add Database Server
              </h2>
              <button
                onClick={() => setShowServerForm(false)}
                className="text-dark-400 hover:text-dark-200 transition-colors"
              >
                <X size={20} />
              </button>
            </div>
            <form onSubmit={handleCreateServer} className="space-y-4">
              {formError && (
                <div className="p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-red-400 text-sm">
                  {formError}
                </div>
              )}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  Infrastructure Server *
                </label>
                <Select
                  value={serverForm.server_id}
                  onValueChange={(v) =>
                    setServerForm((prev) => ({ ...prev, server_id: v }))
                  }
                >
                  <SelectTrigger className="bg-dark-800 border-dark-600 text-dark-100">
                    <SelectValue placeholder="Select a server" />
                  </SelectTrigger>
                  <SelectContent>
                    {infraServers.map((s) => (
                      <SelectItem key={s.id} value={s.id}>
                        {s.name} ({s.ip_address || s.hostname})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  Database Type *
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
                  <SelectTrigger className="bg-dark-800 border-dark-600 text-dark-100">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
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
                  <label className="block text-sm font-medium text-dark-300 mb-1">
                    Version
                  </label>
                  <Input
                    value={serverForm.version}
                    onChange={(e) =>
                      setServerForm((prev) => ({
                        ...prev,
                        version: e.target.value,
                      }))
                    }
                    placeholder="e.g. 8.0"
                    className="bg-dark-800 border-dark-600 text-dark-100"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1">
                    Port
                  </label>
                  <Input
                    type="number"
                    value={serverForm.port}
                    onChange={(e) =>
                      setServerForm((prev) => ({
                        ...prev,
                        port: e.target.value,
                      }))
                    }
                    placeholder="3306"
                    className="bg-dark-800 border-dark-600 text-dark-100"
                  />
                </div>
              </div>
              <div className="flex justify-end gap-3 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowServerForm(false)}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={submitting}>
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
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
          onClick={() => setShowDatabaseForm(false)}
        >
          <div
            className="bg-dark-900 border border-dark-700 rounded-lg w-full max-w-lg p-6 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-lg font-semibold text-dark-50">
                Create Database
              </h2>
              <button
                onClick={() => setShowDatabaseForm(false)}
                className="text-dark-400 hover:text-dark-200 transition-colors"
              >
                <X size={20} />
              </button>
            </div>
            <form onSubmit={handleCreateDatabase} className="space-y-4">
              {formError && (
                <div className="p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-red-400 text-sm">
                  {formError}
                </div>
              )}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  Database Server *
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
                  <SelectTrigger className="bg-dark-800 border-dark-600 text-dark-100">
                    <SelectValue placeholder="Select a database server" />
                  </SelectTrigger>
                  <SelectContent>
                    {servers
                      .filter((s) => s.status === 'active')
                      .map((s) => (
                        <SelectItem key={s.id} value={s.id}>
                          {s.type.toUpperCase()} –{' '}
                          {getInfraServerName(s.server_id)} ({s.port})
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              </div>
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  Database Name *
                </label>
                <Input
                  value={databaseForm.name}
                  onChange={(e) =>
                    setDatabaseForm((prev) => ({ ...prev, name: e.target.value }))
                  }
                  placeholder="my_database"
                  className="bg-dark-800 border-dark-600 text-dark-100"
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1">
                    Username *
                  </label>
                  <Input
                    value={databaseForm.username}
                    onChange={(e) =>
                      setDatabaseForm((prev) => ({
                        ...prev,
                        username: e.target.value,
                      }))
                    }
                    placeholder="db_user"
                    className="bg-dark-800 border-dark-600 text-dark-100"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1">
                    Password *
                  </label>
                  <Input
                    type="password"
                    value={databaseForm.password}
                    onChange={(e) =>
                      setDatabaseForm((prev) => ({
                        ...prev,
                        password: e.target.value,
                      }))
                    }
                    placeholder="••••••••"
                    className="bg-dark-800 border-dark-600 text-dark-100"
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1">
                    Charset
                  </label>
                  <Input
                    value={databaseForm.charset}
                    onChange={(e) =>
                      setDatabaseForm((prev) => ({
                        ...prev,
                        charset: e.target.value,
                      }))
                    }
                    placeholder="utf8mb4"
                    className="bg-dark-800 border-dark-600 text-dark-100"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1">
                    Collation
                  </label>
                  <Input
                    value={databaseForm.collation}
                    onChange={(e) =>
                      setDatabaseForm((prev) => ({
                        ...prev,
                        collation: e.target.value,
                      }))
                    }
                    placeholder="utf8mb4_unicode_ci"
                    className="bg-dark-800 border-dark-600 text-dark-100"
                  />
                </div>
              </div>
              <div className="flex justify-end gap-3 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowDatabaseForm(false)}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={submitting}>
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
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
          onClick={() => setShowUserForm(false)}
        >
          <div
            className="bg-dark-900 border border-dark-700 rounded-lg w-full max-w-md p-6 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-lg font-semibold text-dark-50">
                Create Database User
              </h2>
              <button
                onClick={() => setShowUserForm(false)}
                className="text-dark-400 hover:text-dark-200 transition-colors"
              >
                <X size={20} />
              </button>
            </div>
            <form onSubmit={handleCreateUser} className="space-y-4">
              {formError && (
                <div className="p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-red-400 text-sm">
                  {formError}
                </div>
              )}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  Database Server *
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
                  <SelectTrigger className="bg-dark-800 border-dark-600 text-dark-100">
                    <SelectValue placeholder="Select a database server" />
                  </SelectTrigger>
                  <SelectContent>
                    {servers
                      .filter((s) => s.status === 'active')
                      .map((s) => (
                        <SelectItem key={s.id} value={s.id}>
                          {s.type.toUpperCase()} –{' '}
                          {getInfraServerName(s.server_id)} ({s.port})
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              </div>
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  Database Name *
                </label>
                <Input
                  value={databaseForm.name}
                  onChange={(e) =>
                    setDatabaseForm((prev) => ({ ...prev, name: e.target.value }))
                  }
                  placeholder="my_database"
                  className="bg-dark-800 border-dark-600 text-dark-100"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  Username *
                </label>
                <Input
                  value={databaseForm.username}
                  onChange={(e) =>
                    setDatabaseForm((prev) => ({
                      ...prev,
                      username: e.target.value,
                    }))
                  }
                  placeholder="db_user"
                  className="bg-dark-800 border-dark-600 text-dark-100"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  Password *
                </label>
                <Input
                  type="password"
                  value={databaseForm.password}
                  onChange={(e) =>
                    setDatabaseForm((prev) => ({
                      ...prev,
                      password: e.target.value,
                    }))
                  }
                  placeholder="••••••••"
                  className="bg-dark-800 border-dark-600 text-dark-100"
                />
              </div>
              <div className="flex justify-end gap-3 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowUserForm(false)}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={submitting}>
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
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
          onClick={() => setShowPasswordForm(false)}
        >
          <div
            className="bg-dark-900 border border-dark-700 rounded-lg w-full max-w-md p-6 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-semibold text-dark-50">
                Change Password
              </h2>
              <button
                onClick={() => setShowPasswordForm(false)}
                className="text-dark-400 hover:text-dark-200 transition-colors"
              >
                <X size={20} />
              </button>
            </div>
            <p className="text-dark-400 text-sm mb-4">
              Change password for{' '}
              <span className="text-dark-200 font-medium">
                {passwordTarget.name}
              </span>
            </p>
            <form onSubmit={handleChangePassword} className="space-y-4">
              {formError && (
                <div className="p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-red-400 text-sm">
                  {formError}
                </div>
              )}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  New Password *
                </label>
                <Input
                  type="password"
                  value={passwordForm.password}
                  onChange={(e) =>
                    setPasswordForm((prev) => ({
                      ...prev,
                      password: e.target.value,
                    }))
                  }
                  placeholder="••••••••"
                  className="bg-dark-800 border-dark-600 text-dark-100"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  Confirm Password *
                </label>
                <Input
                  type="password"
                  value={passwordForm.confirmPassword}
                  onChange={(e) =>
                    setPasswordForm((prev) => ({
                      ...prev,
                      confirmPassword: e.target.value,
                    }))
                  }
                  placeholder="••••••••"
                  className="bg-dark-800 border-dark-600 text-dark-100"
                />
              </div>
              <div className="flex justify-end gap-3 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowPasswordForm(false)}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={submitting}>
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
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
          onClick={() => setShowDeleteConfirm(false)}
        >
          <div
            className="bg-dark-900 border border-dark-700 rounded-lg w-full max-w-md p-6 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-red-500/10 rounded-lg">
                <AlertTriangle className="text-red-400" size={24} />
              </div>
              <h2 className="text-lg font-semibold text-dark-50">
                Confirm Deletion
              </h2>
            </div>
            <p className="text-dark-300 mb-6">
              Are you sure you want to delete{' '}
              <span className="text-dark-100 font-medium">
                {deleteTarget.name}
              </span>
              ?{deleteTarget.type === 'database' && (
                <span className="block text-red-400 text-sm mt-1">
                  This will also drop the associated database user.
                </span>
              )}
              <span className="block text-dark-500 text-sm mt-1">
                This action cannot be undone.
              </span>
            </p>
            <div className="flex justify-end gap-3">
              <Button
                variant="outline"
                onClick={() => setShowDeleteConfirm(false)}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={handleDelete}
                disabled={deleting}
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
        <div className="fixed bottom-6 right-6 z-50 animate-in fade-in slide-in-from-bottom-2">
          <div
            className={`p-4 rounded-lg shadow-lg border ${
              toast.type === 'success'
                ? 'bg-green-900/90 border-green-700 text-green-200'
                : 'bg-red-900/90 border-red-700 text-red-200'
            }`}
          >
            {toast.message}
          </div>
        </div>
      )}
    </div>
  );
}
