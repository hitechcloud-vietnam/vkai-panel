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

const INPUT_CLASS =
  'border-gray-300 bg-white text-gray-900 placeholder:text-gray-400 focus-visible:ring-1 focus-visible:ring-blue-500 focus-visible:ring-offset-0';
const SELECT_TRIGGER_CLASS =
  'border-gray-300 bg-white text-gray-900 focus:ring-1 focus:ring-blue-500 focus:ring-offset-0';
const PRIMARY_BUTTON_CLASS =
  'bg-blue-600 text-white hover:bg-blue-700 focus-visible:ring-blue-500';
const SECONDARY_BUTTON_CLASS =
  'border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus-visible:ring-blue-500';
const ICON_BUTTON_CLASS =
  'h-8 w-8 p-0 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:ring-blue-500';
const DANGER_ICON_BUTTON_CLASS =
  'h-8 w-8 p-0 text-red-600 hover:bg-red-50 hover:text-red-700 focus-visible:ring-red-500';
const TH_CLASS =
  'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';

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
      const payload = res?.data?.data ?? res?.data;
      setZones(Array.isArray(payload) ? payload : []);
    } catch (err: any) {
      console.error('Failed to load DNS zones:', err);
      setZones([]);
      setZonesError(err?.response?.data?.message || 'Failed to load DNS zones');
    } finally {
      setZonesLoading(false);
    }
  }, []);

  const fetchRecords = useCallback(async (zoneId: string) => {
    try {
      setRecordsLoading(true);
      setRecordsError(null);
      const res = await dnsApi.listRecords(zoneId);
      const payload = res?.data?.data ?? res?.data;
      setRecords(Array.isArray(payload) ? payload : []);
    } catch (err: any) {
      console.error('Failed to load DNS records:', err);
      setRecords([]);
      setRecordsError(err?.response?.data?.message || 'Failed to load DNS records');
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

  const safeZones: DNSZone[] = Array.isArray(zones) ? zones : [];
  const safeRecords: DNSRecord[] = Array.isArray(records) ? records : [];

  const filteredZones = safeZones.filter((z) => {
    if (!z) return false;
    if (zoneStatusFilter !== 'all' && z.status !== zoneStatusFilter) return false;
    if (zoneSearch) {
      const q = zoneSearch.toLowerCase();
      return (
        (z.name || '').toLowerCase().includes(q) ||
        (z.type || '').toLowerCase().includes(q) ||
        (z.provider || '').toLowerCase().includes(q)
      );
    }
    return true;
  });

  const filteredRecords = safeRecords.filter((r) => {
    if (!r) return false;
    if (recordTypeFilter !== 'all' && r.type !== recordTypeFilter) return false;
    if (recordSearch) {
      const q = recordSearch.toLowerCase();
      return (
        (r.name || '').toLowerCase().includes(q) ||
        (r.type || '').toLowerCase().includes(q) ||
        (r.content || '').toLowerCase().includes(q)
      );
    }
    return true;
  });

  const zoneStats = {
    total: safeZones.length,
    active: safeZones.filter((z) => z?.status === 'active').length,
    inactive: safeZones.filter((z) => z?.status !== 'active').length,
    totalRecords: safeZones.reduce((sum, z) => sum + (z?.records_count || 0), 0),
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
      name: zone?.name || '',
      provider: zone?.provider || 'local',
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
      const msg = err?.response?.data?.message || 'An error occurred while saving the zone';
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
      type: record?.type || 'A',
      name: record?.name || '',
      value: record?.content || '',
      ttl: record?.ttl ?? 3600,
      priority: record?.priority != null ? String(record.priority) : '',
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
      const msg = err?.response?.data?.message || 'An error occurred while saving the record';
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
        message: err?.response?.data?.message || 'Failed to delete DNS zone',
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
        message: err?.response?.data?.message || 'Failed to delete DNS record',
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
        <span className="inline-flex w-fit items-center gap-1 rounded-md border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
          <CheckCircle size={12} />
          Active
        </span>
      );
    }
    return (
      <span className="inline-flex w-fit items-center gap-1 rounded-md border border-gray-200 bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600">
        <XCircle size={12} />
        Inactive
      </span>
    );
  };

  const getRecordTypeBadge = (type: string) => {
    const colors: Record<string, string> = {
      A: 'bg-blue-50 text-blue-700 border-blue-200',
      AAAA: 'bg-sky-50 text-sky-700 border-sky-200',
      CNAME: 'bg-emerald-50 text-emerald-700 border-emerald-200',
      MX: 'bg-amber-50 text-amber-700 border-amber-200',
      TXT: 'bg-gray-100 text-gray-700 border-gray-200',
      NS: 'bg-sky-50 text-sky-700 border-sky-200',
      SRV: 'bg-blue-50 text-blue-700 border-blue-200',
      CAA: 'bg-amber-50 text-amber-700 border-amber-200',
      PTR: 'bg-emerald-50 text-emerald-700 border-emerald-200',
    };
    return (
      <span
        className={`inline-flex items-center rounded-md border px-2 py-0.5 font-mono text-xs font-semibold ${
          colors[type] || 'bg-gray-100 text-gray-700 border-gray-200'
        }`}
      >
        {type}
      </span>
    );
  };

  // Deterministic formatting (avoids server/client hydration drift)
  const formatDate = (dateStr?: string | null) => {
    if (!dateStr) return 'N/A';
    const d = new Date(dateStr);
    if (Number.isNaN(d.getTime())) return 'N/A';
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${pad(d.getDate())}/${pad(d.getMonth() + 1)}/${d.getFullYear()} ${pad(
      d.getHours(),
    )}:${pad(d.getMinutes())}`;
  };

  // ---------------------------------------------------------------------------
  // Loading state
  // ---------------------------------------------------------------------------

  if (zonesLoading && activeTab === 'zones') {
    return (
      <div className="flex h-64 items-center justify-center">
        <RefreshCw className="h-6 w-6 animate-spin text-blue-600" aria-hidden="true" />
        <span className="sr-only">Loading DNS zones</span>
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
          role="status"
          className={`fixed right-4 top-4 z-50 flex items-center gap-3 rounded-md border px-4 py-3 shadow-lg ${
            toast.type === 'success'
              ? 'border-emerald-200 bg-emerald-50 text-emerald-700'
              : 'border-red-200 bg-red-50 text-red-700'
          }`}
        >
          {toast.type === 'success' ? <CheckCircle size={16} /> : <XCircle size={16} />}
          <span className="text-sm font-medium">{toast.message}</span>
          <button
            type="button"
            onClick={() => setToast(null)}
            aria-label="Dismiss notification"
            className="ml-2 rounded-md p-0.5 hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
          >
            <X size={14} />
          </button>
        </div>
      )}

      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold text-gray-900">
            <Globe size={20} className="text-blue-600" />
            DNS Management
          </h1>
          <p className="mt-1 text-sm text-gray-600">
            Manage DNS zones and records for your domains
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            onClick={() => {
              fetchZones();
              if (selectedZone) fetchRecords(selectedZone.id);
            }}
            className={SECONDARY_BUTTON_CLASS}
          >
            <RefreshCw size={16} className="mr-2" />
            Refresh
          </Button>
          {activeTab === 'zones' ? (
            <Button onClick={openCreateZoneForm} className={PRIMARY_BUTTON_CLASS}>
              <Plus size={16} className="mr-2" />
              Add Zone
            </Button>
          ) : (
            <Button onClick={openCreateRecordForm} className={PRIMARY_BUTTON_CLASS}>
              <Plus size={16} className="mr-2" />
              Add Record
            </Button>
          )}
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card className="border-gray-200 bg-white shadow-sm">
          <CardContent className="p-5">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                Total Zones
              </span>
              <Layers className="h-4 w-4 text-gray-400" />
            </div>
            <div className="mt-2 text-2xl font-semibold text-gray-900">{zoneStats.total}</div>
            <p className="mt-1 text-xs text-gray-500">All DNS zones</p>
          </CardContent>
        </Card>

        <Card className="border-gray-200 bg-white shadow-sm">
          <CardContent className="p-5">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                Active Zones
              </span>
              <CheckCircle className="h-4 w-4 text-emerald-600" />
            </div>
            <div className="mt-2 text-2xl font-semibold text-gray-900">{zoneStats.active}</div>
            <p className="mt-1 text-xs text-gray-500">Currently active</p>
          </CardContent>
        </Card>

        <Card className="border-gray-200 bg-white shadow-sm">
          <CardContent className="p-5">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                Inactive Zones
              </span>
              <XCircle className="h-4 w-4 text-gray-400" />
            </div>
            <div className="mt-2 text-2xl font-semibold text-gray-900">{zoneStats.inactive}</div>
            <p className="mt-1 text-xs text-gray-500">Currently inactive</p>
          </CardContent>
        </Card>

        <Card className="border-gray-200 bg-white shadow-sm">
          <CardContent className="p-5">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold uppercase tracking-wide text-gray-500">
                Total Records
              </span>
              <FileText className="h-4 w-4 text-blue-600" />
            </div>
            <div className="mt-2 text-2xl font-semibold text-gray-900">
              {zoneStats.totalRecords}
            </div>
            <p className="mt-1 text-xs text-gray-500">Across all zones</p>
          </CardContent>
        </Card>
      </div>

      {/* Tab Navigation */}
      <div className="flex w-fit items-center gap-1 rounded-lg border border-gray-200 bg-white p-1">
        <button
          type="button"
          onClick={() => setActiveTab('zones')}
          aria-current={activeTab === 'zones' ? 'page' : undefined}
          className={`flex items-center gap-2 rounded-md px-3.5 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
            activeTab === 'zones'
              ? 'bg-blue-50 text-blue-700'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
          }`}
        >
          <Layers size={16} />
          DNS Zones
        </button>
        <button
          type="button"
          onClick={() => {
            if (selectedZone) {
              setActiveTab('records');
            }
          }}
          disabled={!selectedZone}
          aria-current={activeTab === 'records' ? 'page' : undefined}
          className={`flex items-center gap-2 rounded-md px-3.5 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
            activeTab === 'records'
              ? 'bg-blue-50 text-blue-700'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
          } ${!selectedZone ? 'cursor-not-allowed opacity-50' : ''}`}
        >
          <FileText size={16} />
          DNS Records
          {selectedZone && (
            <span className="ml-1 text-xs text-gray-500">({selectedZone.name})</span>
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
            <div className="flex items-center gap-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-red-700">
              <AlertTriangle size={16} />
              <span className="text-sm">{zonesError}</span>
              <button
                type="button"
                onClick={() => setZonesError(null)}
                aria-label="Dismiss error"
                className="ml-auto rounded-md p-0.5 hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
              >
                <X size={14} />
              </button>
            </div>
          )}

          {/* Zone Filters */}
          <Card className="border-gray-200 bg-white shadow-sm">
            <CardContent className="p-5">
              <div className="flex flex-col gap-4 md:flex-row">
                <div className="relative flex-1">
                  <Filter
                    className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
                    size={16}
                  />
                  <Input
                    aria-label="Search DNS zones"
                    placeholder="Search by zone name, type, provider..."
                    value={zoneSearch}
                    onChange={(e) => setZoneSearch(e.target.value)}
                    className={`pl-10 ${INPUT_CLASS}`}
                  />
                </div>
                <Select value={zoneStatusFilter} onValueChange={setZoneStatusFilter}>
                  <SelectTrigger
                    aria-label="Filter zones by status"
                    className={`w-[160px] ${SELECT_TRIGGER_CLASS}`}
                  >
                    <SelectValue placeholder="Status" />
                  </SelectTrigger>
                  <SelectContent className="border-gray-200 bg-white">
                    <SelectItem value="all">All Statuses</SelectItem>
                    <SelectItem value="active">Active</SelectItem>
                    <SelectItem value="inactive">Inactive</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </CardContent>
          </Card>

          {/* Zones Table */}
          <Card className="border-gray-200 bg-white shadow-sm">
            <CardHeader className="border-b border-gray-200 px-5 py-4">
              <CardTitle className="flex items-center justify-between text-sm font-semibold text-gray-900">
                <span>DNS Zones</span>
                <span className="text-xs font-normal text-gray-500">
                  {filteredZones.length} of {safeZones.length} zones
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              {filteredZones.length === 0 ? (
                <div className="px-5 py-12 text-center">
                  <Globe className="mx-auto text-gray-300" size={40} />
                  <h3 className="mt-3 text-sm font-semibold text-gray-900">
                    {safeZones.length === 0 ? 'No DNS zones' : 'No matching zones'}
                  </h3>
                  <p className="mt-1 text-sm text-gray-600">
                    {safeZones.length === 0
                      ? 'Add your first DNS zone to get started'
                      : 'Try adjusting your filters or search query'}
                  </p>
                  {safeZones.length === 0 && (
                    <Button
                      onClick={openCreateZoneForm}
                      className={`mt-4 ${PRIMARY_BUTTON_CLASS}`}
                    >
                      <Plus size={16} className="mr-2" />
                      Add Zone
                    </Button>
                  )}
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead className="border-b border-gray-200 bg-gray-50">
                      <tr>
                        <th className={TH_CLASS}>Name</th>
                        <th className={TH_CLASS}>Type</th>
                        <th className={TH_CLASS}>Provider</th>
                        <th className={TH_CLASS}>Status</th>
                        <th className={TH_CLASS}>Records</th>
                        <th className={TH_CLASS}>Created At</th>
                        <th className={`${TH_CLASS} text-right`}>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredZones.map((zone) => (
                        <tr
                          key={zone.id}
                          className="border-b border-gray-100 hover:bg-gray-50"
                        >
                          <td className="px-4 py-3">
                            <button
                              type="button"
                              onClick={() => viewZoneRecords(zone)}
                              className="flex items-center gap-2 rounded-md text-sm font-medium text-blue-600 hover:text-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                            >
                              <Globe size={14} />
                              {zone.name}
                            </button>
                          </td>
                          <td className="px-4 py-3">
                            <span className="inline-flex items-center rounded-md border border-gray-200 bg-gray-100 px-2 py-0.5 font-mono text-xs font-medium uppercase text-gray-700">
                              {zone.type || 'master'}
                            </span>
                          </td>
                          <td className="px-4 py-3 text-sm text-gray-700">
                            {zone.provider || 'local'}
                          </td>
                          <td className="px-4 py-3">{getStatusBadge(zone.status)}</td>
                          <td className="px-4 py-3">
                            <button
                              type="button"
                              onClick={() => viewZoneRecords(zone)}
                              className="flex items-center gap-1.5 rounded-md text-sm text-gray-700 hover:text-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                            >
                              <Hash size={14} className="text-gray-400" />
                              {zone.records_count ?? 0}
                            </button>
                          </td>
                          <td className="px-4 py-3 text-sm text-gray-600">
                            {formatDate(zone.created_at)}
                          </td>
                          <td className="px-4 py-3">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => viewZoneRecords(zone)}
                                className={ICON_BUTTON_CLASS}
                                aria-label={`View records of ${zone.name}`}
                                title="View Records"
                              >
                                <FileText size={14} />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => openEditZoneForm(zone)}
                                className={ICON_BUTTON_CLASS}
                                aria-label={`Edit zone ${zone.name}`}
                                title="Edit Zone"
                              >
                                <Edit size={14} />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => confirmDeleteZone(zone)}
                                className={DANGER_ICON_BUTTON_CLASS}
                                aria-label={`Delete zone ${zone.name}`}
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
          <Card className="border-gray-200 bg-white shadow-sm">
            <CardContent className="p-5">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-center gap-3">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={backToZones}
                    className="text-gray-600 hover:bg-gray-100 hover:text-gray-900 focus-visible:ring-blue-500"
                  >
                    <ArrowLeft size={16} className="mr-1" />
                    Back to Zones
                  </Button>
                  <div className="h-6 w-px bg-gray-200" />
                  <div className="flex items-center gap-2">
                    <Globe size={16} className="text-blue-600" />
                    <span className="text-sm font-medium text-gray-900">
                      {selectedZone.name}
                    </span>
                    {getStatusBadge(selectedZone.status)}
                  </div>
                </div>
                <div className="flex items-center gap-2 text-sm text-gray-600">
                  <Clock size={14} />
                  Updated: {formatDate(selectedZone.updated_at)}
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Error banner */}
          {recordsError && (
            <div className="flex items-center gap-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-red-700">
              <AlertTriangle size={16} />
              <span className="text-sm">{recordsError}</span>
              <button
                type="button"
                onClick={() => setRecordsError(null)}
                aria-label="Dismiss error"
                className="ml-auto rounded-md p-0.5 hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
              >
                <X size={14} />
              </button>
            </div>
          )}

          {/* Record Filters */}
          <Card className="border-gray-200 bg-white shadow-sm">
            <CardContent className="p-5">
              <div className="flex flex-col gap-4 md:flex-row">
                <div className="relative flex-1">
                  <Filter
                    className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
                    size={16}
                  />
                  <Input
                    aria-label="Search DNS records"
                    placeholder="Search by name, type, content..."
                    value={recordSearch}
                    onChange={(e) => setRecordSearch(e.target.value)}
                    className={`pl-10 ${INPUT_CLASS}`}
                  />
                </div>
                <Select value={recordTypeFilter} onValueChange={setRecordTypeFilter}>
                  <SelectTrigger
                    aria-label="Filter records by type"
                    className={`w-[160px] ${SELECT_TRIGGER_CLASS}`}
                  >
                    <SelectValue placeholder="Type" />
                  </SelectTrigger>
                  <SelectContent className="border-gray-200 bg-white">
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
          <Card className="border-gray-200 bg-white shadow-sm">
            <CardHeader className="border-b border-gray-200 px-5 py-4">
              <CardTitle className="flex items-center justify-between text-sm font-semibold text-gray-900">
                <span>DNS Records</span>
                <span className="text-xs font-normal text-gray-500">
                  {filteredRecords.length} of {safeRecords.length} records
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              {recordsLoading ? (
                <div className="flex items-center justify-center py-12">
                  <RefreshCw className="h-6 w-6 animate-spin text-blue-600" aria-hidden="true" />
                  <span className="sr-only">Loading DNS records</span>
                </div>
              ) : filteredRecords.length === 0 ? (
                <div className="px-5 py-12 text-center">
                  <FileText className="mx-auto text-gray-300" size={40} />
                  <h3 className="mt-3 text-sm font-semibold text-gray-900">
                    {safeRecords.length === 0 ? 'No DNS records' : 'No matching records'}
                  </h3>
                  <p className="mt-1 text-sm text-gray-600">
                    {safeRecords.length === 0
                      ? 'Add your first DNS record to get started'
                      : 'Try adjusting your filters or search query'}
                  </p>
                  {safeRecords.length === 0 && (
                    <Button
                      onClick={openCreateRecordForm}
                      className={`mt-4 ${PRIMARY_BUTTON_CLASS}`}
                    >
                      <Plus size={16} className="mr-2" />
                      Add Record
                    </Button>
                  )}
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead className="border-b border-gray-200 bg-gray-50">
                      <tr>
                        <th className={TH_CLASS}>Name</th>
                        <th className={TH_CLASS}>Type</th>
                        <th className={TH_CLASS}>Content</th>
                        <th className={TH_CLASS}>TTL</th>
                        <th className={TH_CLASS}>Priority</th>
                        <th className={TH_CLASS}>Status</th>
                        <th className={`${TH_CLASS} text-right`}>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredRecords.map((record) => (
                        <tr
                          key={record.id}
                          className="border-b border-gray-100 hover:bg-gray-50"
                        >
                          <td className="px-4 py-3 font-mono text-sm text-gray-900">
                            {record.name}
                          </td>
                          <td className="px-4 py-3">{getRecordTypeBadge(record.type)}</td>
                          <td className="px-4 py-3">
                            <span className="inline-block max-w-xs break-all font-mono text-sm text-gray-700">
                              {record.content}
                            </span>
                          </td>
                          <td className="px-4 py-3 font-mono text-sm text-gray-700">
                            {record.ttl}
                          </td>
                          <td className="px-4 py-3 text-sm text-gray-700">
                            {record.priority != null ? record.priority : '-'}
                          </td>
                          <td className="px-4 py-3">{getStatusBadge(record.status)}</td>
                          <td className="px-4 py-3">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => openEditRecordForm(record)}
                                className={ICON_BUTTON_CLASS}
                                aria-label={`Edit record ${record.name}`}
                                title="Edit Record"
                              >
                                <Edit size={14} />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => confirmDeleteRecord(record)}
                                className={DANGER_ICON_BUTTON_CLASS}
                                aria-label={`Delete record ${record.name}`}
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
          <div className="absolute inset-0 bg-gray-900/40" onClick={closeZoneForm} />

          {/* Modal */}
          <div
            role="dialog"
            aria-modal="true"
            aria-label={editingZone ? 'Edit DNS Zone' : 'Add DNS Zone'}
            className="relative mx-4 w-full max-w-lg rounded-lg border border-gray-200 bg-white shadow-lg"
          >
            {/* Header */}
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                <Globe size={16} className="text-blue-600" />
                {editingZone ? 'Edit DNS Zone' : 'Add DNS Zone'}
              </h2>
              <button
                type="button"
                onClick={closeZoneForm}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
              >
                <X size={16} />
              </button>
            </div>

            {/* Form */}
            <form onSubmit={handleZoneSubmit} className="space-y-4 px-5 py-4">
              {zoneFormError && (
                <div className="flex items-center gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
                  <AlertTriangle size={14} />
                  {zoneFormError}
                </div>
              )}

              {/* Zone Name */}
              <div>
                <label
                  htmlFor="zone-name"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Zone Name <span className="text-red-600">*</span>
                </label>
                <Input
                  id="zone-name"
                  placeholder="e.g. example.com"
                  value={zoneFormData.name}
                  onChange={(e) => handleZoneFormChange('name', e.target.value)}
                  className={`${INPUT_CLASS} disabled:bg-gray-50`}
                  disabled={!!editingZone}
                  required
                />
                {editingZone && (
                  <p className="mt-1 text-xs text-gray-500">
                    Zone name cannot be changed after creation
                  </p>
                )}
              </div>

              {/* Provider */}
              <div>
                <label
                  htmlFor="zone-provider"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Provider
                </label>
                <Select
                  value={zoneFormData.provider}
                  onValueChange={(v) => handleZoneFormChange('provider', v)}
                >
                  <SelectTrigger
                    id="zone-provider"
                    aria-label="Provider"
                    className={SELECT_TRIGGER_CLASS}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className="border-gray-200 bg-white">
                    <SelectItem value="local">Local</SelectItem>
                    <SelectItem value="cloudflare">Cloudflare</SelectItem>
                    <SelectItem value="route53">AWS Route 53</SelectItem>
                    <SelectItem value="digitalocean">DigitalOcean</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {/* Actions */}
              <div className="flex items-center justify-end gap-3 border-t border-gray-200 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={closeZoneForm}
                  className={SECONDARY_BUTTON_CLASS}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={zoneSubmitting}
                  className={`${PRIMARY_BUTTON_CLASS} disabled:opacity-50`}
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
          <div className="absolute inset-0 bg-gray-900/40" onClick={closeRecordForm} />

          {/* Modal */}
          <div
            role="dialog"
            aria-modal="true"
            aria-label={editingRecord ? 'Edit DNS Record' : 'Add DNS Record'}
            className="relative mx-4 w-full max-w-lg rounded-lg border border-gray-200 bg-white shadow-lg"
          >
            {/* Header */}
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                <FileText size={16} className="text-blue-600" />
                {editingRecord ? 'Edit DNS Record' : 'Add DNS Record'}
              </h2>
              <button
                type="button"
                onClick={closeRecordForm}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
              >
                <X size={16} />
              </button>
            </div>

            {/* Form */}
            <form onSubmit={handleRecordSubmit} className="space-y-4 px-5 py-4">
              {recordFormError && (
                <div className="flex items-center gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
                  <AlertTriangle size={14} />
                  {recordFormError}
                </div>
              )}

              {/* Type & Name */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label
                    htmlFor="record-type"
                    className="mb-1.5 block text-sm font-medium text-gray-700"
                  >
                    Type <span className="text-red-600">*</span>
                  </label>
                  <Select
                    value={recordFormData.type}
                    onValueChange={(v) => handleRecordFormChange('type', v)}
                  >
                    <SelectTrigger
                      id="record-type"
                      aria-label="Record type"
                      className={SELECT_TRIGGER_CLASS}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="border-gray-200 bg-white">
                      {RECORD_TYPES.map((t) => (
                        <SelectItem key={t} value={t}>
                          {t}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <label
                    htmlFor="record-name"
                    className="mb-1.5 block text-sm font-medium text-gray-700"
                  >
                    Name <span className="text-red-600">*</span>
                  </label>
                  <Input
                    id="record-name"
                    placeholder="e.g. www, @, mail"
                    value={recordFormData.name}
                    onChange={(e) => handleRecordFormChange('name', e.target.value)}
                    className={INPUT_CLASS}
                    required
                  />
                </div>
              </div>

              {/* Content */}
              <div>
                <label
                  htmlFor="record-content"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Content <span className="text-red-600">*</span>
                </label>
                <Input
                  id="record-content"
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
                  className={INPUT_CLASS}
                  required
                />
              </div>

              {/* TTL & Priority */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label
                    htmlFor="record-ttl"
                    className="mb-1.5 block text-sm font-medium text-gray-700"
                  >
                    TTL (seconds)
                  </label>
                  <Input
                    id="record-ttl"
                    type="number"
                    placeholder="3600"
                    value={recordFormData.ttl}
                    onChange={(e) =>
                      handleRecordFormChange('ttl', parseInt(e.target.value, 10) || 3600)
                    }
                    className={INPUT_CLASS}
                    min={60}
                    max={86400}
                  />
                </div>
                <div>
                  <label
                    htmlFor="record-priority"
                    className="mb-1.5 block text-sm font-medium text-gray-700"
                  >
                    Priority
                    {['MX', 'SRV'].includes(recordFormData.type) && (
                      <span className="ml-1 text-red-600">*</span>
                    )}
                  </label>
                  <Input
                    id="record-priority"
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
                    className={INPUT_CLASS}
                    min={0}
                    max={65535}
                  />
                </div>
              </div>

              {/* Actions */}
              <div className="flex items-center justify-end gap-3 border-t border-gray-200 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={closeRecordForm}
                  className={SECONDARY_BUTTON_CLASS}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={recordSubmitting}
                  className={`${PRIMARY_BUTTON_CLASS} disabled:opacity-50`}
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
            className="absolute inset-0 bg-gray-900/40"
            onClick={() => !deleting && setDeletingZone(null)}
          />

          {/* Modal */}
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Delete Zone"
            className="relative mx-4 w-full max-w-md rounded-lg border border-gray-200 bg-white shadow-lg"
          >
            <div className="px-5 py-5">
              <div className="mb-4 flex items-center gap-3">
                <div className="rounded-md border border-red-200 bg-red-50 p-2">
                  <AlertTriangle size={20} className="text-red-600" />
                </div>
                <div>
                  <h3 className="text-sm font-semibold text-gray-900">Delete Zone</h3>
                  <p className="text-sm text-gray-600">This action cannot be undone</p>
                </div>
              </div>

              <div className="mb-5 rounded-md border border-gray-200 bg-gray-50 p-3">
                <p className="text-sm text-gray-700">
                  Are you sure you want to delete the zone{' '}
                  <span className="font-mono font-semibold text-gray-900">
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
                  className={SECONDARY_BUTTON_CLASS}
                >
                  Cancel
                </Button>
                <Button
                  variant="destructive"
                  onClick={handleDeleteZone}
                  disabled={deleting}
                  className="bg-red-600 text-white hover:bg-red-700 focus-visible:ring-red-500 disabled:opacity-50"
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
            className="absolute inset-0 bg-gray-900/40"
            onClick={() => !deleting && setDeletingRecord(null)}
          />

          {/* Modal */}
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Delete Record"
            className="relative mx-4 w-full max-w-md rounded-lg border border-gray-200 bg-white shadow-lg"
          >
            <div className="px-5 py-5">
              <div className="mb-4 flex items-center gap-3">
                <div className="rounded-md border border-red-200 bg-red-50 p-2">
                  <AlertTriangle size={20} className="text-red-600" />
                </div>
                <div>
                  <h3 className="text-sm font-semibold text-gray-900">Delete Record</h3>
                  <p className="text-sm text-gray-600">This action cannot be undone</p>
                </div>
              </div>

              <div className="mb-5 rounded-md border border-gray-200 bg-gray-50 p-3">
                <p className="text-sm text-gray-700">
                  Are you sure you want to delete the{' '}
                  <span className="font-mono font-semibold uppercase text-gray-900">
                    {deletingRecord.type}
                  </span>{' '}
                  record{' '}
                  <span className="font-mono font-semibold text-gray-900">
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
                  className={SECONDARY_BUTTON_CLASS}
                >
                  Cancel
                </Button>
                <Button
                  variant="destructive"
                  onClick={handleDeleteRecord}
                  disabled={deleting}
                  className="bg-red-600 text-white hover:bg-red-700 focus-visible:ring-red-500 disabled:opacity-50"
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
