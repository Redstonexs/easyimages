export interface CaptchaData {
  type: 'disabled' | 'builtin' | 'turnstile' | 'recaptcha'
  question?: string
  token?: string
  site_key?: string
}

export interface SiteConfig {
  title: string
  description: string
  max_size: number
  api_status: number
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
  del?: string
}

export interface GalleryFile {
  name: string
  url: string
  webp_url?: string
  info_url: string
  down_url: string
}

export interface GalleryBootstrap {
  title: string
  date: string
  search: string
  limit: number
  total: number
  files: GalleryFile[]
  today: string
  yesterday: string
  date_links: Array<{ date: string; label: string }>
  extensions: string[]
}
