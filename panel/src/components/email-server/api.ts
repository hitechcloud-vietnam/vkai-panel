/**
 * Every call this section is allowed to make.
 *
 * The rule for this file: a function only exists here if the route exists in
 * core/internal/handler/router.go AND the service behind it does something.
 * Anything the backend cannot do yet has no wrapper - so a pane cannot
 * accidentally render a control for it. What is missing is listed at the bottom
 * of this file and surfaced to the operator by <BackendGap>.
 *
 * These two handlers answer with a bare object ({"domains": [...]}), not the
 * {success, data} envelope used elsewhere, so nothing here goes through
 * services/api's unwrap helpers.
 */

import { api } from '@/services/api';

import type {
  EmailCampaign,
  EmailContact,
  EmailList,
  EmailStats,
  EmailTemplate,
  MailAccount,
  MailAlias,
  MailDomain,
  MailQueueItem,
  MailServerConfig,
  MailSpamFilter,
  MailStats,
} from './types';

const MAIL = '/api/v1/mail-server';
const MARKETING = '/api/v1/email-marketing';

/** Go serialises a nil slice as null; every list reader lands here. */
function list<T>(value: unknown): T[] {
  return Array.isArray(value) ? (value as T[]) : [];
}

/* ------------------------------------------------------------------ *
 * Mail server
 * ------------------------------------------------------------------ */

export const mailServerApi = {
  stats: async (): Promise<MailStats | null> => {
    const res = await api.get(`${MAIL}/stats`);
    return (res.data?.stats as MailStats) ?? null;
  },

  listDomains: async (): Promise<MailDomain[]> => {
    const res = await api.get(`${MAIL}/domains`);
    return list<MailDomain>(res.data?.domains);
  },
  createDomain: (domain: string) => api.post(`${MAIL}/domains`, { domain }),
  deleteDomain: (id: string) => api.delete(`${MAIL}/domains/${id}`),

  listAccounts: async (): Promise<MailAccount[]> => {
    const res = await api.get(`${MAIL}/accounts`);
    return list<MailAccount>(res.data?.accounts);
  },
  createAccount: (body: {
    domain_id: string;
    email: string;
    password: string;
    quota_mb: number;
  }) => api.post(`${MAIL}/accounts`, body),
  updateAccount: (
    id: string,
    body: Partial<{
      quota_mb: number;
      is_active: boolean;
      forward_to: string;
      auto_reply: boolean;
      auto_reply_msg: string;
    }>
  ) => api.put(`${MAIL}/accounts/${id}`, body),
  deleteAccount: (id: string) => api.delete(`${MAIL}/accounts/${id}`),

  listAliases: async (): Promise<MailAlias[]> => {
    const res = await api.get(`${MAIL}/aliases`);
    return list<MailAlias>(res.data?.aliases);
  },
  createAlias: (body: { domain_id: string; source: string; destination: string }) =>
    api.post(`${MAIL}/aliases`, body),
  deleteAlias: (id: string) => api.delete(`${MAIL}/aliases/${id}`),

  /** The repository caps this at the newest 100 rows. */
  listQueue: async (): Promise<MailQueueItem[]> => {
    const res = await api.get(`${MAIL}/queue`);
    return list<MailQueueItem>(res.data?.queue);
  },
  deleteQueueItem: (id: string) => api.delete(`${MAIL}/queue/${id}`),
  /** Deletes the failed rows only - see MailServerRepository.FlushQueue. */
  flushFailedQueue: () => api.post(`${MAIL}/queue/flush`),

  getSpamFilter: async (): Promise<MailSpamFilter | null> => {
    const res = await api.get(`${MAIL}/spam-filter`);
    return (res.data?.spam_filter as MailSpamFilter) ?? null;
  },
  updateSpamFilter: (body: {
    enabled?: boolean;
    spam_threshold?: number;
    reject_score?: number;
    greylisting?: boolean;
    blacklist?: string[];
    whitelist?: string[];
  }) => api.put(`${MAIL}/spam-filter`, body),

  getConfig: async (): Promise<MailServerConfig | null> => {
    const res = await api.get(`${MAIL}/config`);
    return (res.data?.config as MailServerConfig) ?? null;
  },
  updateConfig: (body: {
    hostname?: string;
    smtp_port?: number;
    smtps_port?: number;
    imap_port?: number;
    imaps_port?: number;
    max_message_size?: number;
    tls_enabled?: boolean;
    cert_path?: string;
    key_path?: string;
  }) => api.put(`${MAIL}/config`, body),
};

