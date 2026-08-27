'use client';

import { useState, useEffect } from 'react';
import { Users, Plus, Trash2, RefreshCw, Shield, Key, Activity, X } from 'lucide-react';

interface Role {
  id: string;
  name: string;
  description: string;
  is_system: boolean;
  permissions: Permission[];
  created_at: string;
}

interface Permission {
  id: string;
  resource: string;
  action: string;
}

interface UserSession {
  id: string;
  user_id: string;
  ip_address: string;
  user_agent: string;
  last_active_at: string;
  created_at: string;
}

interface UserActivity {
  id: string;
  user_id: string;
  action: string;
  resource: string;
  details: string;
  ip_address: string;
  created_at: string;
}

interface MultiUserStats {
  total_users: number;
  active_users: number;
  online_users: number;
  total_roles: number;
  total_sessions: number;
  activities_today: number;
}

const CARD = 'bg-white border border-gray-200 rounded-lg shadow-sm';
const CARD_HEADER = 'px-5 py-4 border-b border-gray-200';
const TH = 'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
const TD = 'px-4 py-3 text-sm text-gray-700';
const INPUT =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none';
const BTN_PRIMARY =
  'inline-flex items-center gap-2 rounded-md bg-blue-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-1';
const BTN_SECONDARY =
  'inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-1';

function formatDateTime(value?: string): string {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString();
}

function shortId(value?: string): string {
  if (!value) return '—';
  return value.length > 8 ? `${value.substring(0, 8)}...` : value;
}

