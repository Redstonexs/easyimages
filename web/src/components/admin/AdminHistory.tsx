import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Copy, ExternalLink, ImageOff, Info, Loader2, Search, Trash2, X } from 'lucide-react'
import type { GalleryBootstrap, GalleryFile } from '@/types'
import type { NoticeType } from '@/shared/notify'
import { adminApi } from '@/shared/adminApi'
import { runBatchDelete, summarizeBatchDelete } from '@/shared/batchDelete'
import { useCopy } from '@/shared/useCopy'
import { useConfirmDialog } from '@/components/ConfirmDialog'
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

  const [loading, setLoading] = useState(true)
  const [gallery, setGallery] = useState<GalleryBootstrap | null>(null)
  const [selectedImage, setSelectedImage] = useState<GalleryFile | null>(null)
  const [failedThumbs, setFailedThumbs] = useState<Set<string>>(() => new Set())
  const [queryInput, setQueryInput] = useState('')
  const [selected, setSelected] = useState<Set<string>>(() => new Set())

  const inflightRef = useRef<AbortController | null>(null)
  const galleryRef = useRef(gallery)
  galleryRef.current = gallery
  const queryRef = useRef(queryInput)
  queryRef.current = queryInput

  const load = useCallback(async (next: Partial<{ date: string; ext: string; q: string }> = {}) => {
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

    const typedQuery = queryRef.current
    setLoading(true)
    try {
      const payload = await adminApi.history(params, { signal: controller.signal })
      setGallery(payload)
      setSelected(new Set())
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
        setGallery(prev => (prev ? { ...prev, files: prev.files.filter(item => item.url !== file.url) } : prev))
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
    setGallery(prev => (prev ? { ...prev, files: prev.files.filter(item => !deleted.has(item.url)) } : prev))
    setSelected(prev => new Set([...prev].filter(url => !deleted.has(url))))
    const summary = summarizeBatchDelete(outcome)
    onNotice(summary.message, summary.type)
  }

  if (loading && !gallery) {
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

      <Card className="gap-3 p-4">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex flex-wrap items-center gap-1.5">
            <Button
              type="button"
              size="xs"
              variant={activeDate === gallery.today ? 'default' : 'outline'}
              onClick={() => void load({ date: gallery.today })}
            >
              今日
            </Button>
            <Button
              type="button"
              size="xs"
              variant={activeDate === gallery.yesterday ? 'default' : 'outline'}
              onClick={() => void load({ date: gallery.yesterday })}
            >
              昨日
            </Button>
            {gallery.date_links.map(link => (
              <Button
                key={link.date}
                type="button"
                size="xs"
                variant={activeDate === link.date ? 'default' : 'outline'}
                className="hidden sm:inline-flex"
                onClick={() => void load({ date: link.date })}
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
              onClick={() => void load({ ext: '' })}
            >
              全部
            </Button>
            {gallery.extensions.map(ext => (
              <Button
                key={ext}
                type="button"
                size="xs"
                variant={activeExt === ext ? 'default' : 'secondary'}
                className="font-mono uppercase"
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
          <label className="flex items-center gap-2 text-sm font-medium cursor-pointer select-none">
            <Checkbox checked={allSelected} onCheckedChange={toggleAll} />
            <span>全选当前（{files.length}）</span>
          </label>
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
        <section className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
          {files.map(file => {
            const name = displayName(file)
            const isSelected = selected.has(file.url)
            return (
              <article
                key={file.url}
                className={`group relative flex flex-col overflow-hidden rounded-xl border bg-card transition-all ${
                  isSelected ? 'ring-2 ring-foreground' : 'hover:border-foreground/30'
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
                  {failedThumbs.has(file.url) ? (
                    <div className="flex h-full w-full flex-col items-center justify-center gap-1 p-4 text-xs text-muted-foreground">
                      <ImageOff className="size-5 opacity-50" />
                      <span className="line-clamp-2 break-all">{name}</span>
                    </div>
                  ) : (
                    <img
                      src={file.thumb_url}
                      alt={name}
                      loading="lazy"
                      decoding="async"
                      onError={() => setFailedThumbs(prev => new Set([...prev, file.url]))}
                      className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.03]"
                    />
                  )}
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

      <Dialog open={Boolean(selectedImage)} onOpenChange={open => { if (!open) setSelectedImage(null) }}>
        <DialogContent className="sm:max-w-4xl p-4">
          {selectedImage && (
            <>
              <DialogHeader>
                <DialogTitle className="truncate pr-8 text-sm font-medium">{displayName(selectedImage)}</DialogTitle>
                <DialogDescription className="truncate font-mono text-xs">{selectedImage.url}</DialogDescription>
              </DialogHeader>
              <div className="checker-well flex max-h-[72vh] items-center justify-center overflow-hidden rounded-lg border p-2">
                <img
                  src={selectedImage.url}
                  alt={displayName(selectedImage)}
                  className="max-h-[68vh] max-w-full rounded object-contain"
                />
              </div>
              <DialogFooter>
                <Button type="button" size="sm" onClick={() => void copy(selectedImage.url)}>
                  <Copy className="size-3.5" />
                  复制链接
                </Button>
                <Button variant="outline" size="sm" asChild>
                  <a href={selectedImage.url} target="_blank" rel="noopener noreferrer">
                    <ExternalLink className="size-3.5" />
                    新窗口打开
                  </a>
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
