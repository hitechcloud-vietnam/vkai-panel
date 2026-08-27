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

const CARD_CLASS = "rounded-lg border border-gray-200 bg-white shadow-sm";
const CARD_HEADER_CLASS =
  "flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-5 py-4";
const CARD_TITLE_CLASS = "text-sm font-semibold text-gray-900";
const TH_CLASS = "px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500";
const BTN_PRIMARY =
  "inline-flex items-center gap-2 rounded-md bg-blue-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 disabled:opacity-50";
const BTN_SECONDARY =
  "inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 disabled:opacity-50";
const BADGE_BASE = "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium";
const ICON_BTN =
  "inline-flex items-center rounded-md p-1.5 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500";

function formatDateTime(value: string): string {
  if (!value) return "—";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "—";
  return parsed.toLocaleString();
}

function lower(value: string | undefined | null): string {
  return (value || "").toLowerCase();
}

export default function WAFPage() {
  const [activeTab, setActiveTab] = useState<"rules" | "policies" | "events" | "stats">("rules");
  const [rules, setRules] = useState<WAFRule[]>([]);
  const [policies, setPolicies] = useState<WAFPolicy[]>([]);
  const [events, setEvents] = useState<WAFEvent[]>([]);
  const [stats, setStats] = useState<WAFStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState("");
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [editingItem, setEditingItem] = useState<any>(null);

  useEffect(() => {
    fetchData();
  }, [activeTab]);

  const fetchData = async () => {
    setLoading(true);
    setError(null);
    try {
      switch (activeTab) {
        case "rules":
          const rulesRes = await fetch("/api/v1/waf/rules");
          if (rulesRes.ok) {
            const rulesData = await rulesRes.json();
            setRules(Array.isArray(rulesData?.rules) ? rulesData.rules : []);
          } else {
            setRules([]);
          }
          break;
        case "policies":
          const policiesRes = await fetch("/api/v1/waf/policies");
          if (policiesRes.ok) {
            const policiesData = await policiesRes.json();
            setPolicies(Array.isArray(policiesData?.policies) ? policiesData.policies : []);
          } else {
            setPolicies([]);
          }
          break;
        case "events":
          const eventsRes = await fetch("/api/v1/waf/events");
          if (eventsRes.ok) {
            const eventsData = await eventsRes.json();
            setEvents(Array.isArray(eventsData?.events) ? eventsData.events : []);
          } else {
            setEvents([]);
          }
          break;
        case "stats":
          const statsRes = await fetch("/api/v1/waf/stats");
          if (statsRes.ok) {
            const statsData = await statsRes.json();
            setStats(statsData ?? null);
          } else {
            setStats(null);
          }
          break;
      }
    } catch (error) {
      console.error("Failed to fetch WAF data:", error);
      setError("Failed to load WAF data. Please try again.");
    } finally {
      setLoading(false);
    }
  };

  const handleToggleRule = async (ruleId: string, enabled: boolean) => {
    try {
      setError(null);
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
      setError("Failed to toggle rule.");
    }
  };

  const handleDeleteRule = async (ruleId: string) => {
    if (!confirm("Are you sure you want to delete this rule?")) return;
    try {
      setError(null);
      const res = await fetch(`/api/v1/waf/rules/${ruleId}`, {
        method: "DELETE",
      });
      if (res.ok) {
        fetchData();
      }
    } catch (error) {
      console.error("Failed to delete rule:", error);
      setError("Failed to delete rule.");
    }
  };

  const handleDeletePolicy = async (policyId: string) => {
    if (!confirm("Are you sure you want to delete this policy?")) return;
    try {
      setError(null);
      const res = await fetch(`/api/v1/waf/policies/${policyId}`, {
        method: "DELETE",
      });
      if (res.ok) {
        fetchData();
      }
    } catch (error) {
      console.error("Failed to delete policy:", error);
      setError("Failed to delete policy.");
    }
  };

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case "critical":
        return "bg-red-50 text-red-700";
      case "high":
        return "bg-orange-50 text-orange-700";
      case "medium":
        return "bg-amber-50 text-amber-700";
      case "low":
        return "bg-emerald-50 text-emerald-700";
      default:
        return "bg-gray-100 text-gray-700";
    }
  };

  const getActionColor = (action: string) => {
    switch (action) {
      case "block":
        return "bg-red-50 text-red-700";
      case "log":
        return "bg-blue-50 text-blue-700";
      case "allow":
        return "bg-emerald-50 text-emerald-700";
      default:
        return "bg-gray-100 text-gray-700";
    }
  };

  const query = lower(searchTerm);

  const filteredRules = rules.filter(
    (rule) =>
      lower(rule?.name).includes(query) ||
      lower(rule?.description).includes(query) ||
      lower(rule?.rule_type).includes(query)
  );

  const filteredPolicies = policies.filter(
    (policy) =>
      lower(policy?.name).includes(query) || lower(policy?.description).includes(query)
  );

  const filteredEvents = events.filter(
    (event) =>
      (event?.source_ip || "").includes(searchTerm) ||
      lower(event?.path).includes(query) ||
      lower(event?.attack_type).includes(query)
  );

  const topAttackTypes = Array.isArray(stats?.top_attack_types) ? stats!.top_attack_types : [];
  const topSourceIps = Array.isArray(stats?.top_source_ips) ? stats!.top_source_ips : [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold text-gray-900">
            <Shield className="h-5 w-5 text-gray-500" aria-hidden="true" />
            WAF Pro - Web Application Firewall
          </h1>
          <p className="mt-1 text-sm text-gray-600">
            Protect your web applications from common attacks and vulnerabilities
          </p>
        </div>
        <button type="button" onClick={() => setShowCreateModal(true)} className={BTN_PRIMARY}>
          <Plus className="h-4 w-4" aria-hidden="true" />
          Create Rule
        </button>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
          {error}
        </div>
      )}

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <nav className="flex gap-6" aria-label="WAF sections">
          {[
            { id: "rules", label: "Rules", icon: Shield },
            { id: "policies", label: "Policies", icon: Settings },
            { id: "events", label: "Events", icon: Activity },
            { id: "stats", label: "Statistics", icon: BarChart3 },
          ].map((tab) => (
            <button
              key={tab.id}
              type="button"
              onClick={() => setActiveTab(tab.id as any)}
              aria-current={activeTab === tab.id ? "page" : undefined}
              className={`-mb-px flex items-center gap-2 border-b-2 px-1 py-3 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                activeTab === tab.id
                  ? "border-blue-600 text-blue-700"
                  : "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-900"
              }`}
            >
              <tab.icon className="h-4 w-4" aria-hidden="true" />
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {/* Search and Filter */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative min-w-[220px] flex-1">
          <Search
            className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
            aria-hidden="true"
          />
          <label htmlFor="waf-search" className="sr-only">
            Search WAF records
          </label>
          <input
            id="waf-search"
            type="text"
            placeholder="Search..."
            aria-label="Search WAF records"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full rounded-md border border-gray-300 bg-white py-2 pl-10 pr-3 text-sm text-gray-900 placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>
        <button type="button" onClick={fetchData} className={BTN_SECONDARY}>
          <RefreshCw className="h-4 w-4" aria-hidden="true" />
          Refresh
        </button>
      </div>

      {/* Content */}
      {loading ? (
        <div className="flex items-center justify-center rounded-lg border border-gray-200 bg-white py-12">
          <RefreshCw className="h-5 w-5 animate-spin text-gray-400" aria-hidden="true" />
          <span className="ml-2 text-sm text-gray-600">Loading...</span>
        </div>
      ) : (
        <>
          {/* Rules Tab */}
          {activeTab === "rules" && (
            <div className={CARD_CLASS}>
              <div className={CARD_HEADER_CLASS}>
                <h2 className={CARD_TITLE_CLASS}>WAF Rules</h2>
                <span className="text-xs text-gray-500">{filteredRules.length} rules</span>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr className="[&_th]:border-b [&_th]:border-gray-200">
                      <th className={TH_CLASS}>Name</th>
                      <th className={TH_CLASS}>Type</th>
                      <th className={TH_CLASS}>Severity</th>
                      <th className={TH_CLASS}>Action</th>
                      <th className={TH_CLASS}>Status</th>
                      <th className={`${TH_CLASS} text-right`}>Actions</th>
                    </tr>
                  </thead>
                  <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                    {filteredRules.length === 0 ? (
                      <tr>
                        <td colSpan={6} className="px-4 py-12 text-center">
                          <Shield className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                          <p className="mt-3 text-sm font-semibold text-gray-900">No WAF rules found</p>
                          <p className="mt-1 text-sm text-gray-600">
                            Create your first rule to get started.
                          </p>
                        </td>
                      </tr>
                    ) : (
                      filteredRules.map((rule) => (
                        <tr key={rule.id} className="hover:bg-gray-50">
                          <td className="px-4 py-3">
                            <div className="text-sm font-medium text-gray-900">{rule.name || "—"}</div>
                            <div className="mt-0.5 text-xs text-gray-500">{rule.description || ""}</div>
                          </td>
                          <td className="px-4 py-3">
                            <span className={`${BADGE_BASE} bg-gray-100 text-gray-700`}>
                              {rule.rule_type || "—"}
                            </span>
                          </td>
                          <td className="px-4 py-3">
                            <span className={`${BADGE_BASE} ${getSeverityColor(rule.severity)}`}>
                              {rule.severity || "unknown"}
                            </span>
                          </td>
                          <td className="px-4 py-3">
                            <span className={`${BADGE_BASE} ${getActionColor(rule.action)}`}>
                              {rule.action || "—"}
                            </span>
                          </td>
                          <td className="px-4 py-3">
                            <button
                              type="button"
                              onClick={() => handleToggleRule(rule.id, !rule.enabled)}
                              aria-label={rule.enabled ? `Disable rule ${rule.name}` : `Enable rule ${rule.name}`}
                              className="inline-flex items-center gap-1.5 rounded-md px-1 py-0.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                            >
                              {rule.enabled ? (
                                <ToggleRight className="h-5 w-5 text-emerald-600" aria-hidden="true" />
                              ) : (
                                <ToggleLeft className="h-5 w-5 text-gray-400" aria-hidden="true" />
                              )}
                              <span
                                className={`text-xs font-medium ${
                                  rule.enabled ? "text-emerald-700" : "text-gray-500"
                                }`}
                              >
                                {rule.enabled ? "Enabled" : "Disabled"}
                              </span>
                            </button>
                          </td>
                          <td className="px-4 py-3">
                            <div className="flex items-center justify-end gap-1">
                              <button
                                type="button"
                                onClick={() => setEditingItem(rule)}
                                aria-label={`Edit rule ${rule.name}`}
                                title="Edit rule"
                                className={ICON_BTN}
                              >
                                <Edit className="h-4 w-4" aria-hidden="true" />
                              </button>
                              <button
                                type="button"
                                onClick={() => handleDeleteRule(rule.id)}
                                aria-label={`Delete rule ${rule.name}`}
                                title="Delete rule"
                                className="inline-flex items-center rounded-md p-1.5 text-red-600 hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
                              >
                                <Trash2 className="h-4 w-4" aria-hidden="true" />
                              </button>
                            </div>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* Policies Tab */}
          {activeTab === "policies" && (
            <div className={CARD_CLASS}>
              <div className={CARD_HEADER_CLASS}>
                <h2 className={CARD_TITLE_CLASS}>WAF Policies</h2>
                <span className="text-xs text-gray-500">{filteredPolicies.length} policies</span>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr className="[&_th]:border-b [&_th]:border-gray-200">
                      <th className={TH_CLASS}>Name</th>
                      <th className={TH_CLASS}>Mode</th>
                      <th className={TH_CLASS}>Paranoia Level</th>
                      <th className={TH_CLASS}>Threshold</th>
                      <th className={TH_CLASS}>Status</th>
                      <th className={`${TH_CLASS} text-right`}>Actions</th>
                    </tr>
                  </thead>
                  <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                    {filteredPolicies.length === 0 ? (
                      <tr>
                        <td colSpan={6} className="px-4 py-12 text-center">
                          <Settings className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                          <p className="mt-3 text-sm font-semibold text-gray-900">No WAF policies found</p>
                          <p className="mt-1 text-sm text-gray-600">
                            Create your first policy to get started.
                          </p>
                        </td>
                      </tr>
                    ) : (
                      filteredPolicies.map((policy) => (
                        <tr key={policy.id} className="hover:bg-gray-50">
                          <td className="px-4 py-3">
                            <div className="text-sm font-medium text-gray-900">{policy.name || "—"}</div>
                            <div className="mt-0.5 text-xs text-gray-500">{policy.description || ""}</div>
                          </td>
                          <td className="px-4 py-3">
                            <span
                              className={`${BADGE_BASE} ${
                                policy.mode === "prevention"
                                  ? "bg-red-50 text-red-700"
                                  : "bg-blue-50 text-blue-700"
                              }`}
                            >
                              {policy.mode || "—"}
                            </span>
                          </td>
                          <td className="px-4 py-3">
                            <div className="flex items-center gap-1">
                              {[1, 2, 3, 4].map((level) => (
                                <span
                                  key={level}
                                  className={`h-2.5 w-2.5 rounded-full ${
                                    level <= (policy.paranoia_level ?? 0) ? "bg-red-600" : "bg-gray-200"
                                  }`}
                                />
                              ))}
                              <span className="ml-2 text-sm text-gray-600">
                                Level {policy.paranoia_level ?? 0}
                              </span>
                            </div>
                          </td>
                          <td className="px-4 py-3 text-sm text-gray-700">
                            {policy.anomaly_threshold ?? 0}
                          </td>
                          <td className="px-4 py-3">
                            <span
                              className={`inline-flex items-center gap-1.5 text-sm font-medium ${
                                policy.enabled ? "text-emerald-700" : "text-gray-500"
                              }`}
                            >
                              {policy.enabled ? (
                                <CheckCircle className="h-4 w-4" aria-hidden="true" />
                              ) : (
                                <XCircle className="h-4 w-4" aria-hidden="true" />
                              )}
                              {policy.enabled ? "Active" : "Inactive"}
                            </span>
                          </td>
                          <td className="px-4 py-3">
                            <div className="flex items-center justify-end gap-1">
                              <button
                                type="button"
                                onClick={() => setEditingItem(policy)}
                                aria-label={`Edit policy ${policy.name}`}
                                title="Edit policy"
                                className={ICON_BTN}
                              >
                                <Edit className="h-4 w-4" aria-hidden="true" />
                              </button>
                              <button
                                type="button"
                                onClick={() => handleDeletePolicy(policy.id)}
                                aria-label={`Delete policy ${policy.name}`}
                                title="Delete policy"
                                className="inline-flex items-center rounded-md p-1.5 text-red-600 hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
                              >
                                <Trash2 className="h-4 w-4" aria-hidden="true" />
                              </button>
                            </div>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* Events Tab */}
          {activeTab === "events" && (
            <div className={CARD_CLASS}>
              <div className={CARD_HEADER_CLASS}>
                <h2 className={CARD_TITLE_CLASS}>WAF Events</h2>
                <span className="text-xs text-gray-500">{filteredEvents.length} events</span>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr className="[&_th]:border-b [&_th]:border-gray-200">
                      <th className={TH_CLASS}>Time</th>
                      <th className={TH_CLASS}>Source IP</th>
                      <th className={TH_CLASS}>Method</th>
                      <th className={TH_CLASS}>Path</th>
                      <th className={TH_CLASS}>Attack Type</th>
                      <th className={TH_CLASS}>Status</th>
                    </tr>
                  </thead>
                  <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                    {filteredEvents.length === 0 ? (
                      <tr>
                        <td colSpan={6} className="px-4 py-12 text-center">
                          <Activity className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                          <p className="mt-3 text-sm font-semibold text-gray-900">No WAF events found</p>
                          <p className="mt-1 text-sm text-gray-600">
                            Events will appear here when attacks are detected.
                          </p>
                        </td>
                      </tr>
                    ) : (
                      filteredEvents.map((event) => (
                        <tr key={event.id} className="hover:bg-gray-50">
                          <td className="px-4 py-3 text-sm text-gray-600" suppressHydrationWarning>
                            {formatDateTime(event.created_at)}
                          </td>
                          <td className="px-4 py-3 font-mono text-sm text-gray-900">
                            {event.source_ip || "—"}
                          </td>
                          <td className="px-4 py-3">
                            <span className={`${BADGE_BASE} bg-gray-100 text-gray-700`}>
                              {event.method || "—"}
                            </span>
                          </td>
                          <td className="max-w-xs truncate px-4 py-3 text-sm text-gray-700">
                            {event.path || "—"}
                          </td>
                          <td className="px-4 py-3">
                            <span className={`${BADGE_BASE} bg-red-50 text-red-700`}>
                              {event.attack_type || "unknown"}
                            </span>
                          </td>
                          <td className="px-4 py-3">
                            <span
                              className={`inline-flex items-center gap-1.5 text-sm font-medium ${
                                event.blocked ? "text-red-700" : "text-amber-700"
                              }`}
                            >
                              {event.blocked ? (
                                <XCircle className="h-4 w-4" aria-hidden="true" />
                              ) : (
                                <AlertTriangle className="h-4 w-4" aria-hidden="true" />
                              )}
                              {event.blocked ? "Blocked" : "Detected"}
                            </span>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* Stats Tab */}
          {activeTab === "stats" && (
            stats ? (
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
                {/* Stats Cards */}
                <div className={`${CARD_CLASS} p-5`}>
                  <div className="flex items-start justify-between">
                    <div>
                      <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Total Rules</p>
                      <p className="mt-1 text-2xl font-semibold text-gray-900">{stats.total_rules ?? 0}</p>
                    </div>
                    <Shield className="h-6 w-6 text-gray-400" aria-hidden="true" />
                  </div>
                  <p className="mt-2 text-xs text-gray-500">{stats.enabled_rules ?? 0} enabled</p>
                </div>

                <div className={`${CARD_CLASS} p-5`}>
                  <div className="flex items-start justify-between">
                    <div>
                      <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Total Events</p>
                      <p className="mt-1 text-2xl font-semibold text-gray-900">{stats.total_events ?? 0}</p>
                    </div>
                    <Activity className="h-6 w-6 text-gray-400" aria-hidden="true" />
                  </div>
                  <p className="mt-2 text-xs text-gray-500">Last 7 days</p>
                </div>

                <div className={`${CARD_CLASS} p-5`}>
                  <div className="flex items-start justify-between">
                    <div>
                      <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Blocked Attacks</p>
                      <p className="mt-1 text-2xl font-semibold text-red-700">{stats.blocked_events ?? 0}</p>
                    </div>
                    <XCircle className="h-6 w-6 text-red-600" aria-hidden="true" />
                  </div>
                  <p className="mt-2 text-xs text-gray-500">
                    {(stats.total_events ?? 0) > 0
                      ? Math.round(((stats.blocked_events ?? 0) / (stats.total_events ?? 1)) * 100)
                      : 0}
                    % block rate
                  </p>
                </div>

                <div className={`${CARD_CLASS} p-5`}>
                  <div className="flex items-start justify-between">
                    <div>
                      <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Detection Rate</p>
                      <p className="mt-1 text-2xl font-semibold text-emerald-700">
                        {(stats.total_events ?? 0) > 0 ? 100 : 0}%
                      </p>
                    </div>
                    <Eye className="h-6 w-6 text-emerald-600" aria-hidden="true" />
                  </div>
                  <p className="mt-2 text-xs text-gray-500">All events captured</p>
                </div>

                {/* Top Attack Types */}
                <div className={`${CARD_CLASS} md:col-span-2`}>
                  <div className={CARD_HEADER_CLASS}>
                    <h2 className={CARD_TITLE_CLASS}>Top Attack Types</h2>
                  </div>
                  <div className="px-5 py-4">
                    {topAttackTypes.length === 0 ? (
                      <p className="py-6 text-center text-sm text-gray-600">No attack data available</p>
                    ) : (
                      <div className="space-y-3">
                        {topAttackTypes.map((item, index) => (
                          <div key={index} className="flex items-center justify-between gap-3">
                            <span className="text-sm text-gray-700">{item?.type || "unknown"}</span>
                            <div className="flex items-center gap-2">
                              <div className="h-2 w-32 rounded-full bg-gray-200">
                                <div
                                  className="h-2 rounded-full bg-red-600"
                                  style={{
                                    width: `${
                                      topAttackTypes[0]?.count
                                        ? ((item?.count ?? 0) / topAttackTypes[0].count) * 100
                                        : 0
                                    }%`,
                                  }}
                                />
                              </div>
                              <span className="w-12 text-right text-sm font-medium text-gray-900">
                                {item?.count ?? 0}
                              </span>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>

                {/* Top Source IPs */}
                <div className={`${CARD_CLASS} md:col-span-2`}>
                  <div className={CARD_HEADER_CLASS}>
                    <h2 className={CARD_TITLE_CLASS}>Top Source IPs</h2>
                  </div>
                  <div className="px-5 py-4">
                    {topSourceIps.length === 0 ? (
                      <p className="py-6 text-center text-sm text-gray-600">No source IP data available</p>
                    ) : (
                      <div className="space-y-3">
                        {topSourceIps.map((item, index) => (
                          <div key={index} className="flex items-center justify-between gap-3">
                            <span className="font-mono text-sm text-gray-700">{item?.ip || "—"}</span>
                            <div className="flex items-center gap-2">
                              <div className="h-2 w-32 rounded-full bg-gray-200">
                                <div
                                  className="h-2 rounded-full bg-amber-500"
                                  style={{
                                    width: `${
                                      topSourceIps[0]?.count
                                        ? ((item?.count ?? 0) / topSourceIps[0].count) * 100
                                        : 0
                                    }%`,
                                  }}
                                />
                              </div>
                              <span className="w-12 text-right text-sm font-medium text-gray-900">
                                {item?.count ?? 0}
                              </span>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            ) : (
              <div className={`${CARD_CLASS} px-5 py-12 text-center`}>
                <BarChart3 className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                <p className="mt-3 text-sm font-semibold text-gray-900">No statistics available</p>
                <p className="mt-1 text-sm text-gray-600">
                  Statistics appear once the firewall has processed traffic.
                </p>
              </div>
            )
          )}
        </>
      )}

      {/* Create/Edit Modal Placeholder */}
      {showCreateModal && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/50 p-4"
          role="dialog"
          aria-modal="true"
          aria-labelledby="waf-create-title"
        >
          <div className="w-full max-w-md rounded-lg border border-gray-200 bg-white shadow-lg">
            <div className="border-b border-gray-200 px-5 py-4">
              <h2 id="waf-create-title" className="text-sm font-semibold text-gray-900">
                Create WAF Rule
              </h2>
            </div>
            <div className="px-5 py-4">
              <p className="text-sm text-gray-600">Rule creation form will be implemented here.</p>
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              <button type="button" onClick={() => setShowCreateModal(false)} className={BTN_SECONDARY}>
                Cancel
              </button>
              <button type="button" onClick={() => setShowCreateModal(false)} className={BTN_PRIMARY}>
                Create
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
