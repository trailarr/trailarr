import { describe, it, expect, vi } from 'vitest'
import * as isDarkModule from '../../utils/isDark'

describe('isDark utilities', () => {
  const origMatchMedia = globalThis.matchMedia

  afterEach(() => {
    // restore original
    globalThis.matchMedia = origMatchMedia
    vi.resetModules()
  })

  it('isDarkNow returns true when matchMedia reports matches', () => {
    globalThis.matchMedia = () => ({ matches: true })
    // import the function fresh
    const { isDarkNow } = require('../../utils/isDark')
    expect(isDarkNow()).toBe(true)
  })

  it('isDarkNow returns false when matchMedia not available', () => {
    delete globalThis.matchMedia
    const { isDarkNow } = require('../../utils/isDark')
    expect(isDarkNow()).toBe(false)
  })

  it('addDarkModeListener registers listener and cleanup removes it', () => {
    const listeners = {}
    globalThis.matchMedia = () => ({
      matches: false,
      addEventListener: (evt, handler) => {
        listeners[evt] = handler
      },
      removeEventListener: (evt, handler) => {
        if (listeners[evt] === handler) delete listeners[evt]
      },
    })

    // import fresh to ensure module uses our mocked matchMedia
    const { addDarkModeListener } = require('../../utils/isDark')
    const cb = vi.fn()
    const cleanup = addDarkModeListener(cb)

    // simulate a change event
    listeners.change({ matches: true })
    expect(cb).toHaveBeenCalledWith(true)

    // cleanup and ensure further events don't call callback
    cleanup()
    // simulate another change (no-op since listener removed)
    if (listeners.change) listeners.change({ matches: false })
    expect(cb).toHaveBeenCalledTimes(1)
  })
})
