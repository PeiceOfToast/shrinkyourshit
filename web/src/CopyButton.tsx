// SPDX-License-Identifier: AGPL-3.0-only

/**
 * CopyButton (task 39, spec 4.1): copies `text` to the clipboard and shows
 * "Copied" for two seconds. It is extracted from NodesPage so the node
 * join command, the webhook secret, the job log and the container
 * instructions share one control. The button is `btn`, so it keeps the
 * 44 px target.
 */
import { useEffect, useState } from 'react'
import { btn } from './uiControls'
import Icon from './Icon'

const COPIED_MS = 2000

export default function CopyButton({
  text,
  label = 'Copy',
  testId,
}: {
  text: string
  label?: string
  testId?: string
}) {
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), COPIED_MS)
    return () => window.clearTimeout(timer)
  }, [copied])

  const copy = () => {
    // A refused clipboard (no permission, or an insecure page) leaves the
    // label as it is; the user can still select the text by hand.
    navigator.clipboard?.writeText(text).then(
      () => setCopied(true),
      () => setCopied(false),
    )
  }

  return (
    <button
      type="button"
      onClick={copy}
      data-testid={testId}
      className={btn}
      aria-live="polite"
    >
      <Icon name={copied ? 'check' : 'copy'} className="h-4 w-4 shrink-0" />
      {copied ? 'Copied' : label}
    </button>
  )
}
