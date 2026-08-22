import { describe, expect, it } from 'vitest'
import { parseGalleryParams } from './galleryParams'

describe('parseGalleryParams', () => {
  it('reads every supported filter', () => {
    expect(parseGalleryParams('?date=2026/08/20&ext=png&q=cat&num=50'))
      .toEqual({ date: '2026/08/20', ext: 'png', q: 'cat', num: 50 })
  })

  it('returns empty values when params are absent so callers reset to defaults', () => {
    expect(parseGalleryParams('')).toEqual({ date: '', ext: '', q: '', num: undefined })
    expect(parseGalleryParams('?q=cat').date).toBe('')
  })

  it('falls back to the legacy search param for ext', () => {
    expect(parseGalleryParams('?search=gif').ext).toBe('gif')
    expect(parseGalleryParams('?ext=png&search=gif').ext).toBe('png')
  })

  it('ignores a non-numeric or non-positive num', () => {
    expect(parseGalleryParams('?num=abc').num).toBeUndefined()
    expect(parseGalleryParams('?num=0').num).toBeUndefined()
    expect(parseGalleryParams('?num=-5').num).toBeUndefined()
  })

  it('works with or without the leading question mark', () => {
    expect(parseGalleryParams('date=2026/08/20').date).toBe('2026/08/20')
  })
})
