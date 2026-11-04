// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The /setup route: redirects away once setup is complete, otherwise runs
 * the three-step wizard (task 06). When another server was found on the
 * network the wizard's join card starts the pairing flow (task 08): the
 * machine switches to worker mode, this window shows what happened next.
 */
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { completeSetup, getSetupServers, login } from './api'
import { meKey, useMarkSetupComplete } from './queries'
import { RedirectWhenSetupComplete } from './guards'
import SetupWizard from './SetupWizard'
import { initialWizardState, type WizardState } from './wizardModel'
import { useToast } from './toastContext'
import Banner from './Banner'
import Card from './Card'
import Icon from './Icon'

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
  const toast = useToast()
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [joinedServer, setJoinedServer] = useState<string | null>(null)
  const [joining, setJoining] = useState(false)

  // Discovered servers for the join card; refreshed while the wizard
  // waits on step 1 so a late-appearing server still shows up.
  const servers = useQuery({
    queryKey: ['setup-servers'],
    queryFn: getSetupServers,
    refetchInterval: 8000,
  })

  const setup = useMutation({
    mutationFn: completeSetup,
    onSuccess: (status) => {
      markSetupComplete(status)
      if (status.mode === 'join') {
        // This machine is switching to worker mode; the local UI goes
        // away. Tell the user what happens instead of navigating.
        toast.success(`Joining ${status.server ?? 'the server'} as a helper.`)
        setJoinedServer(status.server ?? 'the server')
        return
      }
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
      toast.failure('Setup failed', error)
      setSubmitError(error.message)
      setJoining(false)
    },
  })

  if (joinedServer) {
    return (
      <main className="grid min-h-dvh place-items-center bg-canvas p-4 text-fg">
        <div className="flex w-full max-w-lg flex-col gap-6">
          <Wordmark />
          <Card padding="none" className="p-6 sm:p-8" testId="joined-panel">
            <div className="flex flex-col gap-4">
              <h1 className="text-title text-fg">Joining {joinedServer}</h1>
              <Banner tone="info">
                Approve this machine on the server's Nodes page.
              </Banner>
              <p className="text-body text-fg-muted">
                This machine is now a helper of that server. Its window will
                show “Waiting for approval” until you approve it. This page goes
                away while the machine switches over.
              </p>
            </div>
          </Card>
        </div>
      </main>
    )
  }

  return (
    <main className="grid min-h-dvh place-items-center bg-canvas p-4 text-fg">
      <div className="flex w-full max-w-lg flex-col gap-6">
        <Wordmark />
        <SetupWizard
          state={state}
          setState={setState}
          onFinish={async (request) => setup.mutate(request)}
          servers={servers.data?.servers ?? []}
          joining={joining}
          onJoin={(server, token) => {
            setJoining(true)
            setSubmitError(null)
            setup.mutate({
              mode: 'join',
              server: server.url,
              ...(token ? { token } : {}),
            })
          }}
          submitting={setup.isPending}
          submitError={submitError}
        />
      </div>
    </main>
  )
}

/** The brand mark and name. A div, not a heading: each screen's h1 is its title. */
function Wordmark() {
  return (
    <div className="flex items-center justify-center gap-2 text-base font-semibold text-fg">
      <Icon name="brand" className="h-5 w-5 shrink-0 text-accent-text" />
      ShrinkYourShit
    </div>
  )
}
