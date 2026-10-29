// SPDX-License-Identifier: AGPL-3.0-only
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import SetupWizard from './SetupWizard'
import type { DiscoveredServer } from './api'
import {
  initialWizardState,
  validateAccountStep,
  type WizardState,
} from './wizardModel'

function renderWizard(
  state?: Partial<WizardState>,
  opts?: {
    servers?: DiscoveredServer[]
    onJoin?: (server: DiscoveredServer, token: string) => void
    joining?: boolean
  },
) {
  let current: WizardState = { ...initialWizardState, ...state }
  const setState = vi.fn((next: WizardState) => {
    current = next
  })
  const onFinish = vi.fn(() => Promise.resolve())
  const props = {
    state: current,
    setState,
    onFinish,
    servers: opts?.servers,
    onJoin: opts?.onJoin,
    joining: opts?.joining,
  }
  const utils = render(<SetupWizard {...props} />)
  const rerenderWith = (next: WizardState) => {
    current = next
    utils.rerender(<SetupWizard {...props} state={current} />)
  }
  return { setState, onFinish, rerenderWith, get: () => current }
}

describe('SetupWizard', () => {
  it('starts on the server step; without servers the join card is inert', () => {
    renderWizard()
    expect(screen.getByTestId('wizard-step-1')).toBeInTheDocument()
    expect(screen.getByTestId('mode-server')).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(screen.getByTestId('mode-join')).toHaveTextContent(
      'No other server was found on your network.',
    )
    expect(screen.queryByTestId('join-servers')).not.toBeInTheDocument()
    // Clicking an inert card opens nothing.
    fireEvent.click(screen.getByTestId('mode-join'))
    expect(screen.queryByTestId('join-servers')).not.toBeInTheDocument()
    expect(screen.getByTestId('wizard-back')).toBeDisabled()
  })

  it('lists discovered servers and starts the pairing flow (task 08)', () => {
    const onJoin = vi.fn()
    renderWizard(undefined, {
      servers: [
        {
          id: 'srv-1',
          name: 'media-box',
          url: 'http://10.0.0.5:8765',
          version: 'v1.2.3',
        },
      ],
      onJoin,
    })
    expect(screen.getByTestId('mode-join')).toHaveTextContent(
      'Servers were found on your network.',
    )

    // Open the picker and choose the server.
    fireEvent.click(screen.getByTestId('mode-join'))
    expect(screen.getByTestId('join-servers')).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('join-server-srv-1'))

    // The join panel explains what happens next; the token is optional.
    expect(screen.getByText(/Joining/)).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('join-confirm'))
    expect(onJoin).toHaveBeenCalledWith(
      {
        id: 'srv-1',
        name: 'media-box',
        url: 'http://10.0.0.5:8765',
        version: 'v1.2.3',
      },
      '',
    )

    // With a token typed, it rides along.
    fireEvent.change(screen.getByTestId('join-token'), {
      target: { value: 'one-time-code' },
    })
    fireEvent.click(screen.getByTestId('join-confirm'))
    expect(onJoin).toHaveBeenLastCalledWith(
      {
        id: 'srv-1',
        name: 'media-box',
        url: 'http://10.0.0.5:8765',
        version: 'v1.2.3',
      },
      'one-time-code',
    )
  })

  it('shows the joining state while the request runs', () => {
    renderWizard(undefined, {
      servers: [
        {
          id: 'srv-1',
          name: 'media-box',
          url: 'http://10.0.0.5:8765',
          version: '',
        },
      ],
      onJoin: () => {},
      joining: true,
    })
    fireEvent.click(screen.getByTestId('mode-join'))
    fireEvent.click(screen.getByTestId('join-server-srv-1'))
    expect(screen.getByTestId('join-confirm')).toHaveTextContent('Joining…')
    expect(screen.getByTestId('join-confirm')).toBeDisabled()
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
