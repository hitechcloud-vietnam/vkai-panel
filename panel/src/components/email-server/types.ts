/**
 * Shapes returned by the two backend modules this section sits on top of:
 * core/internal/models/mail_server.go and core/internal/models/email_marketing.go.
 *
 * These handlers do NOT use the {success, data} envelope the rest of the API
 * uses - they answer with a bare object such as {"domains": [...]}. Every
 * reader in api.ts therefore reaches into the named key directly. Getting that
 * wrong is silent: the request succeeds and the table renders empty.
 */

/* ------------------------------------------------------------------ *
 * Mail server (core/internal/handler/mail_server.go)
 * ------------------------------------------------------------------ */

export interface MailDomain {
  id: string;
  tenant_id: string;
  domain: string;
  is_verified: boolean;
  mx_record: string;
  spf_record: string;
  dkim_enabled: boolean;
  dmarc_record: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface MailAccount {
  id: string;
  tenant_id: string;
  domain_id: string;
  email: string;
  quota_mb: number;
  used_mb: number;
  is_active: boolean;
  forward_to: string;
  auto_reply: boolean;
  auto_reply_msg: string;
  last_login_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface MailAlias {
  id: string;
  tenant_id: string;
  domain_id: string;
  source: string;
  destination: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

/** One row of mail_queue. `status` is queued | sent | failed | deferred. */
export interface MailQueueItem {
  id: string;
  tenant_id: string;
  from: string;
  to: string;
  subject: string;
  status: string;
  retry_count: number;
  last_error: string;
  scheduled_at: string | null;
  sent_at: string | null;
  created_at: string;
}

export interface MailSpamFilter {
  id: string;
  tenant_id: string;
  enabled: boolean;
  spam_threshold: number;
  reject_score: number;
  greylisting: boolean;
  blacklist: string[] | null;
  whitelist: string[] | null;
  created_at: string;
  updated_at: string;
}

export interface MailServerConfig {
  id: string;
  tenant_id: string;
  hostname: string;
  smtp_port: number;
  smtps_port: number;
  imap_port: number;
  imaps_port: number;
  max_message_size: number;
  max_mailboxes: number;
  tls_enabled: boolean;
  cert_path: string;
  key_path: string;
  is_running: boolean;
  created_at: string;
  updated_at: string;
}

export interface MailStats {
  total_domains: number;
  total_accounts: number;
  total_aliases: number;
  queue_size: number;
  sent_today: number;
  received_today: number;
  failed_today: number;
  storage_used_mb: number;
}

/* ------------------------------------------------------------------ *
 * Email marketing (core/internal/handler/email_marketing.go)
 * ------------------------------------------------------------------ */

export interface EmailCampaign {
  id: string;
  tenant_id: string;
  name: string;
  subject: string;
  html_content: string;
  plain_text: string;
  /** draft | scheduled | sending | sent | paused | cancelled */
  status: string;
  scheduled_at: string | null;
  sent_at: string | null;
  total_recipients: number;
  sent_count: number;
  open_count: number;
  click_count: number;
  bounce_count: number;
  unsubscribe_count: number;
  from_name: string;
  from_email: string;
  reply_to: string;
  tags: string[] | null;
  created_at: string;
  updated_at: string;
}

export interface EmailContact {
  id: string;
  tenant_id: string;
  email: string;
  first_name: string;
  last_name: string;
  /** active | unsubscribed | bounced | complained */
  status: string;
  /** manual | import | api | signup_form */
  source: string;
  tags: string[] | null;
  metadata: Record<string, string> | null;
  confirmed_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface EmailList {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  contact_count: number;
  double_opt_in: boolean;
  created_at: string;
  updated_at: string;
}

export interface EmailTemplate {
  id: string;
  tenant_id: string;
  name: string;
  subject: string;
  html_content: string;
  category: string;
  thumbnail: string;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}

export interface EmailStats {
  total_campaigns: number;
  total_contacts: number;
  total_lists: number;
  total_sent: number;
  total_opened: number;
  total_clicked: number;
  total_bounced: number;
  avg_open_rate: number;
  avg_click_rate: number;
  avg_bounce_rate: number;
}

/* ------------------------------------------------------------------ *
 * Navigation
 * ------------------------------------------------------------------ */

export const TOP_TABS = [
  { id: 'mail-marketing', label: 'Mail Marketing' },
  { id: 'mail-domain', label: 'Mail Domain' },
  { id: 'mailboxes', label: 'Mailboxes' },
  { id: 'email', label: 'Email' },
  { id: 'other-settings', label: 'Other Settings' },
] as const;

export type TopTabId = (typeof TOP_TABS)[number]['id'];

export const MARKETING_SUBTABS = [
  { id: 'overview', label: 'Overview' },
  { id: 'task', label: 'Marketing Task' },
  { id: 'template', label: 'Template' },
  { id: 'subscribers', label: 'Subscribers' },
  { id: 'groups', label: 'Groups' },
  { id: 'suspend-list', label: 'Suspend List' },
  { id: 'automation', label: 'Automation' },
] as const;

export const EMAIL_SUBTABS = [
  { id: 'inbox', label: 'Inbox' },
  { id: 'outbox', label: 'Outbox' },
  { id: 'spam', label: 'Spam' },
  { id: 'sender', label: 'Sender' },
] as const;

export const SETTINGS_SUBTABS = [
  { id: 'common', label: 'Common settings' },
  { id: 'bcc', label: 'BCC' },
  { id: 'forward', label: 'Mail forward' },
  { id: 'responder', label: 'Auto Responder' },
  { id: 'backup', label: 'Backup' },
] as const;

/** Every sub-tab id the URL may carry, per top tab. */
export const SUBTABS: Record<TopTabId, readonly { id: string; label: string }[]> = {
  'mail-marketing': MARKETING_SUBTABS,
  'mail-domain': [],
  mailboxes: [],
  email: EMAIL_SUBTABS,
  'other-settings': SETTINGS_SUBTABS,
};

export function isTopTab(value: string | null | undefined): value is TopTabId {
  return TOP_TABS.some((t) => t.id === value);
}

/** The sub-tab to use when the URL names none, or names one that does not exist. */
export function defaultSub(tab: TopTabId): string {
  const list = SUBTABS[tab];
  return list.length > 0 ? list[0].id : '';
}

export function normaliseSub(tab: TopTabId, value: string | null | undefined): string {
  const list = SUBTABS[tab];
  if (list.length === 0) return '';
  return list.some((s) => s.id === value) ? (value as string) : list[0].id;
}
