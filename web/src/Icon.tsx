// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Inline-SVG icons (task 38, spec 4.1). One set of named paths, drawn on a
 * 24-unit grid with a 1.8 stroke, so every icon in the app matches. No icon
 * package: STACK forbids one, and these paths are all the UI needs.
 */

export const ICONS = {
  dash: 'M3 10.5 12 3l9 7.5M5 9.5V21h5v-6h4v6h5V9.5',
  library:
    'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z',
  nodes: 'M4 5h16v6H4zM4 13h16v6H4zM7.5 8h.01M7.5 16h.01',
  encode: 'M4 6h16M4 12h16M4 18h16M9 4v4M15 10v4M11 16v4',
  app: 'M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6ZM12 4v2M12 18v2M4 12h2M18 12h2M6.3 6.3l1.4 1.4M16.3 17.7l1.4 1.4M17.7 6.3l-1.4 1.4M7.7 17.7l-1.4 1.4',
  // Two arrows in opposite directions: a sync or exchange with another system.
  integrations: 'M4 8h13l-3-3M20 16H7l3 3',
  menu: 'M4 6h16M4 12h16M4 18h16',
  close: 'M6 6l12 12M18 6 6 18',
  collapse: 'M11 17l-5-5 5-5M18 17l-5-5 5-5',
  expand: 'M13 17l5-5-5-5M6 17l5-5-5-5',
  check: 'M5 12.5 10 17.5 19 7',
  alert: 'M12 3 2.5 20h19L12 3ZM12 10v4M12 17.5v.01',
  info: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18ZM12 11v5M12 8v.01',
  clock: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18ZM12 7v5l3 2',
  pause: 'M9 5v14M15 5v14',
  play: 'M7 5v14l12-7Z',
  'chevron-right': 'M9 6l6 6-6 6',
  'chevron-down': 'M6 9l6 6 6-6',
  copy: 'M9 9h10v10H9zM5 15V5h10',
  folder:
    'M3 6.5A1.5 1.5 0 0 1 4.5 5H9l2 2.5h8.5A1.5 1.5 0 0 1 21 9v9.5a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 18.5Z',
  file: 'M7 3h7l5 5v13H7zM14 3v5h5',
  external: 'M14 4h6v6M20 4l-9 9M18 14v6H4V6h6',
  logout: 'M15 4h4v16h-4M10 8l-4 4 4 4M6 12h10',
  // The app mark: a square with an inward arrow.
  brand: 'M4 4h16v16H4zM12 7v7M8.5 10.5 12 14l3.5-3.5',
} as const

export type IconName = keyof typeof ICONS

export default function Icon({
  name,
  className = 'h-5 w-5 shrink-0',
}: {
  name: IconName
  className?: string
}) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden="true"
      data-icon={name}
    >
      <path d={ICONS[name]} />
    </svg>
  )
}
