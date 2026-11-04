// SPDX-License-Identifier: AGPL-3.0-only

/**
 * CodeBlock (task 39, spec 4.1): a block of commands or other text the
 * user may copy. It wraps long lines instead of clipping them, because a
 * `pre` that scrolls sideways is hard to read on a phone. With `copy`, a
 * CopyButton sits in the top-right corner, on its own row above the text,
 * so it never covers a long first line.
 */
import CopyButton from './CopyButton'

export default function CodeBlock({
  children,
  copy = false,
  testId,
  copyTestId,
}: {
  children: string
  copy?: boolean
  testId?: string
  /** The copy button's test id. Defaults to `${testId}-copy`. */
  copyTestId?: string
}) {
  return (
    <div
      className="rounded-control bg-canvas ring-1 ring-line"
      data-testid={testId}
    >
      {copy && (
        <div className="flex justify-end p-2 pb-0">
          <CopyButton
            text={children}
            testId={copyTestId ?? (testId ? `${testId}-copy` : undefined)}
          />
        </div>
      )}
      <pre className="whitespace-pre-wrap break-words p-3 font-mono text-caption text-fg">
        {children}
      </pre>
    </div>
  )
}
