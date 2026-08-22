export interface GalleryParams {
  date: string
  ext: string
  q: string
  num?: number
}

/**
 * Reads gallery filters straight out of a location search string.
 *
 * Used when restoring a history entry: unlike the component's `buildQuery`, this
 * never merges with current state. An absent `date` means "today" (that is how the
 * URL is built in the first place), so callers must reset to today rather than
 * keeping whatever date is on screen.
 */
export function parseGalleryParams(search: string): GalleryParams {
  const params = new URLSearchParams(search)
  const rawNum = params.get('num')
  const num = rawNum === null ? undefined : Number.parseInt(rawNum, 10)
  return {
    date: params.get('date') || '',
    ext: params.get('ext') || params.get('search') || '',
    q: params.get('q') || '',
    num: num !== undefined && Number.isFinite(num) && num > 0 ? num : undefined
  }
}
