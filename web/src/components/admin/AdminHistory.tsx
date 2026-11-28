import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ChevronLeft, ChevronRight, Copy, Download, ExternalLink, ImageOff, Info, Loader2, Search, Trash2, X } from 'lucide-react'
import type { GalleryBootstrap, GalleryFile } from '@/types'
import type { NoticeType } from '@/shared/notify'
import { adminApi } from '@/shared/adminApi'
import { runBatchDelete, summarizeBatchDelete } from '@/shared/batchDelete'
import { fetchCached, getCached, invalidateCached, setCached } from '@/shared/swrCache'
import { useCopy } from '@/shared/useCopy'
import { useDelayedLoading } from '@/shared/useDelayedLoading'
import { useConfirmDialog } from '@/components/ConfirmDialog'
import { FadeImage } from '@/components/FadeImage'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
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

function displayName(file: GalleryFile): string {
  return file.original_name || file.name
}

export default function AdminHistory({ onNotice }: { onNotice: (message: string, type?: NoticeType) => void }) {
  const copy = useMemo(() => useCopy(onNotice), [onNotice])
  const { confirm, dialog: confirmDialog } = useConfirmDialog()

  const initialGallery = getCached<GalleryBootstrap>('admin:history:')
  const [loading, setLoading] = useState(!initialGallery)
  const [gallery, setGallery] = useState<GalleryBootstrap | null>(initialGallery ?? null)
  const [selectedImage, setSelectedImage] = useState<GalleryFile | null>(null)
  const [queryInput, setQueryInput] = useState(initialGallery?.q || '')
  const [selected, setSelected] = useState<Set<string>>(() => new Set())
  const showInitialSkeleton = useDelayedLoading(loading && !gallery)

  const inflightRef = useRef<AbortController | null>(null)
  const galleryRef = useRef(gallery)
  galleryRef.current = gallery
  const queryRef = useRef(queryInput)
  queryRef.current = queryInput

  const load = useCallback(async (next: Partial<{ date: string; ext: string; q: string; num: number }> = {}) => {
    inflightRef.current?.abort()
    const controller = new AbortController()
    inflightRef.current = controller

    const current = galleryRef.current
    const params = new URLSearchParams()
    const date = next.date ?? current?.date
    const ext = next.ext ?? (current?.ext || current?.search)
    const q = next.q ?? current?.q
    if (date && date !== current?.today) params.set('date', date)
    if (ext) params.set('ext', ext)
    if (q) params.set('q', q)
    if (next.num && next.num > 0) params.set('num', String(next.num))

    const cacheKey = `admin:history:${params.toString()}`
    const cached = getCached<GalleryBootstrap>(cacheKey)
    if (cached) setGallery(cached)

    const typedQuery = queryRef.current
    setLoading(true)
    try {
      const payload = params.toString() === ''
        ? await fetchCached(cacheKey, () => adminApi.history(new URLSearchParams()), 5_000)
        : await adminApi.history(params, { signal: controller.signal })
      if (controller.signal.aborted) return
      setCached(cacheKey, payload)
      setGallery(payload)
      setSelected(prev => (next.num ? prev : new Set()))
      if (queryRef.current === typedQuery) setQueryInput(payload.q || '')
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') return
      onNotice(error instanceof Error ? error.message : '加载失败', 'danger')
    } finally {
      if (inflightRef.current === controller) {
        inflightRef.current = null
        setLoading(false)
      }
    }
  }, [onNotice])

  useEffect(() => {
    void load()
    return () => inflightRef.current?.abort()
  }, [load])

  const activeDate = gallery?.date || ''
  const activeExt = gallery?.ext || gallery?.search || ''
  const files = gallery?.files || []
  const selectedFiles = useMemo(() => files.filter(file => selected.has(file.url)), [files, selected])
  const allSelected = files.length > 0 && selectedFiles.length === files.length

  const previewIndex = useMemo(() => {
    if (!selectedImage) return -1
    return files.findIndex(file => file.url === selectedImage.url)
  }, [files, selectedImage])

  const stepPreview = useCallback((delta: number) => {
    if (files.length <= 1 || previewIndex < 0) return
    const nextIdx = (previewIndex + delta + files.length) % files.length
    setSelectedImage(files[nextIdx])
  }, [files, previewIndex])

  useEffect(() => {
    if (!selectedImage || files.length <= 1) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'ArrowLeft') {
        event.preventDefault()
        stepPreview(-1)
      } else if (event.key === 'ArrowRight') {
        event.preventDefault()
        stepPreview(1)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [selectedImage, files.length, stepPreview])

  function toggleFile(file: GalleryFile) {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(file.url)) next.delete(file.url)
      else next.add(file.url)
      return next
    })
  }

  function toggleAll() {
    setSelected(allSelected ? new Set() : new Set(files.map(file => file.url)))
  }

  async function removeOne(file: GalleryFile) {
    const ok = await confirm({
      title: '删除图片？',
      description: `确认永久删除「${displayName(file)}」？此操作不可恢复。`,
      confirmLabel: '确认删除'
    })
    if (!ok) return
    try {
      const result = await adminApi.deleteHistory(file.path)
      if (result.code === 200) {
        invalidateCached('admin:history:')
        invalidateCached('admin:filer:')
        invalidateCached('admin:chart')
        invalidateCached('admin:overview')
        invalidateCached('ws:images:')
        invalidateCached('ws:folders:')
        setGallery(prev => (prev ? {
          ...prev,
          total: Math.max(0, prev.total - 1),
          files: prev.files.filter(item => item.url !== file.url)
        } : prev))
        if (selectedImage?.url === file.url) setSelectedImage(null)
        setSelected(prev => {
          const next = new Set(prev)
          next.delete(file.url)
          return next
        })
        onNotice(result.msg || '删除成功', 'success')
      } else {
        onNotice(result.msg || '删除失败', 'danger')
      }
    } catch (error) {
      onNotice(error instanceof Error ? error.message : '删除失败', 'danger')
    }
  }

  async function removeSelected() {
    if (selectedFiles.length === 0) return
    const ok = await confirm({
      title: `批量删除 ${selectedFiles.length} 张图片？`,
      description: '选中的图片将被永久删除且无法恢复。',
      confirmLabel: '确认批量删除'
    })
    if (!ok) return

    const byUrl = new Map(selectedFiles.map(file => [file.url, file]))
    const outcome = await runBatchDelete(
      selectedFiles.map(file => file.url),
      async url => adminApi.deleteHistory(byUrl.get(url)!.path)
    )
    const deleted = new Set(outcome.succeeded)
    if (deleted.size > 0) {
      invalidateCached('admin:history:')
      invalidateCached('admin:filer:')
      invalidateCached('admin:chart')
      invalidateCached('admin:overview')
      invalidateCached('ws:images:')
      invalidateCached('ws:folders:')
    }
    setGallery(prev => (prev ? {
      ...prev,
      total: Math.max(0, prev.total - deleted.size),
      files: prev.files.filter(item => !deleted.has(item.url))
    } : prev))
    if (selectedImage && deleted.has(selectedImage.url)) setSelectedImage(null)
    setSelected(prev => new Set([...prev].filter(url => !deleted.has(url))))
    const summary = summarizeBatchDelete(outcome)
    onNotice(summary.message, summary.type)
  }

  if (loading && !gallery) {
    if (!showInitialSkeleton) return <div className="min-h-[360px]" aria-busy="true" />
    return (
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
        {Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="aspect-[4/3] rounded-xl" />)}
      </div>
    )
  }

  if (!gallery) return null

  return (
    <div className="space-y-4">
      {confirmDialog}

      <Card className="relative gap-3 overflow-hidden p-4">
        {loading && <div className="loading-bar absolute inset-x-0 top-0" aria-hidden="true" />}
        <div className="flex flex-col gap-2.5 lg:flex-row lg:items-center lg:justify-between">
          <div className="no-scrollbar flex flex-nowrap items-center gap-1 overflow-x-auto pb-0.5 lg:pb-0">
            <Button
              type="button"
              size="xs"
              variant={activeDate === gallery.today ? 'secondary' : 'ghost'}
              className={`shrink-0 ${activeDate === gallery.today ? 'ring-1 ring-border font-medium text-foreground' : 'text-muted-foreground'}`}
              onClick={() => void load({ date: gallery.today })}
            >
              今日
            </Button>
            <Button
              type="button"
              size="xs"
              variant={activeDate === gallery.yesterday ? 'secondary' : 'ghost'}
              className={`shrink-0 ${activeDate === gallery.yesterday ? 'ring-1 ring-border font-medium text-foreground' : 'text-muted-foreground'}`}
              onClick={() => void load({ date: gallery.yesterday })}
            >
              昨日
            </Button>
            {gallery.date_links.map(link => (
              <Button
                key={link.date}
                type="button"
                size="xs"
                variant={activeDate === link.date ? 'secondary' : 'ghost'}
                className={`shrink-0 ${activeDate === link.date ? 'ring-1 ring-border font-medium text-foreground' : 'text-muted-foreground'}`}
                onClick={() => void load({ date: link.date })}
              >
                {link.label}
              </Button>
            ))}
          </div>

          <div className="no-scrollbar flex shrink-0 flex-nowrap items-center gap-1 overflow-x-auto">
            <Button
              type="button"
              size="xs"
              variant={activeExt === '' ? 'secondary' : 'ghost'}
              className={`shrink-0 ${activeExt === '' ? 'ring-1 ring-border font-medium text-foreground' : 'text-muted-foreground'}`}
              onClick={() => void load({ ext: '' })}
            >
              全部
            </Button>
            {gallery.extensions.map(ext => (
              <Button
                key={ext}
                type="button"
                size="xs"
                variant={activeExt === ext ? 'secondary' : 'ghost'}
                className={`shrink-0 font-mono uppercase ${activeExt === ext ? 'ring-1 ring-border font-medium text-foreground' : 'text-muted-foreground'}`}
                onClick={() => void load({ ext })}
              >
                {ext}
              </Button>
            ))}
          </div>
        </div>

        <form
          role="search"
          onSubmit={event => {
            event.preventDefault()
            void load({ q: queryInput.trim() })
          }}
          className="flex flex-col gap-2 sm:flex-row"
        >
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="search"
              value={queryInput}
              onChange={e => setQueryInput(e.target.value)}
              placeholder="搜索原文件名或存储文件名"
              className="pl-9"
            />
          </div>
          <div className="flex items-center gap-2">
            <Button type="submit" disabled={loading}>
              {loading && <Loader2 className="size-4 animate-spin" />}
              搜索
            </Button>
            {gallery.q && (
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setQueryInput('')
                  void load({ q: '' })
                }}
              >
                <X className="size-4" />
                清空
              </Button>
            )}
          </div>
        </form>
      </Card>

      {files.length > 0 && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-muted/40 px-3.5 py-2.5">
          <div className="flex items-center gap-3">
            <label className="flex items-center gap-2 text-sm font-medium cursor-pointer select-none">
              <Checkbox checked={allSelected} onCheckedChange={toggleAll} />
              <span>全选当前（{files.length}）</span>
            </label>
            <Badge variant="outline" className="font-mono text-[11px] font-normal tabular-nums">
              显示 {files.length} / {gallery.total} 张
            </Badge>
          </div>
          {selectedFiles.length > 0 && (
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="outline" className="font-mono tabular-nums">
                已选 {selectedFiles.length}
              </Badge>
              <Button
                type="button"
                size="xs"
                variant="outline"
                onClick={() => void copy(selectedFiles.map(file => file.url).join('\n'), `已复制 ${selectedFiles.length} 条链接`)}
              >
                <Copy className="size-3.5" />
                复制链接
              </Button>
              <Button type="button" size="xs" onClick={() => void removeSelected()}>
                <Trash2 className="size-3.5" />
                批量删除
              </Button>
            </div>
          )}
        </div>
      )}

      {files.length > 0 ? (
        <section className={`grid grid-cols-2 gap-4 transition-opacity duration-200 sm:grid-cols-3 lg:grid-cols-4 ${loading ? 'opacity-60' : 'opacity-100'}`}>
          {files.map(file => {
            const name = displayName(file)
            const isSelected = selected.has(file.url)
            return (
              <article
                key={file.url}
                className={`group relative flex flex-col overflow-hidden rounded-xl border bg-card transition-[border-color,box-shadow,transform] duration-200 ${
                  isSelected ? 'ring-2 ring-foreground' : 'hover:-translate-y-0.5 hover:border-foreground/30 hover:shadow-sm'
                }`}
              >
                <div
                  className="absolute top-2 left-2 z-10 rounded-md bg-background/90 p-1 shadow-xs backdrop-blur-xs"
                  onClick={event => event.stopPropagation()}
                >
                  <Checkbox
                    checked={isSelected}
                    aria-label={`选择 ${name}`}
                    onCheckedChange={() => toggleFile(file)}
                  />
                </div>

                <button
                  type="button"
                  onClick={() => setSelectedImage(file)}
                  className="checker-well block aspect-[4/3] w-full overflow-hidden border-b cursor-pointer"
                >
                  <FadeImage
                    src={file.thumb_url}
                    alt={name}
                    fallbackLabel={name}
                    iconClassName="size-5"
                    className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.04]"
                  />
                </button>

                <div className="flex flex-1 flex-col justify-between gap-2 p-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium" title={name}>{name}</p>
                    {file.original_name && (
                      <p className="truncate font-mono text-[11px] text-muted-foreground" title={file.name}>
                        {file.name}
                      </p>
                    )}
                  </div>
                  <div className="flex items-center justify-between gap-1 border-t pt-2">
                    <Button type="button" size="xs" variant="ghost" onClick={() => void copy(file.url)}>
                      <Copy className="size-3" />
                      复制
                    </Button>
                    <div className="flex items-center gap-1">
                      <Button size="xs" variant="ghost" asChild>
                        <a href={file.info_url} target="_blank" rel="noopener noreferrer">
                          <Info className="size-3" />
                          详情
                        </a>
                      </Button>
                      <Button type="button" size="xs" variant="outline" onClick={() => void removeOne(file)}>
                        <Trash2 className="size-3" />
                        删除
                      </Button>
                    </div>
                  </div>
                </div>
              </article>
            )
          })}
        </section>
      ) : (
        <Card className="flex flex-col items-center justify-center gap-2 py-14 text-center">
          <ImageOff className="size-7 text-muted-foreground/60" />
          <p className="text-sm font-medium">{gallery.q ? '没有匹配的图片' : '该日期还没有上传图片'}</p>
        </Card>
      )}

      {gallery.total > files.length && (
        <div className="text-center">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={loading}
            onClick={() => void load({ num: Math.min(500, files.length + Math.max(gallery.limit || 20, 40)) })}
          >
            {loading && <Loader2 className="size-3.5 animate-spin" />}
            加载更多（还有 {gallery.total - files.length} 张）
          </Button>
        </div>
      )}

      <Dialog open={Boolean(selectedImage)} onOpenChange={open => { if (!open) setSelectedImage(null) }}>
        <DialogContent className="sm:max-w-4xl p-4">
          {selectedImage && (
            <>
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2 pr-8 text-sm font-medium">
                  <span className="truncate">{displayName(selectedImage)}</span>
                  {files.length > 1 && previewIndex >= 0 && (
                    <Badge variant="outline" className="shrink-0 font-mono text-[10px] font-normal tabular-nums">
                      {previewIndex + 1} / {files.length}
                    </Badge>
                  )}
                </DialogTitle>
                <DialogDescription className="truncate font-mono text-xs">{selectedImage.url}</DialogDescription>
              </DialogHeader>
              <div className="checker-well relative flex max-h-[72vh] min-h-[240px] items-center justify-center overflow-hidden rounded-lg border p-2">
                {files.length > 1 && (
                  <>
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="secondary"
                      aria-label="上一张"
                      onClick={() => stepPreview(-1)}
                      className="absolute left-2.5 top-1/2 z-10 -translate-y-1/2 rounded-full border bg-background/85 shadow-sm backdrop-blur-xs hover:bg-background"
                    >
                      <ChevronLeft className="size-4" />
                    </Button>
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="secondary"
                      aria-label="下一张"
                      onClick={() => stepPreview(1)}
                      className="absolute right-2.5 top-1/2 z-10 -translate-y-1/2 rounded-full border bg-background/85 shadow-sm backdrop-blur-xs hover:bg-background"
                    >
                      <ChevronRight className="size-4" />
                    </Button>
                  </>
                )}
                <FadeImage
                  key={selectedImage.url}
                  src={selectedImage.url}
                  alt={displayName(selectedImage)}
                  loading="eager"
                  className="max-h-[68vh] max-w-full rounded object-contain"
                />
              </div>
              <DialogFooter className="sm:justify-between">
                <Button
                  type="button"
                  size="sm"
                  variant="destructive"
                  onClick={() => {
                    const target = selectedImage
                    setSelectedImage(null)
                    void removeOne(target)
                  }}
                >
                  <Trash2 className="size-3.5" />
                  删除
                </Button>
                <div className="flex flex-wrap items-center gap-2">
                  <Button type="button" size="sm" onClick={() => void copy(selectedImage.url)}>
                    <Copy className="size-3.5" />
                    复制链接
                  </Button>
                  {selectedImage.webp_url && (
                    <Button type="button" size="sm" variant="outline" onClick={() => void copy(selectedImage.webp_url!, '已复制 WebP 链接')}>
                      <Copy className="size-3.5" />
                      复制 WebP
                    </Button>
                  )}
                  <Button variant="outline" size="sm" asChild>
                    <a href={selectedImage.down_url}>
                      <Download className="size-3.5" />
                      下载
                    </a>
                  </Button>
                  <Button variant="outline" size="sm" asChild>
                    <a href={selectedImage.url} target="_blank" rel="noopener noreferrer">
                      <ExternalLink className="size-3.5" />
                      新窗口打开
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
