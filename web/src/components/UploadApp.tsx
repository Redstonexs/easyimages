import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertCircle,
  Check,
  Clock,
  CloudUpload,
  Code2,
  Copy,
  Database,
  ExternalLink,
  FolderOpen,
  History,
  Images,
  Loader2,
  LogIn,
  RefreshCw,
  Settings,
  Trash2,
  Upload,
  X
} from 'lucide-react'
import type { CapWidgetElement, CaptchaData, ProgressItem, UploadBootstrap, UploadResult } from '@/types'
import { fetchJSON } from '@/shared/api'
import { extractImageFiles, isTextEntryTarget } from '@/shared/clipboardFiles'
import { createSemaphore } from '@/shared/concurrency'
import { formatSize } from '@/shared/format'
import type { LinkFormat } from '@/shared/uploadFormats'
import { formatResults, resultName } from '@/shared/uploadFormats'
import { useCopy } from '@/shared/useCopy'
import { notifyToast } from '@/shared/useNotices'
import { SiteHeader } from '@/components/SiteHeader'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Toaster } from '@/components/ui/sonner'

const DEFAULT_CHUNK_SIZE = 16 * 1024 * 1024
const MAX_CONCURRENT_CHUNKS = 4
const MAX_CONCURRENT_FILES = 3
const MAX_CONCURRENT_REQUESTS = 6
const MAX_RETRIES = 3

const requestSlots = createSemaphore(MAX_CONCURRENT_REQUESTS)

const COPY_FORMATS: Array<{ format: LinkFormat; label: string; shortLabel: string }> = [
  { format: 'url', label: '全部直链', shortLabel: '直链' },
  { format: 'markdown', label: '全部 Markdown', shortLabel: 'Markdown' },
  { format: 'html', label: '全部 HTML', shortLabel: 'HTML' },
  { format: 'bbcode', label: '全部 BBCode', shortLabel: 'BBCode' }
]

