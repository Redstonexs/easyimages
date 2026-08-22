import type { NoticeType } from './notify'
import { createSemaphore } from './concurrency'

export interface DeleteResponse {
  code: number
  msg: string
}

export interface BatchDeleteOutcome {
  succeeded: string[]
  failed: Array<{ key: string; message: string }>
}

/**
 * Deletes many items through the single-item admin endpoints, a few at a time.
 * The backend has no bulk-delete route, so failures are collected per item rather
 * than aborting the run.
 */
export async function runBatchDelete(
  keys: string[],
  remove: (key: string) => Promise<DeleteResponse>,
  concurrency = 4
): Promise<BatchDeleteOutcome> {
  const slots = createSemaphore(concurrency)
  const succeeded: string[] = []
  const failed: Array<{ key: string; message: string }> = []

  await Promise.all(keys.map(key => slots.run(async () => {
    try {
      const result = await remove(key)
      if (result.code === 200) succeeded.push(key)
      else failed.push({ key, message: result.msg || '删除失败' })
    } catch (error) {
      failed.push({ key, message: error instanceof Error ? error.message : '删除失败' })
    }
  })))

  return { succeeded, failed }
}

/** One summary line for a batch run, so partial failures are never swallowed. */
export function summarizeBatchDelete(outcome: BatchDeleteOutcome): { message: string; type: NoticeType } {
  const ok = outcome.succeeded.length
  const bad = outcome.failed.length
  if (bad === 0) return { message: `已删除 ${ok} 个文件`, type: 'success' }
  const reason = outcome.failed[0]?.message || '删除失败'
  if (ok === 0) return { message: `删除失败：${bad} 个文件（${reason}）`, type: 'danger' }
  return { message: `${ok} 个已删除，${bad} 个失败（${reason}）`, type: 'warning' }
}
