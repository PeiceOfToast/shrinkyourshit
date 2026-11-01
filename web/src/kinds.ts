// SPDX-License-Identifier: AGPL-3.0-only

/**
 * File kinds (task 34): what a library looks for. Shared by the picker,
 * the library pages and the tests; plain-language labels only, no codec or
 * engine words on the user-facing screens (AGENTS.md rule 14).
 */

/** The kinds the API knows, with their plain-language labels. */
export const KIND_CHOICES = [
  { value: 'video', label: 'Video' },
  { value: 'audio', label: 'Audio' },
  { value: 'image', label: 'Photos' },
  { value: 'subtitle', label: 'Subtitles' },
  { value: 'other', label: 'Anything else' },
] as const

/** The plain-language label of a kind; unknown kinds show as-is. */
export function kindLabel(kind: string): string {
  for (const c of KIND_CHOICES) {
    if (c.value === kind) return c.label
  }
  return kind
}

/** The flow step types that act on a file of any kind (task 34). */
export const KIND_AGNOSTIC_STEPS = [
  'rename',
  'move',
  'skip_if_smaller',
  'plugin',
]
