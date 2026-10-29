// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Settings page state model, kept apart from the component so fast refresh
 * stays happy and the logic is testable.
 */
import type { Settings, SettingsUpdate } from './api'

/** Editable mirror of the settings the page renders. */
export interface FormState {
  worker_concurrency: string
  scratch_dir: string
  default_result_policy: string
  preferred_languages: string
  default_video_codec: string
  login_enabled: boolean
  admin_username: string
}

export function toForm(s: Settings): FormState {
  return {
    worker_concurrency: String(s.worker_concurrency),
    scratch_dir: s.scratch_dir,
    default_result_policy: s.default_result_policy,
    preferred_languages: s.preferred_languages.join(', '),
    default_video_codec: s.default_video_codec,
    login_enabled: s.login_enabled,
    admin_username: s.admin_username ?? '',
  }
}

/** Builds the PUT body from the form; extra carries the login/password part. */
export function buildUpdate(
  form: FormState,
  extra: SettingsUpdate,
): SettingsUpdate {
  const body: SettingsUpdate = { ...extra }
  body.worker_concurrency = Number(form.worker_concurrency)
  body.scratch_dir = form.scratch_dir
  body.default_result_policy = form.default_result_policy
  body.preferred_languages = form.preferred_languages
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s !== '')
  body.default_video_codec = form.default_video_codec
  return body
}
