/**
 * Nguon SU THAT DUY NHAT cho moi chuoi thuong hieu cua giao dien.
 *
 * Khong hardcode ten san pham / ten doanh nghiep o bat ky noi nao khac:
 * moi component can hien thi thuong hieu deu phai import tu tep nay.
 *
 *   import { productName, company, brand } from '@/lib/brand';
 */

/** Ten san pham hien thi cho nguoi dung cuoi. */
export const productName = 'VKAI Panel';

/** Ten doanh nghiep phat hanh san pham. */
export const company = 'HiTechCloud';

/** Trang chu doanh nghiep. */
export const companyUrl = 'https://hitechcloud.vn';

/** Hom thu ho tro ky thuat. */
export const supportEmail = 'support@hitechcloud.vn';

/** Tai lieu huong dan su dung. */
export const docsUrl = 'https://hitechcloud.vn/docs';

/** Phien ban giao dien - dong bo voi CHANGELOG.md va package.json. */
export const version = '0.2.1';

/** Thang navy - mau chinh, rut tu logo hitechcloud.vn. Trung voi Tailwind `brand-*`. */
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

/** Thang cyan - diem nhan phu. Trung voi Tailwind `accent-*`. */
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
 * Bang mau thuong hieu - nguon duy nhat cho code TypeScript can mau
 * (bieu do SVG, canvas, inline style). CSS/Tailwind dung `brand-*` / `accent-*`.
 */
export const colors = {
  navy: brandScale[600],
  cyan: accentScale[500],
  brandScale,
  accentScale,
} as const;

/**
 * Mau chuoi du lieu cho bieu do, theo dung thu tu quy uoc:
 * chuoi 1 navy, chuoi 2 cyan, chuoi 3 emerald-600, chuoi 4 amber-600.
 */
export const chartSeries = ['#0B398C', '#1791C8', '#059669', '#D97706'] as const;

/** Mau khung bieu do: luoi, truc va vien tooltip. */
export const chartAxis = {
  grid: '#E5E7EB',
  axis: '#6B7280',
  tooltipBg: '#FFFFFF',
  tooltipBorder: '#E5E7EB',
  tooltipRadius: 6,
} as const;

/** Phien ban dang hien thi, vi du "v0.2.1". */
export const versionLabel = `v${version}`;

/** Dong phu de duoi khoi thuong hieu, vi du "by HiTechCloud". */
export const byline = `by ${company}`;

/** Ten day du dung cho tieu de trang / metadata. */
export const fullName = `${productName} - ${company}`;

/** Mo ta ngan bang tieng Viet, dung cho metadata va man hinh dang nhap. */
export const description =
  'Bảng điều khiển quản trị máy chủ, website và hạ tầng hosting của HiTechCloud.';

/** Dong ban quyen, tu dong cap nhat nam. */
export function copyright(year: number = new Date().getFullYear()): string {
  return `© ${year} ${company}. Bảo lưu mọi quyền.`;
}

/** Toan bo thong tin thuong hieu gom trong mot object tien dung. */
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
