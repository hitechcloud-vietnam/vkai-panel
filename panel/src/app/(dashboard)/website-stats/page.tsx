'use client';

import { useState, useEffect } from 'react';
import {
  BarChart3, Globe, Clock, Users, Eye, Wifi,
  TrendingUp, ArrowDownRight, RefreshCw, Activity
} from 'lucide-react';

interface DailyStats {
  date: string;
  page_views: number;
  unique_visitors: number;
  bandwidth: number;
}

interface TopPage {
  path: string;
  page_views: number;
}

interface TopReferrer {
  referer: string;
  visits: number;
}

interface TopCountry {
  country: string;
  visitors: number;
}

interface RecentVisitor {
  id: string;
  visitor_ip: string;
  path: string;
  method: string;
  status_code: number;
  response_time: number;
  referer: string;
  country: string;
  created_at: string;
}

interface StatsOverview {
  total_page_views: number;
  total_unique_visitors: number;
  total_bandwidth: number;
  avg_response_time: number;
  avg_bounce_rate: number;
  top_pages: TopPage[] | null;
  top_referrers: TopReferrer[] | null;
  top_countries: TopCountry[] | null;
  daily_stats: DailyStats[] | null;
  recent_visitors: RecentVisitor[] | null;
}

const CARD = 'bg-white border border-gray-200 rounded-lg shadow-sm';
const CARD_HEADER = 'px-5 py-4 border-b border-gray-200';
const TH = 'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
const TD = 'px-4 py-3 text-sm text-gray-700';
const ROW = 'border-b border-gray-100 hover:bg-gray-50';
const BADGE = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium';
const BTN_PRIMARY =
  'inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1';

