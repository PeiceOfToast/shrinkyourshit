// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Forms (task 38, spec 3.6): `FormSection` puts the title and description on
 * the left and the fields in a Card on the right (stacked below `lg`). `Field`
 * is one labelled control: it wires `aria-describedby` and `aria-invalid` onto
 * the control, so screen readers read the help and the error with it.
 * `FormActions` is the one action bar after the last section.
 */
import {
  Children,
  cloneElement,
  isValidElement,
  type ReactElement,
  type ReactNode,
} from 'react'
import Card from './Card'

export function FormSection({
  title,
  description,
  children,
  testId,
}: {
  title: ReactNode
  description?: ReactNode
  children: ReactNode
  testId?: string
}) {
  return (
    <section
      className="grid grid-cols-1 gap-x-8 gap-y-4 lg:grid-cols-[minmax(0,18rem)_minmax(0,1fr)]"
      data-testid={testId}
    >
      <div>
        <h2 className="text-heading text-fg">{title}</h2>
        {description && (
          <p className="mt-1 text-body text-fg-muted">{description}</p>
        )}
      </div>
      <Card>
        <div className="flex max-w-2xl flex-col gap-4">{children}</div>
      </Card>
    </section>
  )
}

export function Field({
  label,
  htmlFor,
  optional = false,
  help,
  error,
  children,
}: {
  label: ReactNode
  /** The id of the control inside `children`. Field needs it to wire the label. */
  htmlFor: string
  optional?: boolean
  help?: ReactNode
  error?: ReactNode
  children: ReactNode
}) {
  const helpId = help ? `${htmlFor}-help` : undefined
  const errorId = error ? `${htmlFor}-error` : undefined
  const describedBy = [helpId, errorId].filter(Boolean).join(' ') || undefined

  // Put the aria wiring on the single control element without the caller
  // having to repeat the ids. Keep any describedby the caller already set.
  let control: ReactNode = children
  if (isValidElement(children) && Children.count(children) === 1) {
    const el = children as ReactElement<Record<string, unknown>>
    const existing = el.props['aria-describedby'] as string | undefined
    control = cloneElement(el, {
      'aria-describedby':
        [existing, describedBy].filter(Boolean).join(' ') || undefined,
      'aria-invalid': error ? true : el.props['aria-invalid'],
    })
  }

  return (
    <div className="flex flex-col gap-2">
      <label htmlFor={htmlFor} className="text-body font-medium text-fg">
        {label}
        {optional && (
          <span className="font-normal text-fg-muted"> (optional)</span>
        )}
      </label>
      {control}
      {help && (
        <p id={helpId} className="text-caption text-fg-muted">
          {help}
        </p>
      )}
      {error && (
        <p id={errorId} role="alert" className="text-caption text-danger">
          {error}
        </p>
      )}
    </div>
  )
}

/**
 * The one action bar after a form's last section. Right-aligned from `lg`,
 * where the fields start after the 18rem title column. `sticky` pins it to the
 * bottom of the screen on phones for forms taller than one screen.
 */
export function FormActions({
  children,
  sticky = false,
}: {
  children: ReactNode
  sticky?: boolean
}) {
  const stickyClass = sticky
    ? 'sticky bottom-0 -mx-4 border-t border-line bg-canvas/95 px-4 py-3 backdrop-blur sm:mx-0 sm:border-0 sm:bg-transparent sm:px-0 sm:py-0 sm:backdrop-blur-none'
    : ''
  return (
    <div
      className={`flex flex-wrap items-center gap-2 lg:pl-[calc(18rem+2rem)] ${stickyClass}`}
    >
      {children}
    </div>
  )
}
