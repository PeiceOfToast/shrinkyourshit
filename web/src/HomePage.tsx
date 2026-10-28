// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from '@tanstack/react-query'
import { getHealth } from './api'

export default function HomePage() {
  const health = useQuery({ queryKey: ['health'], queryFn: getHealth })

  return (
    <main className="flex min-h-full items-center justify-center bg-slate-950 p-4 text-slate-100">
      <div className="w-full max-w-md rounded-2xl bg-slate-900 p-10 text-center shadow-xl ring-1 ring-slate-800">
        <h1 className="text-4xl font-bold tracking-tight">ShrinkYourShit</h1>
        <p className="mt-2 text-slate-400">
          Watch a folder. Shrink what needs it.
        </p>
        <div className="mt-8 text-sm" data-testid="health">
          {health.isPending && (
            <span className="text-slate-500">checking API…</span>
          )}
          {health.isError && (
            <span className="text-red-400">API unreachable</span>
          )}
          {health.data && (
            <span className="text-emerald-400">
              API {health.data.status} · {health.data.version}
            </span>
          )}
        </div>
      </div>
    </main>
  )
}
