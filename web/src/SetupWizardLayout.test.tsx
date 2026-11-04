// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Setup wizard layout (task 51, spec 5.1): one h1 per step, a visual stepper
 * with aria-current, the join option as a real button, 44 px controls from
 * uiControls, the login choice as a Switch, and step errors as a danger Banner.
 * Behaviour (navigation, validation, submit) stays in SetupWizard.test.tsx.
 */
import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import SetupWizard from './SetupWizard'
import type { DiscoveredServer } from './api'
import { initialWizardState, type WizardState } from './wizardModel'

const server: DiscoveredServer = {
  id: 'srv-1',
  name: 'media-box',
  url: 'http://10.0.0.5:8765',
  version: 'v1.2.3',
}

function renderWizard(
  state?: Partial<WizardState>,
  servers?: DiscoveredServer[],
) {
  let current: WizardState = { ...initialWizardState, ...state }
  const setState = vi.fn((next: WizardState) => {
    current = next
  })
  const props = {
    state: current,
    setState,
    onFinish: vi.fn(() => Promise.resolve()),
    servers,
  }
  const utils = render(<SetupWizard {...props} />)
  const rerenderWith = (next: WizardState) => {
    current = next
    utils.rerender(<SetupWizard {...props} state={current} />)
  }
  return { setState, rerenderWith, get: () => current }
}

describe('SetupWizard layout (task 51)', () => {
  it('has exactly one h1 on each step', () => {
    const harness = renderWizard({ step: 1 })
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
    harness.rerenderWith({ ...harness.get(), step: 2 })
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
    harness.rerenderWith({ ...harness.get(), step: 3 })
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
      'Ready to start',
    )
  })

  it('marks the current step with aria-current and checks off finished steps', () => {
    renderWizard({ step: 2 })
    const items = within(
      screen.getByRole('list', { name: 'Progress' }),
    ).getAllByRole('listitem')
    expect(items).toHaveLength(3)
    expect(items[0]).not.toHaveAttribute('aria-current')
    expect(items[0]).toHaveTextContent('(done)')
    expect(items[1]).toHaveAttribute('aria-current', 'step')
    expect(items[2]).not.toHaveAttribute('aria-current')
    expect(items[2]).not.toHaveTextContent('(done)')
  })

  it('renders the join option as a button, and it is disabled with no server', () => {
    renderWizard()
    const join = screen.getByTestId('mode-join')
    expect(join.tagName).toBe('BUTTON')
    expect(join).toBeDisabled()
    expect(join).toHaveAttribute('aria-pressed', 'false')
  })

  it('keeps the join option a button when a server is found, and the list sits outside it', () => {
    renderWizard(undefined, [server])
    const join = screen.getByTestId('mode-join')
    expect(join.tagName).toBe('BUTTON')
    expect(join).toBeEnabled()
    fireEvent.click(join)
    // A button cannot hold interactive children, so the list is a sibling.
    expect(join.contains(screen.getByTestId('join-servers'))).toBe(false)
    expect(join).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByTestId('join-server-srv-1').tagName).toBe('BUTTON')
  })

  it('uses the 44 px control classes for inputs and buttons', () => {
    renderWizard({ step: 2, loginEnabled: true })
    expect(screen.getByTestId('username')).toHaveClass('min-h-11', 'w-full')
    expect(screen.getByTestId('password')).toHaveClass('min-h-11', 'w-full')
    expect(screen.getByTestId('confirm')).toHaveClass('min-h-11', 'w-full')
    expect(screen.getByTestId('wizard-next')).toHaveClass('min-h-11')
    expect(screen.getByTestId('wizard-back')).toHaveClass('min-h-11')
  })

  it('hides Back on step 1 with invisible, keeping it disabled and in place', () => {
    renderWizard({ step: 1 })
    const back = screen.getByTestId('wizard-back')
    expect(back).toHaveClass('invisible')
    expect(back).toBeDisabled()
  })

  it('shows step 2 validation problems as a danger Banner', () => {
    renderWizard({ step: 2, loginEnabled: true })
    fireEvent.click(screen.getByTestId('wizard-next'))
    const banner = screen.getByTestId('step-errors')
    expect(banner).toHaveAttribute('role', 'alert')
    expect(banner).toHaveClass('bg-danger/10')
    expect(banner).toHaveTextContent('Enter a username.')
  })

  it('offers the login choice as a labelled switch', () => {
    const harness = renderWizard({ step: 2 })
    const toggle = screen.getByTestId('login-toggle')
    expect(toggle).toHaveAttribute('role', 'switch')
    expect(toggle).toHaveAttribute('aria-checked', 'false')
    expect(
      screen.getByRole('switch', { name: 'Require a login on this server' }),
    ).toBe(toggle)
    fireEvent.click(toggle)
    expect(harness.setState).toHaveBeenCalledWith(
      expect.objectContaining({ loginEnabled: true }),
    )
  })
})
