'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  Shield,
  ShieldAlert,
  ShieldCheck,
  Bug,
  AlertTriangle,
  CheckCircle,
  XCircle,
  RefreshCw,
  Plus,
  Trash2,
  Play,
  Search,
  Eye,
  Clock,
  FileText,
  Lock,
  Loader2,
  ChevronLeft,
  ChevronRight,
  X,
  Activity,
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
import { securityApi } from '@/services/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface SecurityScan {
  id: string;
  tenant_id: string;
  server_id: string;
  scan_type: string;
  status: string;
  started_at: string;
  completed_at: string | null;
  score: number;
  total_checks: number;
  passed_checks: number;
  failed_checks: number;
  warnings: number;
  created_at: string;
  updated_at: string;
}

interface SecurityVulnerability {
  id: string;
  scan_id: string;
  tenant_id: string;
  severity: 'critical' | 'high' | 'medium' | 'low';
  title: string;
  description: string;
  affected: string;
  solution: string;
  cve: string;
  cvss: number;
  status: string;
  resolved_at: string | null;
  created_at: string;
  updated_at: string;
}

interface SecurityCheck {
  id: string;
  scan_id: string;
  tenant_id: string;
  category: string;
  name: string;
  description: string;
  status: 'pass' | 'fail' | 'warning';
  details: string;
  created_at: string;
}

interface SecurityPolicy {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  category: string;
  rules: Record<string, any>;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

// ---------------------------------------------------------------------------
// Style tokens
// ---------------------------------------------------------------------------

const CARD_CLASS = 'rounded-lg border border-gray-200 bg-white shadow-sm';
const CARD_HEADER_CLASS =
  'flex flex-row items-center justify-between space-y-0 border-b border-gray-200 px-5 py-4';
const CARD_TITLE_CLASS = 'text-sm font-semibold text-gray-900';
const TH_CLASS = 'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
const BTN_PRIMARY = 'bg-brand-600 text-white hover:bg-brand-700 focus-visible:ring-brand-500';
const BTN_SECONDARY =
  'border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus-visible:ring-brand-500';
const BTN_GHOST = 'text-gray-500 hover:bg-gray-100 hover:text-gray-900';
const INPUT_CLASS =
  'w-full rounded-md border-gray-300 bg-white text-sm text-gray-900 placeholder:text-gray-400 focus-visible:ring-1 focus-visible:ring-brand-500 focus-visible:ring-offset-0';
const SELECT_CLASS =
  'border-gray-300 bg-white text-sm text-gray-900 focus:ring-1 focus:ring-brand-500 focus:ring-offset-0';
const BADGE_BASE = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium';
const STATUS_PILL = 'inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const severityConfig: Record<string, { color: string; icon: React.ReactNode; label: string }> = {
  critical: {
    color: 'bg-red-50 text-red-700',
    icon: <ShieldAlert className="h-3.5 w-3.5" aria-hidden="true" />,
    label: 'Critical',
  },
  high: {
    color: 'bg-orange-50 text-orange-700',
    icon: <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />,
    label: 'High',
  },
  medium: {
    color: 'bg-amber-50 text-amber-700',
    icon: <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />,
    label: 'Medium',
  },
  low: {
    color: 'bg-sky-50 text-sky-700',
    icon: <Bug className="h-3.5 w-3.5" aria-hidden="true" />,
    label: 'Low',
  },
};

const scanStatusConfig: Record<string, { color: string; icon: React.ReactNode }> = {
  completed: {
    color: 'bg-emerald-50 text-emerald-700',
    icon: <CheckCircle className="h-3.5 w-3.5" aria-hidden="true" />,
  },
  running: {
    color: 'bg-brand-50 text-brand-700',
    icon: <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />,
  },
  pending: {
    color: 'bg-gray-100 text-gray-700',
    icon: <Clock className="h-3.5 w-3.5" aria-hidden="true" />,
  },
  failed: {
    color: 'bg-red-50 text-red-700',
    icon: <XCircle className="h-3.5 w-3.5" aria-hidden="true" />,
  },
};

function formatDate(dateStr: string | null): string {
  if (!dateStr) return '—';
  const parsed = new Date(dateStr);
  if (Number.isNaN(parsed.getTime())) return '—';
  return parsed.toLocaleString();
}

function truncateId(id: string): string {
  if (!id) return '—';
  return id.length > 8 ? id.slice(0, 8) + '…' : id;
}

function formatScore(value: number | undefined | null): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function formatCvss(value: number | undefined | null): string {
  return typeof value === 'number' && Number.isFinite(value) ? value.toFixed(1) : '0.0';
}

function cvssColor(value: number | undefined | null): string {
  const score = formatScore(value);
  if (score >= 9) return 'text-red-700';
  if (score >= 7) return 'text-orange-700';
  if (score >= 4) return 'text-amber-700';
  return 'text-sky-700';
}

function vulnStatusColor(status: string): string {
  if (status === 'resolved') return 'bg-emerald-50 text-emerald-700';
  if (status === 'open') return 'bg-red-50 text-red-700';
  return 'bg-gray-100 text-gray-700';
}

