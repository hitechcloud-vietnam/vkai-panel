'use client';

import { useState, useEffect } from 'react';
import { 
  BarChart3, Globe, Clock, Users, Eye, Wifi, 
  TrendingUp, ArrowUpRight, ArrowDownRight, RefreshCw,
  ChevronLeft, ChevronRight, Activity
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

export default function WebsiteStatsPage() {
  const [overview, setOverview] = useState<StatsOverview | null>(null);
  const [loading, setLoading] = useState(true);
  const [days, setDays] = useState(30);
  const [activeTab, setActiveTab] = useState<'overview' | 'pages' | 'visitors' | 'referrers'>('overview');

  const fetchStats = async () => {
    setLoading(true);
    try {
      const token = localStorage.getItem('token');
      // Using a sample website_id - in production this would come from website selection
      const websiteId = 'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11';
      const res = await fetch(`/api/v1/website-stats/overview?website_id=${websiteId}&days=${days}`, {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setOverview(data);
      }
    } catch (error) {
      console.error('Failed to fetch stats:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStats();
  }, [days]);

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const formatNumber = (num: number) => {
    if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
    if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
    return num.toString();
  };

  const getMaxPageViews = () => {
    if (!overview?.daily_stats) return 1;
    return Math.max(...overview.daily_stats.map(d => d.page_views), 1);
  };

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
            <BarChart3 className="text-blue-500" size={28} />
            Website Statistics Pro
          </h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">
            Monitor website traffic, visitors, and performance metrics
          </p>
        </div>
        <div className="flex items-center gap-3">
          <select
            value={days}
            onChange={(e) => setDays(Number(e.target.value))}
            className="px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white"
          >
            <option value={7}>Last 7 days</option>
            <option value={30}>Last 30 days</option>
            <option value={90}>Last 90 days</option>
          </select>
          <button
            onClick={fetchStats}
            className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 flex items-center gap-2"
          >
            <RefreshCw size={16} />
            Refresh
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex space-x-1 bg-gray-100 dark:bg-gray-800 p-1 rounded-lg w-fit">
        {[
          { id: 'overview', label: 'Overview', icon: <Activity size={16} /> },
          { id: 'pages', label: 'Top Pages', icon: <Eye size={16} /> },
          { id: 'visitors', label: 'Recent Visitors', icon: <Users size={16} /> },
          { id: 'referrers', label: 'Referrers', icon: <Globe size={16} /> },
        ].map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id as any)}
            className={`px-4 py-2 rounded-md text-sm font-medium flex items-center gap-2 transition-colors ${
              activeTab === tab.id
                ? 'bg-white dark:bg-gray-700 text-blue-600 dark:text-blue-400 shadow-sm'
                : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
            }`}
          >
            {tab.icon}
            {tab.label}
          </button>
        ))}
      </div>

      {loading ? (
        <div className="flex items-center justify-center h-64">
          <RefreshCw className="animate-spin text-blue-500" size={32} />
        </div>
      ) : (
        <>
          {/* Stats Cards */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
            <div className="bg-white dark:bg-gray-800 rounded-xl p-5 border border-gray-200 dark:border-gray-700">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-gray-500 dark:text-gray-400">Page Views</p>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white mt-1">
                    {formatNumber(overview?.total_page_views || 0)}
                  </p>
                </div>
                <div className="p-3 bg-blue-100 dark:bg-blue-900/30 rounded-lg">
                  <Eye className="text-blue-500" size={20} />
                </div>
              </div>
            </div>

            <div className="bg-white dark:bg-gray-800 rounded-xl p-5 border border-gray-200 dark:border-gray-700">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-gray-500 dark:text-gray-400">Unique Visitors</p>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white mt-1">
                    {formatNumber(overview?.total_unique_visitors || 0)}
                  </p>
                </div>
                <div className="p-3 bg-green-100 dark:bg-green-900/30 rounded-lg">
                  <Users className="text-green-500" size={20} />
                </div>
              </div>
            </div>

            <div className="bg-white dark:bg-gray-800 rounded-xl p-5 border border-gray-200 dark:border-gray-700">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-gray-500 dark:text-gray-400">Bandwidth</p>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white mt-1">
                    {formatBytes(overview?.total_bandwidth || 0)}
                  </p>
                </div>
                <div className="p-3 bg-purple-100 dark:bg-purple-900/30 rounded-lg">
                  <Wifi className="text-purple-500" size={20} />
                </div>
              </div>
            </div>

            <div className="bg-white dark:bg-gray-800 rounded-xl p-5 border border-gray-200 dark:border-gray-700">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-gray-500 dark:text-gray-400">Avg Response</p>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white mt-1">
                    {(overview?.avg_response_time || 0).toFixed(1)}ms
                  </p>
                </div>
                <div className="p-3 bg-orange-100 dark:bg-orange-900/30 rounded-lg">
                  <Clock className="text-orange-500" size={20} />
                </div>
              </div>
            </div>

            <div className="bg-white dark:bg-gray-800 rounded-xl p-5 border border-gray-200 dark:border-gray-700">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-gray-500 dark:text-gray-400">Bounce Rate</p>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white mt-1">
                    {(overview?.avg_bounce_rate || 0).toFixed(1)}%
                  </p>
                </div>
                <div className="p-3 bg-red-100 dark:bg-red-900/30 rounded-lg">
                  <ArrowDownRight className="text-red-500" size={20} />
                </div>
              </div>
            </div>
          </div>

          {/* Tab Content */}
          {activeTab === 'overview' && (
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              {/* Daily Traffic Chart */}
              <div className="bg-white dark:bg-gray-800 rounded-xl p-6 border border-gray-200 dark:border-gray-700">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
                  <TrendingUp size={18} />
                  Daily Traffic
                </h3>
                {overview?.daily_stats && overview.daily_stats.length > 0 ? (
                  <div className="space-y-2">
                    {overview.daily_stats.map((day, idx) => (
                      <div key={idx} className="flex items-center gap-3">
                        <span className="text-xs text-gray-500 w-16">{day.date.slice(5)}</span>
                        <div className="flex-1 bg-gray-100 dark:bg-gray-700 rounded-full h-6 overflow-hidden">
                          <div
                            className="bg-blue-500 h-full rounded-full flex items-center justify-end pr-2"
                            style={{ width: `${(day.page_views / getMaxPageViews()) * 100}%` }}
                          >
                            {day.page_views > 0 && (
                              <span className="text-xs text-white font-medium">{day.page_views}</span>
                            )}
                          </div>
                        </div>
                        <span className="text-xs text-gray-500 w-20 text-right">
                          {day.unique_visitors} visitors
                        </span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-gray-500 dark:text-gray-400 text-center py-8">No data available</p>
                )}
              </div>

              {/* Top Countries */}
              <div className="bg-white dark:bg-gray-800 rounded-xl p-6 border border-gray-200 dark:border-gray-700">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
                  <Globe size={18} />
                  Top Countries
                </h3>
                {overview?.top_countries && overview.top_countries.length > 0 ? (
                  <div className="space-y-3">
                    {overview.top_countries.map((country, idx) => (
                      <div key={idx} className="flex items-center justify-between">
                        <div className="flex items-center gap-3">
                          <span className="text-sm font-medium text-gray-900 dark:text-white">
                            {country.country || 'Unknown'}
                          </span>
                        </div>
                        <span className="text-sm text-gray-500">{country.visitors} visitors</span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-gray-500 dark:text-gray-400 text-center py-8">No data available</p>
                )}
              </div>
            </div>
          )}

          {activeTab === 'pages' && (
            <div className="bg-white dark:bg-gray-800 rounded-xl p-6 border border-gray-200 dark:border-gray-700">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
                <Eye size={18} />
                Top Pages
              </h3>
              {overview?.top_pages && overview.top_pages.length > 0 ? (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-gray-200 dark:border-gray-700">
                        <th className="text-left py-3 px-4 text-sm font-medium text-gray-500 dark:text-gray-400">Page Path</th>
                        <th className="text-right py-3 px-4 text-sm font-medium text-gray-500 dark:text-gray-400">Page Views</th>
                      </tr>
                    </thead>
                    <tbody>
                      {overview.top_pages.map((page, idx) => (
                        <tr key={idx} className="border-b border-gray-100 dark:border-gray-700/50">
                          <td className="py-3 px-4 text-sm text-gray-900 dark:text-white font-mono">{page.path}</td>
                          <td className="py-3 px-4 text-sm text-gray-900 dark:text-white text-right">{formatNumber(page.page_views)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="text-gray-500 dark:text-gray-400 text-center py-8">No page data available</p>
              )}
            </div>
          )}

          {activeTab === 'visitors' && (
            <div className="bg-white dark:bg-gray-800 rounded-xl p-6 border border-gray-200 dark:border-gray-700">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
                <Users size={18} />
                Recent Visitors
              </h3>
              {overview?.recent_visitors && overview.recent_visitors.length > 0 ? (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-gray-200 dark:border-gray-700">
                        <th className="text-left py-3 px-4 text-sm font-medium text-gray-500 dark:text-gray-400">IP</th>
                        <th className="text-left py-3 px-4 text-sm font-medium text-gray-500 dark:text-gray-400">Path</th>
                        <th className="text-left py-3 px-4 text-sm font-medium text-gray-500 dark:text-gray-400">Method</th>
                        <th className="text-left py-3 px-4 text-sm font-medium text-gray-500 dark:text-gray-400">Status</th>
                        <th className="text-left py-3 px-4 text-sm font-medium text-gray-500 dark:text-gray-400">Time</th>
                        <th className="text-left py-3 px-4 text-sm font-medium text-gray-500 dark:text-gray-400">Country</th>
                      </tr>
                    </thead>
                    <tbody>
                      {overview.recent_visitors.map((visitor) => (
                        <tr key={visitor.id} className="border-b border-gray-100 dark:border-gray-700/50">
                          <td className="py-3 px-4 text-sm text-gray-900 dark:text-white font-mono">{visitor.visitor_ip}</td>
                          <td className="py-3 px-4 text-sm text-gray-900 dark:text-white font-mono">{visitor.path}</td>
                          <td className="py-3 px-4">
                            <span className={`px-2 py-1 text-xs rounded-full ${
                              visitor.method === 'GET' ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' :
                              visitor.method === 'POST' ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400' :
                              'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300'
                            }`}>
                              {visitor.method}
                            </span>
                          </td>
                          <td className="py-3 px-4">
                            <span className={`px-2 py-1 text-xs rounded-full ${
                              visitor.status_code < 300 ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' :
                              visitor.status_code < 400 ? 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400' :
                              'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
                            }`}>
                              {visitor.status_code}
                            </span>
                          </td>
                          <td className="py-3 px-4 text-sm text-gray-500">{visitor.response_time.toFixed(1)}ms</td>
                          <td className="py-3 px-4 text-sm text-gray-500">{visitor.country || '-'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="text-gray-500 dark:text-gray-400 text-center py-8">No visitor data available</p>
              )}
            </div>
          )}

          {activeTab === 'referrers' && (
            <div className="bg-white dark:bg-gray-800 rounded-xl p-6 border border-gray-200 dark:border-gray-700">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
                <Globe size={18} />
                Top Referrers
              </h3>
              {overview?.top_referrers && overview.top_referrers.length > 0 ? (
                <div className="space-y-3">
                  {overview.top_referrers.map((ref, idx) => (
                    <div key={idx} className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
                      <div className="flex items-center gap-3">
                        <span className="text-sm font-medium text-gray-900 dark:text-white truncate max-w-md">
                          {ref.referer || 'Direct'}
                        </span>
                      </div>
                      <span className="text-sm text-gray-500">{ref.visits} visits</span>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-gray-500 dark:text-gray-400 text-center py-8">No referrer data available</p>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}
