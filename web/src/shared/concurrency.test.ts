import { describe, expect, it } from 'vitest'
import { createSemaphore } from './concurrency'

function deferred<T = void>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

describe('createSemaphore', () => {
  it('never runs more tasks than the limit at once', async () => {
    const semaphore = createSemaphore(2)
    let running = 0
    let peak = 0

    const tasks = Array.from({ length: 10 }, () => semaphore.run(async () => {
      running++
      peak = Math.max(peak, running)
      await Promise.resolve()
      running--
    }))

    await Promise.all(tasks)
    expect(peak).toBe(2)
    expect(semaphore.active()).toBe(0)
  })

  it('holds later tasks until a slot frees', async () => {
    const semaphore = createSemaphore(1)
    const first = deferred()
    const started: string[] = []

    const a = semaphore.run(async () => { started.push('a'); await first.promise })
    const b = semaphore.run(async () => { started.push('b') })

    await Promise.resolve()
    expect(started).toEqual(['a'])

    first.resolve()
    await Promise.all([a, b])
    expect(started).toEqual(['a', 'b'])
  })

  it('releases the slot when a task rejects', async () => {
    const semaphore = createSemaphore(1)
    await expect(semaphore.run(() => Promise.reject(new Error('boom')))).rejects.toThrow('boom')
    expect(semaphore.active()).toBe(0)
    await expect(semaphore.run(() => Promise.resolve('ok'))).resolves.toBe('ok')
  })

  it('treats a limit below one as one', async () => {
    const semaphore = createSemaphore(0)
    let running = 0
    let peak = 0
    await Promise.all(Array.from({ length: 3 }, () => semaphore.run(async () => {
      running++
      peak = Math.max(peak, running)
      await Promise.resolve()
      running--
    })))
    expect(peak).toBe(1)
  })

  it('returns the task result', async () => {
    const semaphore = createSemaphore(3)
    await expect(semaphore.run(async () => 42)).resolves.toBe(42)
  })
})