export default function WebsiteStatsPage() {
  const [overview, setOverview] = useState<StatsOverview | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [days, setDays] = useState(30);
  const [activeTab, setActiveTab] = useState<'overview' | 'pages' | 'visitors' | 'referrers'>('overview');

  const fetchStats = async () => {
    setLoading(true);
    setError('');
    try {
      let token = '';
      if (typeof window !== 'undefined') {
        try { token = localStorage.getItem('token') || ''; } catch { token = ''; }
      }
      // Using a sample website_id - in production this would come from website selection
      const websiteId = 'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11';
      const res = await fetch(`/api/v1/website-stats/overview?website_id=${websiteId}&days=${days}`, {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setOverview(data ?? null);
      } else {
        setError('Unable to load website statistics.');
      }
    } catch (error) {
      console.error('Failed to fetch stats:', error);
      setError('Unable to load website statistics. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStats();
  }, [days]);

  const formatBytes = (bytes: number) => {
    if (!bytes || bytes <= 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const formatNumber = (num: number) => {
    const n = typeof num === 'number' && Number.isFinite(num) ? num : 0;
    if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
    if (n >= 1000) return (n / 1000).toFixed(1) + 'K';
    return n.toString();
  };

  const dailyStats = Array.isArray(overview?.daily_stats) ? overview!.daily_stats : [];
  const topPages = Array.isArray(overview?.top_pages) ? overview!.top_pages : [];
  const topReferrers = Array.isArray(overview?.top_referrers) ? overview!.top_referrers : [];
  const topCountries = Array.isArray(overview?.top_countries) ? overview!.top_countries : [];
  const recentVisitors = Array.isArray(overview?.recent_visitors) ? overview!.recent_visitors : [];

  const getMaxPageViews = () => {
    if (dailyStats.length === 0) return 1;
    return Math.max(...dailyStats.map((d) => d?.page_views ?? 0), 1);
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold text-gray-900">
            <BarChart3 className="text-gray-500" size={20} />
            Website Statistics Pro
          </h1>
          <p className="mt-1 text-sm text-gray-600">
            Monitor website traffic, visitors, and performance metrics
          </p>
        </div>
        <div className="flex items-center gap-2">
          <label htmlFor="stats-range" className="sr-only">Date range</label>
          <select
            id="stats-range"
            value={days}
            onChange={(e) => setDays(Number(e.target.value))}
            className="rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus:outline-none"
          >
            <option value={7}>Last 7 days</option>
            <option value={30}>Last 30 days</option>
            <option value={90}>Last 90 days</option>
          </select>
          <button type="button" onClick={fetchStats} className={BTN_PRIMARY}>
            <RefreshCw size={16} />
            Refresh
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="Website statistics sections">
          {[
            { id: 'overview', label: 'Overview', icon: <Activity size={16} /> },
            { id: 'pages', label: 'Top Pages', icon: <Eye size={16} /> },
            { id: 'visitors', label: 'Recent Visitors', icon: <Users size={16} /> },
            { id: 'referrers', label: 'Referrers', icon: <Globe size={16} /> },
          ].map((tab) => (
            <button
              key={tab.id}
              type="button"
              aria-current={activeTab === tab.id ? 'page' : undefined}
              onClick={() => setActiveTab(tab.id as any)}
              className={`flex items-center gap-2 border-b-2 px-1 pb-3 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 ${
                activeTab === tab.id
                  ? 'border-brand-600 text-brand-700'
                  : 'border-transparent text-gray-600 hover:border-gray-300 hover:text-gray-900'
              }`}
            >
              {tab.icon}
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {loading ? (
        <div className={`${CARD} px-5 py-12 text-center text-sm text-gray-500`}>Loading…</div>
      ) : (
        <>
          {/* Stats Cards */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
            <div className={`${CARD} p-5`}>
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Page Views</p>
                  <p className="mt-1 text-2xl font-semibold text-gray-900">
                    {formatNumber(overview?.total_page_views || 0)}
                  </p>
                </div>
                <div className="rounded-md bg-brand-50 p-2.5">
                  <Eye className="text-brand-600" size={20} />
                </div>
              </div>
            </div>

            <div className={`${CARD} p-5`}>
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Unique Visitors</p>
                  <p className="mt-1 text-2xl font-semibold text-gray-900">
                    {formatNumber(overview?.total_unique_visitors || 0)}
                  </p>
                </div>
                <div className="rounded-md bg-emerald-50 p-2.5">
                  <Users className="text-emerald-600" size={20} />
                </div>
              </div>
            </div>

            <div className={`${CARD} p-5`}>
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Bandwidth</p>
                  <p className="mt-1 text-2xl font-semibold text-gray-900">
                    {formatBytes(overview?.total_bandwidth || 0)}
                  </p>
                </div>
                <div className="rounded-md bg-sky-50 p-2.5">
                  <Wifi className="text-sky-600" size={20} />
                </div>
              </div>
            </div>

            <div className={`${CARD} p-5`}>
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Avg Response</p>
                  <p className="mt-1 text-2xl font-semibold text-gray-900">
                    {(overview?.avg_response_time || 0).toFixed(1)}ms
                  </p>
                </div>
                <div className="rounded-md bg-amber-50 p-2.5">
                  <Clock className="text-amber-600" size={20} />
                </div>
              </div>
            </div>

            <div className={`${CARD} p-5`}>
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Bounce Rate</p>
                  <p className="mt-1 text-2xl font-semibold text-gray-900">
                    {(overview?.avg_bounce_rate || 0).toFixed(1)}%
                  </p>
                </div>
                <div className="rounded-md bg-red-50 p-2.5">
                  <ArrowDownRight className="text-red-600" size={20} />
                </div>
              </div>
            </div>
          </div>

          {/* Tab Content */}
          {activeTab === 'overview' && (
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              {/* Daily Traffic Chart */}
              <div className={CARD}>
                <div className={CARD_HEADER}>
                  <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                    <TrendingUp size={16} className="text-gray-500" />
                    Daily Traffic
                  </h2>
                </div>
                <div className="p-5">
                  {dailyStats.length > 0 ? (
                    <div className="space-y-2">
                      {dailyStats.map((day, idx) => (
                        <div key={idx} className="flex items-center gap-3">
                          <span className="w-16 text-xs text-gray-500">{(day?.date || '').slice(5) || '—'}</span>
                          <div className="h-6 flex-1 overflow-hidden rounded-md bg-gray-100">
                            <div
                              className="flex h-full items-center justify-end rounded-md bg-brand-600 pr-2"
                              style={{ width: `${((day?.page_views ?? 0) / getMaxPageViews()) * 100}%` }}
                            >
                              {(day?.page_views ?? 0) > 0 && (
                                <span className="text-xs font-medium text-white">{day.page_views}</span>
                              )}
                            </div>
                          </div>
                          <span className="w-20 text-right text-xs text-gray-500">
                            {day?.unique_visitors ?? 0} visitors
                          </span>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="py-8 text-center text-sm text-gray-500">No data available</p>
                  )}
                </div>
              </div>

              {/* Top Countries */}
              <div className={CARD}>
                <div className={CARD_HEADER}>
                  <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                    <Globe size={16} className="text-gray-500" />
                    Top Countries
                  </h2>
                </div>
                <div className="p-5">
                  {topCountries.length > 0 ? (
                    <div className="space-y-3">
                      {topCountries.map((country, idx) => (
                        <div key={idx} className="flex items-center justify-between">
                          <span className="text-sm font-medium text-gray-900">
                            {country?.country || 'Unknown'}
                          </span>
                          <span className="text-sm text-gray-600">{country?.visitors ?? 0} visitors</span>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="py-8 text-center text-sm text-gray-500">No data available</p>
                  )}
                </div>
              </div>
            </div>
          )}

          {activeTab === 'pages' && (
            <div className={CARD}>
              <div className={CARD_HEADER}>
                <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                  <Eye size={16} className="text-gray-500" />
                  Top Pages
                </h2>
              </div>
              {topPages.length > 0 ? (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead className="bg-gray-50 border-b border-gray-200">
                      <tr>
                        <th className={TH}>Page Path</th>
                        <th className={`${TH} text-right`}>Page Views</th>
                      </tr>
                    </thead>
                    <tbody>
                      {topPages.map((page, idx) => (
                        <tr key={idx} className={ROW}>
                          <td className={`${TD} font-mono text-gray-900`}>{page?.path || '—'}</td>
                          <td className={`${TD} text-right text-gray-900`}>{formatNumber(page?.page_views ?? 0)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="px-5 py-10 text-center text-sm text-gray-500">No page data available</p>
              )}
            </div>
          )}

          {activeTab === 'visitors' && (
            <div className={CARD}>
              <div className={CARD_HEADER}>
                <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                  <Users size={16} className="text-gray-500" />
                  Recent Visitors
                </h2>
              </div>
              {recentVisitors.length > 0 ? (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead className="bg-gray-50 border-b border-gray-200">
                      <tr>
                        <th className={TH}>IP</th>
                        <th className={TH}>Path</th>
                        <th className={TH}>Method</th>
                        <th className={TH}>Status</th>
                        <th className={TH}>Time</th>
                        <th className={TH}>Country</th>
                      </tr>
                    </thead>
                    <tbody>
                      {recentVisitors.map((visitor) => (
                        <tr key={visitor.id} className={ROW}>
                          <td className={`${TD} font-mono text-gray-900`}>{visitor?.visitor_ip || '—'}</td>
                          <td className={`${TD} font-mono text-gray-900`}>{visitor?.path || '—'}</td>
                          <td className="px-4 py-3">
                            <span className={`${BADGE} ${
                              visitor.method === 'GET' ? 'bg-emerald-50 text-emerald-700' :
                              visitor.method === 'POST' ? 'bg-brand-50 text-brand-700' :
                              'bg-gray-100 text-gray-700'
                            }`}>
                              {visitor?.method || '—'}
                            </span>
                          </td>
                          <td className="px-4 py-3">
                            <span className={`${BADGE} ${
                              (visitor.status_code ?? 0) < 300 ? 'bg-emerald-50 text-emerald-700' :
                              (visitor.status_code ?? 0) < 400 ? 'bg-amber-50 text-amber-700' :
                              'bg-red-50 text-red-700'
                            }`}>
                              {visitor?.status_code ?? '—'}
                            </span>
                          </td>
                          <td className={TD}>{(visitor?.response_time ?? 0).toFixed(1)}ms</td>
                          <td className={TD}>{visitor?.country || '-'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="px-5 py-10 text-center text-sm text-gray-500">No visitor data available</p>
              )}
            </div>
          )}

          {activeTab === 'referrers' && (
            <div className={CARD}>
              <div className={CARD_HEADER}>
                <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                  <Globe size={16} className="text-gray-500" />
                  Top Referrers
                </h2>
              </div>
              <div className="p-5">
                {topReferrers.length > 0 ? (
                  <div className="space-y-2">
                    {topReferrers.map((ref, idx) => (
                      <div key={idx} className="flex items-center justify-between rounded-md border border-gray-200 bg-gray-50 px-3 py-2.5">
                        <span className="max-w-md truncate text-sm font-medium text-gray-900">
                          {ref?.referer || 'Direct'}
                        </span>
                        <span className="text-sm text-gray-600">{ref?.visits ?? 0} visits</span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="py-8 text-center text-sm text-gray-500">No referrer data available</p>
                )}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
