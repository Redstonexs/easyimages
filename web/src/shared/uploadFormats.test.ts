import { describe, expect, it } from 'vitest'
import type { UploadResult } from '../types'
import { formatResults, resultName } from './uploadFormats'

const results: UploadResult[] = [
  { url: 'https://img.test/a.png', original_name: 'a.png' },
  { url: 'https://img.test/b.png', srcName: 'b.png' }
]

describe('resultName', () => {
  it('prefers original_name, then srcName, then empty', () => {
    expect(resultName({ original_name: 'x', srcName: 'y' })).toBe('x')
    expect(resultName({ srcName: 'y' })).toBe('y')
    expect(resultName({})).toBe('')
  })
})

describe('formatResults', () => {
  it('joins direct links with newlines', () => {
    expect(formatResults(results, 'url')).toBe('https://img.test/a.png\nhttps://img.test/b.png')
  })

  it('joins markdown with blank lines', () => {
    expect(formatResults(results, 'markdown'))
      .toBe('![a.png](https://img.test/a.png)\n\n![b.png](https://img.test/b.png)')
  })

  it('emits html img tags', () => {
    expect(formatResults(results, 'html'))
      .toBe('<img src="https://img.test/a.png" alt="a.png" />\n<img src="https://img.test/b.png" alt="b.png" />')
  })

  it('emits bbcode', () => {
    expect(formatResults(results, 'bbcode'))
      .toBe('[img]https://img.test/a.png[/img]\n[img]https://img.test/b.png[/img]')
  })

  it('skips results with no url instead of emitting empty links', () => {
    const mixed: UploadResult[] = [{ url: 'https://img.test/a.png' }, { message: 'failed' }]
    expect(formatResults(mixed, 'url')).toBe('https://img.test/a.png')
    expect(formatResults(mixed, 'markdown')).toBe('![](https://img.test/a.png)')
  })

  it('returns an empty string for an empty list', () => {
    expect(formatResults([], 'url')).toBe('')
    expect(formatResults([{ message: 'failed' }], 'html')).toBe('')
  })
})
