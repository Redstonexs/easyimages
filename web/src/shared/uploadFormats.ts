import type { UploadResult } from '../types'

export type LinkFormat = 'url' | 'markdown' | 'html' | 'bbcode'

/** Display name for a result, falling back through the two names the API may send. */
export function resultName(result: UploadResult): string {
  return result.original_name || result.srcName || ''
}

function formatOne(result: UploadResult, format: LinkFormat): string {
  const url = result.url || ''
  const alt = resultName(result)
  switch (format) {
    case 'markdown':
      return `![${alt}](${url})`
    case 'html':
      return `<img src="${url}" alt="${alt}" />`
    case 'bbcode':
      return `[img]${url}[/img]`
    default:
      return url
  }
}

const SEPARATORS: Record<LinkFormat, string> = {
  url: '\n',
  markdown: '\n\n',
  html: '\n',
  bbcode: '\n'
}

/**
 * Joins every result that actually has a URL into one copyable block.
 * Results without a URL (failed uploads) are skipped rather than emitting empty links.
 */
export function formatResults(results: UploadResult[], format: LinkFormat): string {
  return results
    .filter(result => Boolean(result.url))
    .map(result => formatOne(result, format))
    .join(SEPARATORS[format])
}
