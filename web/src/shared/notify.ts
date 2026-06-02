export type NoticeType = 'success' | 'danger' | 'warning' | 'info'

export interface Notice {
  id: number
  message: string
  type: NoticeType
}

let nextNoticeId = 1

export function createNotice(message: string, type: NoticeType = 'info'): Notice {
  return {
    id: nextNoticeId++,
    message,
    type
  }
}
