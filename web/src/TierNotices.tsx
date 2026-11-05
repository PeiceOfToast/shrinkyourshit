// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Tier notices (task 62): the downgrade banner on the library page and the
 * read only reason on a lower tier editor. Both name the steps the tier
 * cannot express and the tier that can edit each one, in plain words.
 */
import { Link } from 'react-router-dom'
import type { FlowFieldError, RuleTier } from './api'
import Banner from './Banner'
import { linkTouch } from './uiControls'
import { editableIn, stepOf, tierLabel } from './tierModel'

/**
 * One line per problem: a named step gets its id and the lowest tier that can
 * edit it; a graph-level problem shows the server's own sentence.
 */
function StepList({ steps }: { steps: FlowFieldError[] }) {
  return (
    <ul className="list-disc space-y-1 pl-5" data-testid="tier-step-list">
      {steps.map((s) => {
        const id = stepOf(s)
        const can = editableIn(s)
        return (
          <li key={`${s.path}-${s.message}`} data-testid="tier-step-item">
            {id ? (
              <>
                <span className="font-mono">{id}</span>
                {can ? ` can be edited in the ${tierLabel(can)} tier.` : '.'}
              </>
            ) : (
              s.message
            )}
          </li>
        )
      })}
    </ul>
  )
}

/**
 * The warning after a downgrade (or on any library whose own tier cannot
 * show a step). The stored flow is never touched by a tier change.
 */
export function UnrepresentableBanner({
  tier,
  steps,
  libraryId,
}: {
  tier: RuleTier
  steps: FlowFieldError[]
  libraryId: string
}) {
  return (
    <Banner
      tone="warning"
      title={`Not fully representable in ${tierLabel(tier)}`}
      testId="tier-banner"
    >
      <div className="flex flex-col gap-2">
        <p>
          This library has {steps.length === 1 ? 'a step' : 'steps'} the{' '}
          {tierLabel(tier)} editor cannot show or change:
        </p>
        <StepList steps={steps} />
        <p>
          Nothing was changed. The flow was not modified, and each step is kept
          as it was.{' '}
          <Link
            to={`/libraries/${libraryId}/flow`}
            className={linkTouch}
            data-testid="tier-banner-advanced"
          >
            Open the visual editor
          </Link>
        </p>
      </div>
    </Banner>
  )
}

/**
 * The reason a lower tier editor is read only. It replaces the Save action,
 * so the page says why nothing can be saved here.
 */
export function ReadOnlyReason({
  tier,
  steps,
  libraryId,
  testId,
}: {
  tier: RuleTier
  steps: FlowFieldError[]
  libraryId: string
  testId: string
}) {
  return (
    <Banner tone="info" title="Read only in this editor" testId={testId}>
      <div className="flex flex-col gap-2">
        <p>
          The {tierLabel(tier)} editor cannot change{' '}
          {steps.length === 1 ? 'this step' : 'these steps'}, so saving is off
          here. Nothing is lost: the steps stay as they are.
        </p>
        <StepList steps={steps} />
        <p>
          <Link
            to={`/libraries/${libraryId}/flow`}
            className={linkTouch}
            data-testid={`${testId}-link`}
          >
            Edit them in the visual editor
          </Link>
        </p>
      </div>
    </Banner>
  )
}
