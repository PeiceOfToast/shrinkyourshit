// SPDX-License-Identifier: AGPL-3.0-only
import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'

/**
 * Guardrails from the UI redesign spec (docs/design/ui-redesign-2026-11-04.md,
 * 2.2 and 6): retired classes must not appear in JSX `className` values.
 * Tokens (`text-fg`, `bg-surface`, `text-danger`...) are the only colours
 * components may use; raw palette names live in web/src/index.css only.
 *
 * Each pattern is matched against every string literal or template part
 * inside a `className` attribute, so `className="…"`, `className={cond ? "…" : "…"}`
 * and `className={`… ${x}`}` are all covered.
 */
const retiredClassPatterns = [
  {
    // Raw palette names: slate, emerald, red, amber, sky, rose (`rose-*` is
    // retired too). Covers `text-slate-500`, `bg-emerald-600`, `!text-red-300`
    // and `hover:bg-slate-800`.
    regex: String.raw`\b(?:slate|emerald|red|amber|sky|rose)-\w`,
    message:
      'Raw palette class. Use a token from 2.2 (text-fg, text-fg-muted, bg-surface, text-danger, bg-accent…). See docs/design/ui-redesign-2026-11-04.md.',
  },
  {
    regex: String.raw`\bopacity-50\b`,
    message:
      'opacity-50 is retired as a "zero" or disabled signal. Use text-fg-muted, or disabled:opacity-60 on a control.',
  },
  {
    regex: String.raw`\btext-\[(?:\d|1[01])(?:\.\d+)?px\]`,
    message:
      'Text below 12px is not allowed. Use text-caption (12px) or larger. See 2.3.',
  },
]

const classAttribute = "JSXAttribute[name.name='className']"
const retiredClassRules = retiredClassPatterns.flatMap(({ regex, message }) => [
  {
    selector: `${classAttribute} Literal[value=/${regex}/]`,
    message,
  },
  {
    selector: `${classAttribute} TemplateElement[value.raw=/${regex}/]`,
    message,
  },
])

export default tseslint.config(
  { ignores: ['dist', 'coverage', 'node_modules'] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'no-restricted-syntax': ['error', ...retiredClassRules],
      'react-refresh/only-export-components': [
        'warn',
        { allowConstantExport: true },
      ],
    },
  },
)
