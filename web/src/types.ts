export interface CaptchaData {
  type: 'disabled' | 'builtin' | 'turnstile' | 'recaptcha' | 'cap'
  question?: string
  token?: string
  site_key?: string
  /** Cap only: instance URL + site key, trailing slash included. */
  api_endpoint?: string
  /** Cap only: where to load the widget script from. */
  widget_url?: string
  /** Cap only: overrides the widget's default (CDN) WASM source. */
  wasm_url?: string
}

/** The <cap-widget> custom element, as far as this app uses it. */
export interface CapWidgetElement extends HTMLElement {
  reset: () => void
}

export interface SiteConfig {
  title: string
  description: string
  max_size: number
  chunk_size: number
  api_status: number
  default_storage_source: string
  storage_sources: StorageSourceOption[]
}

export interface StorageSourceOption {
  id: string
  name: string
  type: string
}

export interface UploadBootstrap {
  config: SiteConfig
  version: string
  is_admin: boolean
  must_login: number
  captcha: CaptchaData
}

export interface UploadResult {
  result?: string
  code?: number
  message?: string
  url?: string
  thumb?: string
  srcName?: string
  original_name?: string
  del?: string
  storage_source?: string
}

export type UploadStatus = 'waiting' | 'uploading' | 'processing' | 'success' | 'error'

export interface ProgressItem {
  id: number
  name: string
  size: number
  loaded: number
  status: UploadStatus
  message: string
}

export interface GalleryFile {
  name: string
  original_name?: string
  path: string
  url: string
  thumb_url: string
  webp_url?: string
  info_url: string
  down_url: string
}

export interface GalleryBootstrap {
  title: string
  date: string
  search: string
  q: string
  ext: string
  limit: number
  total: number
  files: GalleryFile[]
  today: string
  yesterday: string
  date_links: Array<{ date: string; label: string }>
  extensions: string[]
}

export interface AdminBootstrap {
  view: AdminView
  version: string
  title: string
}

export type AdminView = 'manager' | 'chart' | 'history' | 'urllist' | 'filer'

export interface AdminConfig {
  title: string
  site_icon: string
  domain: string
  imgurl: string
  maxSize: number
  extensions: string
  mustLogin: number
  compress_ratio: number
  thumbnail: number
  thumbnail_w: number
  thumbnail_h: number
  webp_convert: number
  webp_quality: number
  watermark: number
  waterText: string
  waterPosition: number
  textColor: string
  textSize: number
  textFont: string
  waterImg: string
  captcha: number
  captcha_type: number
  turnstile_site_key: string
  recaptcha_site_key: string
  cap_instance_url: string
  cap_site_key: string
  cap_widget_url: string
  cap_secret_key?: string
  hotlink_protect: number
  hotlink_domains: string
  mime: string
  storage_path: string
  time_format: string
  auto_delete: number
  default_storage_source: string
  storage_sources: StorageSourceConfig[]
  turnstile_secret_set: boolean
  recaptcha_secret_set: boolean
  cap_secret_set: boolean
}

export interface StorageSourceConfig {
  id: string
  name: string
  type: string
  enabled: boolean
  public_base_url: string
  s3_endpoint: string
  s3_region: string
  s3_bucket: string
  s3_prefix: string
  s3_access_key_id: string
  s3_access_key_secret?: string
  s3_force_path_style: boolean
  s3_secret_set?: boolean
}

export interface AdminOverview {
  version: string
  total_files: number
  used_space: number
  used_human: string
  config: AdminConfig
}

export interface AdminChart {
  version: string
  total_files: number
  used_space: number
  used_human: string
  daily_stats: Array<{ date: string; count: number }>
  format_stats: Record<string, number>
}

export interface AdminFileEntry {
  name: string
  original_name?: string
  path: string
  url: string
  thumb_url: string
  webp_url?: string
  ext?: string
  size?: number
  size_human?: string
  modified_at?: string
}

export interface AdminURLList {
  path: string
  q: string
  page: number
  page_size: number
  total: number
  total_pages: number
  files: AdminFileEntry[]
}

export interface AdminFiler {
  root_path: string
  path: string
  parent_path: string
  dirs: string[]
  files: AdminFileEntry[]
}
