/** Formatters shared by the Email Server panes. */

/** A date the operator can read, or null when the backend reported nothing. */
export function formatDateTime(value: string | null | undefined): string | null {
  if (!value) return null;
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleString();
}

export function formatDate(value: string | null | undefined): string | null {
  if (!value) return null;
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleDateString();
}

/** Megabytes as the backend counts them, printed in the largest sane unit. */
export function formatMB(mb: number | null | undefined): string {
  const value = typeof mb === 'number' && Number.isFinite(mb) ? mb : 0;
  if (value < 1024) return `${value} MB`;
  const gb = value / 1024;
  if (gb < 1024) return `${gb.toFixed(gb < 10 ? 1 : 0)} GB`;
  return `${(gb / 1024).toFixed(1)} TB`;
}

export function formatPercent(value: number | null | undefined, decimals = 1): string {
  const num = typeof value === 'number' && Number.isFinite(value) ? value : 0;
  return `${num.toFixed(decimals)}%`;
}

/** 0 quota means unlimited in mail_accounts, so it must not print as "0 MB". */
export function quotaLabel(quotaMB: number): string {
  return quotaMB > 0 ? formatMB(quotaMB) : 'Unlimited';
}

export function usagePercent(usedMB: number, quotaMB: number): number | null {
  if (!quotaMB || quotaMB <= 0) return null;
  return Math.min(100, Math.round((usedMB / quotaMB) * 100));
}

/**
 * An SMTP failure in plain words.
 *
 * Postfix hands back things like "550 5.1.1 <a@b.com>: Recipient address
 * rejected: User unknown". An operator reading only the code cannot tell a
 * typo from a blacklisting, and the two need opposite responses - so the code
 * is kept (it is what a mail admin searches for) and a sentence is added.
 */
export function explainMailFailure(raw: string | null | undefined): string | null {
  if (!raw || !raw.trim()) return null;
  const text = raw.trim();
  const lower = text.toLowerCase();

  const rules: { match: (s: string) => boolean; says: string }[] = [
    {
      match: (s) => /\b5\.1\.1\b/.test(s) || s.includes('user unknown') || s.includes('no such user'),
      says: 'The recipient address does not exist on the receiving server. Check the spelling, or drop the address.',
    },
    {
      match: (s) => /\b5\.2\.2\b/.test(s) || s.includes('quota') || s.includes('mailbox full'),
      says: 'The recipient mailbox is full. Nothing to fix at this end; the message will keep failing until they clear space.',
    },
    {
      match: (s) => s.includes('spf'),
      says: 'The receiving server rejected the message on SPF. Publish the SPF record shown on the Mail Domain tab for this sending domain.',
    },
    {
      match: (s) => s.includes('dkim'),
      says: 'The receiving server rejected the DKIM signature. The signing key and the published DKIM record do not agree.',
    },
    {
      match: (s) => s.includes('dmarc'),
      says: 'The receiving server rejected the message on DMARC policy. SPF or DKIM must pass and align with the From: domain.',
    },
    {
      match: (s) => s.includes('blacklist') || s.includes('blocked using') || s.includes('spamhaus') || s.includes('rbl'),
      says: 'The sending IP is on a blocklist. Delisting has to be requested from the list operator; sending more will make it worse.',
    },
    {
      match: (s) => s.includes('relay access denied') || s.includes('relaying denied'),
      says: 'The receiving server refused to relay for this sender. Usually the wrong smarthost, or credentials the relay does not accept.',
    },
    {
      match: (s) => s.includes('greylist') || /\b4\.7\.1\b/.test(s),
      says: 'Greylisted: the receiver asked to be retried later. This normally clears itself on the next attempt.',
    },
    {
      match: (s) => s.includes('connection timed out') || s.includes('timeout') || s.includes('no route to host'),
      says: 'The receiving server could not be reached. Port 25 outbound is often blocked by the hosting provider.',
    },
    {
      match: (s) => s.includes('certificate') || s.includes('tls'),
      says: 'The TLS handshake with the receiving server failed. Check the certificate and key on the Other Settings tab.',
    },
    {
      match: (s) => s.includes('spam') || /\b5\.7\.1\b/.test(s),
      says: 'The message was refused as spam. Content, sending rate and the domain reputation are all candidates.',
    },
  ];

  const hit = rules.find((r) => r.match(lower));
  return hit ? hit.says : null;
}

/** A short, stable label for a queue row's status. */
export function queueStatusTone(status: string): 'emerald' | 'amber' | 'red' | 'sky' | 'gray' {
  switch ((status || '').toLowerCase()) {
    case 'sent':
      return 'emerald';
    case 'queued':
      return 'sky';
    case 'deferred':
      return 'amber';
    case 'failed':
      return 'red';
    default:
      return 'gray';
  }
}

export function campaignStatusTone(status: string): 'emerald' | 'amber' | 'red' | 'sky' | 'gray' {
  switch ((status || '').toLowerCase()) {
    case 'sent':
      return 'emerald';
    case 'sending':
      return 'sky';
    case 'scheduled':
      return 'sky';
    case 'paused':
      return 'amber';
    case 'cancelled':
      return 'red';
    default:
      return 'gray';
  }
}

export function contactStatusTone(status: string): 'emerald' | 'amber' | 'red' | 'sky' | 'gray' {
  switch ((status || '').toLowerCase()) {
    case 'active':
      return 'emerald';
    case 'unsubscribed':
      return 'amber';
    case 'bounced':
      return 'red';
    case 'complained':
      return 'red';
    default:
      return 'gray';
  }
}

/** Statuses that must never be sent to again. */
export const SUSPENDED_STATUSES = ['unsubscribed', 'bounced', 'complained'];

export function isSuspended(status: string): boolean {
  return SUSPENDED_STATUSES.includes((status || '').toLowerCase());
}
