/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        background: '#090D16',
        surface: '#0F172A',
        surfaceBorder: '#1E293B',
        accentCyan: '#06B6D4',
        accentBlue: '#3B82F6',
        accentPurple: '#8B5CF6',
        riskLow: '#10B981',
        riskMed: '#F59E0B',
        riskHigh: '#EF4444',
        riskCritical: '#DC2626',
      },
      fontFamily: {
        sans: ['Inter', 'Outfit', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },
    },
  },
  plugins: [],
};
