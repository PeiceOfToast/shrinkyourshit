// SPDX-License-Identifier: AGPL-3.0-only

import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Field, FormActions, FormSection } from './FormSection'
import { input } from './uiControls'

describe('Field', () => {
  it('labels the control through htmlFor', () => {
    render(
      <Field label="Files processed at once" htmlFor="concurrency">
        <input id="concurrency" className={input} />
      </Field>,
    )
    expect(screen.getByLabelText('Files processed at once')).toHaveAttribute(
      'id',
      'concurrency',
    )
  })

  it('wires aria-describedby to the help and the error, and aria-invalid on error', () => {
    render(
      <Field
        label="Folder"
        htmlFor="folder"
        help="The folder to watch."
        error="Pick a folder that exists."
      >
        <input id="folder" className={input} />
      </Field>,
    )
    const control = screen.getByLabelText('Folder')
    expect(control).toHaveAttribute(
      'aria-describedby',
      'folder-help folder-error',
    )
    expect(control).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByText('The folder to watch.')).toHaveAttribute(
      'id',
      'folder-help',
    )
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Pick a folder that exists.',
    )
    expect(screen.getByRole('alert')).toHaveAttribute('id', 'folder-error')
  })

  it('leaves aria-invalid off and adds only the help id when there is no error', () => {
    render(
      <Field label="Name" htmlFor="name" help="Shown in the list.">
        <input id="name" />
      </Field>,
    )
    const control = screen.getByLabelText('Name')
    expect(control).toHaveAttribute('aria-describedby', 'name-help')
    expect(control).not.toHaveAttribute('aria-invalid')
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('keeps a describedby the caller already set', () => {
    render(
      <Field label="Name" htmlFor="name" help="Help.">
        <input id="name" aria-describedby="other" />
      </Field>,
    )
    expect(screen.getByLabelText('Name')).toHaveAttribute(
      'aria-describedby',
      'other name-help',
    )
  })

  it('marks an optional field', () => {
    render(
      <Field label="Note" htmlFor="note" optional>
        <input id="note" />
      </Field>,
    )
    expect(screen.getByText('(optional)')).toBeInTheDocument()
  })
})

describe('FormSection', () => {
  it('renders the title, description and fields inside one card', () => {
    render(
      <FormSection title="Server" description="How this machine runs the work.">
        <Field label="Files at once" htmlFor="c">
          <input id="c" />
        </Field>
      </FormSection>,
    )
    expect(
      screen.getByRole('heading', { level: 2, name: 'Server' }),
    ).toBeInTheDocument()
    expect(
      screen.getByText('How this machine runs the work.'),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Files at once')).toBeInTheDocument()
  })

  it('puts the field column at max-w-2xl and the grid at lg', () => {
    const { container } = render(
      <FormSection title="Server">
        <p>body</p>
      </FormSection>,
    )
    expect(container.querySelector('section')).toHaveClass(
      'lg:grid-cols-[minmax(0,18rem)_minmax(0,1fr)]',
    )
    expect(screen.getByText('body').parentElement).toHaveClass('max-w-2xl')
  })
})

describe('FormActions', () => {
  it('renders its buttons in order and the sticky variant on phones', () => {
    const { container } = render(
      <FormActions sticky>
        <button type="button">Save changes</button>
        <button type="button">Cancel</button>
      </FormActions>,
    )
    const bar = container.firstElementChild
    expect(bar).toHaveClass('sticky', 'bottom-0')
    expect(Array.from(bar!.children).map((el) => el.textContent)).toEqual([
      'Save changes',
      'Cancel',
    ])
  })
})
