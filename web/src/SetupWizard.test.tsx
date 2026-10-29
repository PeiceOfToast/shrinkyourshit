// SPDX-License-Identifier: AGPL-3.0-only
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import SetupWizard from './SetupWizard'
import {
  initialWizardState,
  validateAccountStep,
  type WizardState,
} from './wizardModel'

function renderWizard(state?: Partial<WizardState>) {
  let current: WizardState = { ...initialWizardState, ...state }
  const setState = vi.fn((next: WizardState) => {
    current = next
  })
  const onFinish = vi.fn(() => Promise.resolve())
  const utils = render(
    <SetupWizard state={current} setState={setState} onFinish={onFinish} />,
  )
  const rerenderWith = (next: WizardState) => {
    current = next
    utils.rerender(
      <SetupWizard state={current} setState={setState} onFinish={onFinish} />,
    )
  }
  return { setState, onFinish, rerenderWith, get: () => current }
}

describe('SetupWizard', () => {
  it('starts on the server step with join disabled', () => {
    renderWizard()
    expect(screen.getByTestId('wizard-step-1')).toBeInTheDocument()
    expect(screen.getByTestId('mode-server')).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(screen.getByTestId('mode-join')).toHaveAttribute(
      'aria-disabled',
      'true',
    )
    expect(screen.getByText('Coming soon.')).toBeInTheDocument()
    expect(screen.getByTestId('wizard-back')).toBeDisabled()
  })

  it('walks forward through all three steps and back again', () => {
    const harness = renderWizard()
    fireEvent.click(screen.getByTestId('wizard-next'))
    expect(harness.get().step).toBe(2)

    harness.rerenderWith({ ...harness.get(), step: 2 })
    fireEvent.click(screen.getByTestId('wizard-next'))
    expect(harness.get().step).toBe(3)

    harness.rerenderWith({ ...harness.get(), step: 3 })
    expect(screen.getByTestId('wizard-step-3')).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('wizard-back'))
    expect(harness.get().step).toBe(2)
  })

  it('blocks step 2 when the account fields are missing or mismatched', () => {
    const harness = renderWizard({ step: 2, loginEnabled: true })
    fireEvent.click(screen.getByTestId('wizard-next'))
    expect(harness.get().step).toBe(2) // validation failure must not advance
    expect(screen.getByTestId('step-errors')).toHaveTextContent(
      'Enter a username.',
    )

    // Fill username + password but mismatch the confirmation.
    harness.rerenderWith({
      ...harness.get(),
      step: 2,
      loginEnabled: true,
      username: 'admin',
      password: 'secret',
      confirm: 'other',
    })
    fireEvent.click(screen.getByTestId('wizard-next'))
    expect(harness.get().step).toBe(2)
    expect(screen.getByTestId('step-errors')).toHaveTextContent(
      'The passwords do not match.',
    )
  })

  it('advances step 2 when the account fields are valid', () => {
    const harness = renderWizard({
      step: 2,
      loginEnabled: true,
      username: 'admin',
      password: 'secret',
      confirm: 'secret',
    })
    fireEvent.click(screen.getByTestId('wizard-next'))
    expect(harness.get().step).toBe(3)
    expect(screen.queryByTestId('step-errors')).not.toBeInTheDocument()
  })

  it('skips account fields when login stays off', () => {
    const harness = renderWizard({ step: 2, loginEnabled: false })
    fireEvent.click(screen.getByTestId('wizard-next'))
    expect(harness.get().step).toBe(3)
  })

  it('submits with login disabled: no credentials in the request', () => {
    const harness = renderWizard({ step: 3, loginEnabled: false })
    fireEvent.click(screen.getByTestId('wizard-finish'))
    expect(harness.onFinish).toHaveBeenCalledWith({
      mode: 'server',
      login_enabled: false,
    })
  })

  it('submits with login enabled: trimmed username and password', () => {
    const harness = renderWizard({
      step: 3,
      loginEnabled: true,
      username: ' admin ',
      password: 'secret',
      confirm: 'secret',
    })
    fireEvent.click(screen.getByTestId('wizard-finish'))
    expect(harness.onFinish).toHaveBeenCalledWith({
      mode: 'server',
      login_enabled: true,
      username: 'admin',
      password: 'secret',
    })
  })
})

describe('validateAccountStep', () => {
  it('is silent when login is off', () => {
    expect(
      validateAccountStep({
        loginEnabled: false,
        username: '',
        password: '',
        confirm: '',
      }),
    ).toEqual([])
  })

  it('collects every problem when login is on', () => {
    expect(
      validateAccountStep({
        loginEnabled: true,
        username: '',
        password: '',
        confirm: 'x',
      }),
    ).toEqual([
      'Enter a username.',
      'Enter a password.',
      'The passwords do not match.',
    ])
    expect(
      validateAccountStep({
        loginEnabled: true,
        username: 'admin',
        password: 'secret',
        confirm: 'secret',
      }),
    ).toEqual([])
  })
})
