'use client';

/**
 * Per-site protections.
 *
 * The four controls this tab is for - directory listing, PHP function
 * restrictions, upload filtering and hotlink protection - all live in the web
 * server vhost or the PHP pool configuration, and this panel writes neither
 * from a security endpoint. The website routes create and delete sites and
 * manage domains and SSL; the PHP routes carry a config object with memory
 * limits and OPcache settings and no disable_functions field at all.
 *
 * So this pane shows the sites it would apply to, and for each protection the
 * fact that the panel cannot read its state. It renders no switches: a switch
 * labelled "Directory listing: off" that never wrote an autoindex directive
 * would be a claim about a customer's site that the panel cannot support.
 */

import { BackendGap, Panel, PaneHeading, SectionHeader } from './chrome';

interface Protection {
  name: string;
  whatItDoes: string;
  whereItLives: string;
}

const PROTECTIONS: Protection[] = [
  {
    name: 'Directory listing',
    whatItDoes:
      'Stops the web server serving an index of a folder that has no index file, which is how backup archives and .env files get found.',
    whereItLives: 'autoindex off in the nginx server block, or Options -Indexes in Apache.',
  },
  {
    name: 'PHP function restrictions',
    whatItDoes:
      'Blocks the functions a web shell needs - exec, shell_exec, system, passthru, proc_open, popen - so an uploaded file cannot become a command line.',
    whereItLives: 'disable_functions in the php.ini or the pool configuration for that site.',
  },
  {
    name: 'Upload filtering',
    whatItDoes:
      'Refuses to execute anything under the upload directory, so a .php file that arrives through an image upload is served as bytes rather than run.',
    whereItLives:
      'A location block over the upload path that returns 403 for PHP, plus upload size and type limits.',
  },
  {
    name: 'Hotlink protection',
    whatItDoes:
      'Refuses requests for images and media whose Referer is another site, which stops a third party serving their traffic out of this customer’s bandwidth.',
    whereItLives: 'valid_referers plus a 403 on the media location block.',
  },
];

export function WebsiteSecurityPane() {
  return (
    <div className="space-y-4">
      <PaneHeading
        title="Website Security"
        description="Per-site protections written into the web server and PHP configuration. The panel cannot read or write any of them, so this section reports that rather than showing four switches that would save nothing."
      />

      <Panel>
        <SectionHeader
          title="Not available"
          description="What has to exist before these become per-site switches."
        />
        <BackendGap
          title="The panel cannot read or set any per-site protection"
          summary="The website routes manage sites, domains and certificates; the PHP config route carries limits and OPcache settings and has no disable_functions field. No route reads a vhost back, so the panel cannot report the state of a protection even where an operator has set it by hand."
          controls={[
            'Directory listing on or off, per site',
            'PHP function restrictions per site, with the web-shell set as a one-action preset',
            'Upload directory execution blocked, with the paths it covers',
            'Hotlink protection with the referer allow list and the file extensions it covers',
            'A per-site summary of which protections are on, read back from the live configuration',
          ]}
          endpoints={[
            'GET  /api/v1/websites/:id/security',
            'PUT  /api/v1/websites/:id/security',
            'GET  /api/v1/websites/:id/security/php-functions',
            'PUT  /api/v1/websites/:id/security/php-functions',
          ]}
          note="The read matters as much as the write. A PUT that sets autoindex off but no GET that confirms it leaves this screen guessing, and a guess here is a claim about whether a customer's files are exposed."
        />
      </Panel>

      <Panel>
        <SectionHeader
          title="The four protections, and where each one lives"
          description="Reference only. Set these by hand in the site configuration until the endpoints exist."
        />
        <ul className="divide-y divide-gray-100">
          {PROTECTIONS.map((protection) => (
            <li key={protection.name} className="px-4 py-3">
              <p className="text-sm font-semibold text-gray-900">{protection.name}</p>
              <p className="mt-1 text-sm text-gray-600">{protection.whatItDoes}</p>
              <p className="mt-1 text-sm text-gray-500">
                <span className="font-medium text-gray-600">Where it lives: </span>
                {protection.whereItLives}
              </p>
            </li>
          ))}
        </ul>
      </Panel>
    </div>
  );
}

export default WebsiteSecurityPane;
