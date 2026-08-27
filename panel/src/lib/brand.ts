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
export const company = 'HiTech Cloud';

/** Trang chu doanh nghiep. */
export const companyUrl = 'https://hitechcloud.vn';

/** Hom thu ho tro ky thuat. */
export const supportEmail = 'support@hitechcloud.vn';

/** Tai lieu huong dan su dung. */
export const docsUrl = 'https://hitechcloud.vn/docs';

/** Phien ban giao dien - dong bo voi CHANGELOG.md va package.json. */
export const version = '0.2.1';

/** Bang mau thuong hieu. */
export const colors = {
  navy: '#0B398C',
  cyan: '#1791C8',
} as const;

/** Phien ban dang hien thi, vi du "v0.2.1". */
export const versionLabel = `v${version}`;

/** Dong phu de duoi khoi thuong hieu, vi du "by HiTech Cloud". */
export const byline = `by ${company}`;

/** Ten day du dung cho tieu de trang / metadata. */
export const fullName = `${productName} - ${company}`;

/** Mo ta ngan bang tieng Viet, dung cho metadata va man hinh dang nhap. */
export const description =
  'Bảng điều khiển quản trị máy chủ, website và hạ tầng hosting của HiTech Cloud.';

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
