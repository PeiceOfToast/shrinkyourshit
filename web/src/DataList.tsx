// SPDX-License-Identifier: AGPL-3.0-only

/**
 * One data set, two layouts (task 20): a real <table> at `sm` and up, and
 * stacked cards below it. The table stays in the DOM at every width for
 * assistive tech; the card list carries the same information in 44px
 * rows, so nothing is squeezed into a 300px table.
 *
 * Columns can override the card rendering (`cardCell`, `cardLabel`,
 * `cardNoLabel`) and the card row test id (`cardRowTestId`) so migrating a
 * page never has to rename a `data-testid` that an existing test asserts.
 */
import type { ReactNode } from 'react'

export type Column<T> = {
  key: string
  header: string
  cell: (row: T) => ReactNode
  primary?: boolean
  /** Card-only content when it differs from the table cell. */
  cardCell?: (row: T) => ReactNode
  /** Card label, when the table header is not the right words. */
  cardLabel?: string
  /** Hide the label in the card and render the value alone. */
  cardNoLabel?: boolean
  /** Leave the column out of the cards (table-only detail). */
  hideInCard?: boolean | ((row: T) => boolean)
}

type Props<T> = {
  rows: T[]
  columns: Column<T>[]
  /** `data-testid` of the table; cards get `${testid}-cards`. */
  testid: string
  /** Cards' `data-testid`, when an existing name must be kept. */
  cardTestid?: string
  rowTestId?: (row: T) => string
  cardRowTestId?: (row: T) => string
}

function hiddenInCard<T>(column: Column<T>, row: T): boolean {
  if (typeof column.hideInCard === 'function') return column.hideInCard(row)
  return column.hideInCard === true
}

export default function DataList<T extends { id: string }>({
  rows,
  columns,
  testid,
  cardTestid,
  rowTestId,
  cardRowTestId,
}: Props<T>) {
  const primary = columns.find((c) => c.primary) ?? columns[0]
  const rest = columns.filter((c) => c !== primary)

  return (
    <div>
      <ul
        className="space-y-2 sm:hidden"
        data-testid={cardTestid ?? `${testid}-cards`}
      >
        {rows.map((row) => (
          <li
            key={row.id}
            className="min-w-0 rounded-2xl bg-slate-900 p-4 ring-1 ring-slate-800"
            data-testid={cardRowTestId?.(row)}
          >
            <div className="text-sm font-semibold text-slate-100">
              {primary.cardCell ? primary.cardCell(row) : primary.cell(row)}
            </div>
            <dl className="mt-2 space-y-1 text-xs text-slate-400">
              {rest
                .filter((c) => !hiddenInCard(c, row))
                .map((c) => (
                  <div key={c.key} className="flex justify-between gap-3">
                    {c.cardNoLabel ? (
                      <dd className="min-w-0 text-slate-200">
                        {c.cardCell ? c.cardCell(row) : c.cell(row)}
                      </dd>
                    ) : (
                      <>
                        <dt className="shrink-0">{c.cardLabel ?? c.header}</dt>
                        <dd className="min-w-0 text-right text-slate-200">
                          {c.cardCell ? c.cardCell(row) : c.cell(row)}
                        </dd>
                      </>
                    )}
                  </div>
                ))}
            </dl>
          </li>
        ))}
      </ul>

      <div className="relative hidden overflow-x-auto rounded-2xl ring-1 ring-slate-800 sm:block">
        <table className="w-full text-left text-sm" data-testid={testid}>
          <thead className="bg-slate-900 text-xs uppercase tracking-wide text-slate-500">
            <tr>
              {columns.map((c) => (
                <th key={c.key} className="px-4 py-3 font-medium">
                  {c.header || <span className="sr-only">Actions</span>}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800 bg-slate-900/50">
            {rows.map((row) => (
              <tr key={row.id} data-testid={rowTestId?.(row)}>
                {columns.map((c) => (
                  <td key={c.key} className="px-4 py-2">
                    {c.cell(row)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