/* ------------------------------------------------------------------ *
 * Email marketing
 * ------------------------------------------------------------------ */

export interface Paged<T> {
  items: T[];
  total: number;
}

export const marketingApi = {
  stats: async (): Promise<EmailStats | null> => {
    const res = await api.get(`${MARKETING}/stats`);
    return (res.data?.stats as EmailStats) ?? null;
  },

  listCampaigns: async (limit = 50, offset = 0): Promise<Paged<EmailCampaign>> => {
    const res = await api.get(`${MARKETING}/campaigns`, { params: { limit, offset } });
    return {
      items: list<EmailCampaign>(res.data?.campaigns),
      total: typeof res.data?.total === 'number' ? res.data.total : 0,
    };
  },
  createCampaign: (body: {
    name: string;
    subject: string;
    html_content: string;
    plain_text?: string;
    from_name: string;
    from_email: string;
    reply_to?: string;
    tags?: string[];
  }) => api.post(`${MARKETING}/campaigns`, body),
  updateCampaign: (
    id: string,
    body: Partial<{
      name: string;
      subject: string;
      html_content: string;
      plain_text: string;
      from_name: string;
      from_email: string;
      reply_to: string;
    }>
  ) => api.put(`${MARKETING}/campaigns/${id}`, body),
  deleteCampaign: (id: string) => api.delete(`${MARKETING}/campaigns/${id}`),
  /**
   * EmailMarketingService.SendCampaign only sets status = 'sending'. No worker
   * reads that status, so nothing is dispatched. The pane says so next to the
   * button; do not relabel this without checking the service again.
   */
  startCampaign: (id: string) => api.post(`${MARKETING}/campaigns/${id}/send`),
  pauseCampaign: (id: string) => api.post(`${MARKETING}/campaigns/${id}/pause`),

  listContacts: async (limit = 50, offset = 0, search = ''): Promise<Paged<EmailContact>> => {
    const res = await api.get(`${MARKETING}/contacts`, {
      params: { limit, offset, ...(search ? { search } : {}) },
    });
    return {
      items: list<EmailContact>(res.data?.contacts),
      total: typeof res.data?.total === 'number' ? res.data.total : 0,
    };
  },
  createContact: (body: {
    email: string;
    first_name?: string;
    last_name?: string;
    tags?: string[];
    source?: string;
  }) => api.post(`${MARKETING}/contacts`, body),
  deleteContact: (id: string) => api.delete(`${MARKETING}/contacts/${id}`),

  listGroups: async (): Promise<EmailList[]> => {
    const res = await api.get(`${MARKETING}/lists`);
    return list<EmailList>(res.data?.lists);
  },
  createGroup: (body: { name: string; description?: string; double_opt_in?: boolean }) =>
    api.post(`${MARKETING}/lists`, body),
  deleteGroup: (id: string) => api.delete(`${MARKETING}/lists/${id}`),

  listTemplates: async (): Promise<EmailTemplate[]> => {
    const res = await api.get(`${MARKETING}/templates`);
    return list<EmailTemplate>(res.data?.templates);
  },
  createTemplate: (body: {
    name: string;
    subject: string;
    html_content: string;
    category?: string;
  }) => api.post(`${MARKETING}/templates`, body),
  deleteTemplate: (id: string) => api.delete(`${MARKETING}/templates/${id}`),
};

/*
 * Deliberately absent, because the backend has no route for them. Each one is
 * rendered as a named gap rather than a dead control:
 *
 *   mail-server: DNS lookup / domain verification, DKIM key generation and
 *     public-key readback (mail_dkim_keys has a table and a model but no
 *     handler), domain update, inbox and spam message listing, send-a-test
 *     message, mailbox import/export, BCC rules, mail backup and restore.
 *   email-marketing: automations (table + model exist, no routes), template
 *     update, contact update / unsubscribe / re-subscribe, group membership
 *     (email_list_contacts has a table but no route), per-campaign send logs,
 *     and any actual delivery.
 */
