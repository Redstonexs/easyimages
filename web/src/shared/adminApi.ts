import type { AdminChart, AdminConfig, AdminFiler, AdminOverview, AdminURLList, GalleryBootstrap } from '../types'
import { fetchJSON } from './api'

export const adminApi = {
  overview: () => fetchJSON<AdminOverview>('/admin/api/overview'),
  config: () => fetchJSON<AdminConfig>('/admin/api/config'),
  saveConfig: (config: AdminConfig) => fetchJSON<{ result: string; msg: string; config: AdminConfig }>('/admin/api/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config)
  }),
  batchWebP: () => fetchJSON<{ result: string; message: string; total: number; skipped: number; converted: number; failed: number }>('/admin/api/batch-webp', { method: 'POST' }),
  chart: () => fetchJSON<AdminChart>('/admin/api/chart'),
  history: (params: URLSearchParams) => fetchJSON<GalleryBootstrap>(`/admin/api/history?${params.toString()}`),
  deleteHistory: (url: string, mode = 'delete') => fetchJSON<{ code: number; msg: string }>('/admin/api/history/delete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url, mode })
  }),
  urlList: (params: URLSearchParams) => fetchJSON<AdminURLList>(`/admin/api/urllist?${params.toString()}`),
  filer: (params: URLSearchParams) => fetchJSON<AdminFiler>(`/admin/api/filer?${params.toString()}`),
  deleteFile: (url: string, mode = 'delete') => fetchJSON<{ code: number; msg: string }>('/admin/api/delete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url, mode })
  })
}
