// SPDX-License-Identifier: AGPL-3.0-only
import defaultTheme from 'tailwindcss/defaultTheme'

// Colour tokens are CSS variables holding RGB channels (index.css, :root), so
// Tailwind's /<alpha> modifiers work. Spec: docs/design/ui-redesign-2026-11-04.md 2.9.
const c = (name) => `rgb(var(--c-${name}) / <alpha-value>)`

/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        canvas: c('canvas'),
        surface: c('surface'),
        raised: { DEFAULT: c('raised'), hover: c('raised-hover') },
        line: { DEFAULT: c('line'), strong: c('line-strong') },
        fg: { DEFAULT: c('fg'), muted: c('fg-muted'), subtle: c('fg-subtle') },
        accent: {
          DEFAULT: c('accent'),
          hover: c('accent-hover'),
          fg: c('accent-fg'),
          text: c('accent-text'),
        },
        focus: c('focus'),
        success: c('success'),
        info: c('info'),
        warning: c('warning'),
        danger: {
          DEFAULT: c('danger'),
          solid: c('danger-solid'),
          'solid-hover': c('danger-solid-hover'),
        },
      },
      fontFamily: {
        sans: ['"Inter Variable"', ...defaultTheme.fontFamily.sans],
      },
      fontSize: {
        caption: ['0.75rem', { lineHeight: '1rem' }],
        body: ['0.875rem', { lineHeight: '1.25rem' }],
        heading: ['1rem', { lineHeight: '1.5rem', fontWeight: '600' }],
        title: [
          '1.5rem',
          { lineHeight: '2rem', letterSpacing: '-0.01em', fontWeight: '600' },
        ],
      },
      borderRadius: { control: '0.5rem', card: '0.75rem', overlay: '1rem' },
      boxShadow: { overlay: '0 16px 48px -12px rgb(0 0 0 / 0.7)' },
      transitionDuration: { fast: '120ms', base: '200ms', slow: '300ms' },
      transitionTimingFunction: { standard: 'cubic-bezier(0.2, 0, 0, 1)' },
      zIndex: { rail: '30', topbar: '40', toast: '70' },
    },
  },
  plugins: [],
}
