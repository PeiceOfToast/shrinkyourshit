// SPDX-License-Identifier: AGPL-3.0-only

/**
 * One data set, two layouts (task 20, restyled and extended in task 39):
 * a real <table> from a breakpoint up, and stacked cards below it. The
 * table stays in the DOM at every width for assistive tech; CSS alone
 * decides which layout shows.
 *
 * Columns can override the card rendering (`cardCell`, `cardLabel`,
 * `cardNoLabel`) and the card row test id (`cardRowTestId`) so migrating a
 * page never has to rename a `data-testid` that an existing test asserts.
 *
 * Task 39 adds (spec 3.5):
 * - `cardsBelow`: the breakpoint where the table takes over (default `sm`;
 *   the file list uses `md`, since eight columns do not fit 640 to 767 px).
 * - `rowHref`: the whole row is a link. The primary cell's link is stretched
 *   over the row with `after:inset-0`. Columns marked `actions` sit above
 *   that overlay (`relative z-10`), so their buttons still work.
 * - `empty`: shown in place of both layouts when there are no rows.
 * - column `align: 'end'` (numbers), `cardPlacement` ('badge' top-right,
 *   'footer' under the card), and `actions` (last, right-aligned, footer
 *   in cards).
 */
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { card } from './uiControls'

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
  /** `end` right-aligns the column and sets its numbers in tabular figures. */
  align?: 'start' | 'end'
  /** Where the card shows the column: in the body list (default), top-right as a badge, or in the card footer. */
  cardPlacement?: 'body' | 'badge' | 'footer'
  /** An action column: last, right-aligned, and in the card footer. Its controls stay clickable on a row link. */
  actions?: boolean
}

export type CardsBelow = 'sm' | 'md' | 'lg'

type Props<T> = {
  rows: T[]
  columns: Column<T>[]
  /** `data-testid` of the table; cards get `${testid}-cards`. */
  testid: string
  /** Cards' `data-testid`, when an existing name must be kept. */
  cardTestid?: string
  rowTestId?: (row: T) => string
  cardRowTestId?: (row: T) => string
  /** The breakpoint from which the table shows. Below it, cards. */
  cardsBelow?: CardsBelow
  /** Makes each row a link to this route. */
  rowHref?: (row: T) => string
  /** Rendered in place of the table and the cards when there are no rows. */
  empty?: ReactNode
}

// Class strings are spelled out so Tailwind keeps every breakpoint variant.
const CARDS_SHOW: Record<CardsBelow, string> = {
  sm: 'sm:hidden',
  md: 'md:hidden',
  lg: 'lg:hidden',
}
const TABLE_SHOW: Record<CardsBelow, string> = {
  sm: 'hidden sm:block',
  md: 'hidden md:block',
  lg: 'hidden lg:block',
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
  cardsBelow = 'sm',
  rowHref,
  empty,
}: Props<T>) {
  if (rows.length === 0 && empty !== undefined) return <>{empty}</>

  const primary = columns.find((c) => c.primary) ?? columns[0]
  const rest = columns.filter((c) => c !== primary)
  const badgeColumn = rest.find((c) => c.cardPlacement === 'badge')
  const footerColumns = rest.filter(
    (c) => c.cardPlacement === 'footer' || c.actions,
  )
  const bodyColumns = rest.filter(
    (c) => c !== badgeColumn && !footerColumns.includes(c),
  )

  /** The primary cell, as a stretched link when the row is a link. */
  const primaryContent = (row: T, content: ReactNode) =>
    rowHref ? (
      <Link
        to={rowHref(row)}
        className="flex min-h-11 flex-col justify-center font-semibold text-fg after:absolute after:inset-0 focus-visible:outline-none"
      >
        {content}
      </Link>
    ) : (
      content
    )

  /** An action cell, lifted above the row link so its controls stay usable. */
  const actionWrap = (content: ReactNode) =>
    rowHref ? (
      <div className="relative z-10 flex flex-wrap justify-end gap-2">
        {content}
      </div>
    ) : (
      content
    )

  return (
    <div>
      <ul
        className={`flex flex-col gap-3 ${CARDS_SHOW[cardsBelow]}`}
        data-testid={cardTestid ?? `${testid}-cards`}
      >
        {rows.map((row) => {
          const footer = footerColumns.filter((c) => !hiddenInCard(c, row))
          return (
            <li
              key={row.id}
              className={`${card} relative min-w-0 p-4`}
              data-testid={cardRowTestId?.(row)}
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 text-body font-semibold text-fg">
                  {primaryContent(
                    row,
                    primary.cardCell
                      ? primary.cardCell(row)
                      : primary.cell(row),
                  )}
                </div>
                {badgeColumn && !hiddenInCard(badgeColumn, row) && (
                  <div className="shrink-0">
                    {badgeColumn.cardCell
                      ? badgeColumn.cardCell(row)
                      : badgeColumn.cell(row)}
                  </div>
                )}
              </div>
              {bodyColumns.length > 0 && (
                <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-body">
                  {bodyColumns
                    .filter((c) => !hiddenInCard(c, row))
                    .map((c) =>
                      c.cardNoLabel ? (
                        <dd key={c.key} className="col-span-2 min-w-0 text-fg">
                          {c.cardCell ? c.cardCell(row) : c.cell(row)}
                        </dd>
                      ) : (
                        <div key={c.key} className="contents">
                          <dt className="text-fg-muted">
                            {c.cardLabel ?? c.header}
                          </dt>
                          <dd className="min-w-0 text-right text-fg tabular-nums">
                            {c.cardCell ? c.cardCell(row) : c.cell(row)}
                          </dd>
                        </div>
                      ),
                    )}
                </dl>
              )}
              {footer.length > 0 && (
                <div
                  className={`mt-3 flex flex-wrap gap-2 border-t border-line pt-3 ${
                    rowHref ? 'relative z-10' : ''
                  }`}
                >
                  {footer.map((c) => (
                    <div key={c.key} className="contents">
                      {c.cardCell ? c.cardCell(row) : c.cell(row)}
                    </div>
                  ))}
                </div>
              )}
            </li>
          )
        })}
      </ul>

      <div
        className={`relative overflow-x-auto ${card} ${TABLE_SHOW[cardsBelow]}`}
      >
        <table className="w-full text-left text-body" data-testid={testid}>
          <thead className="bg-surface">
            <tr>
              {columns.map((c) => (
                <th
                  key={c.key}
                  className={`px-4 py-3 text-caption font-medium uppercase tracking-wider text-fg-muted ${
                    c.actions
                      ? 'w-px whitespace-nowrap text-right'
                      : c.align === 'end'
                        ? 'text-right'
                        : ''
                  }`}
                >
                  {c.header || <span className="sr-only">Actions</span>}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {rows.map((row) => (
              <tr
                key={row.id}
                className={rowHref ? 'relative hover:bg-raised/40' : undefined}
                data-testid={rowTestId?.(row)}
              >
                {columns.map((c) => {
                  const content = c.cell(row)
                  const isPrimary = c === primary
                  return (
                    <td
                      key={c.key}
                      className={`px-4 py-3 align-middle ${
                        c.actions
                          ? 'w-px whitespace-nowrap text-right'
                          : c.align === 'end'
                            ? 'text-right tabular-nums'
                            : ''
                      }`}
                    >
                      {isPrimary
                        ? primaryContent(row, content)
                        : c.actions
                          ? actionWrap(content)
                          : content}
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
