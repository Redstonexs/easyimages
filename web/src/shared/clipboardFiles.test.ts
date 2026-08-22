import { describe, expect, it } from 'vitest'
import { extractImageFiles, isTextEntryTarget, screenshotName } from './clipboardFiles'

function item(kind: string, type: string, file: File | null): DataTransferItem {
  return { kind, type, getAsFile: () => file } as unknown as DataTransferItem
}

function list(items: DataTransferItem[]): DataTransferItemList {
  return items as unknown as DataTransferItemList
}

describe('screenshotName', () => {
  it('uses the given extension and a screenshot prefix', () => {
    expect(screenshotName('jpg')).toMatch(/^screenshot-\d{8}-\d{6}-\d{3}[a-z0-9]{4}\.jpg$/)
  })

  it('does not collide within the same second', () => {
    const names = new Set(Array.from({ length: 200 }, () => screenshotName('png')))
    expect(names.size).toBe(200)
  })
})

describe('isTextEntryTarget', () => {
  it('detects inputs, textareas and contenteditable', () => {
    expect(isTextEntryTarget(document.createElement('input'))).toBe(true)
    expect(isTextEntryTarget(document.createElement('textarea'))).toBe(true)
    expect(isTextEntryTarget(document.createElement('select'))).toBe(true)

    const editable = document.createElement('div')
    editable.contentEditable = 'true'
    document.body.appendChild(editable)
    expect(isTextEntryTarget(editable)).toBe(true)
    document.body.removeChild(editable)
  })

  it('ignores plain elements and null', () => {
    expect(isTextEntryTarget(document.createElement('div'))).toBe(false)
    expect(isTextEntryTarget(null)).toBe(false)
  })
})

describe('extractImageFiles', () => {
  it('returns an empty list for null items', () => {
    expect(extractImageFiles(null)).toEqual([])
    expect(extractImageFiles(undefined)).toEqual([])
  })

  it('ignores non-file and non-image entries', () => {
    const text = item('string', 'text/plain', null)
    const pdf = item('file', 'application/pdf', new File(['x'], 'doc.pdf', { type: 'application/pdf' }))
    expect(extractImageFiles(list([text, pdf]))).toEqual([])
  })

  it('renames the generic clipboard name', () => {
    const pasted = new File(['x'], 'image.png', { type: 'image/png' })
    const [file] = extractImageFiles(list([item('file', 'image/png', pasted)]))
    expect(file.name).not.toBe('image.png')
    expect(file.name).toMatch(/^screenshot-.*\.png$/)
    expect(file.type).toBe('image/png')
  })

  it('maps image/jpeg to a jpg extension', () => {
    const pasted = new File(['x'], 'image.png', { type: 'image/jpeg' })
    const [file] = extractImageFiles(list([item('file', 'image/jpeg', pasted)]))
    expect(file.name).toMatch(/\.jpg$/)
  })

  it('keeps a meaningful filename from a real dragged file', () => {
    const dragged = new File(['x'], 'holiday.png', { type: 'image/png' })
    const [file] = extractImageFiles(list([item('file', 'image/png', dragged)]))
    expect(file.name).toBe('holiday.png')
  })

  it('gives two pasted images distinct names', () => {
    const a = item('file', 'image/png', new File(['a'], 'image.png', { type: 'image/png' }))
    const b = item('file', 'image/png', new File(['b'], 'image.png', { type: 'image/png' }))
    const files = extractImageFiles(list([a, b]))
    expect(files).toHaveLength(2)
    expect(files[0].name).not.toBe(files[1].name)
  })
})
