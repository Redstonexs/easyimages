import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Copy,
  Download,
  ExternalLink,
  ImageOff,
  Info,
  Loader2,
  Search,
  Upload,
  X
} from 'lucide-react'
import type { GalleryBootstrap, GalleryFile } from '@/types'
import { fetchJSON } from '@/shared/api'
import { parseGalleryParams } from '@/shared/galleryParams'
import { useCopy } from '@/shared/useCopy'
import { notifyToast } from '@/shared/useNotices'
import { SiteHeader } from '@/components/SiteHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Toaster } from '@/components/ui/sonner'

function displayName(file: GalleryFile): string {
  return file.original_name || file.name
}

export default function GalleryApp({ bootstrap }: { bootstrap: GalleryBootstrap }) {
  const copy = useMemo(() => useCopy(notifyToast), [])
  const [gallery, setGallery] = useState<GalleryBootstrap>(() => ({ ...bootstrap }))
  const [loading, setLoading] = useState(false)
  const [selectedImage, setSelectedImage] = useState<GalleryFile | null>(null)
  const [failedThumbs, setFailedThumbs] = useState<Set<string>>(() => new Set())
  const [queryInput, setQueryInput] = useState(bootstrap.q || '')

  const inflightRef = useRef<AbortController | null>(null)
  const galleryRef = useRef(gallery)
  galleryRef.current = gallery
  const queryInputRef = useRef(queryInput)
  queryInputRef.current = queryInput

  const activeDate = gallery.date
  const activeExt = gallery.ext || gallery.search

  const buildQuery = useCallback((next: Partial<{ date: string; ext: string; q: string; num: number }>) => {
    const current = galleryRef.current
    const params = new URLSearchParams()
    const date = next.date ?? current.date
    const ext = next.ext ?? (current.ext || current.search)
    const q = next.q ?? current.q
    const num = next.num ?? current.limit
    if (date && date !== current.today) params.set('date', date)
    if (ext) params.set('ext', ext)
    if (q) params.set('q', q)
    if (num !== current.limit) params.set('num', String(num))
    return params
  }, [])

  const loadGallery = useCallback(async (
    next: Partial<{ date: string; ext: string; q: string; num: number }>,
    options: { push?: boolean; params?: URLSearchParams } = {}
  ) => {
    const { push = true, params } = options
    const pageParams = params ?? buildQuery(next)
    const apiParams = new URLSearchParams(pageParams)
    apiParams.set('num', String(next.num ?? galleryRef.current.limit))

    inflightRef.current?.abort()
    const controller = new AbortController()
    inflightRef.current = controller

    const typedQuery = queryInputRef.current
    setLoading(true)
    try {
      const nextGallery = await fetchJSON<GalleryBootstrap>(`/api/list?${apiParams.toString()}`, {
        signal: controller.signal
      })
      setGallery(nextGallery)
      if (queryInputRef.current === typedQuery) setQueryInput(nextGallery.q || '')
      if (push) {
        const pageQuery = pageParams.toString()
        window.history.pushState(null, '', pageQuery ? `/app/list?${pageQuery}` : '/app/list')
      }
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') return
      notifyToast(error instanceof Error ? error.message : '加载失败', 'danger')
    } finally {
      if (inflightRef.current === controller) {
        inflightRef.current = null
        setLoading(false)
      }
    }
  }, [buildQuery])

  useEffect(() => {
    const onPopState = () => {
      const parsed = parseGalleryParams(window.location.search)
      const params = new URLSearchParams()
      if (parsed.date) params.set('date', parsed.date)
      if (parsed.ext) params.set('ext', parsed.ext)
      if (parsed.q) params.set('q', parsed.q)
      if (parsed.num) params.set('num', String(parsed.num))
      setQueryInput(parsed.q)
      void loadGallery({ num: parsed.num ?? galleryRef.current.limit }, { push: false, params })
    }
    window.addEventListener('popstate', onPopState)
    return () => {
      window.removeEventListener('popstate', onPopState)
      inflightRef.current?.abort()
    }
  }, [loadGallery])

  function markThumbFailed(url: string) {
    setFailedThumbs(prev => new Set([...prev, url]))
  }

  function submitSearch(event: React.FormEvent) {
    event.preventDefault()
    void loadGallery({ q: queryInput.trim() })
  }

  function clearSearch() {
    setQueryInput('')
    void loadGallery({ q: '' })
  }

  return (
    <div className="min-h-screen flex flex-col bg-background text-foreground">
      <Toaster />

      <SiteHeader
        brand={gallery.title || 'EasyImage'}
        brandHref="/"
        subtitle="图床广场"
        right={(
          <Button size="sm" asChild>
            <a href="/">
              <Upload className="size-4" />
              上传图片
            </a>
          </Button>
        )}
      />

      <main className="mx-auto flex w-full max-w-6xl flex-1 flex-col gap-6 px-4 py-8 sm:px-6">
        <header className="flex flex-wrap items-end justify-between gap-3">
          <div className="space-y-1">
            <h1 className="text-2xl font-semibold tracking-tight">图床广场</h1>
            <p className="text-sm text-muted-foreground">
              当前筛选共 <span className="font-mono font-medium text-foreground tabular-nums">{gallery.total}</span> 张图片
              {gallery.q ? ` · 关键词「${gallery.q}」` : ''}
            </p>
          </div>
          {loading && (
            <Badge variant="outline" className="gap-1.5">
              <Loader2 className="size-3 animate-spin" />
              加载中
            </Badge>
          )}
        </header>

        <Card className="gap-3 p-4">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div className="flex flex-wrap items-center gap-1.5">
              <Button
                type="button"
                size="xs"
                variant={activeDate === gallery.today ? 'default' : 'outline'}
                disabled={loading}
                onClick={() => void loadGallery({ date: gallery.today })}
              >
                今日
              </Button>
              <Button
                type="button"
                size="xs"
                variant={activeDate === gallery.yesterday ? 'default' : 'outline'}
                disabled={loading}
                onClick={() => void loadGallery({ date: gallery.yesterday })}
              >
                昨日
              </Button>
              {gallery.date_links.map(link => (
                <Button
                  key={link.date}
                  type="button"
                  size="xs"
                  variant={activeDate === link.date ? 'default' : 'outline'}
                  disabled={loading}
                  className="hidden sm:inline-flex"
                  onClick={() => void loadGallery({ date: link.date })}
                >
                  {link.label}
                </Button>
              ))}
            </div>

            <div className="flex flex-wrap items-center gap-1.5">
              <Button
                type="button"
                size="xs"
                variant={activeExt === '' ? 'default' : 'secondary'}
                disabled={loading}
                onClick={() => void loadGallery({ ext: '' })}
              >
                全部
              </Button>
              {gallery.extensions.map(extension => (
                <Button
                  key={extension}
                  type="button"
                  size="xs"
                  variant={activeExt === extension ? 'default' : 'secondary'}
                  disabled={loading}
                  className="font-mono uppercase"
                  onClick={() => void loadGallery({ ext: extension })}
                >
                  {extension}
                </Button>
              ))}
            </div>
          </div>

          <form role="search" onSubmit={submitSearch} className="flex flex-col gap-2 sm:flex-row">
            <div className="relative flex-1">
              <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="search"
                value={queryInput}
                onChange={event => setQueryInput(event.target.value)}
                placeholder="搜索原文件名或存储文件名"
                disabled={loading}
                className="pl-9"
              />
            </div>
            <div className="flex items-center gap-2">
              <Button type="submit" size="default" disabled={loading}>
                搜索
              </Button>
              {gallery.q && (
                <Button type="button" variant="outline" size="default" disabled={loading} onClick={clearSearch}>
                  <X className="size-4" />
                  清空
                </Button>
              )}
            </div>
          </form>
        </Card>

        {loading && gallery.files.length === 0 ? (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
            {Array.from({ length: 8 }).map((_, i) => (
              <div key={i} className="overflow-hidden rounded-xl border bg-card">
                <Skeleton className="aspect-[4/3] w-full rounded-none" />
                <div className="space-y-2 p-3">
                  <Skeleton className="h-4 w-3/4" />
                  <Skeleton className="h-3 w-1/2" />
                </div>
              </div>
            ))}
          </div>
        ) : gallery.files.length > 0 ? (
          <section className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4" aria-live="polite">
            {gallery.files.map(file => {
              const name = displayName(file)
              return (
                <article
                  key={file.url}
                  className="group flex flex-col overflow-hidden rounded-xl border bg-card transition-all hover:border-foreground/30 hover:shadow-sm"
                >
                  <button
                    type="button"
                    onClick={() => setSelectedImage(file)}
                    className="checker-well relative block aspect-[4/3] w-full overflow-hidden border-b text-left cursor-pointer"
                  >
                    {failedThumbs.has(file.url) ? (
                      <div className="flex h-full w-full flex-col items-center justify-center gap-1.5 p-4 text-center text-xs text-muted-foreground">
                        <ImageOff className="size-6 opacity-50" />
                        <span className="line-clamp-2 break-all">{name}</span>
                      </div>
                    ) : (
                      <img
                        src={file.thumb_url}
                        alt={name}
                        width={320}
                        height={240}
                        loading="lazy"
                        decoding="async"
                        onError={() => markThumbFailed(file.url)}
                        className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.03]"
                      />
                    )}
                  </button>

                  <div className="flex flex-1 flex-col justify-between gap-2.5 p-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium text-foreground" title={name}>
                        {name}
                      </p>
                      {file.original_name && (
                        <p className="truncate font-mono text-[11px] text-muted-foreground" title={file.name}>
                          {file.name}
                        </p>
                      )}
                    </div>

                    <div className="flex items-center justify-between gap-1 border-t pt-2">
                      <Button
                        type="button"
                        size="xs"
                        variant="ghost"
                        title="复制链接"
                        onClick={() => void copy(file.url)}
                        className="h-7 px-2"
                      >
                        <Copy className="size-3.5" />
                        复制
                      </Button>
                      <div className="flex items-center gap-0.5">
                        <Button size="icon-sm" variant="ghost" className="size-7" asChild title="打开原图">
                          <a href={file.url} target="_blank" rel="noopener noreferrer">
                            <ExternalLink className="size-3.5" />
                            <span className="sr-only">打开原图</span>
                          </a>
                        </Button>
                        <Button size="icon-sm" variant="ghost" className="size-7" asChild title="图片信息">
                          <a href={file.info_url} target="_blank" rel="noopener noreferrer">
                            <Info className="size-3.5" />
                            <span className="sr-only">详细信息</span>
                          </a>
                        </Button>
                        <Button size="icon-sm" variant="ghost" className="size-7" asChild title="下载原图">
                          <a href={file.down_url} target="_blank" rel="noopener noreferrer">
                            <Download className="size-3.5" />
                            <span className="sr-only">下载</span>
                          </a>
                        </Button>
                      </div>
                    </div>
                  </div>
                </article>
              )
            })}
          </section>
        ) : (
          <Card className="flex flex-col items-center justify-center gap-3 py-16 text-center">
            <ImageOff className="size-8 text-muted-foreground/60" />
            <div className="space-y-1">
              <p className="text-sm font-medium">{gallery.q ? '没有匹配的图片' : '该日期还没有上传图片'}</p>
              <p className="text-xs text-muted-foreground">
                {gallery.q ? '尝试更换关键词或切换其他日期查看。' : '切换到其他日期，或立即上传第一张图片。'}
              </p>
            </div>
            <div className="mt-1 flex items-center gap-2">
              {gallery.q && (
                <Button type="button" variant="outline" size="sm" onClick={clearSearch}>
                  清空搜索
                </Button>
              )}
              <Button size="sm" asChild>
                <a href="/">
                  <Upload className="size-4" />
                  上传图片
                </a>
              </Button>
            </div>
          </Card>
        )}
      </main>

      <Dialog open={Boolean(selectedImage)} onOpenChange={open => { if (!open) setSelectedImage(null) }}>
        <DialogContent className="sm:max-w-4xl p-4">
          {selectedImage && (
            <>
              <DialogHeader>
                <DialogTitle className="truncate pr-8 text-sm font-medium">
                  {displayName(selectedImage)}
                </DialogTitle>
                <DialogDescription className="truncate font-mono text-xs">
                  {selectedImage.url}
                </DialogDescription>
              </DialogHeader>
              <div className="checker-well flex max-h-[72vh] items-center justify-center overflow-hidden rounded-lg border p-2">
                <img
                  src={selectedImage.url}
                  alt={displayName(selectedImage)}
                  className="max-h-[68vh] max-w-full rounded object-contain"
                />
              </div>
              <DialogFooter className="sm:justify-between">
                <div className="flex items-center gap-2">
                  <Button variant="outline" size="sm" asChild>
                    <a href={selectedImage.info_url} target="_blank" rel="noopener noreferrer">
                      <Info className="size-3.5" />
                      详情
                    </a>
                  </Button>
                  <Button variant="outline" size="sm" asChild>
                    <a href={selectedImage.down_url} target="_blank" rel="noopener noreferrer">
                      <Download className="size-3.5" />
                      下载
                    </a>
                  </Button>
                </div>
                <div className="flex items-center gap-2">
                  <Button type="button" size="sm" onClick={() => void copy(selectedImage.url)}>
                    <Copy className="size-3.5" />
                    复制链接
                  </Button>
                  <Button variant="outline" size="sm" asChild>
                    <a href={selectedImage.url} target="_blank" rel="noopener noreferrer">
                      <ExternalLink className="size-3.5" />
                      原图
                    </a>
                  </Button>
                </div>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
