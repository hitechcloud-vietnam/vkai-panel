/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './src/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {
      colors: {
        // Accent thuong hieu - thang blue chuan (giu khoa "primary" de khong vo code cu)
        primary: {
          50: '#eff6ff',
          100: '#dbeafe',
          200: '#bfdbfe',
          300: '#93c5fd',
          400: '#60a5fa',
          500: '#3b82f6',
          600: '#2563eb',
          700: '#1d4ed8',
          800: '#1e40af',
          900: '#1e3a8a',
          950: '#172554',
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
