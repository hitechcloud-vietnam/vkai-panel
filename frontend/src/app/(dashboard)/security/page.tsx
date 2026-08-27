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
// Helpers
// ---------------------------------------------------------------------------

const severityConfig: Record<string, { color: string; icon: React.ReactNode; label: string }> = {
  critical: {
    color: 'bg-red-600 text-white',
    icon: <ShieldAlert className="h-3.5 w-3.5" />,
    label: 'Critical',
  },
  high: {
    color: 'bg-orange-500 text-white',
    icon: <AlertTriangle className="h-3.5 w-3.5" />,
    label: 'High',
  },
  medium: {
    color: 'bg-yellow-500 text-gray-900',
    icon: <AlertTriangle className="h-3.5 w-3.5" />,
    label: 'Medium',
  },
  low: {
    color: 'bg-blue-500 text-white',
    icon: <Bug className="h-3.5 w-3.5" />,
    label: 'Low',
  },
};

const scanStatusConfig: Record<string, { color: string; icon: React.ReactNode }> = {
  completed: { color: 'bg-green-100 text-green-800', icon: <CheckCircle className="h-3.5 w-3.5" /> },
  running: { color: 'bg-blue-100 text-blue-800', icon: <Loader2 className="h-3.5 w-3.5 animate-spin" /> },
  pending: { color: 'bg-gray-100 text-gray-800', icon: <Clock className="h-3.5 w-3.5" /> },
  failed: { color: 'bg-red-100 text-red-800', icon: <XCircle className="h-3.5 w-3.5" /> },
};

function formatDate(dateStr: string | null): string {
  if (!dateStr) return '—';
  return new Date(dateStr).toLocaleString();
}

