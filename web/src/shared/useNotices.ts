import { toast } from 'sonner'
import type { NoticeType } from './notify'

export function notifyToast(message: string, type: NoticeType = 'info'): void {
  switch (type) {
    case 'success':
      toast.success(message)
      break
    case 'danger':
      toast.error(message)
      break
    case 'warning':
      toast.warning(message)
      break
    default:
      toast(message)
      break
  }
}
