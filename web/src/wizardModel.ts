// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Setup wizard state and validation, kept apart from the component so the
 * logic is unit-testable.
 */
export interface WizardState {
  step: number
  mode: 'server' | 'join'
  loginEnabled: boolean
  username: string
  password: string
  confirm: string
  /** Task 11: the wizard's explicit start choice. Default false —
   * a completed wizard never begins encoding on its own. */
  startNode: boolean
}

export const initialWizardState: WizardState = {
  step: 1,
  mode: 'server',
  loginEnabled: false,
  username: '',
  password: '',
  confirm: '',
  startNode: false,
}

/**
 * Validates step 2. Returns the error messages to show; an empty list
 * means the step may be submitted.
 */
export function validateAccountStep(
  state: Pick<
    WizardState,
    'loginEnabled' | 'username' | 'password' | 'confirm'
  >,
): string[] {
  if (!state.loginEnabled) {
    return []
  }
  const errors: string[] = []
  if (state.username.trim() === '') {
    errors.push('Enter a username.')
  }
  if (state.password === '') {
    errors.push('Enter a password.')
  }
  if (state.password !== state.confirm) {
    errors.push('The passwords do not match.')
  }
  return errors
}
