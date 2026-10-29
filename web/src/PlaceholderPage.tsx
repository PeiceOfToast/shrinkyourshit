// SPDX-License-Identifier: AGPL-3.0-only

/** Placeholder page for routes later tasks fill in (Dashboard, Libraries, Nodes). */
export default function PlaceholderPage({
  title,
  message,
}: {
  title: string
  message: string
}) {
  return (
    <div className="max-w-2xl" data-testid="placeholder">
      <h1 className="text-2xl font-bold">{title}</h1>
      <p className="mt-3 text-slate-400">{message}</p>
    </div>
  )
}
