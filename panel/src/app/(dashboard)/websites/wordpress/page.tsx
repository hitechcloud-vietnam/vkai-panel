import { redirect } from 'next/navigation';

/**
 * WordPress moved to its own section.
 *
 * aaPanel presents WP Toolkit as a top-level area rather than a page under
 * Websites, and everything this page did is covered there - including the
 * plugin controls, which are deliberately not offered any more: the endpoint
 * behind the old "Install Plugin" button writes a database row and installs
 * nothing.
 *
 * This file stays as a redirect so a bookmarked URL keeps working.
 */
export default function WordPressRedirect() {
  redirect('/wp-toolkit');
}
