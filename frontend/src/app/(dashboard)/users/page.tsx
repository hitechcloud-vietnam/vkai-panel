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

export default function UsersPage() {
  const [stats, setStats] = useState<MultiUserStats | null>(null);
  const [roles, setRoles] = useState<Role[]>([]);
  const [sessions, setSessions] = useState<UserSession[]>([]);
  const [activities, setActivities] = useState<UserActivity[]>([]);
  const [permissions, setPermissions] = useState<Permission[]>([]);
  const [activeTab, setActiveTab] = useState('roles');
  const [loading, setLoading] = useState(true);
  const [showRoleModal, setShowRoleModal] = useState(false);
  const [newRole, setNewRole] = useState({ name: '', description: '', permissions: [] as string[] });

  const getToken = () => localStorage.getItem('access_token') || '';

  const fetchAll = async () => {
    setLoading(true);
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
      if (statsRes.ok) { const d = await statsRes.json(); setStats(d.stats); }
      if (rolesRes.ok) { const d = await rolesRes.json(); setRoles(d.roles || []); }
      if (sessionsRes.ok) { const d = await sessionsRes.json(); setSessions(d.sessions || []); }
      if (activitiesRes.ok) { const d = await activitiesRes.json(); setActivities(d.activities || []); }
      if (permsRes.ok) { const d = await permsRes.json(); setPermissions(d.permissions || []); }
    } catch (e) { console.error(e); }
    setLoading(false);
  };

  useEffect(() => { fetchAll(); }, []);

  const createRole = async () => {
    const token = getToken();
    await fetch('/api/v1/multi-user/roles', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify(newRole),
    });
    setShowRoleModal(false);
    setNewRole({ name: '', description: '', permissions: [] });
    fetchAll();
  };

  const deleteRole = async (id: string) => {
    if (!confirm('Delete this role?')) return;
    const token = getToken();
    await fetch(`/api/v1/multi-user/roles/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
    fetchAll();
  };

  const deleteSession = async (id: string) => {
    const token = getToken();
    await fetch(`/api/v1/multi-user/sessions/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
    fetchAll();
  };

  const togglePermission = (perm: string) => {
    setNewRole(prev => ({
      ...prev,
      permissions: prev.permissions.includes(perm)
        ? prev.permissions.filter(p => p !== perm)
        : [...prev.permissions, perm],
    }));
  };

  const permGroups = permissions.reduce((acc, p) => {
    if (!acc[p.resource]) acc[p.resource] = [];
    acc[p.resource].push(p);
    return acc;
  }, {} as Record<string, Permission[]>);

  const tabs = [
    { id: 'roles', label: 'Roles & Permissions', icon: Shield },
    { id: 'sessions', label: 'Active Sessions', icon: Key },
    { id: 'activity', label: 'Activity Log', icon: Activity },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
            <Users className="w-6 h-6" /> Multi-user Management
          </h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">Manage roles, permissions, sessions and user activity</p>
        </div>
        <button onClick={fetchAll} className="flex items-center gap-2 px-4 py-2 bg-gray-100 dark:bg-gray-700 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600">
          <RefreshCw className="w-4 h-4" /> Refresh
        </button>
      </div>

      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
          {[
            { label: 'Total Users', value: stats.total_users, icon: Users },
            { label: 'Active Users', value: stats.active_users, icon: Users },
            { label: 'Online Now', value: stats.online_users, icon: Activity },
            { label: 'Roles', value: stats.total_roles, icon: Shield },
            { label: 'Sessions', value: stats.total_sessions, icon: Key },
            { label: 'Activities Today', value: stats.activities_today, icon: Activity },
          ].map((s) => (
            <div key={s.label} className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700">
              <div className="flex items-center gap-2 mb-1">
                <s.icon className="w-4 h-4 text-gray-400" />
                <p className="text-sm text-gray-500 dark:text-gray-400">{s.label}</p>
              </div>
              <p className="text-2xl font-bold text-gray-900 dark:text-white">{s.value}</p>
            </div>
          ))}
        </div>
      )}

      <div className="border-b border-gray-200 dark:border-gray-700">
        <nav className="flex gap-4">
          {tabs.map((tab) => (
            <button key={tab.id} onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 py-3 px-1 border-b-2 text-sm font-medium ${
                activeTab === tab.id ? 'border-blue-500 text-blue-600 dark:text-blue-400' : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'
              }`}>
              <tab.icon className="w-4 h-4" /> {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {activeTab === 'roles' && (
        <div className="space-y-4">
          <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
            <div className="p-4 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
              <h3 className="font-semibold text-gray-900 dark:text-white">Roles</h3>
              <button onClick={() => setShowRoleModal(true)} className="flex items-center gap-1 px-3 py-1.5 bg-blue-600 text-white rounded-lg text-sm hover:bg-blue-700">
                <Plus className="w-4 h-4" /> Create Role
              </button>
            </div>
            <div className="grid gap-4 p-4 md:grid-cols-2 lg:grid-cols-3">
              {roles.map((role) => (
                <div key={role.id} className="border border-gray-200 dark:border-gray-700 rounded-lg p-4 hover:border-blue-300 dark:hover:border-blue-600 transition-colors">
                  <div className="flex items-start justify-between">
                    <div>
                      <h4 className="font-medium text-gray-900 dark:text-white flex items-center gap-2">
                        <Shield className="w-4 h-4 text-blue-500" /> {role.name}
                      </h4>
                      <p className="text-sm text-gray-500 mt-1">{role.description || 'No description'}</p>
                    </div>
                    {!role.is_system && (
                      <button onClick={() => deleteRole(role.id)} className="text-red-500 hover:text-red-700"><Trash2 className="w-4 h-4" /></button>
                    )}
                  </div>
                  {role.is_system && <span className="inline-block mt-2 px-2 py-0.5 text-xs bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400 rounded">System</span>}
                  <div className="mt-3 flex flex-wrap gap-1">
                    {role.permissions?.slice(0, 6).map((p) => (
                      <span key={p.id} className="px-1.5 py-0.5 text-xs bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400 rounded">{p.resource}:{p.action}</span>
                    ))}
                    {role.permissions && role.permissions.length > 6 && (
                      <span className="px-1.5 py-0.5 text-xs bg-gray-100 dark:bg-gray-700 text-gray-500 rounded">+{role.permissions.length - 6} more</span>
                    )}
                  </div>
                  <p className="text-xs text-gray-400 mt-2">{role.permissions?.length || 0} permissions</p>
                </div>
              ))}
            </div>
          </div>

          <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
            <div className="p-4 border-b border-gray-200 dark:border-gray-700">
              <h3 className="font-semibold text-gray-900 dark:text-white">Available Permissions</h3>
            </div>
            <div className="p-4 grid gap-3 md:grid-cols-2 lg:grid-cols-3">
              {Object.entries(permGroups).map(([resource, perms]) => (
                <div key={resource} className="border border-gray-200 dark:border-gray-700 rounded-lg p-3">
                  <h4 className="font-medium text-gray-900 dark:text-white text-sm capitalize mb-2">{resource}</h4>
                  <div className="flex gap-1">
                    {perms.map((p) => (
                      <span key={p.id} className="px-2 py-0.5 text-xs bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400 rounded">{p.action}</span>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {activeTab === 'sessions' && (
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700"><h3 className="font-semibold text-gray-900 dark:text-white">Active Sessions</h3></div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-700/50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">User ID</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">IP Address</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">User Agent</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Last Active</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {sessions.length === 0 ? (
                  <tr><td colSpan={5} className="px-4 py-8 text-center text-gray-500">No active sessions</td></tr>
                ) : sessions.map((s) => (
                  <tr key={s.id} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                    <td className="px-4 py-3 text-sm font-mono text-gray-900 dark:text-white">{s.user_id.substring(0, 8)}...</td>
                    <td className="px-4 py-3 text-sm text-gray-500">{s.ip_address || '—'}</td>
                    <td className="px-4 py-3 text-sm text-gray-500 truncate max-w-[200px]">{s.user_agent || '—'}</td>
                    <td className="px-4 py-3 text-sm text-gray-500">{new Date(s.last_active_at).toLocaleString()}</td>
                    <td className="px-4 py-3 text-right"><button onClick={() => deleteSession(s.id)} className="text-red-600 hover:text-red-800 text-sm">Terminate</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {activeTab === 'activity' && (
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700"><h3 className="font-semibold text-gray-900 dark:text-white">Activity Log</h3></div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-700/50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">User</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Action</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Resource</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Details</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">IP</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Time</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {activities.length === 0 ? (
                  <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-500">No activity recorded</td></tr>
                ) : activities.map((a) => (
                  <tr key={a.id} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                    <td className="px-4 py-3 text-sm font-mono text-gray-900 dark:text-white">{a.user_id.substring(0, 8)}...</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-1 text-xs rounded-full ${
                        a.action === 'create' ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' :
                        a.action === 'delete' ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400' :
                        a.action === 'login' ? 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400' :
                        'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-400'
                      }`}>{a.action}</span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500">{a.resource || '—'}</td>
                    <td className="px-4 py-3 text-sm text-gray-500 truncate max-w-[200px]">{a.details || '—'}</td>
                    <td className="px-4 py-3 text-sm text-gray-500">{a.ip_address || '—'}</td>
                    <td className="px-4 py-3 text-sm text-gray-500">{new Date(a.created_at).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {showRoleModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-lg max-h-[80vh] overflow-y-auto">
            <div className="flex justify-between items-center mb-4">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Create Role</h3>
              <button onClick={() => setShowRoleModal(false)}><X className="w-5 h-5 text-gray-500" /></button>
            </div>
            <div className="space-y-3">
              <input value={newRole.name} onChange={(e) => setNewRole({ ...newRole, name: e.target.value })} placeholder="Role name"
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
              <input value={newRole.description} onChange={(e) => setNewRole({ ...newRole, description: e.target.value })} placeholder="Description"
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
              <div>
                <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Permissions</p>
                {Object.entries(permGroups).map(([resource, perms]) => (
                  <div key={resource} className="mb-2">
                    <p className="text-xs font-medium text-gray-500 uppercase mb-1">{resource}</p>
                    <div className="flex flex-wrap gap-1">
                      {perms.map((p) => {
                        const key = `${p.resource}:${p.action}`;
                        const selected = newRole.permissions.includes(key);
                        return (
                          <button key={p.id} onClick={() => togglePermission(key)}
                            className={`px-2 py-1 text-xs rounded ${selected ? 'bg-blue-600 text-white' : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400 hover:bg-gray-200 dark:hover:bg-gray-600'}`}>
                            {p.action}
                          </button>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-4">
              <button onClick={() => setShowRoleModal(false)} className="px-4 py-2 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">Cancel</button>
              <button onClick={createRole} className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700">Create Role</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
