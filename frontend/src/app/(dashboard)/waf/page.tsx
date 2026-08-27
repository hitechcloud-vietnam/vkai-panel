"use client";

import { useState, useEffect } from "react";
import {
  Shield,
  Plus,
  Search,
  Filter,
  RefreshCw,
  AlertTriangle,
  CheckCircle,
  XCircle,
  Activity,
  BarChart3,
  Settings,
  Eye,
  Edit,
  Trash2,
  ToggleLeft,
  ToggleRight,
} from "lucide-react";

interface WAFRule {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  rule_type: string;
  severity: string;
  action: string;
  pattern: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

interface WAFPolicy {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  mode: string;
  paranoia_level: number;
  anomaly_threshold: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

interface WAFEvent {
  id: string;
  tenant_id: string;
  rule_id: string;
  source_ip: string;
  method: string;
  path: string;
  user_agent: string;
  attack_type: string;
  blocked: boolean;
  created_at: string;
}

interface WAFStats {
  total_rules: number;
  enabled_rules: number;
  total_events: number;
  blocked_events: number;
  top_attack_types: Array<{ type: string; count: number }>;
  top_source_ips: Array<{ ip: string; count: number }>;
}

export default function WAFPage() {
  const [activeTab, setActiveTab] = useState<"rules" | "policies" | "events" | "stats">("rules");
  const [rules, setRules] = useState<WAFRule[]>([]);
  const [policies, setPolicies] = useState<WAFPolicy[]>([]);
  const [events, setEvents] = useState<WAFEvent[]>([]);
  const [stats, setStats] = useState<WAFStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState("");
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [editingItem, setEditingItem] = useState<any>(null);

  useEffect(() => {
    fetchData();
  }, [activeTab]);

  const fetchData = async () => {
    setLoading(true);
    try {
      switch (activeTab) {
        case "rules":
          const rulesRes = await fetch("/api/v1/waf/rules");
          if (rulesRes.ok) {
            const rulesData = await rulesRes.json();
            setRules(rulesData.rules || []);
          }
          break;
        case "policies":
          const policiesRes = await fetch("/api/v1/waf/policies");
          if (policiesRes.ok) {
            const policiesData = await policiesRes.json();
            setPolicies(policiesData.policies || []);
          }
          break;
        case "events":
          const eventsRes = await fetch("/api/v1/waf/events");
          if (eventsRes.ok) {
            const eventsData = await eventsRes.json();
            setEvents(eventsData.events || []);
          }
          break;
        case "stats":
          const statsRes = await fetch("/api/v1/waf/stats");
          if (statsRes.ok) {
            const statsData = await statsRes.json();
            setStats(statsData);
          }
          break;
      }
    } catch (error) {
      console.error("Failed to fetch WAF data:", error);
    } finally {
      setLoading(false);
    }
  };

  const handleToggleRule = async (ruleId: string, enabled: boolean) => {
    try {
      const res = await fetch(`/api/v1/waf/rules/${ruleId}/toggle`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enabled }),
      });
      if (res.ok) {
        fetchData();
      }
    } catch (error) {
      console.error("Failed to toggle rule:", error);
    }
  };

  const handleDeleteRule = async (ruleId: string) => {
    if (!confirm("Are you sure you want to delete this rule?")) return;
    try {
      const res = await fetch(`/api/v1/waf/rules/${ruleId}`, {
        method: "DELETE",
      });
      if (res.ok) {
        fetchData();
      }
    } catch (error) {
      console.error("Failed to delete rule:", error);
    }
  };

  const handleDeletePolicy = async (policyId: string) => {
    if (!confirm("Are you sure you want to delete this policy?")) return;
    try {
      const res = await fetch(`/api/v1/waf/policies/${policyId}`, {
        method: "DELETE",
      });
      if (res.ok) {
        fetchData();
      }
    } catch (error) {
      console.error("Failed to delete policy:", error);
    }
  };

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case "critical":
        return "text-red-600 bg-red-100";
      case "high":
        return "text-orange-600 bg-orange-100";
      case "medium":
        return "text-yellow-600 bg-yellow-100";
      case "low":
        return "text-green-600 bg-green-100";
      default:
        return "text-gray-600 bg-gray-100";
    }
  };

  const getActionColor = (action: string) => {
    switch (action) {
      case "block":
        return "text-red-600 bg-red-100";
      case "log":
        return "text-blue-600 bg-blue-100";
      case "allow":
        return "text-green-600 bg-green-100";
      default:
        return "text-gray-600 bg-gray-100";
    }
  };

  const filteredRules = rules.filter(
    (rule) =>
      rule.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      rule.description.toLowerCase().includes(searchTerm.toLowerCase()) ||
      rule.rule_type.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const filteredPolicies = policies.filter(
    (policy) =>
      policy.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      policy.description.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const filteredEvents = events.filter(
    (event) =>
      event.source_ip.includes(searchTerm) ||
      event.path.toLowerCase().includes(searchTerm.toLowerCase()) ||
      event.attack_type.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 flex items-center gap-2">
            <Shield className="w-6 h-6 text-blue-600" />
            WAF Pro - Web Application Firewall
          </h1>
          <p className="text-gray-600 mt-1">
            Protect your web applications from common attacks and vulnerabilities
          </p>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
          className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
        >
          <Plus className="w-4 h-4" />
          Create Rule
        </button>
      </div>

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <nav className="flex space-x-8">
          {[
            { id: "rules", label: "Rules", icon: Shield },
            { id: "policies", label: "Policies", icon: Settings },
            { id: "events", label: "Events", icon: Activity },
            { id: "stats", label: "Statistics", icon: BarChart3 },
          ].map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id as any)}
              className={`flex items-center gap-2 py-4 px-1 border-b-2 font-medium text-sm ${
                activeTab === tab.id
                  ? "border-blue-500 text-blue-600"
                  : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
              }`}
            >
              <tab.icon className="w-4 h-4" />
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {/* Search and Filter */}
      <div className="flex items-center gap-4">
        <div className="flex-1 relative">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 w-4 h-4" />
          <input
            type="text"
            placeholder="Search..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent"
          />
        </div>
        <button
          onClick={fetchData}
          className="flex items-center gap-2 px-4 py-2 border border-gray-300 rounded-lg hover:bg-gray-50"
        >
          <RefreshCw className="w-4 h-4" />
          Refresh
        </button>
      </div>

      {/* Content */}
      {loading ? (
        <div className="flex items-center justify-center py-12">
          <RefreshCw className="w-6 h-6 animate-spin text-blue-600" />
          <span className="ml-2 text-gray-600">Loading...</span>
        </div>
      ) : (
        <>
          {/* Rules Tab */}
          {activeTab === "rules" && (
            <div className="bg-white rounded-lg border border-gray-200">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Name
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Type
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Severity
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Action
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Status
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Actions
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {filteredRules.map((rule) => (
                      <tr key={rule.id} className="hover:bg-gray-50">
                        <td className="px-6 py-4">
                          <div>
                            <div className="font-medium text-gray-900">{rule.name}</div>
                            <div className="text-sm text-gray-500">{rule.description}</div>
                          </div>
                        </td>
                        <td className="px-6 py-4">
                          <span className="px-2 py-1 text-xs font-medium bg-gray-100 text-gray-800 rounded">
                            {rule.rule_type}
                          </span>
                        </td>
                        <td className="px-6 py-4">
                          <span
                            className={`px-2 py-1 text-xs font-medium rounded ${getSeverityColor(
                              rule.severity
                            )}`}
                          >
                            {rule.severity}
                          </span>
                        </td>
                        <td className="px-6 py-4">
                          <span
                            className={`px-2 py-1 text-xs font-medium rounded ${getActionColor(
                              rule.action
                            )}`}
                          >
                            {rule.action}
                          </span>
                        </td>
                        <td className="px-6 py-4">
                          <button
                            onClick={() => handleToggleRule(rule.id, !rule.enabled)}
                            className={`flex items-center gap-1 ${
                              rule.enabled ? "text-green-600" : "text-gray-400"
                            }`}
                          >
                            {rule.enabled ? (
                              <ToggleRight className="w-5 h-5" />
                            ) : (
                              <ToggleLeft className="w-5 h-5" />
                            )}
                            <span className="text-sm">{rule.enabled ? "Enabled" : "Disabled"}</span>
                          </button>
                        </td>
                        <td className="px-6 py-4">
                          <div className="flex items-center gap-2">
                            <button
                              onClick={() => setEditingItem(rule)}
                              className="p-1 text-gray-400 hover:text-blue-600"
                            >
                              <Edit className="w-4 h-4" />
                            </button>
                            <button
                              onClick={() => handleDeleteRule(rule.id)}
                              className="p-1 text-gray-400 hover:text-red-600"
                            >
                              <Trash2 className="w-4 h-4" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {filteredRules.length === 0 && (
                <div className="text-center py-12 text-gray-500">
                  No WAF rules found. Create your first rule to get started.
                </div>
              )}
            </div>
          )}

          {/* Policies Tab */}
          {activeTab === "policies" && (
            <div className="bg-white rounded-lg border border-gray-200">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Name
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Mode
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Paranoia Level
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Threshold
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Status
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Actions
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {filteredPolicies.map((policy) => (
                      <tr key={policy.id} className="hover:bg-gray-50">
                        <td className="px-6 py-4">
                          <div>
                            <div className="font-medium text-gray-900">{policy.name}</div>
                            <div className="text-sm text-gray-500">{policy.description}</div>
                          </div>
                        </td>
                        <td className="px-6 py-4">
                          <span
                            className={`px-2 py-1 text-xs font-medium rounded ${
                              policy.mode === "prevention"
                                ? "text-red-600 bg-red-100"
                                : "text-blue-600 bg-blue-100"
                            }`}
                          >
                            {policy.mode}
                          </span>
                        </td>
                        <td className="px-6 py-4">
                          <div className="flex items-center gap-1">
                            {[1, 2, 3, 4].map((level) => (
                              <div
                                key={level}
                                className={`w-3 h-3 rounded-full ${
                                  level <= policy.paranoia_level
                                    ? "bg-red-500"
                                    : "bg-gray-200"
                                }`}
                              />
                            ))}
                            <span className="ml-2 text-sm text-gray-600">
                              Level {policy.paranoia_level}
                            </span>
                          </div>
                        </td>
                        <td className="px-6 py-4 text-sm text-gray-600">
                          {policy.anomaly_threshold}
                        </td>
                        <td className="px-6 py-4">
                          <span
                            className={`flex items-center gap-1 ${
                              policy.enabled ? "text-green-600" : "text-gray-400"
                            }`}
                          >
                            {policy.enabled ? (
                              <CheckCircle className="w-4 h-4" />
                            ) : (
                              <XCircle className="w-4 h-4" />
                            )}
                            <span className="text-sm">
                              {policy.enabled ? "Active" : "Inactive"}
                            </span>
                          </span>
                        </td>
                        <td className="px-6 py-4">
                          <div className="flex items-center gap-2">
                            <button
                              onClick={() => setEditingItem(policy)}
                              className="p-1 text-gray-400 hover:text-blue-600"
                            >
                              <Edit className="w-4 h-4" />
                            </button>
                            <button
                              onClick={() => handleDeletePolicy(policy.id)}
                              className="p-1 text-gray-400 hover:text-red-600"
                            >
                              <Trash2 className="w-4 h-4" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {filteredPolicies.length === 0 && (
                <div className="text-center py-12 text-gray-500">
                  No WAF policies found. Create your first policy to get started.
                </div>
              )}
            </div>
          )}

          {/* Events Tab */}
          {activeTab === "events" && (
            <div className="bg-white rounded-lg border border-gray-200">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Time
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Source IP
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Method
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Path
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Attack Type
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                        Status
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {filteredEvents.map((event) => (
                      <tr key={event.id} className="hover:bg-gray-50">
                        <td className="px-6 py-4 text-sm text-gray-600">
                          {new Date(event.created_at).toLocaleString()}
                        </td>
                        <td className="px-6 py-4 text-sm font-mono text-gray-900">
                          {event.source_ip}
                        </td>
                        <td className="px-6 py-4">
                          <span className="px-2 py-1 text-xs font-medium bg-gray-100 text-gray-800 rounded">
                            {event.method}
                          </span>
                        </td>
                        <td className="px-6 py-4 text-sm text-gray-600 max-w-xs truncate">
                          {event.path}
                        </td>
                        <td className="px-6 py-4">
                          <span className="px-2 py-1 text-xs font-medium bg-red-100 text-red-800 rounded">
                            {event.attack_type}
                          </span>
                        </td>
                        <td className="px-6 py-4">
                          <span
                            className={`flex items-center gap-1 ${
                              event.blocked ? "text-red-600" : "text-yellow-600"
                            }`}
                          >
                            {event.blocked ? (
                              <XCircle className="w-4 h-4" />
                            ) : (
                              <AlertTriangle className="w-4 h-4" />
                            )}
                            <span className="text-sm">
                              {event.blocked ? "Blocked" : "Detected"}
                            </span>
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {filteredEvents.length === 0 && (
                <div className="text-center py-12 text-gray-500">
                  No WAF events found. Events will appear here when attacks are detected.
                </div>
              )}
            </div>
          )}

          {/* Stats Tab */}
          {activeTab === "stats" && stats && (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
              {/* Stats Cards */}
              <div className="bg-white rounded-lg border border-gray-200 p-6">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-sm font-medium text-gray-600">Total Rules</p>
                    <p className="text-2xl font-bold text-gray-900">{stats.total_rules}</p>
                  </div>
                  <Shield className="w-8 h-8 text-blue-600" />
                </div>
                <p className="text-sm text-gray-500 mt-2">
                  {stats.enabled_rules} enabled
                </p>
              </div>

              <div className="bg-white rounded-lg border border-gray-200 p-6">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-sm font-medium text-gray-600">Total Events</p>
                    <p className="text-2xl font-bold text-gray-900">{stats.total_events}</p>
                  </div>
                  <Activity className="w-8 h-8 text-green-600" />
                </div>
                <p className="text-sm text-gray-500 mt-2">Last 7 days</p>
              </div>

              <div className="bg-white rounded-lg border border-gray-200 p-6">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-sm font-medium text-gray-600">Blocked Attacks</p>
                    <p className="text-2xl font-bold text-red-600">{stats.blocked_events}</p>
                  </div>
                  <XCircle className="w-8 h-8 text-red-600" />
                </div>
                <p className="text-sm text-gray-500 mt-2">
                  {stats.total_events > 0
                    ? Math.round((stats.blocked_events / stats.total_events) * 100)
                    : 0}
                  % block rate
                </p>
              </div>

              <div className="bg-white rounded-lg border border-gray-200 p-6">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-sm font-medium text-gray-600">Detection Rate</p>
                    <p className="text-2xl font-bold text-green-600">
                      {stats.total_events > 0
                        ? Math.round((stats.total_events / stats.total_events) * 100)
                        : 0}
                      %
                    </p>
                  </div>
                  <Eye className="w-8 h-8 text-green-600" />
                </div>
                <p className="text-sm text-gray-500 mt-2">All events captured</p>
              </div>

              {/* Top Attack Types */}
              <div className="md:col-span-2 bg-white rounded-lg border border-gray-200 p-6">
                <h3 className="text-lg font-semibold text-gray-900 mb-4">Top Attack Types</h3>
                <div className="space-y-3">
                  {stats.top_attack_types.map((item, index) => (
                    <div key={index} className="flex items-center justify-between">
                      <span className="text-sm text-gray-600">{item.type}</span>
                      <div className="flex items-center gap-2">
                        <div className="w-32 bg-gray-200 rounded-full h-2">
                          <div
                            className="bg-red-500 h-2 rounded-full"
                            style={{
                              width: `${
                                stats.top_attack_types[0]
                                  ? (item.count / stats.top_attack_types[0].count) * 100
                                  : 0
                              }%`,
                            }}
                          />
                        </div>
                        <span className="text-sm font-medium text-gray-900 w-12 text-right">
                          {item.count}
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              {/* Top Source IPs */}
              <div className="md:col-span-2 bg-white rounded-lg border border-gray-200 p-6">
                <h3 className="text-lg font-semibold text-gray-900 mb-4">Top Source IPs</h3>
                <div className="space-y-3">
                  {stats.top_source_ips.map((item, index) => (
                    <div key={index} className="flex items-center justify-between">
                      <span className="text-sm font-mono text-gray-600">{item.ip}</span>
                      <div className="flex items-center gap-2">
                        <div className="w-32 bg-gray-200 rounded-full h-2">
                          <div
                            className="bg-orange-500 h-2 rounded-full"
                            style={{
                              width: `${
                                stats.top_source_ips[0]
                                  ? (item.count / stats.top_source_ips[0].count) * 100
                                  : 0
                              }%`,
                            }}
                          />
                        </div>
                        <span className="text-sm font-medium text-gray-900 w-12 text-right">
                          {item.count}
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}
        </>
      )}

      {/* Create/Edit Modal Placeholder */}
      {showCreateModal && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 w-full max-w-md">
            <h2 className="text-lg font-semibold mb-4">Create WAF Rule</h2>
            <p className="text-gray-600 mb-4">
              Rule creation form will be implemented here.
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setShowCreateModal(false)}
                className="px-4 py-2 border border-gray-300 rounded-lg hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                onClick={() => setShowCreateModal(false)}
                className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
              >
                Create
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
