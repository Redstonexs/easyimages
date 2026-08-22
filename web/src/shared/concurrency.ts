export interface Semaphore {
  /** Runs `task` once a slot is free, releasing the slot when it settles. */
  run<T>(task: () => Promise<T>): Promise<T>
  /** Number of tasks currently holding a slot. Exposed for tests and diagnostics. */
  active(): number
}

/**
 * Bounds how many async tasks run at once.
 *
 * Upload concurrency is gated at the *request* level rather than per file, so the
 * file pool and the per-file chunk pool cannot multiply into more in-flight XHRs
 * than the browser's ~6 connections per origin. Tasks never acquire a second slot
 * while holding one, so this cannot deadlock.
 */
export function createSemaphore(limit: number): Semaphore {
  const slots = Math.max(1, Math.floor(limit))
  const waiting: Array<() => void> = []
  let active = 0

  function acquire(): Promise<void> {
    if (active < slots) {
      active++
      return Promise.resolve()
    }
    return new Promise<void>(resolve => waiting.push(resolve))
  }

  function release() {
    const next = waiting.shift()
    // Hand the slot straight to the next waiter instead of releasing and re-acquiring.
    if (next) next()
    else active--
  }

  return {
    async run<T>(task: () => Promise<T>): Promise<T> {
      await acquire()
      try {
        return await task()
      } finally {
        release()
      }
    },
    active: () => active
  }
}