function truncateId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) + '…' : id;
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
      setScans(res.data.data || []);
      setScansTotal(res.data.total || 0);
    } catch (err: any) {
      console.error('Failed to load scans:', err);
      setScans([]);
    }
  }, [scansPage]);

  const fetchVulnerabilities = useCallback(async () => {
    try {
      const params: any = { page: vulnsPage, per_page: perPage };
      if (vulnSeverityFilter !== 'all') params.severity = vulnSeverityFilter;
      const res = await securityApi.listVulnerabilities(params);
      setVulnerabilities(res.data.data || []);
      setVulnsTotal(res.data.total || 0);
    } catch (err: any) {
      console.error('Failed to load vulnerabilities:', err);
      setVulnerabilities([]);
    }
  }, [vulnsPage, vulnSeverityFilter]);

  const fetchChecks = useCallback(async () => {
    if (!selectedScanId) {
      setChecks([]);
      return;
    }
    try {
      const res = await securityApi.listChecksByScan(selectedScanId);
      setChecks(res.data.data || res.data || []);
    } catch (err: any) {
      console.error('Failed to load checks:', err);
      setChecks([]);
    }
  }, [selectedScanId]);

  const fetchPolicies = useCallback(async () => {
    try {
      const res = await securityApi.listPolicies();
      setPolicies(res.data.data || res.data || []);
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
      setError(err.message || 'Failed to load security data');
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
    runningScans: scans.filter((s) => s.status === 'running').length,
    totalVulns: vulnsTotal,
    criticalVulns: vulnerabilities.filter((v) => v.severity === 'critical').length,
    highVulns: vulnerabilities.filter((v) => v.severity === 'high').length,
    mediumVulns: vulnerabilities.filter((v) => v.severity === 'medium').length,
    lowVulns: vulnerabilities.filter((v) => v.severity === 'low').length,
    activePolicies: policies.filter((p) => p.is_active).length,
    totalPolicies: policies.length,
    complianceRate:
      policies.length > 0
        ? Math.round((policies.filter((p) => p.is_active).length / policies.length) * 100)
        : 0,
  };

  // ---------------------------------------------------------------------------
  // Filtered data
  // ---------------------------------------------------------------------------

  const filteredScans = scans.filter((s) => {
    if (scanStatusFilter !== 'all' && s.status !== scanStatusFilter) return false;
    if (scanSearch) {
      const q = scanSearch.toLowerCase();
      return (
        s.scan_type.toLowerCase().includes(q) ||
        s.server_id.toLowerCase().includes(q) ||
        s.status.toLowerCase().includes(q)
      );
    }
    return true;
  });

  const filteredVulns = vulnerabilities.filter((v) => {
    if (vulnSeverityFilter !== 'all' && v.severity !== vulnSeverityFilter) return false;
    if (vulnStatusFilter !== 'all' && v.status !== vulnStatusFilter) return false;
    return true;
  });

  const filteredChecks = checks.filter((c) => {
    if (checkStatusFilter !== 'all' && c.status !== checkStatusFilter) return false;
    return true;
  });

  const filteredPolicies = policies.filter((p) => {
    if (policyStatusFilter === 'active' && !p.is_active) return false;
    if (policyStatusFilter === 'inactive' && p.is_active) return false;
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
      setFormError(err.response?.data?.message || 'Failed to start scan');
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
      setToast({ type: 'error', message: err.response?.data?.message || 'Failed to delete scan' });
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
      setFormError(err.response?.data?.message || 'Failed to create policy');
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
      setToast({ type: 'error', message: err.response?.data?.message || 'Failed to delete policy' });
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
      setToast({ type: 'error', message: err.response?.data?.message || 'Failed to update policy' });
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
      setToast({ type: 'error', message: err.response?.data?.message || 'Failed to update vulnerability' });
    }
  };

  // ---------------------------------------------------------------------------
  // Loading state
  // ---------------------------------------------------------------------------

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="h-8 w-8 animate-spin text-gray-400" />
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
          className={`fixed top-4 right-4 z-50 flex items-center gap-2 rounded-lg px-4 py-3 shadow-lg text-sm font-medium ${
            toast.type === 'success'
              ? 'bg-green-600 text-white'
              : 'bg-red-600 text-white'
          }`}
        >
          {toast.type === 'success' ? (
            <CheckCircle className="h-4 w-4" />
          ) : (
            <XCircle className="h-4 w-4" />
          )}
          {toast.message}
          <button onClick={() => setToast(null)} className="ml-2">
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gray-50">Security</h1>
          <p className="text-gray-400 mt-1">
            Manage security scans, vulnerabilities, policies, and checks
          </p>
        </div>
        <div className="flex space-x-2">
          <Button variant="outline" onClick={fetchAll} className="border-gray-700 text-gray-300 hover:bg-gray-800">
            <RefreshCw className="h-4 w-4 mr-2" />
            Refresh
          </Button>
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card className="bg-gray-900 border-gray-800">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-gray-300">Total Scans</CardTitle>
            <Activity className="h-4 w-4 text-gray-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-gray-50">{stats.totalScans}</div>
            <p className="text-xs text-gray-500">
              {stats.runningScans} currently running
            </p>
          </CardContent>
        </Card>

        <Card className="bg-gray-900 border-gray-800">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-gray-300">Vulnerabilities</CardTitle>
            <Bug className="h-4 w-4 text-gray-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-gray-50">{stats.totalVulns}</div>
            <div className="flex gap-2 mt-1">
              <span className="text-xs text-red-400">{stats.criticalVulns} critical</span>
              <span className="text-xs text-orange-400">{stats.highVulns} high</span>
              <span className="text-xs text-yellow-400">{stats.mediumVulns} medium</span>
              <span className="text-xs text-blue-400">{stats.lowVulns} low</span>
            </div>
          </CardContent>
        </Card>

        <Card className="bg-gray-900 border-gray-800">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-gray-300">Active Policies</CardTitle>
            <Lock className="h-4 w-4 text-gray-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-gray-50">{stats.activePolicies}</div>
            <p className="text-xs text-gray-500">
              of {stats.totalPolicies} total policies
            </p>
          </CardContent>
        </Card>

        <Card className="bg-gray-900 border-gray-800">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-gray-300">Compliance Rate</CardTitle>
            <ShieldCheck className="h-4 w-4 text-gray-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-gray-50">{stats.complianceRate}%</div>
            <div className="w-full bg-gray-700 rounded-full h-1.5 mt-2">
              <div
                className={`h-1.5 rounded-full ${
                  stats.complianceRate >= 80
                    ? 'bg-green-500'
                    : stats.complianceRate >= 50
                    ? 'bg-yellow-500'
                    : 'bg-red-500'
                }`}
                style={{ width: `${stats.complianceRate}%` }}
              />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Tabs */}
      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList className="bg-gray-800 border-gray-700">
          <TabsTrigger value="scans" className="data-[state=active]:bg-gray-700 data-[state=active]:text-gray-50 text-gray-400">
            <Search className="h-4 w-4 mr-2" />
            Scans
          </TabsTrigger>
          <TabsTrigger value="vulnerabilities" className="data-[state=active]:bg-gray-700 data-[state=active]:text-gray-50 text-gray-400">
            <Bug className="h-4 w-4 mr-2" />
            Vulnerabilities
          </TabsTrigger>
          <TabsTrigger value="policies" className="data-[state=active]:bg-gray-700 data-[state=active]:text-gray-50 text-gray-400">
            <FileText className="h-4 w-4 mr-2" />
            Policies
          </TabsTrigger>
          <TabsTrigger value="checks" className="data-[state=active]:bg-gray-700 data-[state=active]:text-gray-50 text-gray-400">
            <ShieldCheck className="h-4 w-4 mr-2" />
            Checks
          </TabsTrigger>
        </TabsList>

        {/* ================================================================ */}
        {/* SCANS TAB                                                        */}
        {/* ================================================================ */}
        <TabsContent value="scans" className="space-y-4">
          {/* Toolbar */}
          <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
            <div className="flex items-center gap-2 flex-wrap">
              <Input
                placeholder="Search scans..."
                value={scanSearch}
                onChange={(e) => setScanSearch(e.target.value)}
                className="w-64 bg-gray-800 border-gray-700 text-gray-200 placeholder:text-gray-500"
              />
              <Select value={scanStatusFilter} onValueChange={setScanStatusFilter}>
                <SelectTrigger className="w-40 bg-gray-800 border-gray-700 text-gray-200">
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent className="bg-gray-800 border-gray-700">
                  <SelectItem value="all">All Statuses</SelectItem>
                  <SelectItem value="completed">Completed</SelectItem>
                  <SelectItem value="running">Running</SelectItem>
                  <SelectItem value="pending">Pending</SelectItem>
                  <SelectItem value="failed">Failed</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button onClick={() => { setFormError(null); setShowScanModal(true); }} className="bg-blue-600 hover:bg-blue-700 text-white">
              <Play className="h-4 w-4 mr-2" />
              Start Scan
            </Button>
          </div>

          {/* Scans Table */}
          <Card className="bg-gray-900 border-gray-800">
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-gray-800">
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">ID</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Type</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Status</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Score</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Findings</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Started At</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Completed At</th>
                      <th className="text-right py-3 px-4 text-gray-400 font-medium">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredScans.length === 0 ? (
                      <tr>
                        <td colSpan={8} className="text-center py-12 text-gray-500">
                          <Shield className="mx-auto mb-2 h-8 w-8" />
                          No scans found
                        </td>
                      </tr>
                    ) : (
                      filteredScans.map((scan) => {
                        const statusCfg = scanStatusConfig[scan.status] || scanStatusConfig.pending;
                        return (
                          <tr key={scan.id} className="border-b border-gray-800 hover:bg-gray-800/50 transition-colors">
                            <td className="py-3 px-4 text-gray-300 font-mono text-xs">{truncateId(scan.id)}</td>
                            <td className="py-3 px-4">
                              <Badge variant="secondary" className="bg-gray-700 text-gray-200">
                                {scan.scan_type}
                              </Badge>
                            </td>
                            <td className="py-3 px-4">
                              <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusCfg.color}`}>
                                {statusCfg.icon}
                                {scan.status}
                              </span>
                            </td>
                            <td className="py-3 px-4">
                              <span className={`font-bold ${
                                scan.score >= 80 ? 'text-green-400' : scan.score >= 50 ? 'text-yellow-400' : 'text-red-400'
                              }`}>
                                {scan.score}%
                              </span>
                            </td>
                            <td className="py-3 px-4 text-gray-300">
                              <div className="flex items-center gap-2">
                                <span className="text-green-400 text-xs">{scan.passed_checks} pass</span>
                                <span className="text-red-400 text-xs">{scan.failed_checks} fail</span>
                                {scan.warnings > 0 && (
                                  <span className="text-yellow-400 text-xs">{scan.warnings} warn</span>
                                )}
                              </div>
                            </td>
                            <td className="py-3 px-4 text-gray-400 text-xs">{formatDate(scan.started_at)}</td>
                            <td className="py-3 px-4 text-gray-400 text-xs">{formatDate(scan.completed_at)}</td>
                            <td className="py-3 px-4 text-right">
                              <div className="flex items-center justify-end gap-1">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="text-gray-400 hover:text-gray-200 hover:bg-gray-700"
                                  onClick={() => {
                                    setSelectedScanId(scan.id);
                                    setActiveTab('checks');
                                  }}
                                  title="View checks"
                                >
                                  <Eye className="h-4 w-4" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="text-red-400 hover:text-red-300 hover:bg-gray-700"
                                  onClick={() => handleDeleteScan(scan.id)}
                                  title="Delete scan"
                                >
                                  <Trash2 className="h-4 w-4" />
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
              <p className="text-sm text-gray-400">
                Showing {(scansPage - 1) * perPage + 1}–{Math.min(scansPage * perPage, scansTotal)} of {scansTotal}
              </p>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={scansPage <= 1}
                  onClick={() => setScansPage((p) => p - 1)}
                  className="border-gray-700 text-gray-300 hover:bg-gray-800"
                >
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <span className="text-sm text-gray-400">Page {scansPage}</span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={scansPage * perPage >= scansTotal}
                  onClick={() => setScansPage((p) => p + 1)}
                  className="border-gray-700 text-gray-300 hover:bg-gray-800"
                >
                  <ChevronRight className="h-4 w-4" />
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
          <div className="flex items-center gap-2 flex-wrap">
            <Select value={vulnSeverityFilter} onValueChange={setVulnSeverityFilter}>
              <SelectTrigger className="w-40 bg-gray-800 border-gray-700 text-gray-200">
                <SelectValue placeholder="Severity" />
              </SelectTrigger>
              <SelectContent className="bg-gray-800 border-gray-700">
                <SelectItem value="all">All Severities</SelectItem>
                <SelectItem value="critical">Critical</SelectItem>
                <SelectItem value="high">High</SelectItem>
                <SelectItem value="medium">Medium</SelectItem>
                <SelectItem value="low">Low</SelectItem>
              </SelectContent>
            </Select>
            <Select value={vulnStatusFilter} onValueChange={setVulnStatusFilter}>
              <SelectTrigger className="w-40 bg-gray-800 border-gray-700 text-gray-200">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent className="bg-gray-800 border-gray-700">
                <SelectItem value="all">All Statuses</SelectItem>
                <SelectItem value="open">Open</SelectItem>
                <SelectItem value="resolved">Resolved</SelectItem>
                <SelectItem value="false_positive">False Positive</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Vulnerabilities Table */}
          <Card className="bg-gray-900 border-gray-800">
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-gray-800">
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Severity</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Title</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">CVE</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">CVSS</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Affected</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Status</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Created</th>
                      <th className="text-right py-3 px-4 text-gray-400 font-medium">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredVulns.length === 0 ? (
                      <tr>
                        <td colSpan={8} className="text-center py-12 text-gray-500">
                          <ShieldCheck className="mx-auto mb-2 h-8 w-8" />
                          No vulnerabilities found
                        </td>
                      </tr>
                    ) : (
                      filteredVulns.map((vuln) => {
                        const sevCfg = severityConfig[vuln.severity] || severityConfig.low;
                        return (
                          <tr key={vuln.id} className="border-b border-gray-800 hover:bg-gray-800/50 transition-colors">
                            <td className="py-3 px-4">
                              <span className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${sevCfg.color}`}>
                                {sevCfg.icon}
                                {sevCfg.label}
                              </span>
                            </td>
                            <td className="py-3 px-4 text-gray-200 font-medium max-w-xs truncate">{vuln.title}</td>
                            <td className="py-3 px-4 text-gray-400 font-mono text-xs">
                              {vuln.cve || '—'}
                            </td>
                            <td className="py-3 px-4">
                              <span className={`font-bold ${
                                vuln.cvss >= 9 ? 'text-red-400' : vuln.cvss >= 7 ? 'text-orange-400' : vuln.cvss >= 4 ? 'text-yellow-400' : 'text-blue-400'
                              }`}>
                                {vuln.cvss.toFixed(1)}
                              </span>
                            </td>
                            <td className="py-3 px-4 text-gray-400 text-xs max-w-[200px] truncate">{vuln.affected}</td>
                            <td className="py-3 px-4">
                              <Badge
                                variant={vuln.status === 'resolved' ? 'success' : vuln.status === 'open' ? 'destructive' : 'secondary'}
                                className={vuln.status === 'open' ? 'bg-red-900/50 text-red-300 border-red-800' : vuln.status === 'resolved' ? 'bg-green-900/50 text-green-300 border-green-800' : 'bg-gray-700 text-gray-300'}
                              >
                                {vuln.status}
                              </Badge>
                            </td>
                            <td className="py-3 px-4 text-gray-400 text-xs">{formatDate(vuln.created_at)}</td>
                            <td className="py-3 px-4 text-right">
                              <div className="flex items-center justify-end gap-1">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="text-gray-400 hover:text-gray-200 hover:bg-gray-700"
                                  onClick={() => setShowVulnDetail(vuln)}
                                  title="View details"
                                >
                                  <Eye className="h-4 w-4" />
                                </Button>
                                {vuln.status === 'open' && (
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    className="text-green-400 hover:text-green-300 hover:bg-gray-700"
                                    onClick={() => handleUpdateVulnStatus(vuln.id, 'resolved')}
                                    title="Mark as resolved"
                                  >
                                    <CheckCircle className="h-4 w-4" />
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
              <p className="text-sm text-gray-400">
                Showing {(vulnsPage - 1) * perPage + 1}–{Math.min(vulnsPage * perPage, vulnsTotal)} of {vulnsTotal}
              </p>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={vulnsPage <= 1}
                  onClick={() => setVulnsPage((p) => p - 1)}
                  className="border-gray-700 text-gray-300 hover:bg-gray-800"
                >
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <span className="text-sm text-gray-400">Page {vulnsPage}</span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={vulnsPage * perPage >= vulnsTotal}
                  onClick={() => setVulnsPage((p) => p + 1)}
                  className="border-gray-700 text-gray-300 hover:bg-gray-800"
                >
                  <ChevronRight className="h-4 w-4" />
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
          <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <Select value={policyStatusFilter} onValueChange={setPolicyStatusFilter}>
                <SelectTrigger className="w-40 bg-gray-800 border-gray-700 text-gray-200">
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent className="bg-gray-800 border-gray-700">
                  <SelectItem value="all">All</SelectItem>
                  <SelectItem value="active">Active</SelectItem>
                  <SelectItem value="inactive">Inactive</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button onClick={() => { setFormError(null); setShowPolicyModal(true); }} className="bg-blue-600 hover:bg-blue-700 text-white">
              <Plus className="h-4 w-4 mr-2" />
              Create Policy
            </Button>
          </div>

          {/* Policies Grid */}
          {filteredPolicies.length === 0 ? (
            <Card className="bg-gray-900 border-gray-800">
              <CardContent className="py-12 text-center text-gray-500">
                <FileText className="mx-auto mb-2 h-8 w-8" />
                No policies found
              </CardContent>
            </Card>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {filteredPolicies.map((policy) => (
                <Card key={policy.id} className="bg-gray-900 border-gray-800 hover:border-gray-700 transition-colors">
                  <CardHeader className="pb-3">
                    <div className="flex items-start justify-between">
                      <div className="flex items-center gap-2">
                        <div className={`p-2 rounded-lg ${policy.is_active ? 'bg-green-900/50' : 'bg-gray-800'}`}>
                          <Shield className={`h-4 w-4 ${policy.is_active ? 'text-green-400' : 'text-gray-500'}`} />
                        </div>
                        <div>
                          <CardTitle className="text-sm text-gray-200">{policy.name}</CardTitle>
                          <p className="text-xs text-gray-500 mt-0.5">{policy.category}</p>
                        </div>
                      </div>
                      <Badge
                        variant={policy.is_active ? 'success' : 'secondary'}
                        className={policy.is_active ? 'bg-green-900/50 text-green-300 border-green-800' : 'bg-gray-700 text-gray-400'}
                      >
                        {policy.is_active ? 'Active' : 'Inactive'}
                      </Badge>
                    </div>
                  </CardHeader>
                  <CardContent className="pt-0">
                    <p className="text-sm text-gray-400 mb-3 line-clamp-2">
                      {policy.description || 'No description'}
                    </p>
                    <div className="flex items-center justify-between">
                      <span className="text-xs text-gray-500">
                        Created {formatDate(policy.created_at)}
                      </span>
                      <div className="flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-gray-400 hover:text-gray-200 hover:bg-gray-700"
                          onClick={() => handleTogglePolicy(policy)}
                          title={policy.is_active ? 'Deactivate' : 'Activate'}
                        >
                          {policy.is_active ? (
                            <XCircle className="h-4 w-4" />
                          ) : (
                            <CheckCircle className="h-4 w-4" />
                          )}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-red-400 hover:text-red-300 hover:bg-gray-700"
                          onClick={() => handleDeletePolicy(policy.id)}
                          title="Delete policy"
                        >
                          <Trash2 className="h-4 w-4" />
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
          <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
            <div className="flex items-center gap-2 flex-wrap">
              {!selectedScanId ? (
                <p className="text-sm text-gray-400">Select a scan from the Scans tab to view its checks, or choose one below:</p>
              ) : (
                <div className="flex items-center gap-2">
                  <span className="text-sm text-gray-400">
                    Showing checks for scan: <span className="font-mono text-gray-300">{truncateId(selectedScanId)}</span>
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-gray-400 hover:text-gray-200"
                    onClick={() => setSelectedScanId(null)}
                  >
                    <X className="h-3 w-3" />
                  </Button>
                </div>
              )}
              <Select
                value={selectedScanId || ''}
                onValueChange={(val) => setSelectedScanId(val || null)}
              >
                <SelectTrigger className="w-64 bg-gray-800 border-gray-700 text-gray-200">
                  <SelectValue placeholder="Select a scan..." />
                </SelectTrigger>
                <SelectContent className="bg-gray-800 border-gray-700">
                  {scans.map((scan) => (
                    <SelectItem key={scan.id} value={scan.id}>
                      {scan.scan_type} — {truncateId(scan.id)} ({scan.status})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select value={checkStatusFilter} onValueChange={setCheckStatusFilter}>
                <SelectTrigger className="w-36 bg-gray-800 border-gray-700 text-gray-200">
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent className="bg-gray-800 border-gray-700">
                  <SelectItem value="all">All</SelectItem>
                  <SelectItem value="pass">Pass</SelectItem>
                  <SelectItem value="fail">Fail</SelectItem>
                  <SelectItem value="warning">Warning</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          {/* Checks Table */}
          <Card className="bg-gray-900 border-gray-800">
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-gray-800">
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Status</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Name</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Category</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Description</th>
                      <th className="text-left py-3 px-4 text-gray-400 font-medium">Checked At</th>
                      <th className="text-right py-3 px-4 text-gray-400 font-medium">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {!selectedScanId ? (
                      <tr>
                        <td colSpan={6} className="text-center py-12 text-gray-500">
                          <ShieldCheck className="mx-auto mb-2 h-8 w-8" />
                          Select a scan to view security checks
                        </td>
                      </tr>
                    ) : filteredChecks.length === 0 ? (
                      <tr>
                        <td colSpan={6} className="text-center py-12 text-gray-500">
                          <ShieldCheck className="mx-auto mb-2 h-8 w-8" />
                          No checks found for this scan
                        </td>
                      </tr>
                    ) : (
                      filteredChecks.map((check) => (
                        <tr key={check.id} className="border-b border-gray-800 hover:bg-gray-800/50 transition-colors">
                          <td className="py-3 px-4">
                            {check.status === 'pass' ? (
                              <span className="inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold bg-green-900/50 text-green-300 border border-green-800">
                                <CheckCircle className="h-3.5 w-3.5" />
                                Pass
                              </span>
                            ) : check.status === 'fail' ? (
                              <span className="inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold bg-red-900/50 text-red-300 border border-red-800">
                                <XCircle className="h-3.5 w-3.5" />
                                Fail
                              </span>
                            ) : (
                              <span className="inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold bg-yellow-900/50 text-yellow-300 border border-yellow-800">
                                <AlertTriangle className="h-3.5 w-3.5" />
                                Warning
                              </span>
                            )}
                          </td>
                          <td className="py-3 px-4 text-gray-200 font-medium">{check.name}</td>
                          <td className="py-3 px-4">
                            <Badge variant="secondary" className="bg-gray-700 text-gray-300">
                              {check.category}
                            </Badge>
                          </td>
                          <td className="py-3 px-4 text-gray-400 text-xs max-w-xs truncate">{check.description}</td>
                          <td className="py-3 px-4 text-gray-400 text-xs">{formatDate(check.created_at)}</td>
                          <td className="py-3 px-4 text-right">
                            <Button
                              variant="ghost"
                              size="sm"
                              className="text-gray-400 hover:text-gray-200 hover:bg-gray-700"
                              onClick={() => setShowCheckDetail(check)}
                              title="View details"
                            >
                              <Eye className="h-4 w-4" />
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
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
              <Card className="bg-gray-900 border-gray-800">
                <CardContent className="py-4 flex items-center gap-3">
                  <div className="p-2 rounded-lg bg-green-900/50">
                    <CheckCircle className="h-5 w-5 text-green-400" />
                  </div>
                  <div>
                    <p className="text-2xl font-bold text-gray-50">{checks.filter((c) => c.status === 'pass').length}</p>
                    <p className="text-xs text-gray-500">Passed</p>
                  </div>
                </CardContent>
              </Card>
              <Card className="bg-gray-900 border-gray-800">
                <CardContent className="py-4 flex items-center gap-3">
                  <div className="p-2 rounded-lg bg-red-900/50">
                    <XCircle className="h-5 w-5 text-red-400" />
                  </div>
                  <div>
                    <p className="text-2xl font-bold text-gray-50">{checks.filter((c) => c.status === 'fail').length}</p>
                    <p className="text-xs text-gray-500">Failed</p>
                  </div>
                </CardContent>
              </Card>
              <Card className="bg-gray-900 border-gray-800">
                <CardContent className="py-4 flex items-center gap-3">
                  <div className="p-2 rounded-lg bg-yellow-900/50">
                    <AlertTriangle className="h-5 w-5 text-yellow-400" />
                  </div>
                  <div>
                    <p className="text-2xl font-bold text-gray-50">{checks.filter((c) => c.status === 'warning').length}</p>
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
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
          <div className="bg-gray-900 border border-gray-700 rounded-xl shadow-2xl w-full max-w-md mx-4">
            <div className="flex items-center justify-between p-6 border-b border-gray-800">
              <h2 className="text-lg font-semibold text-gray-100">Start Security Scan</h2>
              <button onClick={() => setShowScanModal(false)} className="text-gray-400 hover:text-gray-200">
                <X className="h-5 w-5" />
              </button>
            </div>
            <form onSubmit={handleCreateScan} className="p-6 space-y-4">
              {formError && (
                <div className="flex items-center gap-2 p-3 bg-red-900/30 border border-red-800 rounded-lg text-red-300 text-sm">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  {formError}
                </div>
              )}
              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-300">Server ID</label>
                <Input
                  placeholder="Enter server ID"
                  value={scanForm.server_id}
                  onChange={(e) => setScanForm((prev) => ({ ...prev, server_id: e.target.value }))}
                  className="bg-gray-800 border-gray-700 text-gray-200 placeholder:text-gray-500"
                  required
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-300">Scan Type</label>
                <Select
                  value={scanForm.scan_type}
                  onValueChange={(val) => setScanForm((prev) => ({ ...prev, scan_type: val }))}
                >
                  <SelectTrigger className="bg-gray-800 border-gray-700 text-gray-200">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className="bg-gray-800 border-gray-700">
                    <SelectItem value="full">Full Scan</SelectItem>
                    <SelectItem value="quick">Quick Scan</SelectItem>
                    <SelectItem value="vulnerability">Vulnerability Scan</SelectItem>
                    <SelectItem value="compliance">Compliance Scan</SelectItem>
                    <SelectItem value="port">Port Scan</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex justify-end gap-3 pt-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowScanModal(false)}
                  className="border-gray-700 text-gray-300 hover:bg-gray-800"
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={submitting} className="bg-blue-600 hover:bg-blue-700 text-white">
                  {submitting ? (
                    <>
                      <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                      Starting...
                    </>
                  ) : (
                    <>
                      <Play className="h-4 w-4 mr-2" />
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
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
          <div className="bg-gray-900 border border-gray-700 rounded-xl shadow-2xl w-full max-w-lg mx-4">
            <div className="flex items-center justify-between p-6 border-b border-gray-800">
              <h2 className="text-lg font-semibold text-gray-100">Create Security Policy</h2>
              <button onClick={() => setShowPolicyModal(false)} className="text-gray-400 hover:text-gray-200">
                <X className="h-5 w-5" />
              </button>
            </div>
            <form onSubmit={handleCreatePolicy} className="p-6 space-y-4">
              {formError && (
                <div className="flex items-center gap-2 p-3 bg-red-900/30 border border-red-800 rounded-lg text-red-300 text-sm">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  {formError}
                </div>
              )}
              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-300">Name</label>
                <Input
                  placeholder="Policy name"
                  value={policyForm.name}
                  onChange={(e) => setPolicyForm((prev) => ({ ...prev, name: e.target.value }))}
                  className="bg-gray-800 border-gray-700 text-gray-200 placeholder:text-gray-500"
                  required
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-300">Description</label>
                <Input
                  placeholder="Brief description"
                  value={policyForm.description}
                  onChange={(e) => setPolicyForm((prev) => ({ ...prev, description: e.target.value }))}
                  className="bg-gray-800 border-gray-700 text-gray-200 placeholder:text-gray-500"
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-300">Category</label>
                <Select
                  value={policyForm.category}
                  onValueChange={(val) => setPolicyForm((prev) => ({ ...prev, category: val }))}
                >
                  <SelectTrigger className="bg-gray-800 border-gray-700 text-gray-200">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className="bg-gray-800 border-gray-700">
                    <SelectItem value="general">General</SelectItem>
                    <SelectItem value="network">Network</SelectItem>
                    <SelectItem value="authentication">Authentication</SelectItem>
                    <SelectItem value="encryption">Encryption</SelectItem>
                    <SelectItem value="access_control">Access Control</SelectItem>
                    <SelectItem value="compliance">Compliance</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-300">Rules (JSON)</label>
                <textarea
                  value={policyForm.rules}
                  onChange={(e) => setPolicyForm((prev) => ({ ...prev, rules: e.target.value }))}
                  className="flex min-h-[100px] w-full rounded-md border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-200 placeholder:text-gray-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gray-950 focus-visible:ring-offset-2 font-mono"
                  placeholder='{"min_password_length": 12, "require_2fa": true}'
                />
              </div>
              <div className="flex justify-end gap-3 pt-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowPolicyModal(false)}
                  className="border-gray-700 text-gray-300 hover:bg-gray-800"
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={submitting} className="bg-blue-600 hover:bg-blue-700 text-white">
                  {submitting ? (
                    <>
                      <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                      Creating...
                    </>
                  ) : (
                    <>
                      <Plus className="h-4 w-4 mr-2" />
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
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
          <div className="bg-gray-900 border border-gray-700 rounded-xl shadow-2xl w-full max-w-lg mx-4">
            <div className="flex items-center justify-between p-6 border-b border-gray-800">
              <div className="flex items-center gap-3">
                {(() => {
                  const sevCfg = severityConfig[showVulnDetail.severity] || severityConfig.low;
                  return (
                    <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold ${sevCfg.color}`}>
                      {sevCfg.icon}
                      {sevCfg.label}
                    </span>
                  );
                })()}
                <h2 className="text-lg font-semibold text-gray-100">Vulnerability Details</h2>
              </div>
              <button onClick={() => setShowVulnDetail(null)} className="text-gray-400 hover:text-gray-200">
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="p-6 space-y-4">
              <div>
                <h3 className="text-sm font-medium text-gray-400 mb-1">Title</h3>
                <p className="text-gray-200">{showVulnDetail.title}</p>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <h3 className="text-sm font-medium text-gray-400 mb-1">CVE</h3>
                  <p className="text-gray-200 font-mono text-sm">{showVulnDetail.cve || '—'}</p>
                </div>
                <div>
                  <h3 className="text-sm font-medium text-gray-400 mb-1">CVSS Score</h3>
                  <p className={`font-bold ${
                    showVulnDetail.cvss >= 9 ? 'text-red-400' : showVulnDetail.cvss >= 7 ? 'text-orange-400' : showVulnDetail.cvss >= 4 ? 'text-yellow-400' : 'text-blue-400'
                  }`}>
                    {showVulnDetail.cvss.toFixed(1)}
                  </p>
                </div>
              </div>
              <div>
                <h3 className="text-sm font-medium text-gray-400 mb-1">Affected</h3>
                <p className="text-gray-200 text-sm">{showVulnDetail.affected}</p>
              </div>
              <div>
                <h3 className="text-sm font-medium text-gray-400 mb-1">Description</h3>
                <p className="text-gray-300 text-sm">{showVulnDetail.description}</p>
              </div>
              <div>
                <h3 className="text-sm font-medium text-gray-400 mb-1">Solution</h3>
                <p className="text-gray-300 text-sm">{showVulnDetail.solution}</p>
              </div>
              <div className="flex items-center justify-between pt-2">
                <Badge
                  variant={showVulnDetail.status === 'resolved' ? 'success' : showVulnDetail.status === 'open' ? 'destructive' : 'secondary'}
                  className={showVulnDetail.status === 'open' ? 'bg-red-900/50 text-red-300 border-red-800' : showVulnDetail.status === 'resolved' ? 'bg-green-900/50 text-green-300 border-green-800' : 'bg-gray-700 text-gray-300'}
                >
                  {showVulnDetail.status}
                </Badge>
                {showVulnDetail.status === 'open' && (
                  <Button
                    size="sm"
                    className="bg-green-600 hover:bg-green-700 text-white"
                    onClick={() => handleUpdateVulnStatus(showVulnDetail.id, 'resolved')}
                  >
                    <CheckCircle className="h-4 w-4 mr-2" />
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
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
          <div className="bg-gray-900 border border-gray-700 rounded-xl shadow-2xl w-full max-w-md mx-4">
            <div className="flex items-center justify-between p-6 border-b border-gray-800">
              <h2 className="text-lg font-semibold text-gray-100">Check Details</h2>
              <button onClick={() => setShowCheckDetail(null)} className="text-gray-400 hover:text-gray-200">
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="p-6 space-y-4">
              <div className="flex items-center gap-3">
                {showCheckDetail.status === 'pass' ? (
                  <div className="p-2 rounded-lg bg-green-900/50">
                    <CheckCircle className="h-6 w-6 text-green-400" />
                  </div>
                ) : showCheckDetail.status === 'fail' ? (
                  <div className="p-2 rounded-lg bg-red-900/50">
                    <XCircle className="h-6 w-6 text-red-400" />
                  </div>
                ) : (
                  <div className="p-2 rounded-lg bg-yellow-900/50">
                    <AlertTriangle className="h-6 w-6 text-yellow-400" />
                  </div>
                )}
                <div>
                  <h3 className="text-gray-200 font-medium">{showCheckDetail.name}</h3>
                  <Badge variant="secondary" className="bg-gray-700 text-gray-300 mt-1">
                    {showCheckDetail.category}
                  </Badge>
                </div>
              </div>
              <div>
                <h3 className="text-sm font-medium text-gray-400 mb-1">Description</h3>
                <p className="text-gray-300 text-sm">{showCheckDetail.description}</p>
              </div>
              {showCheckDetail.details && (
                <div>
                  <h3 className="text-sm font-medium text-gray-400 mb-1">Details</h3>
                  <p className="text-gray-300 text-sm bg-gray-800 rounded-lg p-3 font-mono">{showCheckDetail.details}</p>
                </div>
              )}
              <div>
                <h3 className="text-sm font-medium text-gray-400 mb-1">Checked At</h3>
                <p className="text-gray-300 text-sm">{formatDate(showCheckDetail.created_at)}</p>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
