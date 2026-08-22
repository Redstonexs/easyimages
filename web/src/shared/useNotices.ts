import { ref } from 'vue'
import type { Notice, NoticeType } from './notify'
import { createNotice } from './notify'

const DISMISS_AFTER = 3600

/** Toast list plus its auto-dismiss timer, shared by the three island roots. */
export function useNotices(dismissAfter = DISMISS_AFTER) {
  const notices = ref<Notice[]>([])

  function notify(message: string, type: NoticeType = 'info') {
    const notice = createNotice(message, type)
    notices.value.push(notice)
    window.setTimeout(() => {
      notices.value = notices.value.filter(item => item.id !== notice.id)
    }, dismissAfter)
  }

  return { notices, notify }
}