function formatSingleLink(result: UploadResult, format: LinkFormat): string {
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

function resolveUpload(
  xhr: XMLHttpRequest,
  resolve: (value: UploadResult) => void,
  reject: (reason: Error) => void,
  fallback = '服务器拒绝'
) {
  if (xhr.status >= 200 && xhr.status < 300) {
    try {
      const data = JSON.parse(xhr.responseText) as UploadResult
      if (data.result === 'success' || data.code === 200 || data.url) resolve(data)
      else reject(new Error(data.message || fallback))
    } catch {
      reject(new Error('响应解析错误'))
    }
    return
  }

  let message = `HTTP ${xhr.status}`
  try {
    const data = JSON.parse(xhr.responseText) as UploadResult
    message = data.message || message
  } catch {
    // Keep status fallback.
  }
  reject(new Error(message))
}

function sendChunk(formData: FormData, onProgress: (loaded: number) => void, fallback = '分片上传失败'): Promise<UploadResult> {
  return requestSlots.run(() => new Promise<UploadResult>((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.upload.addEventListener('progress', event => {
      if (event.lengthComputable) onProgress(event.loaded)
    })
    xhr.addEventListener('load', () => resolveUpload(xhr, resolve, reject, fallback))
    xhr.addEventListener('error', () => reject(new Error('网络错误')))
    xhr.addEventListener('timeout', () => reject(new Error('分片上传超时')))
    xhr.timeout = 120000
    xhr.open('POST', '/app/upload/chunk')
    xhr.send(formData)
  }))
}

function progressPercent(item: ProgressItem): number {
  if (item.size === 0) return 0
  const percent = Math.round((item.loaded / item.size) * 100)
  return item.status === 'success' ? percent : Math.min(percent, 99)
}

function dragHasFiles(event: DragEvent | React.DragEvent): boolean {
  return Array.from(event.dataTransfer?.types || []).includes('Files')
}

interface UploadResultCardProps {
  result: UploadResult
  activeFormat: LinkFormat
  onCopy: (value: string, successMessage?: string) => Promise<void>
  onPreview: (result: UploadResult) => void
}

function UploadResultCard({ result, activeFormat, onCopy, onPreview }: UploadResultCardProps) {
  const [format, setFormat] = useState<LinkFormat>(activeFormat)
  const [copiedKey, setCopiedKey] = useState<string>('')

  useEffect(() => {
    setFormat(activeFormat)
  }, [activeFormat])

  const formatted = useMemo(() => formatSingleLink(result, format), [result, format])
  const name = resultName(result)

  async function handleCopy(value: string, key: string) {
    await onCopy(value)
    setCopiedKey(key)
    window.setTimeout(() => setCopiedKey(curr => (curr === key ? '' : curr)), 1600)
  }

  return (
    <Card className="overflow-hidden py-0 transition-shadow hover:shadow-sm">
      <CardContent className="flex flex-col gap-4 p-4 sm:flex-row sm:items-start">
        <button
          type="button"
          onClick={() => onPreview(result)}
          className="checker-well group relative h-28 w-full shrink-0 overflow-hidden rounded-lg border sm:size-28 cursor-pointer"
          title="放大预览"
        >
          <img
            src={result.thumb || result.url}
            alt={name || '上传预览'}
            loading="lazy"
            decoding="async"
            className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
          />
        </button>

        <div className="flex min-w-0 flex-1 flex-col gap-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium text-foreground" title={name}>
                {name || '未命名图片'}
              </p>
              {result.storage_source && (
                <p className="text-xs text-muted-foreground">存储源：{result.storage_source}</p>
              )}
            </div>
            <div className="flex items-center gap-1.5">
              {result.url && (
                <Button variant="ghost" size="xs" asChild>
                  <a href={result.url} target="_blank" rel="noopener noreferrer">
                    <ExternalLink className="size-3.5" />
                    打开
                  </a>
                </Button>
              )}
              {result.del && (
                <Button variant="outline" size="xs" asChild>
                  <a href={result.del}>
                    <Trash2 className="size-3.5" />
                    删除
                  </a>
                </Button>
              )}
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-1">
            {COPY_FORMATS.map(option => (
              <button
                key={option.format}
                type="button"
                onClick={() => setFormat(option.format)}
                className={
                  format === option.format
                    ? 'rounded-md bg-primary px-2.5 py-1 text-xs font-medium text-primary-foreground transition-colors cursor-pointer'
                    : 'rounded-md bg-muted px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground cursor-pointer'
                }
              >
                {option.shortLabel}
              </button>
            ))}
          </div>

          <div className="flex items-center gap-2">
            <Input
              readOnly
              value={formatted}
              aria-label={`${format} 链接`}
              className="font-mono text-xs bg-muted/40"
              onFocus={event => event.currentTarget.select()}
            />
            <Button
              type="button"
              size="sm"
              onClick={() => void handleCopy(formatted, format)}
              className="shrink-0"
            >
              {copiedKey === format ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
              {copiedKey === format ? '已复制' : '复制'}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

export default function UploadApp({ bootstrap }: { bootstrap: UploadBootstrap }) {
  const copy = useMemo(() => useCopy(notifyToast), [])

  const fileInputRef = useRef<HTMLInputElement>(null)
  const capWidgetRef = useRef<CapWidgetElement>(null)
  const [isDragging, setIsDragging] = useState(false)
  const [isWindowDragging, setIsWindowDragging] = useState(false)
  const [isUploading, setIsUploading] = useState(false)
  const [progressItems, setProgressItems] = useState<ProgressItem[]>([])
  const [results, setResults] = useState<UploadResult[]>([])
  const [totalSize, setTotalSize] = useState(0)
  const [defaultFormat, setDefaultFormat] = useState<LinkFormat>('url')
  const [previewResult, setPreviewResult] = useState<UploadResult | null>(null)
  const [selectedStorageSource, setSelectedStorageSource] = useState(
    bootstrap.config.default_storage_source || bootstrap.config.storage_sources[0]?.id || 'local'
  )

  const storageSourceRef = useRef(selectedStorageSource)
  storageSourceRef.current = selectedStorageSource

  const queueRef = useRef<File[]>([])
  const queueIdsRef = useRef<number[]>([])
  const queueCursorRef = useRef(0)
  const activeWorkersRef = useRef(0)
  const nextItemIdRef = useRef(1)
  const dragDepthRef = useRef(0)
  const uploadingRef = useRef(false)

  // Login modal state
  const [showLogin, setShowLogin] = useState(false)
  const [loginUser, setLoginUser] = useState('')
  const [loginPass, setLoginPass] = useState('')
  const [loginError, setLoginError] = useState('')
  const [loggingIn, setLoggingIn] = useState(false)
  const [captcha, setCaptcha] = useState<CaptchaData>(bootstrap.captcha)
  const [captchaAnswer, setCaptchaAnswer] = useState('')
  const [externalCaptchaToken, setExternalCaptchaToken] = useState('')
  const turnstileRenderedRef = useRef(false)

  const maxSizeLabel = useMemo(() => formatSize(bootstrap.config.max_size), [bootstrap.config.max_size])
  const chunkSize = bootstrap.config.chunk_size > 0 ? bootstrap.config.chunk_size : DEFAULT_CHUNK_SIZE
  const uploadedTotal = useMemo(() => progressItems.reduce((sum, item) => sum + item.loaded, 0), [progressItems])
  const settledCount = useMemo(
    () => progressItems.filter(item => item.status === 'success' || item.status === 'error').length,
    [progressItems]
  )
  const copyableResults = useMemo(() => results.filter(result => Boolean(result.url)), [results])

  const overallPercent = useMemo(() => {
    if (totalSize === 0) return 0
    const percent = Math.round((uploadedTotal / totalSize) * 100)
    return isUploading && percent >= 100 ? 99 : Math.min(100, percent)
  }, [totalSize, uploadedTotal, isUploading])

  const overallInfo = useMemo(() => {
    if (progressItems.length === 0) return '准备上传...'
    if (!isUploading) {
      const success = progressItems.filter(item => item.status === 'success').length
      const failed = progressItems.filter(item => item.status === 'error').length
      if (failed === 0) return `全部完成：${success} 个文件上传成功`
      if (success === 0) return `上传失败：${failed} 个文件`
      return `${success} 个成功，${failed} 个失败`
    }
    const processingItem = progressItems.find(item => item.status === 'processing')
    if (processingItem) return processingItem.message
    return `上传中 (${settledCount}/${progressItems.length}) · ${formatSize(uploadedTotal)} / ${formatSize(totalSize)}`
  }, [progressItems, isUploading, settledCount, uploadedTotal, totalSize])

  const patchProgressItem = useCallback((id: number, patch: Partial<ProgressItem>) => {
    setProgressItems(prev => prev.map(item => (item.id === id ? { ...item, ...patch } : item)))
  }, [])

  const uploadFileWhole = useCallback((
    file: File,
    onProgress: (loaded: number) => void,
    onProcessing: (message: string) => void
  ): Promise<UploadResult> => {
    return requestSlots.run(() => new Promise<UploadResult>((resolve, reject) => {
      const formData = new FormData()
      formData.append('file', file)
      formData.append('storage_source', storageSourceRef.current)
      const xhr = new XMLHttpRequest()
      xhr.upload.addEventListener('progress', event => {
        if (!event.lengthComputable) return
        onProgress(Math.min(event.loaded, file.size))
        if (event.loaded >= event.total) onProcessing('文件已传完，服务器处理中...')
      })
      xhr.addEventListener('load', () => resolveUpload(xhr, resolve, reject))
      xhr.addEventListener('error', () => reject(new Error('网络错误')))
      xhr.addEventListener('timeout', () => reject(new Error('上传超时')))
      xhr.timeout = 120000
      xhr.open('POST', '/app/upload')
      xhr.send(formData)
    }))
  }, [])

  const uploadFileChunked = useCallback(async (
    file: File,
    onProgress: (loaded: number) => void,
    onProcessing: (message: string) => void
  ): Promise<UploadResult> => {
    const totalChunks = Math.ceil(file.size / chunkSize)
    const uploadId = crypto.randomUUID ? crypto.randomUUID() : `upload-${Date.now()}-${Math.random().toString(16).slice(2)}`
    const chunkProgress = new Array<number>(totalChunks).fill(0)
    const source = storageSourceRef.current
    let nextChunk = 0
    let failed = false

    const updateChunkProgress = () => onProgress(chunkProgress.reduce((sum, value) => sum + value, 0))
    const workers = Array.from({ length: Math.min(MAX_CONCURRENT_CHUNKS, totalChunks) }, async () => {
      while (!failed) {
        const chunkIndex = nextChunk++
        if (chunkIndex >= totalChunks) break

        const start = chunkIndex * chunkSize
        const end = Math.min(start + chunkSize, file.size)
        let lastError: Error | null = null

        for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
          if (failed) break
          if (attempt > 0) await new Promise(resolve => window.setTimeout(resolve, 500 * 2 ** (attempt - 1)))

          const formData = new FormData()
          formData.append('chunk', file.slice(start, end), file.name)
          formData.append('uploadId', uploadId)
          formData.append('chunkIndex', chunkIndex.toString())
          formData.append('totalChunks', totalChunks.toString())
          formData.append('filename', file.name)
          formData.append('storage_source', source)

          try {
            await sendChunk(formData, loaded => {
              chunkProgress[chunkIndex] = Math.min(loaded, end - start)
              updateChunkProgress()
            })
            chunkProgress[chunkIndex] = end - start
            updateChunkProgress()
            lastError = null
            break
          } catch (error) {
            lastError = error instanceof Error ? error : new Error('分片上传失败')
            chunkProgress[chunkIndex] = 0
            updateChunkProgress()
          }
        }

        if (lastError) {
          failed = true
          throw lastError
        }
      }
    })

    try {
      await Promise.all(workers)
    } catch (error) {
      const cleanup = new FormData()
      cleanup.append('uploadId', uploadId)
      cleanup.append('storage_source', source)
      navigator.sendBeacon?.('/app/upload/chunk/cleanup', cleanup)
      throw error
    }

    const mergeData = new FormData()
    mergeData.append('uploadId', uploadId)
    mergeData.append('totalChunks', totalChunks.toString())
    mergeData.append('filename', file.name)
    mergeData.append('merge', 'true')
    mergeData.append('storage_source', source)
    onProgress(file.size)
    onProcessing('文件已传完，服务器合并处理中...')
    return await sendChunk(mergeData, () => undefined, '分片合并失败')
  }, [chunkSize])

  const finishBatch = useCallback((finalItems: ProgressItem[]) => {
    uploadingRef.current = false
    setIsUploading(false)
    if (fileInputRef.current) fileInputRef.current.value = ''
    const success = finalItems.filter(item => item.status === 'success').length
    const failed = finalItems.filter(item => item.status === 'error').length
    if (failed === 0 && success > 0) notifyToast(`成功上传 ${success} 张图片`, 'success')
    else if (success > 0) notifyToast(`${success} 个文件上传成功，${failed} 个失败`, 'warning')
    else if (failed > 0) notifyToast('上传失败', 'danger')
  }, [])

  const runWorker = useCallback(async () => {
    while (queueCursorRef.current < queueRef.current.length) {
      const index = queueCursorRef.current++
      const file = queueRef.current[index]
      const itemId = queueIdsRef.current[index]
      patchProgressItem(itemId, { status: 'uploading' })

      try {
        const result = file.size > chunkSize
          ? await uploadFileChunked(
              file,
              loaded => patchProgressItem(itemId, {
                loaded: Math.min(loaded, file.size),
                message: `${formatSize(loaded)} / ${formatSize(file.size)}`
              }),
              message => patchProgressItem(itemId, { status: 'processing', loaded: file.size, message })
            )
          : await uploadFileWhole(
              file,
              loaded => patchProgressItem(itemId, {
                loaded: Math.min(loaded, file.size),
                message: `${formatSize(loaded)} / ${formatSize(file.size)}`
              }),
              message => patchProgressItem(itemId, { status: 'processing', loaded: file.size, message })
            )
        patchProgressItem(itemId, { status: 'success', loaded: file.size, message: formatSize(file.size) })
        if (result.url) setResults(prev => [result, ...prev])
      } catch (error) {
        patchProgressItem(itemId, {
          status: 'error',
          loaded: file.size,
          message: error instanceof Error ? error.message : '上传失败'
        })
      }
    }

    activeWorkersRef.current--
    if (activeWorkersRef.current === 0) {
      setProgressItems(current => {
        finishBatch(current)
        return current
      })
    }
  }, [chunkSize, finishBatch, patchProgressItem, uploadFileChunked, uploadFileWhole])

  const handleFiles = useCallback((fileList: FileList | File[]) => {
    const accepted: File[] = []
    for (const file of Array.from(fileList)) {
      if (file.size > bootstrap.config.max_size) {
        notifyToast(`"${file.name}" 超过大小限制 (${maxSizeLabel})`, 'warning')
        continue
      }
      accepted.push(file)
    }
    if (accepted.length === 0) return

    const freshBatch = !uploadingRef.current
    if (freshBatch) {
      queueRef.current = []
      queueIdsRef.current = []
      queueCursorRef.current = 0
    }

    const newItems: ProgressItem[] = []
    for (const file of accepted) {
      const id = nextItemIdRef.current++
      queueRef.current.push(file)
      queueIdsRef.current.push(id)
      newItems.push({
        id,
        name: file.name,
        size: file.size,
        loaded: 0,
        status: 'waiting',
        message: formatSize(file.size)
      })
    }

    const addedSize = accepted.reduce((sum, file) => sum + file.size, 0)
    setProgressItems(prev => (freshBatch ? newItems : [...prev, ...newItems]))
    setTotalSize(prev => (freshBatch ? addedSize : prev + addedSize))
    uploadingRef.current = true
    setIsUploading(true)

    while (activeWorkersRef.current < MAX_CONCURRENT_FILES && queueCursorRef.current < queueRef.current.length) {
      activeWorkersRef.current++
      void runWorker()
    }
  }, [bootstrap.config.max_size, maxSizeLabel, runWorker])

  useEffect(() => {
    const onDragEnter = (event: DragEvent) => {
      if (!dragHasFiles(event)) return
      dragDepthRef.current++
      setIsWindowDragging(true)
    }
    const onDragLeave = (event: DragEvent) => {
      if (!dragHasFiles(event)) return
      dragDepthRef.current = Math.max(0, dragDepthRef.current - 1)
      if (dragDepthRef.current === 0) {
        setIsWindowDragging(false)
        setIsDragging(false)
      }
    }
    const onDragOver = (event: DragEvent) => {
      event.preventDefault()
    }
    const onDrop = (event: DragEvent) => {
      event.preventDefault()
      dragDepthRef.current = 0
      setIsDragging(false)
      setIsWindowDragging(false)
      if (event.dataTransfer?.files?.length) handleFiles(event.dataTransfer.files)
    }
    const onPaste = (event: ClipboardEvent) => {
      if (isTextEntryTarget(event.target)) return
      const files = extractImageFiles(event.clipboardData?.items)
      if (files.length === 0) return
      event.preventDefault()
      handleFiles(files)
    }

    window.addEventListener('dragenter', onDragEnter)
    window.addEventListener('dragover', onDragOver)
    window.addEventListener('dragleave', onDragLeave)
    window.addEventListener('drop', onDrop)
    document.addEventListener('paste', onPaste)
    return () => {
      window.removeEventListener('dragenter', onDragEnter)
      window.removeEventListener('dragover', onDragOver)
      window.removeEventListener('dragleave', onDragLeave)
      window.removeEventListener('drop', onDrop)
      document.removeEventListener('paste', onPaste)
    }
  }, [handleFiles])

  const refreshCaptcha = useCallback(async () => {
    try {
      const next = await fetchJSON<CaptchaData>('/api/captcha')
      setCaptcha(next)
      setCaptchaAnswer('')
    } catch {
      // Keep current captcha on transient errors.
    }
  }, [])

  useEffect(() => {
    if (bootstrap.must_login === 1 && bootstrap.captcha.type === 'builtin') {
      void refreshCaptcha()
    }
  }, [bootstrap.must_login, bootstrap.captcha.type, refreshCaptcha])

  // Wire custom element events for <cap-widget> when dialog opens
  useEffect(() => {
    if (!showLogin || captcha.type !== 'cap') return
    const el = capWidgetRef.current
    if (!el) return
    const onSolve = (event: Event) => {
      setExternalCaptchaToken((event as CustomEvent<{ token?: string }>).detail?.token || '')
    }
    const onError = (event: Event) => {
      setExternalCaptchaToken('')
      const detail = (event as CustomEvent<{ message?: string }>).detail
      setLoginError(detail?.message || '人机验证加载失败，请刷新页面重试')
    }
    el.addEventListener('solve', onSolve)
    el.addEventListener('error', onError)
    return () => {
      el.removeEventListener('solve', onSolve)
      el.removeEventListener('error', onError)
    }
  }, [showLogin, captcha.type])

  // Render Turnstile / reCAPTCHA / builtin when login dialog opens
  useEffect(() => {
    if (!showLogin) {
      turnstileRenderedRef.current = false
      return
    }
    let cancelled = false
    if (captcha.type === 'turnstile' && captcha.site_key) {
      const render = () => {
        if (cancelled) return
        if (!window.turnstile || !document.getElementById('login-turnstile-container')) {
          window.setTimeout(render, 100)
          return
        }
        if (turnstileRenderedRef.current) {
          window.turnstile.reset('#login-turnstile-container')
          return
        }
        window.turnstile.render('#login-turnstile-container', {
          sitekey: captcha.site_key || '',
          callback: token => setExternalCaptchaToken(token)
        })
        turnstileRenderedRef.current = true
      }
      render()
    } else if (captcha.type === 'recaptcha' && captcha.site_key) {
      const execute = () => {
        if (cancelled) return
        if (!window.grecaptcha?.ready) {
          window.setTimeout(execute, 100)
          return
        }
        window.grecaptcha.ready(() => {
          void window.grecaptcha?.execute(captcha.site_key || '', { action: 'login' }).then(token => {
            if (!cancelled) setExternalCaptchaToken(token)
          })
        })
      }
      execute()
    } else if (captcha.type === 'builtin') {
      void refreshCaptcha()
    }
    return () => {
      cancelled = true
    }
  }, [showLogin, captcha.type, captcha.site_key, refreshCaptcha])

  function openAdmin() {
    if (bootstrap.is_admin) {
      window.location.href = '/admin/manager'
      return
    }
    setLoginError('')
    setShowLogin(true)
  }

  function resetCaptchaAfterFailure() {
    if (captcha.type === 'builtin') {
      void refreshCaptcha()
    } else if (captcha.type === 'cap') {
      setExternalCaptchaToken('')
      capWidgetRef.current?.reset()
    } else if (captcha.type === 'turnstile' && turnstileRenderedRef.current) {
      setExternalCaptchaToken('')
      window.turnstile?.reset('#login-turnstile-container')
    }
  }

  async function handleLogin(event: React.FormEvent) {
    event.preventDefault()
    setLoginError('')
    if (!loginUser.trim() || !loginPass) {
      setLoginError('请输入用户名和密码')
      return
    }

    const body = new URLSearchParams()
    body.set('user', loginUser.trim())
    body.set('password', loginPass)
    if (captcha.type === 'builtin') {
      body.set('captcha_answer', captchaAnswer)
      body.set('captcha_token', captcha.token || '')
    } else if (captcha.type === 'turnstile') {
      body.set('cf_turnstile_response', externalCaptchaToken)
    } else if (captcha.type === 'recaptcha') {
      body.set('g_recaptcha_response', externalCaptchaToken)
    } else if (captcha.type === 'cap') {
      body.set('cap-token', externalCaptchaToken)
    }

    setLoggingIn(true)
    try {
      const result = await fetchJSON<{ result: string; message?: string; redirect?: string }>('/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body
      })
      if (result.result === 'success') {
        window.location.href = result.redirect || '/admin/manager'
      } else {
        setLoginError(result.message || '登录失败')
        resetCaptchaAfterFailure()
      }
    } catch (error) {
      setLoginError(error instanceof Error ? error.message : '登录失败')
      resetCaptchaAfterFailure()
    } finally {
      setLoggingIn(false)
    }
  }

  function copyAll(format: LinkFormat) {
    setDefaultFormat(format)
    void copy(formatResults(results, format), `已复制 ${copyableResults.length} 条链接`)
  }

  return (
    <div className="min-h-screen flex flex-col bg-background text-foreground">
      <Toaster />

      {isWindowDragging && (
        <div
          aria-hidden="true"
          className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm pointer-events-none"
        >
          <div className="viewfinder-frame flex flex-col items-center gap-3 rounded-2xl border-2 border-dashed border-foreground bg-card px-14 py-12 shadow-xl" data-active="true">
            <CloudUpload className="size-10 text-foreground" />
            <p className="text-lg font-semibold tracking-tight">松开即可开始上传</p>
            <p className="text-xs text-muted-foreground">单文件最大 {maxSizeLabel}</p>
          </div>
        </div>
      )}

      <SiteHeader
        brand={bootstrap.config.title || 'EasyImage'}
        brandHref="/"
        right={(
          <nav className="flex items-center gap-1" aria-label="主导航">
            <Button variant="ghost" size="sm" asChild>
              <a href="/app/list">
                <Images className="size-4" />
                <span className="hidden sm:inline">图床广场</span>
              </a>
            </Button>
            {bootstrap.is_admin && (
              <>
                <Button variant="ghost" size="sm" asChild>
                  <a href="/admin/workspace">
                    <FolderOpen className="size-4" />
                    <span className="hidden md:inline">图片库</span>
                  </a>
                </Button>
                <Button variant="ghost" size="sm" asChild>
                  <a href="/admin/history">
                    <History className="size-4" />
                    <span className="hidden md:inline">历史</span>
                  </a>
                </Button>
              </>
            )}
            {bootstrap.config.api_status === 1 && (
              <Button variant="ghost" size="sm" asChild>
                <a href="/api/">
                  <Code2 className="size-4" />
                  <span className="hidden md:inline">API</span>
                </a>
              </Button>
            )}
            <Button variant={bootstrap.is_admin ? 'outline' : 'default'} size="sm" onClick={openAdmin}>
              {bootstrap.is_admin ? <Settings className="size-4" /> : <LogIn className="size-4" />}
              <span>{bootstrap.is_admin ? '管理后台' : '登录'}</span>
            </Button>
          </nav>
        )}
      />

      <main className="mx-auto flex w-full max-w-4xl flex-1 flex-col gap-8 px-4 py-10 sm:px-6 sm:py-14">
        <header className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div className="space-y-1.5">
            <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">
              {bootstrap.config.title}
            </h1>
            {bootstrap.config.description && (
              <p className="max-w-xl text-sm text-muted-foreground leading-relaxed">
                {bootstrap.config.description}
              </p>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <Badge variant="outline" className="font-mono font-normal">
              单文件 ≤ {maxSizeLabel}
            </Badge>
            <Badge variant="secondary" className="font-normal">
              支持 Ctrl+V 粘贴截图
            </Badge>
          </div>
        </header>

        <section
          role="button"
          tabIndex={0}
          aria-label="选择或拖拽图片上传"
          data-active={isDragging || isWindowDragging}
          onClick={() => fileInputRef.current?.click()}
          onKeyDown={event => {
            if (event.key === 'Enter' || event.key === ' ') {
              event.preventDefault()
              fileInputRef.current?.click()
            }
          }}
          onDragOver={event => {
            event.preventDefault()
            if (dragHasFiles(event)) setIsDragging(true)
          }}
          onDragLeave={event => {
            const next = event.relatedTarget as Node | null
            if (next && event.currentTarget.contains(next)) return
            setIsDragging(false)
          }}
          className={`viewfinder-frame group flex flex-col items-center justify-center gap-4 rounded-2xl border border-dashed px-6 py-14 text-center transition-all outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer ${
            isDragging
              ? 'border-foreground bg-muted/70'
              : isUploading
                ? 'border-foreground/50 bg-muted/30'
                : 'border-border bg-card hover:border-foreground/50 hover:bg-muted/25'
          }`}
        >
          <div className="grid size-12 place-items-center rounded-full border bg-background shadow-xs transition-transform group-hover:-translate-y-0.5">
            {isUploading ? (
              <Loader2 className="size-5 animate-spin text-foreground" />
            ) : (
              <Upload className="size-5 text-foreground" />
            )}
          </div>
          <div className="space-y-1">
            <h2 className="text-base font-medium tracking-tight">
              {isUploading ? '正在上传，可继续拖入新图片' : '拖拽图片到这里，或点击选择文件'}
            </h2>
            <p className="text-xs text-muted-foreground">
              支持 JPG、PNG、GIF、WebP、SVG 等常见格式 · 粘贴剪贴板图片即刻上传
            </p>
          </div>

          <input
            ref={fileInputRef}
            type="file"
            multiple
            accept="image/*"
            className="sr-only"
            onChange={event => {
              if (event.target.files) handleFiles(event.target.files)
              event.target.value = ''
            }}
          />

          <div className="mt-1 flex flex-wrap items-center justify-center gap-2.5" onClick={event => event.stopPropagation()}>
            <Button type="button" size="default" onClick={() => fileInputRef.current?.click()}>
              <Upload className="size-4" />
              选择图片
            </Button>

            {bootstrap.config.storage_sources.length > 1 && (
              <div className="flex items-center gap-2 rounded-md border bg-background px-2.5 py-1">
                <Database className="size-3.5 text-muted-foreground shrink-0" />
                <Select
                  value={selectedStorageSource}
                  onValueChange={setSelectedStorageSource}
                  disabled={isUploading}
                >
                  <SelectTrigger
                    aria-label="存储位置"
                    size="sm"
                    className="h-7 border-0 bg-transparent px-1 shadow-none focus-visible:ring-0"
                  >
                    <SelectValue placeholder="选择存储源" />
                  </SelectTrigger>
                  <SelectContent>
                    {bootstrap.config.storage_sources.map(source => (
                      <SelectItem key={source.id} value={source.id}>
                        {source.name} ({source.type === 's3' ? 'S3' : '本地'})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
          </div>
        </section>

        {progressItems.length > 0 && (
          <Card aria-live="polite" className="gap-4 py-4">
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-2 text-sm">
                  <span className="font-medium">{overallInfo}</span>
                  <span className="font-mono text-xs tabular-nums text-muted-foreground">{overallPercent}%</span>
                </div>
                <Progress value={overallPercent} />
              </div>

              <div className="divide-y rounded-lg border bg-muted/20">
                {progressItems.map(item => (
                  <div key={item.id} className="flex flex-col gap-1.5 px-3.5 py-2.5">
                    <div className="flex items-center justify-between gap-3 text-xs">
                      <span className="flex min-w-0 items-center gap-2 font-medium">
                        {item.status === 'success' && <Check className="size-3.5 shrink-0" />}
                        {item.status === 'error' && <AlertCircle className="size-3.5 shrink-0" />}
                        {(item.status === 'uploading' || item.status === 'processing') && (
                          <Loader2 className="size-3.5 shrink-0 animate-spin" />
                        )}
                        {item.status === 'waiting' && <Clock className="size-3.5 shrink-0 text-muted-foreground" />}
                        <span className="truncate" title={item.name}>{item.name}</span>
                      </span>
                      <span className="shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground">
                        {item.message}
                      </span>
                    </div>
                    <Progress value={progressPercent(item)} className="h-1" />
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        )}

        {results.length > 0 && (
          <section className="space-y-4" aria-label="上传结果">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex items-center gap-2">
                <h2 className="text-base font-semibold tracking-tight">上传结果</h2>
                <Badge variant="secondary" className="font-mono tabular-nums">
                  {copyableResults.length}
                </Badge>
              </div>
              <div className="flex flex-wrap items-center gap-1.5">
                {copyableResults.length > 1 && COPY_FORMATS.map(option => (
                  <Button
                    key={option.format}
                    type="button"
                    size="xs"
                    variant={defaultFormat === option.format ? 'default' : 'outline'}
                    onClick={() => copyAll(option.format)}
                  >
                    <Copy className="size-3" />
                    {option.label}
                  </Button>
                ))}
                <Button type="button" size="xs" variant="ghost" onClick={() => setResults([])}>
                  <X className="size-3.5" />
                  清空
                </Button>
              </div>
            </div>

            <div className="grid gap-3">
              {results.map((result, index) => (
                <UploadResultCard
                  key={`${result.url || 'item'}-${index}`}
                  result={result}
                  activeFormat={defaultFormat}
                  onCopy={copy}
                  onPreview={setPreviewResult}
                />
              ))}
            </div>
          </section>
        )}
      </main>

      <footer className="border-t py-6 text-center text-xs text-muted-foreground">
        <p>
          Powered by{' '}
          <a
            href="https://github.com/Redstonexs/easyimages"
            target="_blank"
            rel="noopener noreferrer"
            className="font-medium text-foreground underline-offset-4 hover:underline"
          >
            EasyImage
          </a>{' '}
          <span className="font-mono">v{bootstrap.version}</span>
        </p>
      </footer>

      {/* Image Preview Dialog */}
      <Dialog open={Boolean(previewResult)} onOpenChange={open => { if (!open) setPreviewResult(null) }}>
        <DialogContent className="sm:max-w-3xl p-4">
          {previewResult && (
            <>
              <DialogHeader>
                <DialogTitle className="truncate pr-8 text-sm font-medium">
                  {resultName(previewResult) || '图片预览'}
                </DialogTitle>
                <DialogDescription className="truncate font-mono text-xs">
                  {previewResult.url}
                </DialogDescription>
              </DialogHeader>
              <div className="checker-well flex max-h-[70vh] items-center justify-center overflow-hidden rounded-lg border p-2">
                <img
                  src={previewResult.url}
                  alt={resultName(previewResult)}
                  className="max-h-[66vh] max-w-full rounded object-contain"
                />
              </div>
              <DialogFooter>
                <Button
                  type="button"
                  size="sm"
                  onClick={() => void copy(previewResult.url || '')}
                >
                  <Copy className="size-3.5" />
                  复制直链
                </Button>
                <Button type="button" size="sm" variant="outline" asChild>
                  <a href={previewResult.url} target="_blank" rel="noopener noreferrer">
                    <ExternalLink className="size-3.5" />
                    新窗口打开
                  </a>
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* Account Login Dialog */}
      <Dialog open={showLogin} onOpenChange={setShowLogin}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>账号登录</DialogTitle>
            <DialogDescription>输入管理员或图片库租户账号以继续。</DialogDescription>
          </DialogHeader>

          <form onSubmit={handleLogin} className="space-y-4">
            {loginError && (
              <Alert variant="emphasis">
                <AlertCircle className="size-4" />
                <AlertDescription>{loginError}</AlertDescription>
              </Alert>
            )}

            <div className="space-y-2">
              <Label htmlFor="login-user">用户名</Label>
              <Input
                id="login-user"
                value={loginUser}
                onChange={event => setLoginUser(event.target.value)}
                autoComplete="username"
                placeholder="输入用户名"
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="login-pass">密码</Label>
              <Input
                id="login-pass"
                type="password"
                value={loginPass}
                onChange={event => setLoginPass(event.target.value)}
                autoComplete="current-password"
                placeholder="输入密码"
                required
              />
            </div>

            {captcha.type === 'builtin' && (
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label htmlFor="captcha-answer">验证码</Label>
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    onClick={() => void refreshCaptcha()}
                  >
                    <RefreshCw className="size-3" />
                    换一题
                  </Button>
                </div>
                <div className="rounded-md border bg-muted/50 px-3 py-2 text-center font-mono text-base font-semibold tracking-widest select-none">
                  {captcha.question}
                </div>
                <Input
                  id="captcha-answer"
                  value={captchaAnswer}
                  onChange={event => setCaptchaAnswer(event.target.value)}
                  autoComplete="off"
                  placeholder="输入计算结果"
                  required
                />
              </div>
            )}

            {captcha.type === 'turnstile' && (
              <div id="login-turnstile-container" className="min-h-16" />
            )}

            {captcha.type === 'cap' && (
              <cap-widget
                ref={capWidgetRef as React.RefObject<HTMLElement>}
                data-cap-api-endpoint={captcha.api_endpoint}
              />
            )}

            <DialogFooter className="pt-2">
              <Button type="button" variant="outline" onClick={() => setShowLogin(false)}>
                取消
              </Button>
              <Button type="submit" disabled={loggingIn}>
                {loggingIn ? <Loader2 className="size-4 animate-spin" /> : <LogIn className="size-4" />}
                {loggingIn ? '登录中...' : '登录'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
