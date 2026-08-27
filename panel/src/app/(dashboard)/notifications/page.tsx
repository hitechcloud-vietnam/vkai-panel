'use client';

import { useState, useEffect } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Bell, Mail, MessageSquare, Settings, Plus, RefreshCw, Check, Trash2 } from 'lucide-react';
import { notificationApi } from '@/services/api';

interface Notification {
  id: string;
  type: string;
  title: string;
  message: string;
  status: string;
  channel: string;
  created_at: string;
  read_at: string | null;
}

interface NotificationTemplate {
  id: string;
  name: string;
  type: string;
  subject: string;
  body: string;
  enabled: boolean;
}

interface NotificationChannel {
  id: string;
  name: string;
  type: string;
  enabled: boolean;
  config: Record<string, any>;
}

const CARD_CLASS = 'rounded-lg border border-gray-200 bg-white shadow-sm';
const CARD_HEADER_CLASS =
  'flex flex-row items-center justify-between space-y-0 border-b border-gray-200 px-5 py-4';
const CARD_TITLE_CLASS = 'text-sm font-semibold text-gray-900';
const BTN_PRIMARY = 'bg-brand-600 text-white hover:bg-brand-700 focus-visible:ring-brand-500';
const BTN_SECONDARY =
  'border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus-visible:ring-brand-500';
const BADGE_BASE = 'rounded-md border-transparent px-2 py-0.5 text-xs font-medium';
const TABS_LIST_CLASS =
  'inline-flex h-auto items-center gap-1 rounded-md border border-gray-200 bg-white p-1 text-gray-600';
const TABS_TRIGGER_CLASS =
  'rounded-md px-3 py-1.5 text-sm font-medium text-gray-600 data-[state=active]:bg-brand-50 data-[state=active]:text-brand-700 data-[state=active]:shadow-none';

function formatTimestamp(value: string): string {
  if (!value) return '—';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return '—';
  return parsed.toLocaleString();
}

