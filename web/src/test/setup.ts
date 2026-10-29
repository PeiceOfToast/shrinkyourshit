// SPDX-License-Identifier: AGPL-3.0-only
import '@testing-library/jest-dom/vitest'
import { FakeEventSource } from './fetchMock'

/**
 * jsdom has no EventSource, but the SSE hook mounts whenever the shell does
 * (DECISIONS.md B6). Install the fake as a baseline global OUTSIDE the
 * `vi.stubGlobal` registry: `vi.unstubAllGlobals()` (called by the fetch
 * mock's restore and by tests that swap routes mid-test) tears down every
 * registered stub at once, and when it runs before the tree is unmounted a
 * freshly mounted LiveEvents could see no global at all and throw
 * `ReferenceError: EventSource is not defined`. With this baseline the global
 * is defined at all times; tests that count instances still reset the list
 * per test via stubEventSource().
 *
 * The cast keeps the DOM lib's EventSource constructor type out of the way —
 * the fake only implements what the hook uses (addEventListener/close).
 */
const host = globalThis as unknown as { EventSource?: unknown }
host.EventSource ??= FakeEventSource

/*
 * jsdom 25 has no dialog implementation at all (no showModal/show/close —
 * verified), so the dialog components would be invisible in tests. Give
 * the element a minimal version of the native contract: opening sets the
 * `open` attribute, closing clears it. Real browsers use the real
 * showModal; this only stands in for the test environment.
 */
if (typeof HTMLDialogElement !== 'undefined') {
  const proto = HTMLDialogElement.prototype as unknown as Record<
    string,
    unknown
  >
  if (typeof proto.show !== 'function') {
    proto.show = function mockShow(this: HTMLDialogElement) {
      this.setAttribute('open', '')
    }
  }
  if (typeof proto.showModal !== 'function') {
    proto.showModal = function mockShowModal(this: HTMLDialogElement) {
      this.setAttribute('open', '')
    }
  }
  if (typeof proto.close !== 'function') {
    proto.close = function mockClose(this: HTMLDialogElement) {
      this.removeAttribute('open')
      this.dispatchEvent(new Event('close'))
    }
  }
}
