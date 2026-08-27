/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './src/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {
      colors: {
        // Thang "brand" - navy rut tu logo hitechcloud.vn (#0B398C). Mau chinh.
        brand: {
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
        },
        // Thang "accent" - cyan rut tu logo (#1791C8). Diem nhan phu, chuoi du
        // lieu thu hai trong bieu do, huy hieu thong tin. KHONG dung lam nen
        // nut chinh (tuong phan voi chu trang khong dat 4.5:1).
        accent: {
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
        },
        // Luoi an toan cho class cu: "primary" duoc map TRUNG voi "brand" nen
        // moi class primary-* con sot lai van hien thi dung mau thuong hieu.
        primary: {
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
        },
        // Thang "dark" duoc map NGUOC ve xam sang: neu con sot class cu
        // (bg-dark-900, text-dark-300...) giao dien van hien thi dung tone light.
        dark: {
          50: '#111827',  // text-dark-50  -> chu dam
          100: '#111827', // text-dark-100 -> chu dam
          200: '#1f2937',
          300: '#4b5563', // text-dark-300 -> chu phu
          400: '#6b7280', // text-dark-400 -> chu phu
          500: '#9ca3af', // text-dark-500 -> chu mo
          600: '#e5e7eb', // border-dark-600 -> vien xam
          700: '#e5e7eb', // border-dark-700 -> vien xam
          800: '#ffffff', // bg-dark-800 -> be mat trang
          900: '#ffffff', // bg-dark-900 -> be mat trang
          950: '#f9fafb', // bg-dark-950 -> canvas nen trang
        },
        // Token ngu nghia
        canvas: '#F7F8FA',
        surface: '#FFFFFF',
        line: '#E5E7EB',
        ink: {
          DEFAULT: '#111827',
          muted: '#6B7280',
        },
      },
      fontFamily: {
        sans: ['var(--font-inter)', 'Inter', 'system-ui', '-apple-system', 'Segoe UI', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'ui-monospace', 'SFMono-Regular', 'monospace'],
      },
      boxShadow: {
        sm: '0 1px 2px 0 rgb(16 24 40 / 0.05)',
        DEFAULT: '0 1px 3px 0 rgb(16 24 40 / 0.08), 0 1px 2px -1px rgb(16 24 40 / 0.06)',
        lg: '0 10px 15px -3px rgb(16 24 40 / 0.08), 0 4px 6px -4px rgb(16 24 40 / 0.05)',
      },
      borderRadius: {
        md: '6px',
        lg: '8px',
      },
    },
  },
  plugins: [],
};
