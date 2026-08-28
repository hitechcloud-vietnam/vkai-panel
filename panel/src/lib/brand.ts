/**
 * The SINGLE SOURCE OF TRUTH for every brand string in the UI.
 *
 * Never hardcode the product name or the company name anywhere else: any
 * component that shows the brand imports it from this file.
 *
 *   import { productName, company, brand } from '@/lib/brand';
 *
 * The version is the one exception to "this file owns it": the product version
 * lives in the file VERSION at the repository root, shared with the Go binaries,
 * the installer and the release workflow. The browser cannot read a file at
 * runtime, so panel/scripts/gen-version.js compiles it into
 * ./version.generated.ts before every build, dev server and type-check. That
 * file is generated, not committed. If it is missing the build stops at
 * "Cannot find module './version.generated'" rather than shipping a wrong
 * version.
 */

import { generatedVersion } from './version.generated';

/** The product name shown to end users. */
export const productName = 'VKAI Panel';

/** The company that publishes the product. */
export const company = 'HiTechCloud';

/** The company home page. */
export const companyUrl = 'https://hitechcloud.vn';

/** Technical support mailbox. */
export const supportEmail = 'support@hitechcloud.vn';

/** User documentation. */
export const docsUrl = 'https://hitechcloud.vn/docs';

/**
 * The subset of SemVer 2.0.0 the panel releases use. Kept deliberately strict:
 * a version that does not match this is a build accident, not a release.
 */
const SEMVER =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

/**
 * Guards against the generated file existing but holding nonsense - an empty
 * string, a placeholder, a half-written file. Shipping a UI that reports the
 * wrong version to an operator deciding whether to upgrade is worse than not
 * building at all, so this throws at import time and the build fails.
 */
function requireVersion(value: string): string {
  if (!SEMVER.test(value)) {
    throw new Error(
      `VKAI Panel: src/lib/version.generated.ts holds ${JSON.stringify(value)}, ` +
        'which is not a semantic version. Regenerate it from the repository ' +
        'VERSION file with "make sync-version" (or "npm run gen:version").',
    );
  }
  return value;
}

/** Product version, taken from the repository VERSION file at build time. */
export const version = requireVersion(generatedVersion);

/** Navy scale - the primary colour, taken from the hitechcloud.vn logo. Matches Tailwind `brand-*`. */
export const brandScale = {
  50: '#EFF4FC',
  100: '#D9E4F7',
  200: '#B3C8EF',
  300: '#7FA3E0',
  400: '#4A78CC',
  500: '#1F53B0',
  600: '#0B398C',
  700: '#092E70',
  800: '#072454',
  900: '#051A3D',
  950: '#03102A',
} as const;

/** Cyan scale - the secondary accent. Matches Tailwind `accent-*`. */
export const accentScale = {
  50: '#ECF7FC',
  100: '#D2ECF7',
  200: '#A5D8EF',
  300: '#6FC0E4',
  400: '#3BA6D6',
  500: '#1791C8',
  600: '#1277A5',
  700: '#0E5D82',
  800: '#0A4360',
  900: '#07303F',
} as const;

/**
 * The brand palette - the only source for TypeScript code that needs a colour
 * (SVG charts, canvas, inline styles). CSS and Tailwind use `brand-*` /
 * `accent-*` instead.
 */
export const colors = {
  navy: brandScale[600],
  cyan: accentScale[500],
  brandScale,
  accentScale,
} as const;

/**
 * Chart series colours, in the agreed order: series 1 navy, series 2 cyan,
 * series 3 emerald-600, series 4 amber-600.
 */
export const chartSeries = ['#0B398C', '#1791C8', '#059669', '#D97706'] as const;

/** Chart frame colours: grid, axes and tooltip border. */
export const chartAxis = {
  grid: '#E5E7EB',
  axis: '#6B7280',
  tooltipBg: '#FFFFFF',
  tooltipBorder: '#E5E7EB',
  tooltipRadius: 6,
} as const;

/** The version as displayed, for example "v0.3.0". */
export const versionLabel = `v${version}`;

/** The line under the brand block, for example "by HiTechCloud". */
export const byline = `by ${company}`;

/** The full name used for page titles and metadata. */
export const fullName = `${productName} - ${company}`;

/** Short description used for metadata and the sign-in screen. */
export const description =
  'Control panel for HiTechCloud servers, websites and hosting infrastructure.';

/** Copyright line, with the year filled in automatically. */
export function copyright(year: number = new Date().getFullYear()): string {
  return `© ${year} ${company}. All rights reserved.`;
}

/** Every brand fact in one convenient object. */
export const brand = {
  productName,
  company,
  companyUrl,
  supportEmail,
  docsUrl,
  version,
  colors,
} as const;

export default brand;
