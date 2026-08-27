'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  Globe,
  Plus,
  Trash2,
  Edit,
  RefreshCw,
  X,
  AlertTriangle,
  CheckCircle,
  XCircle,
  Filter,
  Clock,
  Hash,
  ArrowLeft,
  Layers,
  FileText,
} from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { dnsApi } from '@/services/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface DNSZone {
  id: string;
  tenant_id: string;
  name: string;
  type: string;
  provider: string;
  status: string;
  records_count?: number;
  created_at: string;
  updated_at: string;
}

interface DNSRecord {
  id: string;
  zone_id: string;
  name: string;
  type: string;
  content: string;
  ttl: number;
  priority: number | null;
  status: string;
  created_at: string;
  updated_at: string;
}

interface ZoneFormData {
  name: string;
  provider: string;
}

interface RecordFormData {
  type: string;
  name: string;
  value: string;
  ttl: number;
  priority: string;
}

const EMPTY_ZONE_FORM: ZoneFormData = {
  name: '',
  provider: 'local',
};

const EMPTY_RECORD_FORM: RecordFormData = {
  type: 'A',
  name: '',
  value: '',
  ttl: 3600,
  priority: '',
};

const RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'CAA', 'PTR'];

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function DNSPage() {
  // Tab state
  const [activeTab, setActiveTab] = useState<'zones' | 'records'>('zones');

  // Zone data
  const [zones, setZones] = useState<DNSZone[]>([]);
  const [zonesLoading, setZonesLoading] = useState(true);
  const [zonesError, setZonesError] = useState<string | null>(null);

  // Record data
  const [records, setRecords] = useState<DNSRecord[]>([]);
  const [recordsLoading, setRecordsLoading] = useState(false);
  const [recordsError, setRecordsError] = useState<string | null>(null);
  const [selectedZone, setSelectedZone] = useState<DNSZone | null>(null);

  // Filters
  const [zoneSearch, setZoneSearch] = useState('');
  const [zoneStatusFilter, setZoneStatusFilter] = useState<string>('all');
  const [recordSearch, setRecordSearch] = useState('');
  const [recordTypeFilter, setRecordTypeFilter] = useState<string>('all');

  // Zone modal state
  const [showZoneForm, setShowZoneForm] = useState(false);
  const [editingZone, setEditingZone] = useState<DNSZone | null>(null);
  const [zoneFormData, setZoneFormData] = useState<ZoneFormData>(EMPTY_ZONE_FORM);
  const [zoneSubmitting, setZoneSubmitting] = useState(false);
  const [zoneFormError, setZoneFormError] = useState<string | null>(null);

  // Record modal state
  const [showRecordForm, setShowRecordForm] = useState(false);
  const [editingRecord, setEditingRecord] = useState<DNSRecord | null>(null);
  const [recordFormData, setRecordFormData] = useState<RecordFormData>(EMPTY_RECORD_FORM);
  const [recordSubmitting, setRecordSubmitting] = useState(false);
  const [recordFormError, setRecordFormError] = useState<string | null>(null);

  // Delete confirmation
  const [deletingZone, setDeletingZone] = useState<DNSZone | null>(null);
  const [deletingRecord, setDeletingRecord] = useState<DNSRecord | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Toast
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // ---------------------------------------------------------------------------
  // Data fetching
  // ---------------------------------------------------------------------------

  const fetchZones = useCallback(async () => {
    try {
      setZonesLoading(true);
      setZonesError(null);
      const res = await dnsApi.listZones();
      setZones(res.data.data || res.data || []);
    } catch (err: any) {
      console.error('Failed to load DNS zones:', err);
      setZonesError(err.response?.data?.message || 'Failed to load DNS zones');
    } finally {
      setZonesLoading(false);
    }
  }, []);

  const fetchRecords = useCallback(async (zoneId: string) => {
    try {
      setRecordsLoading(true);
      setRecordsError(null);
      const res = await dnsApi.listRecords(zoneId);
      setRecords(res.data.data || res.data || []);
    } catch (err: any) {
      console.error('Failed to load DNS records:', err);
      setRecordsError(err.response?.data?.message || 'Failed to load DNS records');
    } finally {
      setRecordsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchZones();
  }, [fetchZones]);

  useEffect(() => {
    if (selectedZone) {
      fetchRecords(selectedZone.id);
    }
  }, [selectedZone, fetchRecords]);

  // Auto-dismiss toast
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 4000);
    return () => clearTimeout(t);
  }, [toast]);

  // ---------------------------------------------------------------------------
  // Computed / filtered data
  // ---------------------------------------------------------------------------

  const filteredZones = zones.filter((z) => {
    if (zoneStatusFilter !== 'all' && z.status !== zoneStatusFilter) return false;
    if (zoneSearch) {
      const q = zoneSearch.toLowerCase();
      return (
        z.name.toLowerCase().includes(q) ||
        z.type?.toLowerCase().includes(q) ||
        z.provider?.toLowerCase().includes(q)
      );
    }
    return true;
  });

  const filteredRecords = records.filter((r) => {
    if (recordTypeFilter !== 'all' && r.type !== recordTypeFilter) return false;
    if (recordSearch) {
      const q = recordSearch.toLowerCase();
      return (
        r.name.toLowerCase().includes(q) ||
        r.type.toLowerCase().includes(q) ||
        r.content.toLowerCase().includes(q)
      );
    }
    return true;
  });

  const zoneStats = {
    total: zones.length,
    active: zones.filter((z) => z.status === 'active').length,
    inactive: zones.filter((z) => z.status !== 'active').length,
    totalRecords: zones.reduce((sum, z) => sum + (z.records_count || 0), 0),
  };

  // ---------------------------------------------------------------------------
  // Zone form helpers
  // ---------------------------------------------------------------------------

  const openCreateZoneForm = () => {
    setEditingZone(null);
    setZoneFormData(EMPTY_ZONE_FORM);
    setZoneFormError(null);
    setShowZoneForm(true);
  };

  const openEditZoneForm = (zone: DNSZone) => {
    setEditingZone(zone);
    setZoneFormData({
      name: zone.name,
      provider: zone.provider || 'local',
    });
    setZoneFormError(null);
    setShowZoneForm(true);
  };

  const closeZoneForm = () => {
    setShowZoneForm(false);
    setEditingZone(null);
    setZoneFormError(null);
  };

  const handleZoneFormChange = (field: keyof ZoneFormData, value: string) => {
    setZoneFormData((prev) => ({ ...prev, [field]: value }));
  };

  const handleZoneSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setZoneFormError(null);

    if (!zoneFormData.name) {
      setZoneFormError('Zone name is required');
      return;
    }

    try {
      setZoneSubmitting(true);
      if (editingZone) {
        await dnsApi.updateZone(editingZone.id, {
          provider: zoneFormData.provider,
          status: editingZone.status,
        });
        setToast({ type: 'success', message: 'DNS zone updated successfully' });
      } else {
        await dnsApi.createZone(zoneFormData);
        setToast({ type: 'success', message: 'DNS zone created successfully' });
      }
      closeZoneForm();
      fetchZones();
    } catch (err: any) {
      const msg = err.response?.data?.message || 'An error occurred while saving the zone';
      setZoneFormError(msg);
    } finally {
      setZoneSubmitting(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Record form helpers
  // ---------------------------------------------------------------------------

  const openCreateRecordForm = () => {
    setEditingRecord(null);
    setRecordFormData(EMPTY_RECORD_FORM);
    setRecordFormError(null);
    setShowRecordForm(true);
  };

  const openEditRecordForm = (record: DNSRecord) => {
    setEditingRecord(record);
    setRecordFormData({
      type: record.type,
      name: record.name,
      value: record.content,
      ttl: record.ttl,
      priority: record.priority != null ? String(record.priority) : '',
    });
    setRecordFormError(null);
    setShowRecordForm(true);
  };

  const closeRecordForm = () => {
    setShowRecordForm(false);
    setEditingRecord(null);
    setRecordFormError(null);
  };

  const handleRecordFormChange = (field: keyof RecordFormData, value: string | number) => {
    setRecordFormData((prev) => ({ ...prev, [field]: value }));
  };

  const handleRecordSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setRecordFormError(null);

    if (!recordFormData.name) {
      setRecordFormError('Record name is required');
      return;
    }
    if (!recordFormData.value) {
      setRecordFormError('Record value is required');
      return;
    }

    const payload: any = {
      type: recordFormData.type,
      name: recordFormData.name,
      value: recordFormData.value,
      ttl: recordFormData.ttl || 3600,
    };

    if (recordFormData.priority && ['MX', 'SRV'].includes(recordFormData.type)) {
      payload.priority = parseInt(recordFormData.priority, 10);
    }

    try {
      setRecordSubmitting(true);
      if (editingRecord) {
        await dnsApi.updateRecord(editingRecord.id, payload);
        setToast({ type: 'success', message: 'DNS record updated successfully' });
      } else {
        if (!selectedZone) return;
        await dnsApi.createRecord(selectedZone.id, payload);
        setToast({ type: 'success', message: 'DNS record created successfully' });
      }
      closeRecordForm();
      if (selectedZone) fetchRecords(selectedZone.id);
    } catch (err: any) {
      const msg = err.response?.data?.message || 'An error occurred while saving the record';
      setRecordFormError(msg);
    } finally {
      setRecordSubmitting(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Delete helpers
  // ---------------------------------------------------------------------------

  const confirmDeleteZone = (zone: DNSZone) => {
    setDeletingZone(zone);
  };

  const handleDeleteZone = async () => {
    if (!deletingZone) return;
    try {
      setDeleting(true);
      await dnsApi.deleteZone(deletingZone.id);
      setToast({ type: 'success', message: 'DNS zone deleted successfully' });
      if (selectedZone?.id === deletingZone.id) {
        setSelectedZone(null);
        setRecords([]);
        setActiveTab('zones');
      }
      setDeletingZone(null);
      fetchZones();
    } catch (err: any) {
      setToast({
        type: 'error',
        message: err.response?.data?.message || 'Failed to delete DNS zone',
      });
    } finally {
      setDeleting(false);
    }
  };

  const confirmDeleteRecord = (record: DNSRecord) => {
    setDeletingRecord(record);
  };

  const handleDeleteRecord = async () => {
    if (!deletingRecord) return;
    try {
      setDeleting(true);
      await dnsApi.deleteRecord(deletingRecord.id);
      setToast({ type: 'success', message: 'DNS record deleted successfully' });
      setDeletingRecord(null);
      if (selectedZone) fetchRecords(selectedZone.id);
    } catch (err: any) {
      setToast({
        type: 'error',
        message: err.response?.data?.message || 'Failed to delete DNS record',
      });
    } finally {
      setDeleting(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Navigation helpers
  // ---------------------------------------------------------------------------

  const viewZoneRecords = (zone: DNSZone) => {
    setSelectedZone(zone);
    setActiveTab('records');
    setRecordSearch('');
    setRecordTypeFilter('all');
  };

  const backToZones = () => {
    setSelectedZone(null);
    setActiveTab('zones');
    setRecords([]);
  };

  // ---------------------------------------------------------------------------
  // Render helpers
  // ---------------------------------------------------------------------------

  const getStatusBadge = (status: string) => {
    if (status === 'active') {
      return (
        <Badge variant="success" className="flex items-center gap-1 w-fit">
          <CheckCircle size={12} />
          Active
        </Badge>
      );
    }
    return (
      <Badge variant="secondary" className="flex items-center gap-1 w-fit">
        <XCircle size={12} />
        Inactive
      </Badge>
    );
  };

  const getRecordTypeBadge = (type: string) => {
    const colors: Record<string, string> = {
      A: 'bg-blue-900/50 text-blue-300 border-blue-700',
      AAAA: 'bg-indigo-900/50 text-indigo-300 border-indigo-700',
      CNAME: 'bg-purple-900/50 text-purple-300 border-purple-700',
      MX: 'bg-orange-900/50 text-orange-300 border-orange-700',
      TXT: 'bg-green-900/50 text-green-300 border-green-700',
      NS: 'bg-cyan-900/50 text-cyan-300 border-cyan-700',
      SRV: 'bg-pink-900/50 text-pink-300 border-pink-700',
      CAA: 'bg-yellow-900/50 text-yellow-300 border-yellow-700',
      PTR: 'bg-teal-900/50 text-teal-300 border-teal-700',
    };
    return (
      <span
        className={`inline-flex items-center px-2.5 py-1 rounded-md text-xs font-mono font-semibold border ${
          colors[type] || 'bg-dark-700 text-dark-200 border-dark-600'
        }`}
      >
        {type}
      </span>
    );
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return 'N/A';
    return new Date(dateStr).toLocaleString();
  };

  // ---------------------------------------------------------------------------
  // Loading state
  // ---------------------------------------------------------------------------

  if (zonesLoading && activeTab === 'zones') {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="h-8 w-8 animate-spin text-primary-500" />
      </div>
    );
  }

  // ---------------------------------------------------------------------------
  // Main render
  // ---------------------------------------------------------------------------

  return (
    <div className="space-y-6">
      {/* Toast */}
      {toast && (
        <div
          className={`fixed top-4 right-4 z-50 flex items-center gap-3 px-4 py-3 rounded-lg shadow-lg border ${
            toast.type === 'success'
              ? 'bg-green-900/90 border-green-700 text-green-100'
              : 'bg-red-900/90 border-red-700 text-red-100'
          }`}
        >
          {toast.type === 'success' ? <CheckCircle size={18} /> : <XCircle size={18} />}
          <span className="text-sm font-medium">{toast.message}</span>
          <button onClick={() => setToast(null)} className="ml-2 hover:opacity-70">
            <X size={14} />
          </button>
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50 flex items-center gap-2">
            <Globe size={28} className="text-blue-500" />
            DNS Management
          </h1>
          <p className="text-dark-400 mt-1">
            Manage DNS zones and records for your domains
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            onClick={() => {
              fetchZones();
              if (selectedZone) fetchRecords(selectedZone.id);
            }}
            className="border-dark-600 text-dark-300 hover:bg-dark-700"
          >
            <RefreshCw size={16} className="mr-2" />
            Refresh
          </Button>
          {activeTab === 'zones' ? (
            <Button
              onClick={openCreateZoneForm}
              className="bg-primary-600 hover:bg-primary-700 text-white"
            >
              <Plus size={16} className="mr-2" />
              Add Zone
            </Button>
          ) : (
            <Button
              onClick={openCreateRecordForm}
              className="bg-primary-600 hover:bg-primary-700 text-white"
            >
              <Plus size={16} className="mr-2" />
              Add Record
            </Button>
          )}
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card className="bg-dark-800 border-dark-700">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-dark-300">Total Zones</CardTitle>
            <Layers className="h-4 w-4 text-dark-400" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-dark-50">{zoneStats.total}</div>
            <p className="text-xs text-dark-500 mt-1">All DNS zones</p>
          </CardContent>
        </Card>

        <Card className="bg-dark-800 border-dark-700">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-dark-300">Active Zones</CardTitle>
            <CheckCircle className="h-4 w-4 text-green-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-400">{zoneStats.active}</div>
            <p className="text-xs text-dark-500 mt-1">Currently active</p>
          </CardContent>
        </Card>

        <Card className="bg-dark-800 border-dark-700">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-dark-300">Inactive Zones</CardTitle>
            <XCircle className="h-4 w-4 text-red-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-red-400">{zoneStats.inactive}</div>
            <p className="text-xs text-dark-500 mt-1">Currently inactive</p>
          </CardContent>
        </Card>

        <Card className="bg-dark-800 border-dark-700">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-dark-300">Total Records</CardTitle>
            <FileText className="h-4 w-4 text-blue-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-blue-400">{zoneStats.totalRecords}</div>
            <p className="text-xs text-dark-500 mt-1">Across all zones</p>
          </CardContent>
        </Card>
      </div>

      {/* Tab Navigation */}
      <div className="flex items-center gap-1 p-1 bg-dark-800 rounded-lg border border-dark-700 w-fit">
        <button
          onClick={() => setActiveTab('zones')}
          className={`flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium transition-colors ${
            activeTab === 'zones'
              ? 'bg-dark-600 text-dark-50 shadow-sm'
              : 'text-dark-400 hover:text-dark-200 hover:bg-dark-700'
          }`}
        >
          <Layers size={16} />
          DNS Zones
        </button>
        <button
          onClick={() => {
            if (selectedZone) {
              setActiveTab('records');
            }
          }}
          className={`flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium transition-colors ${
            activeTab === 'records'
              ? 'bg-dark-600 text-dark-50 shadow-sm'
              : 'text-dark-400 hover:text-dark-200 hover:bg-dark-700'
          } ${!selectedZone ? 'opacity-50 cursor-not-allowed' : ''}`}
        >
          <FileText size={16} />
          DNS Records
          {selectedZone && (
            <span className="text-xs text-dark-500 ml-1">({selectedZone.name})</span>
          )}
        </button>
      </div>

      {/* ================================================================== */}
      {/* ZONES TAB                                                          */}
      {/* ================================================================== */}
      {activeTab === 'zones' && (
        <>
          {/* Error banner */}
          {zonesError && (
            <div className="flex items-center gap-3 px-4 py-3 rounded-lg bg-red-900/30 border border-red-700 text-red-300">
              <AlertTriangle size={18} />
              <span className="text-sm">{zonesError}</span>
              <button onClick={() => setZonesError(null)} className="ml-auto hover:opacity-70">
                <X size={14} />
              </button>
            </div>
          )}

          {/* Zone Filters */}
          <Card className="bg-dark-800 border-dark-700">
            <CardContent className="pt-6">
              <div className="flex flex-col md:flex-row gap-4">
                <div className="flex-1 relative">
                  <Filter className="absolute left-3 top-1/2 -translate-y-1/2 text-dark-400" size={16} />
                  <Input
                    placeholder="Search by zone name, type, provider..."
                    value={zoneSearch}
                    onChange={(e) => setZoneSearch(e.target.value)}
                    className="pl-10 bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
                  />
                </div>
                <Select value={zoneStatusFilter} onValueChange={setZoneStatusFilter}>
                  <SelectTrigger className="w-[160px] bg-dark-900 border-dark-600 text-dark-200">
                    <SelectValue placeholder="Status" />
                  </SelectTrigger>
                  <SelectContent className="bg-dark-800 border-dark-600">
                    <SelectItem value="all">All Statuses</SelectItem>
                    <SelectItem value="active">Active</SelectItem>
                    <SelectItem value="inactive">Inactive</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </CardContent>
          </Card>

          {/* Zones Table */}
          <Card className="bg-dark-800 border-dark-700">
            <CardHeader>
              <CardTitle className="text-dark-100 flex items-center justify-between">
                <span>DNS Zones</span>
                <span className="text-sm font-normal text-dark-400">
                  {filteredZones.length} of {zones.length} zones
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent>
              {filteredZones.length === 0 ? (
                <div className="text-center py-16">
                  <Globe className="mx-auto text-dark-600" size={64} />
                  <h3 className="mt-4 text-xl font-medium text-dark-300">
                    {zones.length === 0 ? 'No DNS zones' : 'No matching zones'}
                  </h3>
                  <p className="mt-2 text-dark-500">
                    {zones.length === 0
                      ? 'Add your first DNS zone to get started'
                      : 'Try adjusting your filters or search query'}
                  </p>
                  {zones.length === 0 && (
                    <Button
                      onClick={openCreateZoneForm}
                      className="mt-4 bg-primary-600 hover:bg-primary-700 text-white"
                    >
                      <Plus size={16} className="mr-2" />
                      Add Zone
                    </Button>
                  )}
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-dark-700">
                        <th className="text-left p-3 text-sm font-medium text-dark-400">Name</th>
                        <th className="text-left p-3 text-sm font-medium text-dark-400">Type</th>
                        <th className="text-left p-3 text-sm font-medium text-dark-400">Provider</th>
                        <th className="text-left p-3 text-sm font-medium text-dark-400">Status</th>
                        <th className="text-left p-3 text-sm font-medium text-dark-400">Records</th>
                        <th className="text-left p-3 text-sm font-medium text-dark-400">Created At</th>
                        <th className="text-right p-3 text-sm font-medium text-dark-400">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredZones.map((zone) => (
                        <tr
                          key={zone.id}
                          className="border-b border-dark-700/50 hover:bg-dark-700/30 transition-colors"
                        >
                          <td className="p-3">
                            <button
                              onClick={() => viewZoneRecords(zone)}
                              className="flex items-center gap-2 text-sm font-medium text-blue-400 hover:text-blue-300 transition-colors"
                            >
                              <Globe size={14} />
                              {zone.name}
                            </button>
                          </td>
                          <td className="p-3">
                            <span className="inline-flex items-center px-2.5 py-1 rounded-md bg-dark-700 text-dark-100 text-xs font-mono uppercase">
                              {zone.type || 'master'}
                            </span>
                          </td>
                          <td className="p-3 text-sm text-dark-300">
                            {zone.provider || 'local'}
                          </td>
                          <td className="p-3">{getStatusBadge(zone.status)}</td>
                          <td className="p-3">
                            <button
                              onClick={() => viewZoneRecords(zone)}
                              className="flex items-center gap-1.5 text-sm text-dark-200 hover:text-blue-400 transition-colors"
                            >
                              <Hash size={14} className="text-dark-400" />
                              {zone.records_count ?? 0}
                            </button>
                          </td>
                          <td className="p-3 text-sm text-dark-400">
                            {formatDate(zone.created_at)}
                          </td>
                          <td className="p-3">
                            <div className="flex items-center justify-end gap-2">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => viewZoneRecords(zone)}
                                className="text-dark-300 hover:text-dark-100 hover:bg-dark-700"
                                title="View Records"
                              >
                                <FileText size={14} />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => openEditZoneForm(zone)}
                                className="text-dark-300 hover:text-dark-100 hover:bg-dark-700"
                                title="Edit Zone"
                              >
                                <Edit size={14} />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => confirmDeleteZone(zone)}
                                className="text-red-400 hover:text-red-300 hover:bg-red-900/20"
                                title="Delete Zone"
                              >
                                <Trash2 size={14} />
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
        </>
      )}

      {/* ================================================================== */}
      {/* RECORDS TAB                                                        */}
      {/* ================================================================== */}
      {activeTab === 'records' && selectedZone && (
        <>
          {/* Zone info bar */}
          <Card className="bg-dark-800 border-dark-700">
            <CardContent className="pt-6">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={backToZones}
                    className="text-dark-300 hover:text-dark-100 hover:bg-dark-700"
                  >
                    <ArrowLeft size={16} className="mr-1" />
                    Back to Zones
                  </Button>
                  <div className="h-6 w-px bg-dark-600" />
                  <div className="flex items-center gap-2">
                    <Globe size={18} className="text-blue-400" />
                    <span className="text-dark-100 font-medium">{selectedZone.name}</span>
                    {getStatusBadge(selectedZone.status)}
                  </div>
                </div>
                <div className="flex items-center gap-2 text-sm text-dark-400">
                  <Clock size={14} />
                  Updated: {formatDate(selectedZone.updated_at)}
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Error banner */}
          {recordsError && (
            <div className="flex items-center gap-3 px-4 py-3 rounded-lg bg-red-900/30 border border-red-700 text-red-300">
              <AlertTriangle size={18} />
              <span className="text-sm">{recordsError}</span>
              <button onClick={() => setRecordsError(null)} className="ml-auto hover:opacity-70">
                <X size={14} />
              </button>
            </div>
          )}

          {/* Record Filters */}
          <Card className="bg-dark-800 border-dark-700">
            <CardContent className="pt-6">
              <div className="flex flex-col md:flex-row gap-4">
                <div className="flex-1 relative">
                  <Filter className="absolute left-3 top-1/2 -translate-y-1/2 text-dark-400" size={16} />
                  <Input
                    placeholder="Search by name, type, content..."
                    value={recordSearch}
                    onChange={(e) => setRecordSearch(e.target.value)}
                    className="pl-10 bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
                  />
                </div>
                <Select value={recordTypeFilter} onValueChange={setRecordTypeFilter}>
                  <SelectTrigger className="w-[160px] bg-dark-900 border-dark-600 text-dark-200">
                    <SelectValue placeholder="Type" />
                  </SelectTrigger>
                  <SelectContent className="bg-dark-800 border-dark-600">
                    <SelectItem value="all">All Types</SelectItem>
                    {RECORD_TYPES.map((t) => (
                      <SelectItem key={t} value={t}>
                        {t}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </CardContent>
          </Card>

          {/* Records Table */}
          <Card className="bg-dark-800 border-dark-700">
            <CardHeader>
              <CardTitle className="text-dark-100 flex items-center justify-between">
                <span>DNS Records</span>
                <span className="text-sm font-normal text-dark-400">
                  {filteredRecords.length} of {records.length} records
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent>
              {recordsLoading ? (
                <div className="flex items-center justify-center py-16">
                  <RefreshCw className="h-8 w-8 animate-spin text-primary-500" />
                </div>
              ) : filteredRecords.length === 0 ? (
                <div className="text-center py-16">
                  <FileText className="mx-auto text-dark-600" size={64} />
                  <h3 className="mt-4 text-xl font-medium text-dark-300">
                    {records.length === 0 ? 'No DNS records' : 'No matching records'}
                  </h3>
                  <p className="mt-2 text-dark-500">
                    {records.length === 0
                      ? 'Add your first DNS record to get started'
                      : 'Try adjusting your filters or search query'}
                  </p>
                  {records.length === 0 && (
                    <Button
                      onClick={openCreateRecordForm}
                      className="mt-4 bg-primary-600 hover:bg-primary-700 text-white"
                    >
                      <Plus size={16} className="mr-2" />
                      Add Record
                    </Button>
                  )}
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-dark-700">
                        <th className="text-left p-3 text-sm font-medium text-dark-400">Name</th>
                        <th className="text-left p-3 text-sm font-medium text-dark-400">Type</th>
                        <th className="text-left p-3 text-sm font-medium text-dark-400">Content</th>
                        <th className="text-left p-3 text-sm font-medium text-dark-400">TTL</th>
                        <th className="text-left p-3 text-sm font-medium text-dark-400">Priority</th>
                        <th className="text-left p-3 text-sm font-medium text-dark-400">Status</th>
                        <th className="text-right p-3 text-sm font-medium text-dark-400">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredRecords.map((record) => (
                        <tr
                          key={record.id}
                          className="border-b border-dark-700/50 hover:bg-dark-700/30 transition-colors"
                        >
                          <td className="p-3 text-sm text-dark-100 font-mono">
                            {record.name}
                          </td>
                          <td className="p-3">{getRecordTypeBadge(record.type)}</td>
                          <td className="p-3">
                            <span className="text-sm text-dark-200 font-mono break-all max-w-xs inline-block">
                              {record.content}
                            </span>
                          </td>
                          <td className="p-3 text-sm text-dark-300 font-mono">
                            {record.ttl}
                          </td>
                          <td className="p-3 text-sm text-dark-300">
                            {record.priority != null ? record.priority : '-'}
                          </td>
                          <td className="p-3">{getStatusBadge(record.status)}</td>
                          <td className="p-3">
                            <div className="flex items-center justify-end gap-2">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => openEditRecordForm(record)}
                                className="text-dark-300 hover:text-dark-100 hover:bg-dark-700"
                                title="Edit Record"
                              >
                                <Edit size={14} />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => confirmDeleteRecord(record)}
                                className="text-red-400 hover:text-red-300 hover:bg-red-900/20"
                                title="Delete Record"
                              >
                                <Trash2 size={14} />
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
        </>
      )}

      {/* ================================================================== */}
      {/* Zone Create / Edit Modal                                           */}
      {/* ================================================================== */}
      {showZoneForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          {/* Backdrop */}
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={closeZoneForm}
          />

          {/* Modal */}
          <div className="relative w-full max-w-lg mx-4 bg-dark-800 border border-dark-600 rounded-xl shadow-2xl">
            {/* Header */}
            <div className="flex items-center justify-between px-6 py-4 border-b border-dark-700">
              <h2 className="text-lg font-semibold text-dark-50 flex items-center gap-2">
                <Globe size={20} className="text-primary-400" />
                {editingZone ? 'Edit DNS Zone' : 'Add DNS Zone'}
              </h2>
              <button
                onClick={closeZoneForm}
                className="text-dark-400 hover:text-dark-200 transition-colors"
              >
                <X size={20} />
              </button>
            </div>

            {/* Form */}
            <form onSubmit={handleZoneSubmit} className="px-6 py-4 space-y-4">
              {zoneFormError && (
                <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-red-900/30 border border-red-700 text-red-300 text-sm">
                  <AlertTriangle size={14} />
                  {zoneFormError}
                </div>
              )}

              {/* Zone Name */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1.5">
                  Zone Name <span className="text-red-400">*</span>
                </label>
                <Input
                  placeholder="e.g. example.com"
                  value={zoneFormData.name}
                  onChange={(e) => handleZoneFormChange('name', e.target.value)}
                  className="bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
                  disabled={!!editingZone}
                  required
                />
                {editingZone && (
                  <p className="text-xs text-dark-500 mt-1">Zone name cannot be changed after creation</p>
                )}
              </div>

              {/* Provider */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1.5">
                  Provider
                </label>
                <Select
                  value={zoneFormData.provider}
                  onValueChange={(v) => handleZoneFormChange('provider', v)}
                >
                  <SelectTrigger className="bg-dark-900 border-dark-600 text-dark-200">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className="bg-dark-800 border-dark-600">
                    <SelectItem value="local">Local</SelectItem>
                    <SelectItem value="cloudflare">Cloudflare</SelectItem>
                    <SelectItem value="route53">AWS Route 53</SelectItem>
                    <SelectItem value="digitalocean">DigitalOcean</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {/* Actions */}
              <div className="flex items-center justify-end gap-3 pt-4 border-t border-dark-700">
                <Button
                  type="button"
                  variant="outline"
                  onClick={closeZoneForm}
                  className="border-dark-600 text-dark-300 hover:bg-dark-700"
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={zoneSubmitting}
                  className="bg-primary-600 hover:bg-primary-700 text-white disabled:opacity-50"
                >
                  {zoneSubmitting ? (
                    <>
                      <RefreshCw size={14} className="mr-2 animate-spin" />
                      Saving...
                    </>
                  ) : editingZone ? (
                    'Update Zone'
                  ) : (
                    'Create Zone'
                  )}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* Record Create / Edit Modal                                         */}
      {/* ================================================================== */}
      {showRecordForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          {/* Backdrop */}
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={closeRecordForm}
          />

          {/* Modal */}
          <div className="relative w-full max-w-lg mx-4 bg-dark-800 border border-dark-600 rounded-xl shadow-2xl">
            {/* Header */}
            <div className="flex items-center justify-between px-6 py-4 border-b border-dark-700">
              <h2 className="text-lg font-semibold text-dark-50 flex items-center gap-2">
                <FileText size={20} className="text-primary-400" />
                {editingRecord ? 'Edit DNS Record' : 'Add DNS Record'}
              </h2>
              <button
                onClick={closeRecordForm}
                className="text-dark-400 hover:text-dark-200 transition-colors"
              >
                <X size={20} />
              </button>
            </div>

            {/* Form */}
            <form onSubmit={handleRecordSubmit} className="px-6 py-4 space-y-4">
              {recordFormError && (
                <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-red-900/30 border border-red-700 text-red-300 text-sm">
                  <AlertTriangle size={14} />
                  {recordFormError}
                </div>
              )}

              {/* Type & Name */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1.5">
                    Type <span className="text-red-400">*</span>
                  </label>
                  <Select
                    value={recordFormData.type}
                    onValueChange={(v) => handleRecordFormChange('type', v)}
                  >
                    <SelectTrigger className="bg-dark-900 border-dark-600 text-dark-200">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="bg-dark-800 border-dark-600">
                      {RECORD_TYPES.map((t) => (
                        <SelectItem key={t} value={t}>
                          {t}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1.5">
                    Name <span className="text-red-400">*</span>
                  </label>
                  <Input
                    placeholder="e.g. www, @, mail"
                    value={recordFormData.name}
                    onChange={(e) => handleRecordFormChange('name', e.target.value)}
                    className="bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
                    required
                  />
                </div>
              </div>

              {/* Content */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1.5">
                  Content <span className="text-red-400">*</span>
                </label>
                <Input
                  placeholder={
                    recordFormData.type === 'A'
                      ? 'e.g. 192.168.1.1'
                      : recordFormData.type === 'CNAME'
                      ? 'e.g. example.com'
                      : recordFormData.type === 'MX'
                      ? 'e.g. mail.example.com'
                      : 'Record value'
                  }
                  value={recordFormData.value}
                  onChange={(e) => handleRecordFormChange('value', e.target.value)}
                  className="bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
                  required
                />
              </div>

              {/* TTL & Priority */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1.5">
                    TTL (seconds)
                  </label>
                  <Input
                    type="number"
                    placeholder="3600"
                    value={recordFormData.ttl}
                    onChange={(e) => handleRecordFormChange('ttl', parseInt(e.target.value, 10) || 3600)}
                    className="bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
                    min={60}
                    max={86400}
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1.5">
                    Priority
                    {['MX', 'SRV'].includes(recordFormData.type) && (
                      <span className="text-red-400 ml-1">*</span>
                    )}
                  </label>
                  <Input
                    type="number"
                    placeholder={
                      recordFormData.type === 'MX'
                        ? 'e.g. 10'
                        : recordFormData.type === 'SRV'
                        ? 'e.g. 0'
                        : 'Optional'
                    }
                    value={recordFormData.priority}
                    onChange={(e) => handleRecordFormChange('priority', e.target.value)}
                    className="bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
                    min={0}
                    max={65535}
                  />
                </div>
              </div>

              {/* Actions */}
              <div className="flex items-center justify-end gap-3 pt-4 border-t border-dark-700">
                <Button
                  type="button"
                  variant="outline"
                  onClick={closeRecordForm}
                  className="border-dark-600 text-dark-300 hover:bg-dark-700"
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={recordSubmitting}
                  className="bg-primary-600 hover:bg-primary-700 text-white disabled:opacity-50"
                >
                  {recordSubmitting ? (
                    <>
                      <RefreshCw size={14} className="mr-2 animate-spin" />
                      Saving...
                    </>
                  ) : editingRecord ? (
                    'Update Record'
                  ) : (
                    'Create Record'
                  )}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* Delete Zone Confirmation Modal                                     */}
      {/* ================================================================== */}
      {deletingZone && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          {/* Backdrop */}
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => !deleting && setDeletingZone(null)}
          />

          {/* Modal */}
          <div className="relative w-full max-w-md mx-4 bg-dark-800 border border-dark-600 rounded-xl shadow-2xl">
            <div className="px-6 py-5">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-red-900/30 rounded-lg">
                  <AlertTriangle size={24} className="text-red-400" />
                </div>
                <div>
                  <h3 className="text-lg font-semibold text-dark-50">Delete Zone</h3>
                  <p className="text-sm text-dark-400">This action cannot be undone</p>
                </div>
              </div>

              <div className="mb-5 p-3 rounded-lg bg-dark-900 border border-dark-700">
                <p className="text-sm text-dark-300">
                  Are you sure you want to delete the zone{' '}
                  <span className="font-mono font-semibold text-dark-100">
                    {deletingZone.name}
                  </span>
                  ? All associated DNS records will also be deleted.
                </p>
              </div>

              <div className="flex items-center justify-end gap-3">
                <Button
                  variant="outline"
                  onClick={() => setDeletingZone(null)}
                  disabled={deleting}
                  className="border-dark-600 text-dark-300 hover:bg-dark-700"
                >
                  Cancel
                </Button>
                <Button
                  variant="destructive"
                  onClick={handleDeleteZone}
                  disabled={deleting}
                  className="bg-red-600 hover:bg-red-700 text-white disabled:opacity-50"
                >
                  {deleting ? (
                    <>
                      <RefreshCw size={14} className="mr-2 animate-spin" />
                      Deleting...
                    </>
                  ) : (
                    <>
                      <Trash2 size={14} className="mr-2" />
                      Delete Zone
                    </>
                  )}
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* Delete Record Confirmation Modal                                   */}
      {/* ================================================================== */}
      {deletingRecord && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          {/* Backdrop */}
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => !deleting && setDeletingRecord(null)}
          />

          {/* Modal */}
          <div className="relative w-full max-w-md mx-4 bg-dark-800 border border-dark-600 rounded-xl shadow-2xl">
            <div className="px-6 py-5">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-red-900/30 rounded-lg">
                  <AlertTriangle size={24} className="text-red-400" />
                </div>
                <div>
                  <h3 className="text-lg font-semibold text-dark-50">Delete Record</h3>
                  <p className="text-sm text-dark-400">This action cannot be undone</p>
                </div>
              </div>

              <div className="mb-5 p-3 rounded-lg bg-dark-900 border border-dark-700">
                <p className="text-sm text-dark-300">
                  Are you sure you want to delete the{' '}
                  <span className="font-mono uppercase font-semibold text-dark-100">
                    {deletingRecord.type}
                  </span>{' '}
                  record{' '}
                  <span className="font-mono font-semibold text-dark-100">
                    {deletingRecord.name}
                  </span>
                  ?
                </p>
              </div>

              <div className="flex items-center justify-end gap-3">
                <Button
                  variant="outline"
                  onClick={() => setDeletingRecord(null)}
                  disabled={deleting}
                  className="border-dark-600 text-dark-300 hover:bg-dark-700"
                >
                  Cancel
                </Button>
                <Button
                  variant="destructive"
                  onClick={handleDeleteRecord}
                  disabled={deleting}
                  className="bg-red-600 hover:bg-red-700 text-white disabled:opacity-50"
                >
                  {deleting ? (
                    <>
                      <RefreshCw size={14} className="mr-2 animate-spin" />
                      Deleting...
                    </>
                  ) : (
                    <>
                      <Trash2 size={14} className="mr-2" />
                      Delete Record
                    </>
                  )}
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
