'use client';

/**
 * WP Sets - a saved set of plugins, themes and settings applied to a new
 * installation.
 *
 * Nothing behind it, and the gap is two-deep. There is no endpoint that stores
 * a set, and there is also no endpoint that could apply one: the mounted
 * POST /api/v1/wordpress/:id/plugins writes a database row and installs
 * nothing, and wpcli.Client.InstallPlugin - which does the real work - is not
 * reachable over HTTP from anywhere.
 *
 * Storing sets in the browser was considered and rejected. A set that lives in
 * one operator's localStorage, cannot be shared with the colleague who deploys
 * the twentieth site, and cannot be applied to anything, is a worse answer than
 * an empty screen, because it looks like the feature.
 */

import { Card, CardBody, CardHeader, NotBuiltYet, PageHeader } from './ui';

export function WpSets() {
  return (
    <div className="space-y-5">
      <PageHeader
        title="WP Sets"
        description="Saved sets of plugins, themes and settings, applied to a new installation in one step."
      />

      <NotBuiltYet
        what="Saved installation sets"
        because={
          <>
            Two things are missing, not one. Nothing stores a set, and nothing can apply one either:
            the mounted plugin and theme routes write rows in the panel database without touching the
            installation, and the WP-CLI functions that really install a plugin or a theme
            (wpcli.Client.InstallPlugin and InstallTheme) have no route in front of them.
          </>
        }
        endpoints={[
          {
            endpoint: 'POST /api/v1/wordpress/sets  |  GET /api/v1/wordpress/sets',
            purpose:
              'Create and list sets for the tenant. A set is a name plus a list of plugin slugs with optional versions and an activate flag, a theme slug, and the WordPress options to write (permalink structure, timezone, blog_public, discussion defaults).',
          },
          {
            endpoint: 'GET, PUT, DELETE /api/v1/wordpress/sets/:id',
            purpose: 'Read, edit and remove one set.',
          },
          {
            endpoint: 'POST /api/v1/wordpress/sets/:id/apply',
            purpose:
              'Apply a set to an installation, reporting per item whether it succeeded, so one plugin that no longer exists on wordpress.org does not silently take the other nineteen with it.',
          },
          {
            endpoint: 'POST /api/v1/wordpress/sets/:id/upload',
            purpose:
              'Add a plugin or theme archive that is not on wordpress.org. Every agency has two or three of those - a licensed page builder, an in-house plugin - and a set that cannot hold them is a set nobody uses.',
          },
          {
            endpoint: 'POST /api/v1/wordpress/sets/from-site/:id',
            purpose:
              'Build a set from an installation that is already configured the way the agency wants. This is how a set gets created in practice; typing twenty slugs by hand is the thing the feature exists to avoid.',
          },
          {
            endpoint: 'POST /api/v1/wordpress/:id/plugins/install and /themes/install',
            purpose:
              'The routes a set would be built on: install a plugin or theme for real through WP-CLI, rather than recording that somebody meant to. wpcli.Client already implements both; nothing mounts them.',
          },
        ]}
        footnote={
          <>
            Applying a set to a fresh installation would also want to run inside the Add WordPress
            flow, so the twentieth site is one form rather than two.
          </>
        }
      />

      <Card>
        <CardHeader title="What a set would contain" />
        <CardBody>
          <ul className="list-disc space-y-2 pl-5 text-sm text-gray-600">
            <li>
              <span className="font-medium text-gray-900">Plugins</span> - slugs with an optional
              pinned version and whether to activate on install. Pinning matters: an agency standard
              that silently tracks latest is not a standard.
            </li>
            <li>
              <span className="font-medium text-gray-900">Theme</span> - the slug to install and
              activate, and any child theme to leave in place.
            </li>
            <li>
              <span className="font-medium text-gray-900">Settings</span> - the WordPress options
              worth setting on every site: permalink structure, timezone, date format, whether search
              engines may index it, comment defaults, and the default user role.
            </li>
            <li>
              <span className="font-medium text-gray-900">Cleanup</span> - which of the bundled
              plugins and themes to delete, which is half of why the twenty steps exist.
            </li>
          </ul>
        </CardBody>
      </Card>
    </div>
  );
}

export default WpSets;
