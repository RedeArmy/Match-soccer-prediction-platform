import type { Config } from 'tailwindcss'
import animate from 'tailwindcss-animate'

const config: Config = {
  darkMode: ['class'],
  content: [
    './src/pages/**/*.{js,ts,jsx,tsx,mdx}',
    './src/components/**/*.{js,ts,jsx,tsx,mdx}',
    './src/app/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {
      fontFamily: {
        display: ['var(--font-bebas)', 'Impact', 'sans-serif'],
        sans:    ['var(--font-inter)', 'system-ui', 'sans-serif'],
        mono:    ['var(--font-jetbrains)', 'ui-monospace', 'monospace'],
      },
      colors: {
        // Deep Americas Blue
        blue: {
          950: '#050D1A',
          900: '#0A1628',
          800: '#0D1F3C',
          700: '#112850',
          600: '#1A3A6B',
          500: '#1E4D8C',
          400: '#2E6DB4',
          300: '#4A8FD4',
          200: '#89BCE8',
          100: '#D0E8F7',
        },
        // Eagle Gold
        gold: {
          600: '#A07810',
          500: '#C9940A',
          400: '#D4A017',
          300: '#E8C547',
          200: '#F5E098',
          100: '#FDF6DC',
        },
        // México Verde
        green: {
          700: '#0D5C2E',
          600: '#117A3D',
          500: '#1A8C4E',
          400: '#27AE60',
          300: '#52C97D',
          100: '#D4F5E3',
        },
        // Danger / Canada Red
        red: {
          700: '#8B1A1A',
          600: '#B22222',
          500: '#CC2222',
          400: '#E53535',
          300: '#F08080',
          100: '#FDEAEA',
        },
        // Maple
        maple: {
          500: '#D44500',
          400: '#E85D1A',
          300: '#F5935A',
        },
        // Neutral
        neutral: {
          950: '#080808',
          900: '#111111',
          850: '#161616',
          800: '#1C1C1C',
          700: '#2A2A2A',
          600: '#3D3D3D',
          500: '#6B6B6B',
          400: '#9A9A9A',
          300: '#C5C5C5',
          200: '#E2E2E2',
          100: '#F5F5F5',
          50:  '#FAFAFA',
        },
      },
      borderRadius: {
        lg: '8px',
        md: '6px',
        sm: '4px',
      },
      backgroundImage: {
        'hero-gradient': 'linear-gradient(135deg, #050D1A 0%, #0D1F3C 50%, #1A3A6B 100%)',
        'gold-gradient':  'linear-gradient(90deg, #A07810, #E8C547)',
        'card-gradient':  'linear-gradient(180deg, #0D1F3C 0%, #0A1628 100%)',
      },
      keyframes: {
        'accordion-down': {
          from: { height: '0' },
          to:   { height: 'var(--radix-accordion-content-height)' },
        },
        'accordion-up': {
          from: { height: 'var(--radix-accordion-content-height)' },
          to:   { height: '0' },
        },
        'number-tick': {
          '0%':   { transform: 'translateY(0)' },
          '50%':  { transform: 'translateY(-4px)' },
          '100%': { transform: 'translateY(0)' },
        },
        'pulse-gold': {
          '0%, 100%': { opacity: '1' },
          '50%':      { opacity: '0.6' },
        },
      },
      animation: {
        'accordion-down': 'accordion-down 0.2s ease-out',
        'accordion-up':   'accordion-up 0.2s ease-out',
        'number-tick':    'number-tick 0.3s ease-in-out',
        'pulse-gold':     'pulse-gold 2s ease-in-out infinite',
      },
    },
  },
  plugins: [animate],
}

export default config
