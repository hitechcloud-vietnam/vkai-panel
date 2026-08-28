'use client';

/**
 * Reading the suspend list out of an API that cannot filter by status.
 *
 * GET /api/v1/email-marketing/contacts takes limit, offset and a free-text
 * search, and nothing else - EmailMarketingRepository.ListContacts builds its
 * WHERE clause from tenant and search only. So the addresses that have
 * unsubscribed, bounced or complained can only be found by pulling contacts and
 * sorting them here.
 *
 * That is a page-sized answer to a whole-database question, and pretending
 * otherwise is how a suppressed address gets mailed again. Everything this
 * returns therefore carries `complete`: false means the scan hit its cap and
 * there may be suppressed addresses it never saw. Callers must show that.
 */

import { marketingApi } from './api';
import { isSuspended } from './format';
import type { EmailContact } from './types';

/** How many contacts one scan will pull before it gives up. */
export const SCAN_LIMIT = 500;

export interface SuspensionScan {
  suspended: EmailContact[];
  /** Every contact the scan actually saw. */
  scanned: number;
  /** What the API says the tenant holds in total. */
  total: number;
  /** False when the scan stopped at the cap, so the counts are a floor. */
  complete: boolean;
}

export async function scanSuspended(): Promise<SuspensionScan> {
  const page = await marketingApi.listContacts(SCAN_LIMIT, 0);
  const suspended = page.items.filter((c) => isSuspended(c.status));
  return {
    suspended,
    scanned: page.items.length,
    total: page.total,
    complete: page.total <= page.items.length,
  };
}
