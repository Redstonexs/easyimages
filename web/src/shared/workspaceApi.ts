import { fetchJSON } from './api'

export interface WorkspaceUser {
  id: string
  username: string
  role: 'admin' | 'user'
  quota_bytes: number
  used_bytes: number
  disabled: boolean
  has_token: boolean
}

export interface WorkspaceFolder {
  owner_id: string
  path: string
  browse_only?: boolean
}

export interface WorkspaceAsset {
  id: string
  filename: string
  folder: string
  content_type: string
  size_bytes: number
  width?: number
  height?: number
  image_id?: string
  url: string
  webp_url?: string
  legacy?: boolean
}

export interface WorkspaceTransform {
  width?: number
  height?: number
  scale?: number
  format: 'jpeg' | 'png' | 'webp'
  fit?: 'scale-down' | 'contain' | 'cover'
  quality?: number
}

export interface WorkspaceSettings {
  public_origin: string
  public_prefix: string
  effective_origin: string
}

interface UserInput {
  username: string
  password: string
  quota_bytes: number
}

const base = window.EasyImageAdmin?.workspace_api_base || '/api/v1'

function json(method: string, body: object): RequestInit {
  return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
}

function ownerQuery(owner: string) {
  return new URLSearchParams({ owner_id: owner })
}

export const workspaceApi = {
  openapiURL: `${base}/openapi.json`,
  me: () => fetchJSON<WorkspaceUser>(`${base}/me`),
  users: () => fetchJSON<{ items: WorkspaceUser[] }>(`${base}/users`),
  createUser: (input: UserInput) => fetchJSON<WorkspaceUser>(`${base}/users`, json('POST', input)),
  updateUser: (id: string, input: { quota_bytes: number; disabled: boolean; password?: string }) =>
    fetchJSON<WorkspaceUser>(`${base}/users/${id}`, json('PATCH', input)),
  folders: (owner: string) => fetchJSON<{ items: WorkspaceFolder[] }>(`${base}/folders?${ownerQuery(owner)}&include_legacy=true`),
  createFolder: (owner: string, path: string) =>
    fetchJSON<WorkspaceFolder>(`${base}/folders?${ownerQuery(owner)}`, json('POST', { path })),
  images: (owner: string, folder: string | null, cursor = '') => {
    const query = ownerQuery(owner)
    query.set('include_variants', 'true')
    query.set('include_legacy', 'true')
    if (folder !== null) query.set('folder', folder)
    if (cursor) query.set('cursor', cursor)
    return fetchJSON<{ items: WorkspaceAsset[]; next_cursor: string }>(`${base}/images?${query}`)
  },
  upload: (owner: string, folder: string, file: File) => {
    const body = new FormData()
    body.set('file', file)
    body.set('folder', folder)
    return fetchJSON<WorkspaceAsset>(`${base}/images?${ownerQuery(owner)}`, { method: 'POST', body })
  },
  image: (id: string) => fetchJSON<{ image: WorkspaceAsset; variants: WorkspaceAsset[] }>(`${base}/images/${id}`),
  createVariant: (id: string, folder: string, transform: WorkspaceTransform) =>
    fetchJSON<WorkspaceAsset>(`${base}/images/${id}/variants`, json('POST', { folder, transform })),
  deleteImage: (id: string) => fetchJSON<void>(`${base}/images/${id}`, { method: 'DELETE' }),
  deleteVariant: (id: string, variantId: string) => fetchJSON<void>(`${base}/images/${id}/variants/${variantId}`, { method: 'DELETE' }),
  settings: () => fetchJSON<WorkspaceSettings>(`${base}/settings`),
  updateSettings: (input: Pick<WorkspaceSettings, 'public_origin' | 'public_prefix'>) =>
    fetchJSON<WorkspaceSettings>(`${base}/settings`, json('PUT', input)),
  rotateToken: () => fetchJSON<{ token: string }>(`${base}/tokens`, { method: 'POST' }),
  revokeToken: () => fetchJSON<void>(`${base}/tokens`, { method: 'DELETE' })
}
