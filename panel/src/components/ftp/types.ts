/**
 * The FTP screen's vocabulary, and the record of what the backend can and
 * cannot do today.
 *
 * Everything in this file was read out of the Go source, not assumed:
 *
 *   core/internal/handler/router.go          the complete route table
 *   core/internal/handler/service.go         the systemd handlers
 *   core/internal/service/service_manager.go alwaysManageable, the unit allow list
 *   core/internal/handler/firewall.go        List and GetActiveRules
 *   core/internal/models/models.go           every persisted model
 *
 * The finding that shapes this screen: there is no FTP anything in the API.
 * No route matches /ftp, no handler file mentions it, and no model stores an
 * account. `grep -ril ftp core/internal` returns four test files, one comment
 * about sftp backup destinations, and one line of service_manager.go - the
 * allow list that lets the panel start and stop `vsftpd`, `proftpd` and
 * `pure-ftpd` as systemd units.
 *
 * So this screen drives the FTP *daemon*, which is real, and refuses to draw
 * an account table, a create form or a password field, which would all be
 * wired to nothing. This panel has already shipped that defect three times -
 * two-factor routes that were never mounted, a settings page calling four
 * endpoints that all answered 404, an agent channel that was two TODO stubs -
 * and each time an operator found it in production at the moment they needed
 * it.
 */

/** The panes across the top of the screen. The value lives in `?tab=`. */
export type FtpTabId = 'accounts' | 'service';

export const FTP_TABS: { id: FtpTabId; label: string; blurb: string }[] = [
  {
    id: 'accounts',
    label: 'FTP accounts',
    blurb:
      'Accounts, the directory each one is confined to, quota and status. Not available yet - the API has no FTP endpoints.',
  },
  {
    id: 'service',
    label: 'Service and ports',
    blurb:
      'The FTP daemon on this machine, the ports a client needs through the firewall, and the TLS setting.',
  },
];

export function isFtpTabId(value: string | null | undefined): value is FtpTabId {
  return value === 'accounts' || value === 'service';
}

/** One capability this screen would offer, and the precise reason it cannot. */
export interface CapabilityGap {
  /** What an operator would call it. */
  label: string;
  /** What is missing, named precisely enough to become a backlog item. */
  missing: string;
}

/**
 * The systemd units the panel is allowed to control.
 *
 * service_manager.go `alwaysManageable` is the authority: any other unit name
 * is refused by checkServiceName with `service %q is not managed by this
 * panel`, so a control offered for anything else would return a 500. The
 * daemon is detected by matching a unit from GET /api/v1/services against this
 * list; nothing else on the machine gets a start or stop button from here.
 */
export const MANAGEABLE_FTP_UNITS = ['vsftpd', 'proftpd', 'pure-ftpd'] as const;

export type FtpUnitName = (typeof MANAGEABLE_FTP_UNITS)[number];

export function isManageableFtpUnit(name: string): name is FtpUnitName {
  return (MANAGEABLE_FTP_UNITS as readonly string[]).includes(name);
}

/** How each daemon is spelled for a human, and where its config lives. */
export const FTP_DAEMONS: Record<FtpUnitName, { label: string; configPath: string }> = {
  vsftpd: { label: 'vsftpd', configPath: '/etc/vsftpd.conf' },
  proftpd: { label: 'ProFTPD', configPath: '/etc/proftpd/proftpd.conf' },
  'pure-ftpd': { label: 'Pure-FTPd', configPath: '/etc/pure-ftpd/pure-ftpd.conf' },
};

/** One systemd unit, as GET /api/v1/services returns it. */
export interface ServiceInfo {
  name: string;
  status?: string;
  active_state?: string;
  sub_state?: string;
  description?: string;
  pid?: number;
  memory?: number;
}

/** One firewall rule row, as GET /api/v1/firewall returns it. */
export interface FirewallRule {
  id: string;
  protocol: string;
  port: string;
  source: string;
  action: string;
  direction: string;
  status: string;
}

/** One website, as GET /api/v1/websites returns it (paginated). */
export interface WebsiteRow {
  id: string;
  domain: string;
  root_dir: string;
  status: string;
  site_type?: string;
}

/** The well known FTP ports, and why each one matters through a firewall. */
export const FTP_PORTS: { port: number; label: string; note: string }[] = [
  {
    port: 21,
    label: 'Control (FTP / explicit FTPS)',
    note: 'Every session opens here. Closed, nothing connects at all.',
  },
  {
    port: 990,
    label: 'Control (implicit FTPS)',
    note: 'Only needed when clients are configured for implicit TLS rather than AUTH TLS on 21.',
  },
];

/**
 * What the backend would have to grow before this screen can manage accounts.
 * Rendered on the page, so the list an operator reads and the list the backend
 * team works from are the same list.
 */
export const ACCOUNT_GAPS: CapabilityGap[] = [
  {
    label: 'List accounts',
    missing:
      'No GET /api/v1/ftp. No handler mentions FTP and no model stores an account, so there is nothing to list.',
  },
  {
    label: 'Create an account',
    missing:
      'No POST /api/v1/ftp. Needs a system user or a virtual-user database entry, a home directory and a quota.',
  },
  {
    label: 'Change a password',
    missing: 'No POST /api/v1/ftp/:id/change-password.',
  },
  {
    label: 'Change the home directory',
    missing:
      'No PUT /api/v1/ftp/:id. This is the confinement, so the endpoint must resolve symlinks and refuse a path outside the site tree server-side - a check in this browser is not a security control.',
  },
  {
    label: 'Enable or disable an account',
    missing: 'No POST /api/v1/ftp/:id/enable or /disable.',
  },
  {
    label: 'Delete an account',
    missing: 'No DELETE /api/v1/ftp/:id.',
  },
  {
    label: 'Quota and current use',
    missing:
      'Nothing reports either. GET /api/v1/files/disk-usage exists but answers for a path, not for an FTP account.',
  },
  {
    label: 'Password for a customer',
    missing:
      'No endpoint ever returns or generates an FTP password, so there is no secret for this screen to reveal.',
  },
];

/** The same list, for the daemon configuration this screen can only read around. */
export const CONFIG_GAPS: CapabilityGap[] = [
  {
    label: 'Passive port range',
    missing:
      'No endpoint reads or writes the daemon config. The range below is inferred from the panel firewall rules, which is what the firewall permits - not necessarily what the daemon is configured to use.',
  },
  {
    label: 'TLS / FTPS setting',
    missing:
      'Same gap. Whether the daemon requires AUTH TLS, and which certificate it presents, is in its config file and no route exposes it.',
  },
  {
    label: 'Masquerade address',
    missing:
      'No endpoint reports it. A daemon behind NAT that advertises a private address in PASV fails for every external client, and the panel cannot currently see that setting.',
  },
  {
    label: 'Install the daemon',
    missing:
      'No package endpoint exists. If no FTP unit is present on the machine it has to be installed over SSH.',
  },
];
