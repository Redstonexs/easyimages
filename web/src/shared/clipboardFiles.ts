/** Clipboard images arrive with no useful name, so build one that will not collide. */
export function screenshotName(extension: string): string {
  const now = new Date()
  const pad = (value: number, width = 2) => String(value).padStart(width, '0')
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}`
    + `-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`
  // Milliseconds plus randomness: the server generates some filename modes at
  // second granularity and never checks for an existing file before writing.
  const unique = `${pad(now.getMilliseconds(), 3)}${Math.random().toString(36).slice(2, 6)}`
  return `screenshot-${stamp}-${unique}.${extension}`
}

function extensionFor(type: string): string {
  const subtype = type.split('/')[1] || 'png'
  const cleaned = subtype.split('+')[0].toLowerCase()
  return cleaned === 'jpeg' ? 'jpg' : cleaned
}

/** True when the paste target is a text field that should keep normal paste behaviour. */
export function isTextEntryTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  if (target.isContentEditable) return true
  const tag = target.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT'
}

/** Pulls image files out of a paste/drop item list, naming unnamed ones uniquely. */
export function extractImageFiles(items: DataTransferItemList | null | undefined): File[] {
  if (!items) return []
  const files: File[] = []
  for (const item of Array.from(items)) {
    if (item.kind !== 'file' || !item.type.startsWith('image/')) continue
    const file = item.getAsFile()
    if (!file) continue
    // Browsers name clipboard images "image.png" every time, so rename them.
    const named = !file.name || file.name === 'image.png' || file.name === 'blob'
      ? new File([file], screenshotName(extensionFor(file.type)), { type: file.type })
      : file
    files.push(named)
  }
  return files
}
