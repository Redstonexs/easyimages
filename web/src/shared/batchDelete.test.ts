import { describe, expect, it } from 'vitest'
import { runBatchDelete, summarizeBatchDelete } from './batchDelete'

const ok = { code: 200, msg: 'deleted' }

describe('runBatchDelete', () => {
  it('reports every key as succeeded when all calls return 200', async () => {
    const outcome = await runBatchDelete(['a', 'b', 'c'], async () => ok)
    expect(outcome.succeeded.sort()).toEqual(['a', 'b', 'c'])
    expect(outcome.failed).toEqual([])
  })

  it('keeps going after a rejected call and records the reason', async () => {
    const outcome = await runBatchDelete(['a', 'b', 'c'], async key => {
      if (key === 'b') throw new Error('network down')
      return ok
    })
    expect(outcome.succeeded.sort()).toEqual(['a', 'c'])
    expect(outcome.failed).toEqual([{ key: 'b', message: 'network down' }])
  })

  it('treats a non-200 code as a failure', async () => {
    const outcome = await runBatchDelete(['a'], async () => ({ code: 500, msg: '没有权限' }))
    expect(outcome.succeeded).toEqual([])
    expect(outcome.failed).toEqual([{ key: 'a', message: '没有权限' }])
  })

  it('respects the concurrency limit', async () => {
    let running = 0
    let peak = 0
    await runBatchDelete(Array.from({ length: 12 }, (_, i) => String(i)), async () => {
      running++
      peak = Math.max(peak, running)
      await Promise.resolve()
      running--
      return ok
    }, 3)
    expect(peak).toBe(3)
  })

  it('handles an empty selection', async () => {
    const outcome = await runBatchDelete([], async () => ok)
    expect(outcome).toEqual({ succeeded: [], failed: [] })
  })
})

describe('summarizeBatchDelete', () => {
  it('reports full success', () => {
    expect(summarizeBatchDelete({ succeeded: ['a', 'b'], failed: [] }))
      .toEqual({ message: '已删除 2 个文件', type: 'success' })
  })

  it('reports total failure as danger and surfaces the reason', () => {
    expect(summarizeBatchDelete({ succeeded: [], failed: [{ key: 'a', message: '没有权限' }] }))
      .toEqual({ message: '删除失败：1 个文件（没有权限）', type: 'danger' })
  })

  it('reports partial failure as a warning', () => {
    expect(summarizeBatchDelete({ succeeded: ['a'], failed: [{ key: 'b', message: '找不到文件' }] }))
      .toEqual({ message: '1 个已删除，1 个失败（找不到文件）', type: 'warning' })
  })
})