export default function NotificationsPage() {
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [templates, setTemplates] = useState<NotificationTemplate[]>([]);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState('inbox');

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);
      const [notifRes, templatesRes, channelsRes] = await Promise.all([
        notificationApi.getNotifications(),
        notificationApi.getTemplates(),
        notificationApi.getChannels(),
      ]);
      setNotifications(Array.isArray(notifRes?.data?.notifications) ? notifRes.data.notifications : []);
      setTemplates(Array.isArray(templatesRes?.data?.templates) ? templatesRes.data.templates : []);
      setChannels(Array.isArray(channelsRes?.data?.channels) ? channelsRes.data.channels : []);
    } catch (err: any) {
      console.error('Failed to fetch notifications:', err);
      setNotifications([]);
      setTemplates([]);
      setChannels([]);
      setError(err?.response?.data?.message || 'Failed to load notifications');
    } finally {
      setLoading(false);
    }
  };

  const markAsRead = async (id: string) => {
    try {
      setError(null);
      await notificationApi.markAsRead(id);
      setNotifications((prev) =>
        prev.map((n) => (n.id === id ? { ...n, status: 'read', read_at: new Date().toISOString() } : n))
      );
    } catch (err: any) {
      console.error('Failed to mark as read:', err);
      setError(err?.response?.data?.message || 'Failed to mark notification as read');
    }
  };

  const deleteNotification = async (id: string) => {
    try {
      setError(null);
      await notificationApi.deleteNotification(id);
      setNotifications((prev) => prev.filter((n) => n.id !== id));
    } catch (err: any) {
      console.error('Failed to delete notification:', err);
      setError(err?.response?.data?.message || 'Failed to delete notification');
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'unread':
        return 'bg-brand-50 text-brand-700';
      case 'read':
        return 'bg-gray-100 text-gray-700';
      case 'archived':
        return 'bg-amber-50 text-amber-700';
      default:
        return 'bg-gray-100 text-gray-700';
    }
  };

  const getTypeIcon = (type: string) => {
    switch (type) {
      case 'email':
        return <Mail className="h-4 w-4 text-gray-500" aria-hidden="true" />;
      case 'sms':
        return <MessageSquare className="h-4 w-4 text-gray-500" aria-hidden="true" />;
      case 'webhook':
        return <Settings className="h-4 w-4 text-gray-500" aria-hidden="true" />;
      default:
        return <Bell className="h-4 w-4 text-gray-500" aria-hidden="true" />;
    }
  };

  const unreadCount = notifications.filter((n) => n?.status === 'unread').length;

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <RefreshCw className="h-6 w-6 animate-spin text-gray-400" aria-hidden="true" />
        <span className="ml-2 text-sm text-gray-600">Loading notifications...</span>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Notifications</h1>
          <p className="mt-1 text-sm text-gray-600">Manage alerts, templates, and channels</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="outline" onClick={fetchData} className={BTN_SECONDARY}>
            <RefreshCw className="mr-2 h-4 w-4" aria-hidden="true" />
            Refresh
          </Button>
          <Button className={BTN_PRIMARY}>
            <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
            New Template
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
          {error}
        </div>
      )}

      <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-4">
        <TabsList className={TABS_LIST_CLASS}>
          <TabsTrigger value="inbox" className={TABS_TRIGGER_CLASS}>
            Inbox
            {unreadCount > 0 && (
              <Badge variant="outline" className={`${BADGE_BASE} ml-2 bg-brand-50 text-brand-700`}>{unreadCount}</Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="templates" className={TABS_TRIGGER_CLASS}>Templates</TabsTrigger>
          <TabsTrigger value="channels" className={TABS_TRIGGER_CLASS}>Channels</TabsTrigger>
          <TabsTrigger value="preferences" className={TABS_TRIGGER_CLASS}>Preferences</TabsTrigger>
        </TabsList>

        <TabsContent value="inbox" className="space-y-4">
          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Recent Notifications</CardTitle>
              <span className="text-xs text-gray-500">{notifications.length} items</span>
            </CardHeader>
            <CardContent className="px-5 py-4">
              {notifications.length === 0 ? (
                <div className="py-8 text-center">
                  <Bell className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                  <p className="mt-3 text-sm font-semibold text-gray-900">No notifications</p>
                  <p className="mt-1 text-sm text-gray-600">
                    New alerts and messages will appear here.
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {notifications.map((notification) => (
                    <div
                      key={notification.id}
                      className={`flex flex-wrap items-start justify-between gap-3 rounded-md border border-gray-200 px-4 py-3 ${
                        notification.status === 'unread' ? 'bg-brand-50' : 'bg-white'
                      }`}
                    >
                      <div className="flex items-start gap-3">
                        <div className="mt-0.5">{getTypeIcon(notification.channel)}</div>
                        <div>
                          <p className="text-sm font-medium text-gray-900">
                            {notification.title || '—'}
                          </p>
                          <p className="mt-0.5 text-sm text-gray-600">{notification.message || ''}</p>
                          <p className="mt-1 text-xs text-gray-500" suppressHydrationWarning>
                            {formatTimestamp(notification.created_at)}
                          </p>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <Badge variant="outline" className={`${BADGE_BASE} ${getStatusColor(notification.status)}`}>
                          {notification.status || 'unknown'}
                        </Badge>
                        {notification.status === 'unread' && (
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label="Mark notification as read"
                            title="Mark as read"
                            className="text-gray-600 hover:bg-gray-100 hover:text-gray-900"
                            onClick={() => markAsRead(notification.id)}
                          >
                            <Check className="h-4 w-4" aria-hidden="true" />
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label="Delete notification"
                          title="Delete"
                          className="text-red-600 hover:bg-red-50 hover:text-red-700"
                          onClick={() => deleteNotification(notification.id)}
                        >
                          <Trash2 className="h-4 w-4" aria-hidden="true" />
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="templates" className="space-y-4">
          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Notification Templates</CardTitle>
              <span className="text-xs text-gray-500">{templates.length} templates</span>
            </CardHeader>
            <CardContent className="px-5 py-4">
              {templates.length === 0 ? (
                <div className="py-8 text-center">
                  <Mail className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                  <p className="mt-3 text-sm font-semibold text-gray-900">No templates</p>
                  <p className="mt-1 text-sm text-gray-600">
                    Create a template to standardise outgoing messages.
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {templates.map((template) => (
                    <div
                      key={template.id}
                      className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-gray-200 px-4 py-3"
                    >
                      <div>
                        <p className="text-sm font-medium text-gray-900">{template.name || '—'}</p>
                        <p className="mt-0.5 text-sm text-gray-600">{template.subject || ''}</p>
                      </div>
                      <div className="flex items-center gap-2">
                        <Badge variant="outline" className={`${BADGE_BASE} border border-gray-200 bg-white text-gray-700`}>
                          {template.type || 'unknown'}
                        </Badge>
                        <Badge variant="outline"
                          className={`${BADGE_BASE} ${
                            template.enabled ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-700'
                          }`}
                        >
                          {template.enabled ? 'Enabled' : 'Disabled'}
                        </Badge>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="channels" className="space-y-4">
          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Notification Channels</CardTitle>
              <span className="text-xs text-gray-500">{channels.length} channels</span>
            </CardHeader>
            <CardContent className="px-5 py-4">
              {channels.length === 0 ? (
                <div className="py-8 text-center">
                  <Settings className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                  <p className="mt-3 text-sm font-semibold text-gray-900">No channels</p>
                  <p className="mt-1 text-sm text-gray-600">
                    Add an email, SMS, or webhook channel to start delivering notifications.
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {channels.map((channel) => (
                    <div
                      key={channel.id}
                      className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-gray-200 px-4 py-3"
                    >
                      <div className="flex items-center gap-3">
                        {getTypeIcon(channel.type)}
                        <div>
                          <p className="text-sm font-medium text-gray-900">{channel.name || '—'}</p>
                          <p className="mt-0.5 text-sm text-gray-600">{channel.type || ''}</p>
                        </div>
                      </div>
                      <Badge variant="outline"
                        className={`${BADGE_BASE} ${
                          channel.enabled ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-700'
                        }`}
                      >
                        {channel.enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="preferences" className="space-y-4">
          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Notification Preferences</CardTitle>
            </CardHeader>
            <CardContent className="px-5 py-4">
              <p className="text-sm text-gray-600">Configure your notification preferences here.</p>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
