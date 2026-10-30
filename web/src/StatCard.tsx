// SPDX-License-Identifier: AGPL-3.0-only

type Props = {
  testid: string
  label: string
  value: string | number
  /** An SVG path `d` string drawn on a 24x24 stroke grid. */
  icon: string
  tone?: 'default' | 'good'
}

/**
 * One dashboard statistic (task 21). The icon and the type scale with the
 * viewport via clamp(), so a 320 px phone and a 4K monitor both look
 * deliberate rather than zoomed. No icon package: the path is drawn inline.
 */
export default function StatCard({
  testid,
  label,
  value,
  icon,
  tone = 'default',
}: Props) {
  return (
    <div
      className="rounded-2xl bg-slate-900 p-3 ring-1 ring-slate-800 sm:p-4"
      data-testid={testid}
    >
      <div className="flex items-center gap-2">
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth={1.8}
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
          className="h-[clamp(16px,4vw,22px)] w-[clamp(16px,4vw,22px)] shrink-0 text-slate-500"
        >
          <path d={icon} />
        </svg>
        <p className="text-[clamp(10px,2.6vw,12px)] uppercase tracking-wide text-slate-500">
          {label}
        </p>
      </div>
      <p
        className={`mt-1 font-bold text-[clamp(18px,5.5vw,24px)] ${
          tone === 'good' ? 'text-emerald-400' : 'text-slate-100'
        }`}
      >
        {value}
      </p>
    </div>
  )
}
