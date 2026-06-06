import type { StorageSourceConfig } from '../types'

const storageSourceIDPattern = /^[A-Za-z0-9_-]+$/

function localStorageSource(): StorageSourceConfig {
  return {
    id: 'local',
    name: '本地存储',
    type: 'local',
    enabled: true,
    public_base_url: '',
    s3_endpoint: '',
    s3_region: '',
    s3_bucket: '',
    s3_prefix: '',
    s3_access_key_id: '',
    s3_access_key_secret: '',
    s3_force_path_style: false,
    s3_secret_set: false
  }
}

export function normalizeStorageSource(source: Partial<StorageSourceConfig>): StorageSourceConfig {
  const id = String(source.id || '').trim()
  const type = id === 'local' ? 'local' : String(source.type || 's3').trim()
  const defaultName = id === 'local' ? '本地存储' : (id || 'S3 存储')
  const name = String(source.name || defaultName).trim() || defaultName

  return {
    id,
    name,
    type,
    enabled: id === 'local' ? true : Boolean(source.enabled),
    public_base_url: String(source.public_base_url || '').trim(),
    s3_endpoint: String(source.s3_endpoint || '').trim(),
    s3_region: String(source.s3_region || '').trim(),
    s3_bucket: String(source.s3_bucket || '').trim(),
    s3_prefix: String(source.s3_prefix || '').trim(),
    s3_access_key_id: String(source.s3_access_key_id || '').trim(),
    s3_access_key_secret: source.s3_access_key_secret || '',
    s3_force_path_style: Boolean(source.s3_force_path_style),
    s3_secret_set: source.s3_secret_set
  }
}

export function normalizeStorageSources(sources: StorageSourceConfig[]): StorageSourceConfig[] {
  const normalized = sources.map(normalizeStorageSource)
  if (!normalized.some(source => source.id === 'local')) {
    normalized.unshift(localStorageSource())
  }
  return normalized.map(source => source.id === 'local' ? { ...source, type: 'local', enabled: true } : source)
}

export function createS3StorageSource(existing: StorageSourceConfig[]): StorageSourceConfig {
  const existingIDs = new Set(existing.map(source => source.id))
  let suffix = 1
  let id = `s3-${suffix}`
  while (existingIDs.has(id)) {
    suffix += 1
    id = `s3-${suffix}`
  }

  return normalizeStorageSource({
    id,
    name: 'S3 存储',
    type: 's3',
    enabled: false,
    s3_region: 'auto'
  })
}

export function validateStorageSources(sources: StorageSourceConfig[], defaultSource: string): string[] {
  const messages: string[] = []
  const normalized = normalizeStorageSources(sources)
  const seenIDs = new Map<string, number>()
  let hasLocal = false
  let defaultIsEnabled = false

  normalized.forEach((source, index) => {
    const label = source.name || source.id || `第 ${index + 1} 个存储源`
    if (!source.id) {
      messages.push(`${label} 缺少存储源 ID`)
      return
    }
    if (!storageSourceIDPattern.test(source.id)) {
      messages.push(`${label} 的 ID 只能包含字母、数字、横线和下划线`)
    }

    seenIDs.set(source.id, (seenIDs.get(source.id) || 0) + 1)
    if (source.id === 'local') hasLocal = true
    if (source.id === defaultSource && source.enabled) defaultIsEnabled = true

    if (source.id === 'local' && source.type !== 'local') {
      messages.push('本地存储源 local 的类型必须为 local')
    } else if (source.id !== 'local' && source.type !== 's3') {
      messages.push(`${label} 的类型暂仅支持 s3`)
    }

    if (source.type === 's3' && source.enabled) {
      if (!source.s3_bucket) messages.push(`${label} 启用时必须填写 Bucket`)
      if (!source.s3_access_key_id) messages.push(`${label} 启用时必须填写 Access Key ID`)
      if (!source.s3_access_key_secret && !source.s3_secret_set) {
        messages.push(`${label} 启用时必须填写 Access Key Secret`)
      }
    }
  })

  for (const [id, count] of seenIDs) {
    if (count > 1) messages.push(`存储源 ID ${id} 重复`)
  }
  if (!hasLocal) messages.push('必须保留本地存储源 local')
  if (!defaultIsEnabled) messages.push('默认上传源必须指向已启用的存储源')

  return messages
}