function toArray<T>(value: any): T[] {
  return Array.isArray(value) ? (value as T[]) : [];
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function SecurityPage() {
  // Data
  const [scans, setScans] = useState<SecurityScan[]>([]);
  const [vulnerabilities, setVulnerabilities] = useState<SecurityVulnerability[]>([]);
  const [checks, setChecks] = useState<SecurityCheck[]>([]);
  const [policies, setPolicies] = useState<SecurityPolicy[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Tab
  const [activeTab, setActiveTab] = useState('scans');

  // Filters
  const [scanSearch, setScanSearch] = useState('');
  const [scanStatusFilter, setScanStatusFilter] = useState('all');
  const [vulnSeverityFilter, setVulnSeverityFilter] = useState('all');
  const [vulnStatusFilter, setVulnStatusFilter] = useState('all');
  const [checkStatusFilter, setCheckStatusFilter] = useState('all');
  const [policyStatusFilter, setPolicyStatusFilter] = useState('all');

  // Selected scan for checks drill-down
  const [selectedScanId, setSelectedScanId] = useState<string | null>(null);

  // Modals
  const [showScanModal, setShowScanModal] = useState(false);
  const [showPolicyModal, setShowPolicyModal] = useState(false);
  const [showVulnDetail, setShowVulnDetail] = useState<SecurityVulnerability | null>(null);
  const [showCheckDetail, setShowCheckDetail] = useState<SecurityCheck | null>(null);

  // Form state
  const [scanForm, setScanForm] = useState({ server_id: '', scan_type: 'full' });
  const [policyForm, setPolicyForm] = useState({
    name: '',
    description: '',
    category: 'general',
    rules: '{}',
  });
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  // Toast
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // Pagination
  const [scansPage, setScansPage] = useState(1);
  const [scansTotal, setScansTotal] = useState(0);
  const [vulnsPage, setVulnsPage] = useState(1);
  const [vulnsTotal, setVulnsTotal] = useState(0);
  const perPage = 20;

  // ---------------------------------------------------------------------------
  // Data fetching
  // ---------------------------------------------------------------------------

  const fetchScans = useCallback(async () => {
    try {
      const res = await securityApi.listScans({ page: scansPage, per_page: perPage });
      setScans(toArray<SecurityScan>(res?.data?.data));
      setScansTotal(typeof res?.data?.total === 'number' ? res.data.total : 0);
    } catch (err: any) {
      console.error('Failed to load scans:', err);
      setScans([]);
      setScansTotal(0);
    }
  }, [scansPage]);

  const fetchVulnerabilities = useCallback(async () => {
    try {
      const params: any = { page: vulnsPage, per_page: perPage };
      if (vulnSeverityFilter !== 'all') params.severity = vulnSeverityFilter;
      const res = await securityApi.listVulnerabilities(params);
      setVulnerabilities(toArray<SecurityVulnerability>(res?.data?.data));
      setVulnsTotal(typeof res?.data?.total === 'number' ? res.data.total : 0);
    } catch (err: any) {
      console.error('Failed to load vulnerabilities:', err);
      setVulnerabilities([]);
      setVulnsTotal(0);
    }
  }, [vulnsPage, vulnSeverityFilter]);

  const fetchChecks = useCallback(async () => {
    if (!selectedScanId) {
      setChecks([]);
      return;
    }
    try {
      const res = await securityApi.listChecksByScan(selectedScanId);
      const list = Array.isArray(res?.data?.data) ? res.data.data : res?.data;
      setChecks(toArray<SecurityCheck>(list));
    } catch (err: any) {
      console.error('Failed to load checks:', err);
      setChecks([]);
    }
  }, [selectedScanId]);

  const fetchPolicies = useCallback(async () => {
    try {
      const res = await securityApi.listPolicies();
      const list = Array.isArray(res?.data?.data) ? res.data.data : res?.data;
      setPolicies(toArray<SecurityPolicy>(list));
    } catch (err: any) {
      console.error('Failed to load policies:', err);
      setPolicies([]);
    }
  }, []);

  const fetchAll = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      await Promise.all([fetchScans(), fetchVulnerabilities(), fetchPolicies()]);
    } catch (err: any) {
      setError(err?.message || 'Failed to load security data');
    } finally {
      setLoading(false);
    }
  }, [fetchScans, fetchVulnerabilities, fetchPolicies]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  useEffect(() => {
    fetchChecks();
  }, [fetchChecks]);

  // Auto-dismiss toast
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 4000);
    return () => clearTimeout(t);
  }, [toast]);

  // ---------------------------------------------------------------------------
  // Computed stats
  // ---------------------------------------------------------------------------

  const stats = {
    totalScans: scansTotal,
    runningScans: scans.filter((s) => s?.status === 'running').length,
    totalVulns: vulnsTotal,
    criticalVulns: vulnerabilities.filter((v) => v?.severity === 'critical').length,
    highVulns: vulnerabilities.filter((v) => v?.severity === 'high').length,
    mediumVulns: vulnerabilities.filter((v) => v?.severity === 'medium').length,
    lowVulns: vulnerabilities.filter((v) => v?.severity === 'low').length,
    activePolicies: policies.filter((p) => p?.is_active).length,
    totalPolicies: policies.length,
    complianceRate:
      policies.length > 0
        ? Math.round((policies.filter((p) => p?.is_active).length / policies.length) * 100)
        : 0,
  };

  // ---------------------------------------------------------------------------
  // Filtered data
  // ---------------------------------------------------------------------------

  const filteredScans = scans.filter((s) => {
    if (scanStatusFilter !== 'all' && s?.status !== scanStatusFilter) return false;
    if (scanSearch) {
      const q = scanSearch.toLowerCase();
      return (
        (s?.scan_type || '').toLowerCase().includes(q) ||
        (s?.server_id || '').toLowerCase().includes(q) ||
        (s?.status || '').toLowerCase().includes(q)
      );
    }
    return true;
  });

  const filteredVulns = vulnerabilities.filter((v) => {
    if (vulnSeverityFilter !== 'all' && v?.severity !== vulnSeverityFilter) return false;
    if (vulnStatusFilter !== 'all' && v?.status !== vulnStatusFilter) return false;
    return true;
  });

  const filteredChecks = checks.filter((c) => {
    if (checkStatusFilter !== 'all' && c?.status !== checkStatusFilter) return false;
    return true;
  });

  const filteredPolicies = policies.filter((p) => {
    if (policyStatusFilter === 'active' && !p?.is_active) return false;
    if (policyStatusFilter === 'inactive' && p?.is_active) return false;
    return true;
  });

  // ---------------------------------------------------------------------------
  // Actions
  // ---------------------------------------------------------------------------

  const handleCreateScan = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!scanForm.server_id) {
      setFormError('Server ID is required');
      return;
    }
    setSubmitting(true);
    setFormError(null);
    try {
      await securityApi.createScan(scanForm);
      setShowScanModal(false);
      setScanForm({ server_id: '', scan_type: 'full' });
      setToast({ type: 'success', message: 'Security scan started successfully' });
      fetchScans();
    } catch (err: any) {
      setFormError(err?.response?.data?.message || 'Failed to start scan');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDeleteScan = async (id: string) => {
    if (!confirm('Are you sure you want to delete this scan?')) return;
    try {
      await securityApi.deleteScan(id);
      setToast({ type: 'success', message: 'Scan deleted successfully' });
      fetchScans();
    } catch (err: any) {
      setToast({ type: 'error', message: err?.response?.data?.message || 'Failed to delete scan' });
    }
  };

  const handleCreatePolicy = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!policyForm.name) {
      setFormError('Policy name is required');
      return;
    }
    setSubmitting(true);
    setFormError(null);
    try {
      let rulesObj: Record<string, any>;
      try {
        rulesObj = JSON.parse(policyForm.rules);
      } catch {
        setFormError('Rules must be valid JSON');
        setSubmitting(false);
        return;
      }
      await securityApi.createPolicy({
        name: policyForm.name,
        description: policyForm.description,
        category: policyForm.category,
        rules: rulesObj,
      });
      setShowPolicyModal(false);
      setPolicyForm({ name: '', description: '', category: 'general', rules: '{}' });
      setToast({ type: 'success', message: 'Policy created successfully' });
      fetchPolicies();
    } catch (err: any) {
      setFormError(err?.response?.data?.message || 'Failed to create policy');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDeletePolicy = async (id: string) => {
    if (!confirm('Are you sure you want to delete this policy?')) return;
    try {
      await securityApi.deletePolicy(id);
      setToast({ type: 'success', message: 'Policy deleted successfully' });
      fetchPolicies();
    } catch (err: any) {
      setToast({ type: 'error', message: err?.response?.data?.message || 'Failed to delete policy' });
    }
  };

  const handleTogglePolicy = async (policy: SecurityPolicy) => {
    try {
      await securityApi.updatePolicy(policy.id, { is_active: !policy.is_active });
      setToast({
        type: 'success',
        message: `Policy ${policy.is_active ? 'deactivated' : 'activated'} successfully`,
      });
      fetchPolicies();
    } catch (err: any) {
      setToast({ type: 'error', message: err?.response?.data?.message || 'Failed to update policy' });
    }
  };

  const handleUpdateVulnStatus = async (id: string, status: string) => {
    try {
      await securityApi.updateVulnerability(id, { status });
      setToast({ type: 'success', message: 'Vulnerability status updated' });
      fetchVulnerabilities();
      if (showVulnDetail?.id === id) {
        setShowVulnDetail(null);
      }
    } catch (err: any) {
      setToast({
        type: 'error',
        message: err?.response?.data?.message || 'Failed to update vulnerability',
      });
    }
  };

  // ---------------------------------------------------------------------------
  // Loading state
  // ---------------------------------------------------------------------------

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <RefreshCw className="h-6 w-6 animate-spin text-gray-400" aria-hidden="true" />
        <span className="ml-2 text-sm text-gray-600">Loading security data...</span>
      </div>
    );
  }

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div className="space-y-6">
      {/* Toast */}
      {toast && (
        <div
          role="status"
          className={`fixed right-4 top-4 z-50 flex items-center gap-2 rounded-md border px-4 py-3 text-sm font-medium shadow-lg ${
            toast.type === 'success'
              ? 'border-emerald-200 bg-emerald-50 text-emerald-700'
              : 'border-red-200 bg-red-50 text-red-700'
          }`}
        >
          {toast.type === 'success' ? (
            <CheckCircle className="h-4 w-4" aria-hidden="true" />
          ) : (
            <XCircle className="h-4 w-4" aria-hidden="true" />
          )}
          {toast.message}
          <button
            type="button"
            onClick={() => setToast(null)}
            aria-label="Dismiss notification"
            className="ml-2 rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      )}

      {/* Header */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Security</h1>
          <p className="mt-1 text-sm text-gray-600">
            Manage security scans, vulnerabilities, policies, and checks
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={fetchAll} className={BTN_SECONDARY}>
            <RefreshCw className="mr-2 h-4 w-4" aria-hidden="true" />
            Refresh
          </Button>
        </div>
      </div>

      {error && (
        <div
          className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
          role="alert"
        >
          {error}
        </div>
      )}

      {/* Summary Cards */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card className={CARD_CLASS}>
          <CardHeader className={CARD_HEADER_CLASS}>
            <CardTitle className={CARD_TITLE_CLASS}>Total Scans</CardTitle>
            <Activity className="h-4 w-4 text-gray-400" aria-hidden="true" />
          </CardHeader>
          <CardContent className="px-5 py-4">
            <div className="text-2xl font-semibold text-gray-900">{stats.totalScans}</div>
            <p className="mt-1 text-xs text-gray-500">{stats.runningScans} currently running</p>
          </CardContent>
        </Card>

        <Card className={CARD_CLASS}>
          <CardHeader className={CARD_HEADER_CLASS}>
            <CardTitle className={CARD_TITLE_CLASS}>Vulnerabilities</CardTitle>
            <Bug className="h-4 w-4 text-gray-400" aria-hidden="true" />
          </CardHeader>
          <CardContent className="px-5 py-4">
            <div className="text-2xl font-semibold text-gray-900">{stats.totalVulns}</div>
            <div className="mt-2 flex flex-wrap gap-2">
              <span className="text-xs text-red-700">{stats.criticalVulns} critical</span>
              <span className="text-xs text-orange-700">{stats.highVulns} high</span>
              <span className="text-xs text-amber-700">{stats.mediumVulns} medium</span>
              <span className="text-xs text-sky-700">{stats.lowVulns} low</span>
            </div>
          </CardContent>
        </Card>

        <Card className={CARD_CLASS}>
          <CardHeader className={CARD_HEADER_CLASS}>
            <CardTitle className={CARD_TITLE_CLASS}>Active Policies</CardTitle>
            <Lock className="h-4 w-4 text-gray-400" aria-hidden="true" />
          </CardHeader>
          <CardContent className="px-5 py-4">
            <div className="text-2xl font-semibold text-gray-900">{stats.activePolicies}</div>
            <p className="mt-1 text-xs text-gray-500">of {stats.totalPolicies} total policies</p>
          </CardContent>
        </Card>

        <Card className={CARD_CLASS}>
          <CardHeader className={CARD_HEADER_CLASS}>
            <CardTitle className={CARD_TITLE_CLASS}>Compliance Rate</CardTitle>
            <ShieldCheck className="h-4 w-4 text-gray-400" aria-hidden="true" />
          </CardHeader>
          <CardContent className="px-5 py-4">
            <div className="text-2xl font-semibold text-gray-900">{stats.complianceRate}%</div>
            <div className="mt-2 h-1.5 w-full rounded-full bg-gray-200">
              <div
                className={`h-1.5 rounded-full ${
                  stats.complianceRate >= 80
                    ? 'bg-emerald-600'
                    : stats.complianceRate >= 50
                    ? 'bg-amber-500'
                    : 'bg-red-600'
                }`}
                style={{ width: `${stats.complianceRate}%` }}
              />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Tabs */}
      <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-4">
        <TabsList className="inline-flex h-auto items-center gap-1 rounded-md border border-gray-200 bg-white p-1 text-gray-600">
          <TabsTrigger
            value="scans"
            className="rounded-md px-3 py-1.5 text-sm font-medium text-gray-600 data-[state=active]:bg-brand-50 data-[state=active]:text-brand-700 data-[state=active]:shadow-none"
          >
            <Search className="mr-2 h-4 w-4" aria-hidden="true" />
            Scans
          </TabsTrigger>
          <TabsTrigger
            value="vulnerabilities"
            className="rounded-md px-3 py-1.5 text-sm font-medium text-gray-600 data-[state=active]:bg-brand-50 data-[state=active]:text-brand-700 data-[state=active]:shadow-none"
          >
            <Bug className="mr-2 h-4 w-4" aria-hidden="true" />
            Vulnerabilities
          </TabsTrigger>
          <TabsTrigger
            value="policies"
            className="rounded-md px-3 py-1.5 text-sm font-medium text-gray-600 data-[state=active]:bg-brand-50 data-[state=active]:text-brand-700 data-[state=active]:shadow-none"
          >
            <FileText className="mr-2 h-4 w-4" aria-hidden="true" />
            Policies
          </TabsTrigger>
          <TabsTrigger
            value="checks"
            className="rounded-md px-3 py-1.5 text-sm font-medium text-gray-600 data-[state=active]:bg-brand-50 data-[state=active]:text-brand-700 data-[state=active]:shadow-none"
          >
            <ShieldCheck className="mr-2 h-4 w-4" aria-hidden="true" />
            Checks
          </TabsTrigger>
        </TabsList>

        {/* ================================================================ */}
        {/* SCANS TAB                                                        */}
        {/* ================================================================ */}
        <TabsContent value="scans" className="space-y-4">
          {/* Toolbar */}
          <div className="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-center">
            <div className="flex flex-wrap items-center gap-2">
              <label htmlFor="scan-search" className="sr-only">
                Search scans
              </label>
              <Input
                id="scan-search"
                placeholder="Search scans..."
                aria-label="Search scans"
                value={scanSearch}
                onChange={(e) => setScanSearch(e.target.value)}
                className={`w-64 ${INPUT_CLASS}`}
              />
              <Select value={scanStatusFilter} onValueChange={setScanStatusFilter}>
                <SelectTrigger className={`w-40 ${SELECT_CLASS}`} aria-label="Filter scans by status">
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Statuses</SelectItem>
                  <SelectItem value="completed">Completed</SelectItem>
                  <SelectItem value="running">Running</SelectItem>
                  <SelectItem value="pending">Pending</SelectItem>
                  <SelectItem value="failed">Failed</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button
              onClick={() => {
                setFormError(null);
                setShowScanModal(true);
              }}
              className={BTN_PRIMARY}
            >
              <Play className="mr-2 h-4 w-4" aria-hidden="true" />
              Start Scan
            </Button>
          </div>

          {/* Scans Table */}
          <Card className={CARD_CLASS}>
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr className="[&_th]:border-b [&_th]:border-gray-200">
                      <th className={TH_CLASS}>ID</th>
                      <th className={TH_CLASS}>Type</th>
                      <th className={TH_CLASS}>Status</th>
                      <th className={TH_CLASS}>Score</th>
                      <th className={TH_CLASS}>Findings</th>
                      <th className={TH_CLASS}>Started At</th>
                      <th className={TH_CLASS}>Completed At</th>
                      <th className={`${TH_CLASS} text-right`}>Actions</th>
                    </tr>
                  </thead>
                  <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                    {filteredScans.length === 0 ? (
                      <tr>
                        <td colSpan={8} className="px-4 py-12 text-center">
                          <Shield className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                          <p className="mt-3 text-sm font-semibold text-gray-900">No scans found</p>
                          <p className="mt-1 text-sm text-gray-600">
                            Start a security scan to see results here.
                          </p>
                        </td>
                      </tr>
                    ) : (
                      filteredScans.map((scan) => {
                        const statusCfg = scanStatusConfig[scan.status] || scanStatusConfig.pending;
                        return (
                          <tr key={scan.id} className="hover:bg-gray-50">
                            <td className="px-4 py-3 font-mono text-xs text-gray-600">
                              {truncateId(scan.id)}
                            </td>
                            <td className="px-4 py-3">
                              <Badge variant="outline" className={`${BADGE_BASE} border-transparent bg-gray-100 text-gray-700`}>
                                {scan.scan_type || '—'}
                              </Badge>
                            </td>
                            <td className="px-4 py-3">
                              <span className={`${STATUS_PILL} ${statusCfg.color}`}>
                                {statusCfg.icon}
                                {scan.status || 'pending'}
                              </span>
                            </td>
                            <td className="px-4 py-3">
                              <span
                                className={`text-sm font-semibold ${
                                  formatScore(scan.score) >= 80
                                    ? 'text-emerald-700'
                                    : formatScore(scan.score) >= 50
                                    ? 'text-amber-700'
                                    : 'text-red-700'
                                }`}
                              >
                                {formatScore(scan.score)}%
                              </span>
                            </td>
                            <td className="px-4 py-3">
                              <div className="flex items-center gap-2 text-xs">
                                <span className="text-emerald-700">{scan.passed_checks ?? 0} pass</span>
                                <span className="text-red-700">{scan.failed_checks ?? 0} fail</span>
                                {(scan.warnings ?? 0) > 0 && (
                                  <span className="text-amber-700">{scan.warnings} warn</span>
                                )}
                              </div>
                            </td>
                            <td className="px-4 py-3 text-xs text-gray-600" suppressHydrationWarning>
                              {formatDate(scan.started_at)}
                            </td>
                            <td className="px-4 py-3 text-xs text-gray-600" suppressHydrationWarning>
                              {formatDate(scan.completed_at)}
                            </td>
                            <td className="px-4 py-3 text-right">
                              <div className="flex items-center justify-end gap-1">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className={BTN_GHOST}
                                  onClick={() => {
                                    setSelectedScanId(scan.id);
                                    setActiveTab('checks');
                                  }}
                                  aria-label="View checks for this scan"
                                  title="View checks"
                                >
                                  <Eye className="h-4 w-4" aria-hidden="true" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="text-red-600 hover:bg-red-50 hover:text-red-700"
                                  onClick={() => handleDeleteScan(scan.id)}
                                  aria-label="Delete scan"
                                  title="Delete scan"
                                >
                                  <Trash2 className="h-4 w-4" aria-hidden="true" />
                                </Button>
                              </div>
                            </td>
                          </tr>
                        );
                      })
                    )}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>

          {/* Pagination */}
          {scansTotal > perPage && (
            <div className="flex items-center justify-between">
              <p className="text-sm text-gray-600">
                Showing {(scansPage - 1) * perPage + 1}–{Math.min(scansPage * perPage, scansTotal)} of{' '}
                {scansTotal}
              </p>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={scansPage <= 1}
                  onClick={() => setScansPage((p) => p - 1)}
                  className={BTN_SECONDARY}
                  aria-label="Previous page"
                >
                  <ChevronLeft className="h-4 w-4" aria-hidden="true" />
                </Button>
                <span className="text-sm text-gray-600">Page {scansPage}</span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={scansPage * perPage >= scansTotal}
                  onClick={() => setScansPage((p) => p + 1)}
                  className={BTN_SECONDARY}
                  aria-label="Next page"
                >
                  <ChevronRight className="h-4 w-4" aria-hidden="true" />
                </Button>
              </div>
            </div>
          )}
        </TabsContent>

        {/* ================================================================ */}
        {/* VULNERABILITIES TAB                                              */}
        {/* ================================================================ */}
        <TabsContent value="vulnerabilities" className="space-y-4">
          {/* Toolbar */}
          <div className="flex flex-wrap items-center gap-2">
            <Select value={vulnSeverityFilter} onValueChange={setVulnSeverityFilter}>
              <SelectTrigger className={`w-40 ${SELECT_CLASS}`} aria-label="Filter by severity">
                <SelectValue placeholder="Severity" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Severities</SelectItem>
                <SelectItem value="critical">Critical</SelectItem>
                <SelectItem value="high">High</SelectItem>
                <SelectItem value="medium">Medium</SelectItem>
                <SelectItem value="low">Low</SelectItem>
              </SelectContent>
            </Select>
            <Select value={vulnStatusFilter} onValueChange={setVulnStatusFilter}>
              <SelectTrigger className={`w-40 ${SELECT_CLASS}`} aria-label="Filter by status">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Statuses</SelectItem>
                <SelectItem value="open">Open</SelectItem>
                <SelectItem value="resolved">Resolved</SelectItem>
                <SelectItem value="false_positive">False Positive</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Vulnerabilities Table */}
          <Card className={CARD_CLASS}>
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr className="[&_th]:border-b [&_th]:border-gray-200">
                      <th className={TH_CLASS}>Severity</th>
                      <th className={TH_CLASS}>Title</th>
                      <th className={TH_CLASS}>CVE</th>
                      <th className={TH_CLASS}>CVSS</th>
                      <th className={TH_CLASS}>Affected</th>
                      <th className={TH_CLASS}>Status</th>
                      <th className={TH_CLASS}>Created</th>
                      <th className={`${TH_CLASS} text-right`}>Actions</th>
                    </tr>
                  </thead>
                  <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                    {filteredVulns.length === 0 ? (
                      <tr>
                        <td colSpan={8} className="px-4 py-12 text-center">
                          <ShieldCheck className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                          <p className="mt-3 text-sm font-semibold text-gray-900">
                            No vulnerabilities found
                          </p>
                          <p className="mt-1 text-sm text-gray-600">
                            Findings from completed scans will be listed here.
                          </p>
                        </td>
                      </tr>
                    ) : (
                      filteredVulns.map((vuln) => {
                        const sevCfg = severityConfig[vuln.severity] || severityConfig.low;
                        return (
                          <tr key={vuln.id} className="hover:bg-gray-50">
                            <td className="px-4 py-3">
                              <span className={`${STATUS_PILL} ${sevCfg.color}`}>
                                {sevCfg.icon}
                                {sevCfg.label}
                              </span>
                            </td>
                            <td className="max-w-xs truncate px-4 py-3 text-sm font-medium text-gray-900">
                              {vuln.title || '—'}
                            </td>
                            <td className="px-4 py-3 font-mono text-xs text-gray-600">
                              {vuln.cve || '—'}
                            </td>
                            <td className="px-4 py-3">
                              <span className={`text-sm font-semibold ${cvssColor(vuln.cvss)}`}>
                                {formatCvss(vuln.cvss)}
                              </span>
                            </td>
                            <td className="max-w-[200px] truncate px-4 py-3 text-xs text-gray-600">
                              {vuln.affected || '—'}
                            </td>
                            <td className="px-4 py-3">
                              <Badge variant="outline"
                                className={`${BADGE_BASE} border-transparent ${vulnStatusColor(vuln.status)}`}
                              >
                                {vuln.status || 'unknown'}
                              </Badge>
                            </td>
                            <td className="px-4 py-3 text-xs text-gray-600" suppressHydrationWarning>
                              {formatDate(vuln.created_at)}
                            </td>
                            <td className="px-4 py-3 text-right">
                              <div className="flex items-center justify-end gap-1">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className={BTN_GHOST}
                                  onClick={() => setShowVulnDetail(vuln)}
                                  aria-label="View vulnerability details"
                                  title="View details"
                                >
                                  <Eye className="h-4 w-4" aria-hidden="true" />
                                </Button>
                                {vuln.status === 'open' && (
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    className="text-emerald-700 hover:bg-emerald-50 hover:text-emerald-700"
                                    onClick={() => handleUpdateVulnStatus(vuln.id, 'resolved')}
                                    aria-label="Mark vulnerability as resolved"
                                    title="Mark as resolved"
                                  >
                                    <CheckCircle className="h-4 w-4" aria-hidden="true" />
                                  </Button>
                                )}
                              </div>
                            </td>
                          </tr>
                        );
                      })
                    )}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>

          {/* Pagination */}
          {vulnsTotal > perPage && (
            <div className="flex items-center justify-between">
              <p className="text-sm text-gray-600">
                Showing {(vulnsPage - 1) * perPage + 1}–{Math.min(vulnsPage * perPage, vulnsTotal)} of{' '}
                {vulnsTotal}
              </p>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={vulnsPage <= 1}
                  onClick={() => setVulnsPage((p) => p - 1)}
                  className={BTN_SECONDARY}
                  aria-label="Previous page"
                >
                  <ChevronLeft className="h-4 w-4" aria-hidden="true" />
                </Button>
                <span className="text-sm text-gray-600">Page {vulnsPage}</span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={vulnsPage * perPage >= vulnsTotal}
                  onClick={() => setVulnsPage((p) => p + 1)}
                  className={BTN_SECONDARY}
                  aria-label="Next page"
                >
                  <ChevronRight className="h-4 w-4" aria-hidden="true" />
                </Button>
              </div>
            </div>
          )}
        </TabsContent>

        {/* ================================================================ */}
        {/* POLICIES TAB                                                     */}
        {/* ================================================================ */}
        <TabsContent value="policies" className="space-y-4">
          {/* Toolbar */}
          <div className="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-center">
            <div className="flex items-center gap-2">
              <Select value={policyStatusFilter} onValueChange={setPolicyStatusFilter}>
                <SelectTrigger className={`w-40 ${SELECT_CLASS}`} aria-label="Filter policies by status">
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All</SelectItem>
                  <SelectItem value="active">Active</SelectItem>
                  <SelectItem value="inactive">Inactive</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button
              onClick={() => {
                setFormError(null);
                setShowPolicyModal(true);
              }}
              className={BTN_PRIMARY}
            >
              <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
              Create Policy
            </Button>
          </div>

          {/* Policies Grid */}
          {filteredPolicies.length === 0 ? (
            <Card className={CARD_CLASS}>
              <CardContent className="px-5 py-12 text-center">
                <FileText className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                <p className="mt-3 text-sm font-semibold text-gray-900">No policies found</p>
                <p className="mt-1 text-sm text-gray-600">
                  Create a security policy to enforce your standards.
                </p>
              </CardContent>
            </Card>
          ) : (
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
              {filteredPolicies.map((policy) => (
                <Card key={policy.id} className={`${CARD_CLASS} hover:border-gray-300`}>
                  <CardHeader className="border-b border-gray-200 px-5 py-4">
                    <div className="flex items-start justify-between gap-3">
                      <div className="flex items-center gap-2">
                        <div className={`rounded-md p-2 ${policy.is_active ? 'bg-emerald-50' : 'bg-gray-100'}`}>
                          <Shield
                            className={`h-4 w-4 ${policy.is_active ? 'text-emerald-600' : 'text-gray-500'}`}
                            aria-hidden="true"
                          />
                        </div>
                        <div>
                          <CardTitle className={CARD_TITLE_CLASS}>{policy.name || '—'}</CardTitle>
                          <p className="mt-0.5 text-xs text-gray-500">{policy.category || 'general'}</p>
                        </div>
                      </div>
                      <Badge variant="outline"
                        className={`${BADGE_BASE} border-transparent ${
                          policy.is_active ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-700'
                        }`}
                      >
                        {policy.is_active ? 'Active' : 'Inactive'}
                      </Badge>
                    </div>
                  </CardHeader>
                  <CardContent className="px-5 py-4">
                    <p className="mb-3 line-clamp-2 text-sm text-gray-600">
                      {policy.description || 'No description'}
                    </p>
                    <div className="flex items-center justify-between">
                      <span className="text-xs text-gray-500" suppressHydrationWarning>
                        Created {formatDate(policy.created_at)}
                      </span>
                      <div className="flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          className={BTN_GHOST}
                          onClick={() => handleTogglePolicy(policy)}
                          aria-label={policy.is_active ? 'Deactivate policy' : 'Activate policy'}
                          title={policy.is_active ? 'Deactivate' : 'Activate'}
                        >
                          {policy.is_active ? (
                            <XCircle className="h-4 w-4" aria-hidden="true" />
                          ) : (
                            <CheckCircle className="h-4 w-4" aria-hidden="true" />
                          )}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-red-600 hover:bg-red-50 hover:text-red-700"
                          onClick={() => handleDeletePolicy(policy.id)}
                          aria-label="Delete policy"
                          title="Delete policy"
                        >
                          <Trash2 className="h-4 w-4" aria-hidden="true" />
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </TabsContent>

        {/* ================================================================ */}
        {/* CHECKS TAB                                                       */}
        {/* ================================================================ */}
        <TabsContent value="checks" className="space-y-4">
          {/* Toolbar */}
          <div className="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-center">
            <div className="flex flex-wrap items-center gap-2">
              {!selectedScanId ? (
                <p className="text-sm text-gray-600">
                  Select a scan from the Scans tab to view its checks, or choose one below:
                </p>
              ) : (
                <div className="flex items-center gap-2">
                  <span className="text-sm text-gray-600">
                    Showing checks for scan:{' '}
                    <span className="font-mono text-gray-900">{truncateId(selectedScanId)}</span>
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    className={BTN_GHOST}
                    onClick={() => setSelectedScanId(null)}
                    aria-label="Clear selected scan"
                    title="Clear selection"
                  >
                    <X className="h-3 w-3" aria-hidden="true" />
                  </Button>
                </div>
              )}
              <Select
                value={selectedScanId || ''}
                onValueChange={(val) => setSelectedScanId(val || null)}
              >
                <SelectTrigger className={`w-64 ${SELECT_CLASS}`} aria-label="Select a scan">
                  <SelectValue placeholder="Select a scan..." />
                </SelectTrigger>
                <SelectContent>
                  {scans.map((scan) => (
                    <SelectItem key={scan.id} value={scan.id}>
                      {scan.scan_type} — {truncateId(scan.id)} ({scan.status})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select value={checkStatusFilter} onValueChange={setCheckStatusFilter}>
                <SelectTrigger className={`w-36 ${SELECT_CLASS}`} aria-label="Filter checks by status">
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All</SelectItem>
                  <SelectItem value="pass">Pass</SelectItem>
                  <SelectItem value="fail">Fail</SelectItem>
                  <SelectItem value="warning">Warning</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          {/* Checks Table */}
          <Card className={CARD_CLASS}>
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr className="[&_th]:border-b [&_th]:border-gray-200">
                      <th className={TH_CLASS}>Status</th>
                      <th className={TH_CLASS}>Name</th>
                      <th className={TH_CLASS}>Category</th>
                      <th className={TH_CLASS}>Description</th>
                      <th className={TH_CLASS}>Checked At</th>
                      <th className={`${TH_CLASS} text-right`}>Actions</th>
                    </tr>
                  </thead>
                  <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                    {!selectedScanId ? (
                      <tr>
                        <td colSpan={6} className="px-4 py-12 text-center">
                          <ShieldCheck className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                          <p className="mt-3 text-sm font-semibold text-gray-900">
                            Select a scan to view security checks
                          </p>
                        </td>
                      </tr>
                    ) : filteredChecks.length === 0 ? (
                      <tr>
                        <td colSpan={6} className="px-4 py-12 text-center">
                          <ShieldCheck className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                          <p className="mt-3 text-sm font-semibold text-gray-900">
                            No checks found for this scan
                          </p>
                        </td>
                      </tr>
                    ) : (
                      filteredChecks.map((check) => (
                        <tr key={check.id} className="hover:bg-gray-50">
                          <td className="px-4 py-3">
                            {check.status === 'pass' ? (
                              <span className={`${STATUS_PILL} bg-emerald-50 text-emerald-700`}>
                                <CheckCircle className="h-3.5 w-3.5" aria-hidden="true" />
                                Pass
                              </span>
                            ) : check.status === 'fail' ? (
                              <span className={`${STATUS_PILL} bg-red-50 text-red-700`}>
                                <XCircle className="h-3.5 w-3.5" aria-hidden="true" />
                                Fail
                              </span>
                            ) : (
                              <span className={`${STATUS_PILL} bg-amber-50 text-amber-700`}>
                                <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
                                Warning
                              </span>
                            )}
                          </td>
                          <td className="px-4 py-3 text-sm font-medium text-gray-900">
                            {check.name || '—'}
                          </td>
                          <td className="px-4 py-3">
                            <Badge variant="outline" className={`${BADGE_BASE} border-transparent bg-gray-100 text-gray-700`}>
                              {check.category || '—'}
                            </Badge>
                          </td>
                          <td className="max-w-xs truncate px-4 py-3 text-xs text-gray-600">
                            {check.description || '—'}
                          </td>
                          <td className="px-4 py-3 text-xs text-gray-600" suppressHydrationWarning>
                            {formatDate(check.created_at)}
                          </td>
                          <td className="px-4 py-3 text-right">
                            <Button
                              variant="ghost"
                              size="sm"
                              className={BTN_GHOST}
                              onClick={() => setShowCheckDetail(check)}
                              aria-label="View check details"
                              title="View details"
                            >
                              <Eye className="h-4 w-4" aria-hidden="true" />
                            </Button>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>

          {/* Check summary when a scan is selected */}
          {selectedScanId && checks.length > 0 && (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <Card className={CARD_CLASS}>
                <CardContent className="flex items-center gap-3 px-5 py-4">
                  <div className="rounded-md bg-emerald-50 p-2">
                    <CheckCircle className="h-5 w-5 text-emerald-600" aria-hidden="true" />
                  </div>
                  <div>
                    <p className="text-2xl font-semibold text-gray-900">
                      {checks.filter((c) => c?.status === 'pass').length}
                    </p>
                    <p className="text-xs text-gray-500">Passed</p>
                  </div>
                </CardContent>
              </Card>
              <Card className={CARD_CLASS}>
                <CardContent className="flex items-center gap-3 px-5 py-4">
                  <div className="rounded-md bg-red-50 p-2">
                    <XCircle className="h-5 w-5 text-red-600" aria-hidden="true" />
                  </div>
                  <div>
                    <p className="text-2xl font-semibold text-gray-900">
                      {checks.filter((c) => c?.status === 'fail').length}
                    </p>
                    <p className="text-xs text-gray-500">Failed</p>
                  </div>
                </CardContent>
              </Card>
              <Card className={CARD_CLASS}>
                <CardContent className="flex items-center gap-3 px-5 py-4">
                  <div className="rounded-md bg-amber-50 p-2">
                    <AlertTriangle className="h-5 w-5 text-amber-600" aria-hidden="true" />
                  </div>
                  <div>
                    <p className="text-2xl font-semibold text-gray-900">
                      {checks.filter((c) => c?.status === 'warning').length}
                    </p>
                    <p className="text-xs text-gray-500">Warnings</p>
                  </div>
                </CardContent>
              </Card>
            </div>
          )}
        </TabsContent>
      </Tabs>

      {/* ================================================================== */}
      {/* START SCAN MODAL                                                   */}
      {/* ================================================================== */}
      {showScanModal && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/50 p-4"
          role="dialog"
          aria-modal="true"
          aria-labelledby="scan-modal-title"
        >
          <div className="w-full max-w-md rounded-lg border border-gray-200 bg-white shadow-lg">
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <h2 id="scan-modal-title" className="text-sm font-semibold text-gray-900">
                Start Security Scan
              </h2>
              <button
                type="button"
                onClick={() => setShowScanModal(false)}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
            <form onSubmit={handleCreateScan} className="space-y-4 px-5 py-4">
              {formError && (
                <div className="flex items-center gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
                  {formError}
                </div>
              )}
              <div className="space-y-1.5">
                <label htmlFor="scan-server-id" className="text-sm font-medium text-gray-700">
                  Server ID
                </label>
                <Input
                  id="scan-server-id"
                  placeholder="Enter server ID"
                  value={scanForm.server_id}
                  onChange={(e) => setScanForm((prev) => ({ ...prev, server_id: e.target.value }))}
                  className={INPUT_CLASS}
                  required
                />
              </div>
              <div className="space-y-1.5">
                <label htmlFor="scan-type" className="text-sm font-medium text-gray-700">
                  Scan Type
                </label>
                <Select
                  value={scanForm.scan_type}
                  onValueChange={(val) => setScanForm((prev) => ({ ...prev, scan_type: val }))}
                >
                  <SelectTrigger id="scan-type" className={SELECT_CLASS} aria-label="Scan type">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="full">Full Scan</SelectItem>
                    <SelectItem value="quick">Quick Scan</SelectItem>
                    <SelectItem value="vulnerability">Vulnerability Scan</SelectItem>
                    <SelectItem value="compliance">Compliance Scan</SelectItem>
                    <SelectItem value="port">Port Scan</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex justify-end gap-2 pt-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowScanModal(false)}
                  className={BTN_SECONDARY}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={submitting} className={BTN_PRIMARY}>
                  {submitting ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                      Starting...
                    </>
                  ) : (
                    <>
                      <Play className="mr-2 h-4 w-4" aria-hidden="true" />
                      Start Scan
                    </>
                  )}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* CREATE POLICY MODAL                                                */}
      {/* ================================================================== */}
      {showPolicyModal && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/50 p-4"
          role="dialog"
          aria-modal="true"
          aria-labelledby="policy-modal-title"
        >
          <div className="w-full max-w-lg rounded-lg border border-gray-200 bg-white shadow-lg">
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <h2 id="policy-modal-title" className="text-sm font-semibold text-gray-900">
                Create Security Policy
              </h2>
              <button
                type="button"
                onClick={() => setShowPolicyModal(false)}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
            <form onSubmit={handleCreatePolicy} className="space-y-4 px-5 py-4">
              {formError && (
                <div className="flex items-center gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
                  {formError}
                </div>
              )}
              <div className="space-y-1.5">
                <label htmlFor="policy-name" className="text-sm font-medium text-gray-700">
                  Name
                </label>
                <Input
                  id="policy-name"
                  placeholder="Policy name"
                  value={policyForm.name}
                  onChange={(e) => setPolicyForm((prev) => ({ ...prev, name: e.target.value }))}
                  className={INPUT_CLASS}
                  required
                />
              </div>
              <div className="space-y-1.5">
                <label htmlFor="policy-description" className="text-sm font-medium text-gray-700">
                  Description
                </label>
                <Input
                  id="policy-description"
                  placeholder="Brief description"
                  value={policyForm.description}
                  onChange={(e) => setPolicyForm((prev) => ({ ...prev, description: e.target.value }))}
                  className={INPUT_CLASS}
                />
              </div>
              <div className="space-y-1.5">
                <label htmlFor="policy-category" className="text-sm font-medium text-gray-700">
                  Category
                </label>
                <Select
                  value={policyForm.category}
                  onValueChange={(val) => setPolicyForm((prev) => ({ ...prev, category: val }))}
                >
                  <SelectTrigger id="policy-category" className={SELECT_CLASS} aria-label="Policy category">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="general">General</SelectItem>
                    <SelectItem value="network">Network</SelectItem>
                    <SelectItem value="authentication">Authentication</SelectItem>
                    <SelectItem value="encryption">Encryption</SelectItem>
                    <SelectItem value="access_control">Access Control</SelectItem>
                    <SelectItem value="compliance">Compliance</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <label htmlFor="policy-rules" className="text-sm font-medium text-gray-700">
                  Rules (JSON)
                </label>
                <textarea
                  id="policy-rules"
                  value={policyForm.rules}
                  onChange={(e) => setPolicyForm((prev) => ({ ...prev, rules: e.target.value }))}
                  className="flex min-h-[100px] w-full rounded-md border border-gray-300 bg-white px-3 py-2 font-mono text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
                  placeholder='{"min_password_length": 12, "require_2fa": true}'
                />
              </div>
              <div className="flex justify-end gap-2 pt-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowPolicyModal(false)}
                  className={BTN_SECONDARY}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={submitting} className={BTN_PRIMARY}>
                  {submitting ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                      Creating...
                    </>
                  ) : (
                    <>
                      <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
                      Create Policy
                    </>
                  )}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* VULNERABILITY DETAIL MODAL                                         */}
      {/* ================================================================== */}
      {showVulnDetail && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/50 p-4"
          role="dialog"
          aria-modal="true"
          aria-labelledby="vuln-modal-title"
        >
          <div className="max-h-[85vh] w-full max-w-lg overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg">
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <div className="flex items-center gap-3">
                {(() => {
                  const sevCfg = severityConfig[showVulnDetail.severity] || severityConfig.low;
                  return (
                    <span className={`${STATUS_PILL} ${sevCfg.color}`}>
                      {sevCfg.icon}
                      {sevCfg.label}
                    </span>
                  );
                })()}
                <h2 id="vuln-modal-title" className="text-sm font-semibold text-gray-900">
                  Vulnerability Details
                </h2>
              </div>
              <button
                type="button"
                onClick={() => setShowVulnDetail(null)}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
            <div className="space-y-4 px-5 py-4">
              <div>
                <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">Title</h3>
                <p className="text-sm text-gray-900">{showVulnDetail.title || '—'}</p>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">CVE</h3>
                  <p className="font-mono text-sm text-gray-900">{showVulnDetail.cve || '—'}</p>
                </div>
                <div>
                  <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">
                    CVSS Score
                  </h3>
                  <p className={`text-sm font-semibold ${cvssColor(showVulnDetail.cvss)}`}>
                    {formatCvss(showVulnDetail.cvss)}
                  </p>
                </div>
              </div>
              <div>
                <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">Affected</h3>
                <p className="text-sm text-gray-700">{showVulnDetail.affected || '—'}</p>
              </div>
              <div>
                <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">
                  Description
                </h3>
                <p className="text-sm text-gray-700">{showVulnDetail.description || '—'}</p>
              </div>
              <div>
                <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">Solution</h3>
                <p className="text-sm text-gray-700">{showVulnDetail.solution || '—'}</p>
              </div>
              <div className="flex items-center justify-between border-t border-gray-200 pt-4">
                <Badge variant="outline"
                  className={`${BADGE_BASE} border-transparent ${vulnStatusColor(showVulnDetail.status)}`}
                >
                  {showVulnDetail.status || 'unknown'}
                </Badge>
                {showVulnDetail.status === 'open' && (
                  <Button
                    size="sm"
                    className={BTN_PRIMARY}
                    onClick={() => handleUpdateVulnStatus(showVulnDetail.id, 'resolved')}
                  >
                    <CheckCircle className="mr-2 h-4 w-4" aria-hidden="true" />
                    Mark Resolved
                  </Button>
                )}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* CHECK DETAIL MODAL                                                 */}
      {/* ================================================================== */}
      {showCheckDetail && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/50 p-4"
          role="dialog"
          aria-modal="true"
          aria-labelledby="check-modal-title"
        >
          <div className="max-h-[85vh] w-full max-w-md overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg">
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <h2 id="check-modal-title" className="text-sm font-semibold text-gray-900">
                Check Details
              </h2>
              <button
                type="button"
                onClick={() => setShowCheckDetail(null)}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
            <div className="space-y-4 px-5 py-4">
              <div className="flex items-center gap-3">
                {showCheckDetail.status === 'pass' ? (
                  <div className="rounded-md bg-emerald-50 p-2">
                    <CheckCircle className="h-5 w-5 text-emerald-600" aria-hidden="true" />
                  </div>
                ) : showCheckDetail.status === 'fail' ? (
                  <div className="rounded-md bg-red-50 p-2">
                    <XCircle className="h-5 w-5 text-red-600" aria-hidden="true" />
                  </div>
                ) : (
                  <div className="rounded-md bg-amber-50 p-2">
                    <AlertTriangle className="h-5 w-5 text-amber-600" aria-hidden="true" />
                  </div>
                )}
                <div>
                  <h3 className="text-sm font-semibold text-gray-900">{showCheckDetail.name || '—'}</h3>
                  <Badge variant="outline" className={`${BADGE_BASE} mt-1 border-transparent bg-gray-100 text-gray-700`}>
                    {showCheckDetail.category || '—'}
                  </Badge>
                </div>
              </div>
              <div>
                <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">
                  Description
                </h3>
                <p className="text-sm text-gray-700">{showCheckDetail.description || '—'}</p>
              </div>
              {showCheckDetail.details && (
                <div>
                  <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">
                    Details
                  </h3>
                  <p className="overflow-x-auto rounded-md border border-gray-200 bg-gray-50 p-3 font-mono text-sm text-gray-700">
                    {showCheckDetail.details}
                  </p>
                </div>
              )}
              <div>
                <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">
                  Checked At
                </h3>
                <p className="text-sm text-gray-700" suppressHydrationWarning>
                  {formatDate(showCheckDetail.created_at)}
                </p>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