export default function UsersPage() {
  const [stats, setStats] = useState<MultiUserStats | null>(null);
  const [roles, setRoles] = useState<Role[]>([]);
  const [sessions, setSessions] = useState<UserSession[]>([]);
  const [activities, setActivities] = useState<UserActivity[]>([]);
  const [permissions, setPermissions] = useState<Permission[]>([]);
  const [activeTab, setActiveTab] = useState('roles');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [showRoleModal, setShowRoleModal] = useState(false);
  const [newRole, setNewRole] = useState({ name: '', description: '', permissions: [] as string[] });

  const getToken = () => {
    if (typeof window === 'undefined') return '';
    try {
      return localStorage.getItem('access_token') || '';
    } catch {
      return '';
    }
  };

  const fetchAll = async () => {
    setLoading(true);
    setError('');
    const token = getToken();
    const headers = { Authorization: `Bearer ${token}` };
    try {
      const [statsRes, rolesRes, sessionsRes, activitiesRes, permsRes] = await Promise.all([
        fetch('/api/v1/multi-user/stats', { headers }),
        fetch('/api/v1/multi-user/roles', { headers }),
        fetch('/api/v1/multi-user/sessions', { headers }),
        fetch('/api/v1/multi-user/activities?limit=50', { headers }),
        fetch('/api/v1/multi-user/permissions', { headers }),
      ]);
      if (statsRes.ok) { const d = await statsRes.json(); setStats(d?.stats ?? null); }
      if (rolesRes.ok) { const d = await rolesRes.json(); setRoles(Array.isArray(d?.roles) ? d.roles : []); }
      if (sessionsRes.ok) { const d = await sessionsRes.json(); setSessions(Array.isArray(d?.sessions) ? d.sessions : []); }
      if (activitiesRes.ok) { const d = await activitiesRes.json(); setActivities(Array.isArray(d?.activities) ? d.activities : []); }
      if (permsRes.ok) { const d = await permsRes.json(); setPermissions(Array.isArray(d?.permissions) ? d.permissions : []); }
    } catch (e) {
      console.error(e);
      setError('Unable to load multi-user data. Please try again.');
    }
    setLoading(false);
  };

  useEffect(() => { fetchAll(); }, []);

  const createRole = async () => {
    const token = getToken();
    try {
      await fetch('/api/v1/multi-user/roles', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify(newRole),
      });
      setShowRoleModal(false);
      setNewRole({ name: '', description: '', permissions: [] });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Unable to create role. Please try again.');
    }
  };

  const deleteRole = async (id: string) => {
    if (!confirm('Delete this role?')) return;
    const token = getToken();
    try {
      await fetch(`/api/v1/multi-user/roles/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Unable to delete role. Please try again.');
    }
  };

  const deleteSession = async (id: string) => {
    const token = getToken();
    try {
      await fetch(`/api/v1/multi-user/sessions/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Unable to terminate session. Please try again.');
    }
  };

  const togglePermission = (perm: string) => {
    setNewRole(prev => ({
      ...prev,
      permissions: prev.permissions.includes(perm)
        ? prev.permissions.filter(p => p !== perm)
        : [...prev.permissions, perm],
    }));
  };

  const permGroups = (Array.isArray(permissions) ? permissions : []).reduce((acc, p) => {
    const resource = p?.resource || 'other';
    if (!acc[resource]) acc[resource] = [];
    acc[resource].push(p);
    return acc;
  }, {} as Record<string, Permission[]>);

  const tabs = [
    { id: 'roles', label: 'Roles & Permissions', icon: Shield },
    { id: 'sessions', label: 'Active Sessions', icon: Key },
    { id: 'activity', label: 'Activity Log', icon: Activity },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold text-gray-900">
            <Users className="w-5 h-5 text-gray-500" /> Multi-user Management
          </h1>
          <p className="mt-1 text-sm text-gray-600">Manage roles, permissions, sessions and user activity</p>
        </div>
        <button type="button" onClick={fetchAll} className={BTN_SECONDARY}>
          <RefreshCw className="w-4 h-4" /> Refresh
        </button>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
          {[
            { label: 'Total Users', value: stats.total_users ?? 0, icon: Users },
            { label: 'Active Users', value: stats.active_users ?? 0, icon: Users },
            { label: 'Online Now', value: stats.online_users ?? 0, icon: Activity },
            { label: 'Roles', value: stats.total_roles ?? 0, icon: Shield },
            { label: 'Sessions', value: stats.total_sessions ?? 0, icon: Key },
            { label: 'Activities Today', value: stats.activities_today ?? 0, icon: Activity },
          ].map((s) => (
            <div key={s.label} className={`${CARD} p-4`}>
              <div className="flex items-center gap-2 mb-1">
                <s.icon className="w-4 h-4 text-gray-400" />
                <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{s.label}</p>
              </div>
              <p className="text-2xl font-semibold text-gray-900">{s.value}</p>
            </div>
          ))}
        </div>
      )}

      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="Multi-user sections">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              aria-current={activeTab === tab.id ? 'page' : undefined}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 border-b-2 px-1 pb-3 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                activeTab === tab.id
                  ? 'border-blue-600 text-blue-700'
                  : 'border-transparent text-gray-600 hover:border-gray-300 hover:text-gray-900'
              }`}
            >
              <tab.icon className="w-4 h-4" /> {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {loading && (
        <div className={`${CARD} px-5 py-8 text-center text-sm text-gray-500`}>Loading…</div>
      )}

      {!loading && activeTab === 'roles' && (
        <div className="space-y-6">
          <div className={CARD}>
            <div className={`${CARD_HEADER} flex items-center justify-between gap-4`}>
              <h2 className="text-sm font-semibold text-gray-900">Roles</h2>
              <button type="button" onClick={() => setShowRoleModal(true)} className={BTN_PRIMARY}>
                <Plus className="w-4 h-4" /> Create Role
              </button>
            </div>
            {roles.length === 0 ? (
              <div className="px-5 py-10 text-center text-sm text-gray-500">No roles defined yet</div>
            ) : (
              <div className="grid gap-4 p-5 md:grid-cols-2 lg:grid-cols-3">
                {roles.map((role) => (
                  <div key={role.id} className="rounded-md border border-gray-200 p-4 hover:border-gray-300">
                    <div className="flex items-start justify-between gap-2">
                      <div>
                        <h3 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                          <Shield className="w-4 h-4 text-blue-600" /> {role.name}
                        </h3>
                        <p className="mt-1 text-sm text-gray-600">{role.description || 'No description'}</p>
                      </div>
                      {!role.is_system && (
                        <button
                          type="button"
                          aria-label={`Delete role ${role.name}`}
                          onClick={() => deleteRole(role.id)}
                          className="rounded-md p-1 text-red-600 hover:bg-red-50 hover:text-red-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      )}
                    </div>
                    {role.is_system && (
                      <span className="mt-2 inline-flex items-center rounded-md bg-blue-50 px-2 py-0.5 text-xs font-medium text-blue-700">System</span>
                    )}
                    <div className="mt-3 flex flex-wrap gap-1">
                      {(role.permissions ?? []).slice(0, 6).map((p) => (
                        <span key={p.id} className="inline-flex items-center rounded-md bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600">{p.resource}:{p.action}</span>
                      ))}
                      {(role.permissions?.length ?? 0) > 6 && (
                        <span className="inline-flex items-center rounded-md bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-500">+{(role.permissions?.length ?? 0) - 6} more</span>
                      )}
                    </div>
                    <p className="mt-2 text-xs text-gray-500">{role.permissions?.length || 0} permissions</p>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className={CARD}>
            <div className={CARD_HEADER}>
              <h2 className="text-sm font-semibold text-gray-900">Available Permissions</h2>
            </div>
            {Object.keys(permGroups).length === 0 ? (
              <div className="px-5 py-10 text-center text-sm text-gray-500">No permissions available</div>
            ) : (
              <div className="grid gap-3 p-5 md:grid-cols-2 lg:grid-cols-3">
                {Object.entries(permGroups).map(([resource, perms]) => (
                  <div key={resource} className="rounded-md border border-gray-200 p-3">
                    <h3 className="mb-2 text-sm font-semibold capitalize text-gray-900">{resource}</h3>
                    <div className="flex flex-wrap gap-1">
                      {(perms ?? []).map((p) => (
                        <span key={p.id} className="inline-flex items-center rounded-md bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600">{p.action}</span>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {!loading && activeTab === 'sessions' && (
        <div className={CARD}>
          <div className={CARD_HEADER}><h2 className="text-sm font-semibold text-gray-900">Active Sessions</h2></div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  <th className={TH}>User ID</th>
                  <th className={TH}>IP Address</th>
                  <th className={TH}>User Agent</th>
                  <th className={TH}>Last Active</th>
                  <th className={`${TH} text-right`}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {sessions.length === 0 ? (
                  <tr><td colSpan={5} className="px-4 py-10 text-center text-sm text-gray-500">No active sessions</td></tr>
                ) : sessions.map((s) => (
                  <tr key={s.id} className="border-b border-gray-100 hover:bg-gray-50">
                    <td className={`${TD} font-mono text-gray-900`}>{shortId(s.user_id)}</td>
                    <td className={TD}>{s.ip_address || '—'}</td>
                    <td className={`${TD} max-w-[200px] truncate`}>{s.user_agent || '—'}</td>
                    <td className={TD} suppressHydrationWarning>{formatDateTime(s.last_active_at)}</td>
                    <td className="px-4 py-3 text-right">
                      <button
                        type="button"
                        onClick={() => deleteSession(s.id)}
                        className="rounded-md px-2 py-1 text-sm font-medium text-red-600 hover:bg-red-50 hover:text-red-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
                      >
                        Terminate
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {!loading && activeTab === 'activity' && (
        <div className={CARD}>
          <div className={CARD_HEADER}><h2 className="text-sm font-semibold text-gray-900">Activity Log</h2></div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  <th className={TH}>User</th>
                  <th className={TH}>Action</th>
                  <th className={TH}>Resource</th>
                  <th className={TH}>Details</th>
                  <th className={TH}>IP</th>
                  <th className={TH}>Time</th>
                </tr>
              </thead>
              <tbody>
                {activities.length === 0 ? (
                  <tr><td colSpan={6} className="px-4 py-10 text-center text-sm text-gray-500">No activity recorded</td></tr>
                ) : activities.map((a) => (
                  <tr key={a.id} className="border-b border-gray-100 hover:bg-gray-50">
                    <td className={`${TD} font-mono text-gray-900`}>{shortId(a.user_id)}</td>
                    <td className="px-4 py-3">
                      <span className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${
                        a.action === 'create' ? 'bg-emerald-50 text-emerald-700' :
                        a.action === 'delete' ? 'bg-red-50 text-red-700' :
                        a.action === 'login' ? 'bg-blue-50 text-blue-700' :
                        'bg-gray-100 text-gray-700'
                      }`}>{a.action || '—'}</span>
                    </td>
                    <td className={TD}>{a.resource || '—'}</td>
                    <td className={`${TD} max-w-[200px] truncate`}>{a.details || '—'}</td>
                    <td className={TD}>{a.ip_address || '—'}</td>
                    <td className={TD} suppressHydrationWarning>{formatDateTime(a.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {showRoleModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40 p-4">
          <div className="w-full max-w-lg max-h-[80vh] overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg">
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <h2 className="text-sm font-semibold text-gray-900">Create Role</h2>
              <button
                type="button"
                aria-label="Close dialog"
                onClick={() => setShowRoleModal(false)}
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            <div className="space-y-4 p-5">
              <div>
                <label htmlFor="role-name" className="mb-1.5 block text-sm font-medium text-gray-700">Role name</label>
                <input
                  id="role-name"
                  value={newRole.name}
                  onChange={(e) => setNewRole({ ...newRole, name: e.target.value })}
                  placeholder="Role name"
                  className={INPUT}
                />
              </div>
              <div>
                <label htmlFor="role-description" className="mb-1.5 block text-sm font-medium text-gray-700">Description</label>
                <input
                  id="role-description"
                  value={newRole.description}
                  onChange={(e) => setNewRole({ ...newRole, description: e.target.value })}
                  placeholder="Description"
                  className={INPUT}
                />
              </div>
              <div>
                <p className="mb-2 text-sm font-medium text-gray-700">Permissions</p>
                {Object.keys(permGroups).length === 0 ? (
                  <p className="text-sm text-gray-500">No permissions available</p>
                ) : Object.entries(permGroups).map(([resource, perms]) => (
                  <div key={resource} className="mb-3">
                    <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">{resource}</p>
                    <div className="flex flex-wrap gap-1">
                      {(perms ?? []).map((p) => {
                        const key = `${p.resource}:${p.action}`;
                        const selected = newRole.permissions.includes(key);
                        return (
                          <button
                            key={p.id}
                            type="button"
                            aria-pressed={selected}
                            onClick={() => togglePermission(key)}
                            className={`rounded-md px-2 py-1 text-xs font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                              selected ? 'bg-blue-600 text-white hover:bg-blue-700' : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                            }`}
                          >
                            {p.action}
                          </button>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              <button type="button" onClick={() => setShowRoleModal(false)} className={BTN_SECONDARY}>Cancel</button>
              <button type="button" onClick={createRole} className={BTN_PRIMARY}>Create Role</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
