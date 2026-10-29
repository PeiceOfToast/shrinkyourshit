// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The /setup route: redirects away once setup is complete, otherwise runs
 * the three-step wizard (task 06).
 */
import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { completeSetup, login } from './api'
import { meKey, useMarkSetupComplete } from './queries'
import { RedirectWhenSetupComplete } from './guards'
import SetupWizard from './SetupWizard'
import { initialWizardState, type WizardState } from './wizardModel'

export default function SetupPage() {
  return (
    <RedirectWhenSetupComplete>
      <SetupFlow />
    </RedirectWhenSetupComplete>
  )
}

function SetupFlow() {
  const [state, setState] = useState<WizardState>(initialWizardState)
  const navigate = useNavigate()
  const client = useQueryClient()
  const markSetupComplete = useMarkSetupComplete()
  const [submitError, setSubmitError] = useState<string | null>(null)

  const setup = useMutation({
    mutationFn: completeSetup,
    onSuccess: (status) => {
      markSetupComplete(status)
      if (status.login_enabled) {
        // Log the user in with the account they just created.
        login({
          username: state.username.trim(),
          password: state.password,
        })
          .then((me) => {
            client.setQueryData(meKey, me)
            void navigate('/')
          })
          .catch(() => void navigate('/login'))
      } else {
        client.setQueryData(meKey, status)
        void navigate('/')
      }
    },
    onError: (error: Error) => {
      setSubmitError(error.message)
    },
  })

  return (
    <main className="flex min-h-full items-center justify-center bg-slate-950 p-4 text-slate-100">
      <div className="w-full max-w-xl">
        <h1 className="mb-6 text-center text-3xl font-bold tracking-tight">
          ShrinkYourShit
        </h1>
        <SetupWizard
          state={state}
          setState={setState}
          onFinish={async (request) => setup.mutate(request)}
          submitting={setup.isPending}
          submitError={submitError}
        />
      </div>
    </main>
  )
}
