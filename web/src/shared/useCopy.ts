import { copyText } from './clipboard'
import type { NoticeType } from './notify'

type Notifier = (message: string, type?: NoticeType) => void

/** copyText plus the success/failure toast that every call site repeated. */
export function useCopy(notify: Notifier) {
  return async function copy(value: string, successMessage = '复制成功') {
    if (!value) {
      notify('没有可复制的内容', 'warning')
      return
    }
    try {
      await copyText(value)
      notify(successMessage, 'success')
    } catch {
      notify('复制失败，请手动复制', 'danger')
    }
  }
}
