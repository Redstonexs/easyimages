import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ArrowUp,
  ChevronRight,
  Copy,
  ExternalLink,
  Folder,
  ImageOff,
  Inbox,
  RefreshCw,
  Search,
  Trash2,
  X
} from 'lucide-react'
import type { AdminFileEntry, AdminFiler as AdminFilerData } from '@/types'
import type { NoticeType } from '@/shared/notify'
import { adminApi } from '@/shared/adminApi'
import { runBatchDelete, summarizeBatchDelete } from '@/shared/batchDelete'
import { formatSize } from '@/shared/format'
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

function displayName(file: AdminFileEntry): string {
  return file.original_name || file.name
}

function fileExt(file: AdminFileEntry): string {
  return file.ext || file.name.split('.').pop()?.toUpperCase() || 'IMG'
}

export default function AdminFiler({ onNotice }: { onNotice: (message: string, type?: NoticeType) => void }) {
  const copy = useMemo(() => useCopy(onNotice), [onNotice])
  const { confirm, dialog: confirmDialog } = useConfirmDialog()

  const [loading, setLoading] = useState(true)
  const [data, setData] = useState<AdminFilerData | null>(null)
  const [failedThumbs, setFailedThumbs] = useState<Set<string>>(() => new Set())
  const [queryInput, setQueryInput] = useState('')
  const [selectedImage, setSelectedImage] = useState<AdminFileEntry | null>(null)
  const [selected, setSelected] = useState<Set<string>>(() => new Set())

  const load = useCallback(async (path?: string) => {
    setLoading(true)
    const params = new URLSearchParams()
    if (path) params.set('path', path)
    try {
      const payload = await adminApi.filer(params)
      payload.dirs = payload.dirs ?? []
      payload.files = payload.files ?? []
      setData(payload)
      setQueryInput('')
      setSelected(new Set())
    } catch (error) {
      onNotice(error instanceof Error ? error.message : '加载目录失败', 'danger')
    } finally {
      setLoading(false)
    }
  }, [onNotice])

  useEffect(() => {
    void load()
  }, [load])

  const breadcrumbs = useMemo(() => {
    if (!data) return []
    let cumulative = '/'
    return data.path.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean).map(segment => {
      cumulative += `${segment}/`
      return { label: segment, path: cumulative }
    })
  }, [data])

  const dirs = data?.dirs ?? []
  const files = data?.files ?? []

  const filteredFiles = useMemo(() => {
    const query = queryInput.trim().toLowerCase()
    if (!query) return files
    return files.filter(file =>
      [file.name, file.original_name, file.path, file.url, file.ext].some(value =>
        value?.toLowerCase().includes(query)
      )
    )
  }, [files, queryInput])

  const selectedFiles = useMemo(
    () => filteredFiles.filter(file => selected.has(file.url)),
    [filteredFiles, selected]
  )
  const selectedSize = useMemo(
    () => selectedFiles.reduce((sum, file) => sum + (file.size || 0), 0),
    [selectedFiles]
  )
  const allSelected = filteredFiles.length > 0 && selectedFiles.length === filteredFiles.length

  function toggleFile(file: AdminFileEntry) {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(file.url)) next.delete(file.url)
      else next.add(file.url)
      return next
    })
  }

  function toggleAll() {
    setSelected(allSelected ? new Set() : new Set(filteredFiles.map(file => file.url)))
  }

  async function removeOne(file: AdminFileEntry) {
    const ok = await confirm({
      title: '删除文件？',
      description: `确认永久删除「${displayName(file)}」？此操作不可恢复。`,
      confirmLabel: '确认删除'
    })
    if (!ok) return
    try {
      const result = await adminApi.deleteFile(file.path)
      if (result.code === 200) {
        setData(prev => (prev ? { ...prev, files: prev.files.filter(item => item.url !== file.url) } : prev))
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
      title: `批量删除 ${selectedFiles.length} 个文件？`,
      description: `总计 ${formatSize(selectedSize)}，删除后无法恢复。`,
      confirmLabel: '确认批量删除'
    })
    if (!ok) return

    const byUrl = new Map(selectedFiles.map(file => [file.url, file]))
    const outcome = await runBatchDelete(
      selectedFiles.map(file => file.url),
      async url => adminApi.deleteFile(byUrl.get(url)!.path)
    )
    const deleted = new Set(outcome.succeeded)
    setData(prev => (prev ? { ...prev, files: prev.files.filter(item => !deleted.has(item.url)) } : prev))
    if (selectedImage && deleted.has(selectedImage.url)) setSelectedImage(null)
    setSelected(prev => new Set([...prev].filter(url => !deleted.has(url))))
    const summary = summarizeBatchDelete(outcome)
    onNotice(summary.message, summary.type)
  }

  if (loading && !data) {
    return <Skeleton className="h-[520px] rounded-xl" />
  }

  if (!data) return null

  return (
    <div className="space-y-4">
      {confirmDialog}

      <Card className="overflow-hidden gap-0 py-0">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-muted/30 px-4 py-3">
          <nav className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto" aria-label="文件路径">
            <Button
              type="button"
              size="xs"
              variant={data.path === data.root_path ? 'default' : 'outline'}
              onClick={() => void load(data.root_path)}
            >
              根目录
            </Button>
            {breadcrumbs.map(crumb => (
              <div key={crumb.path} className="flex items-center gap-1 shrink-0">
                <ChevronRight className="size-3.5 text-muted-foreground" />
                <Button
                  type="button"
                  size="xs"
                  variant={crumb.path === data.path ? 'default' : 'ghost'}
                  className="font-mono"
                  onClick={() => void load(crumb.path)}
                >
                  {crumb.label}
                </Button>
              </div>
            ))}
          </nav>

          <div className="flex items-center gap-2 shrink-0">
            <Badge variant="outline" className="font-mono text-xs tabular-nums">
              {dirs.length} 目录 · {files.length} 文件
            </Badge>
            {data.parent_path && (
              <Button type="button" size="xs" variant="outline" onClick={() => void load(data.parent_path)}>
                <ArrowUp className="size-3.5" />
                上级
              </Button>
            )}
            <Button type="button" size="xs" variant="outline" disabled={loading} onClick={() => void load(data.path)}>
              <RefreshCw className={`size-3.5 ${loading ? 'animate-spin' : ''}`} />
              刷新
            </Button>
          </div>
        </div>

        <div className="grid lg:grid-cols-[220px_minmax(0,1fr)]">
          <aside className="border-b bg-muted/15 p-3 lg:border-r lg:border-b-0" aria-label="子目录">
            <div className="mb-2 flex items-center justify-between px-1 text-xs font-medium text-muted-foreground">
              <span>子目录</span>
              <span className="font-mono tabular-nums">{dirs.length}</span>
            </div>
            {dirs.length > 0 ? (
              <div className="grid grid-cols-2 gap-1 sm:grid-cols-3 lg:grid-cols-1">
                {dirs.map(dir => (
                  <button
                    key={dir}
                    type="button"
                    onClick={() => void load(data.path + dir)}
                    className="flex items-center gap-2 rounded-md px-2.5 py-2 text-left text-sm transition-colors hover:bg-muted cursor-pointer"
                  >
                    <Folder className="size-4 shrink-0 text-muted-foreground" />
                    <span className="truncate font-mono text-xs" title={dir}>{dir.replace(/\/$/, '')}</span>
                  </button>
                ))}
              </div>
            ) : (
              <p className="rounded-md border border-dashed py-6 text-center text-xs text-muted-foreground">
                无子目录
              </p>
            )}
          </aside>

          <div className="min-w-0 space-y-3 p-4">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <p className="font-mono text-xs text-muted-foreground">
                显示 {filteredFiles.length} / {files.length} 个文件
              </p>
              <div className="flex items-center gap-2">
                <div className="relative flex-1 sm:w-64">
                  <Search className="pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    type="search"
                    value={queryInput}
                    onChange={e => setQueryInput(e.target.value)}
                    placeholder="过滤当前目录..."
                    className="h-8 pl-8 text-xs"
                  />
                </div>
                {queryInput && (
                  <Button type="button" size="xs" variant="ghost" onClick={() => setQueryInput('')}>
                    <X className="size-3.5" />
                  </Button>
                )}
              </div>
            </div>

            {filteredFiles.length > 0 && selectedFiles.length > 0 && (
              <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/40 px-3 py-2 text-xs">
                <span className="font-medium tabular-nums">
                  已选 {selectedFiles.length} 个文件 · {formatSize(selectedSize)}
                </span>
                <Button
                  type="button"
                  size="xs"
                  variant="outline"
                  onClick={() => void copy(selectedFiles.map(file => file.url).join('\n'), `已复制 ${selectedFiles.length} 条链接`)}
                >
                  <Copy className="size-3" />
                  复制链接
                </Button>
                <Button type="button" size="xs" onClick={() => void removeSelected()}>
                  <Trash2 className="size-3" />
                  批量删除
                </Button>
                <Button type="button" size="xs" variant="ghost" onClick={() => setSelected(new Set())}>
                  取消选择
                </Button>
              </div>
            )}

            {filteredFiles.length > 0 ? (
              <div className="rounded-lg border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-10">
                        <Checkbox checked={allSelected} aria-label="全选当前文件" onCheckedChange={toggleAll} />
                      </TableHead>
                      <TableHead className="w-16">预览</TableHead>
                      <TableHead>文件信息</TableHead>
                      <TableHead className="w-20">格式</TableHead>
                      <TableHead className="w-24">大小</TableHead>
                      <TableHead className="w-36">修改时间</TableHead>
                      <TableHead className="w-40 text-right">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredFiles.map(file => {
                      const name = displayName(file)
                      const isSelected = selected.has(file.url)
                      return (
                        <TableRow key={file.url} data-state={isSelected ? 'selected' : undefined}>
                          <TableCell>
                            <Checkbox
                              checked={isSelected}
                              aria-label={`选择 ${name}`}
                              onCheckedChange={() => toggleFile(file)}
                            />
                          </TableCell>
                          <TableCell>
                            <button
                              type="button"
                              onClick={() => setSelectedImage(file)}
                              className="checker-well block size-11 overflow-hidden rounded-md border cursor-pointer"
                              title={`预览 ${name}`}
                            >
                              {failedThumbs.has(file.url) ? (
                                <span className="grid h-full w-full place-items-center text-muted-foreground">
                                  <ImageOff className="size-4" />
                                </span>
                              ) : (
                                <img
                                  src={file.thumb_url}
                                  alt={name}
                                  loading="lazy"
                                  decoding="async"
                                  onError={() => setFailedThumbs(prev => new Set([...prev, file.url]))}
                                  className="h-full w-full object-cover"
                                />
                              )}
                            </button>
                          </TableCell>
                          <TableCell className="max-w-[260px]">
                            <p className="truncate font-medium" title={name}>{name}</p>
                            {file.original_name && (
                              <p className="truncate font-mono text-[11px] text-muted-foreground" title={file.name}>
                                {file.name}
                              </p>
                            )}
                            <p className="truncate font-mono text-[11px] text-muted-foreground/80" title={file.path}>
                              {file.path}
                            </p>
                          </TableCell>
                          <TableCell>
                            <Badge variant="secondary" className="font-mono text-[10px]">{fileExt(file)}</Badge>
                          </TableCell>
                          <TableCell className="font-mono text-xs whitespace-nowrap tabular-nums">
                            {file.size_human || '-'}
                          </TableCell>
                          <TableCell className="font-mono text-xs text-muted-foreground whitespace-nowrap tabular-nums">
                            {file.modified_at || '-'}
                          </TableCell>
                          <TableCell className="text-right">
                            <div className="inline-flex items-center justify-end gap-1">
                              <Button type="button" size="xs" variant="ghost" onClick={() => void copy(file.url)}>
                                <Copy className="size-3" />
                                复制
                              </Button>
                              {file.webp_url && (
                                <Button type="button" size="xs" variant="ghost" onClick={() => void copy(file.webp_url!)}>
                                  WebP
                                </Button>
                              )}
                              <Button size="icon-sm" variant="ghost" className="size-7" asChild title="查看">
                                <a href={file.url} target="_blank" rel="noopener noreferrer">
                                  <ExternalLink className="size-3.5" />
                                </a>
                              </Button>
                              <Button
                                type="button"
                                size="icon-sm"
                                variant="ghost"
                                className="size-7"
                                title="删除"
                                onClick={() => void removeOne(file)}
                              >
                                <Trash2 className="size-3.5" />
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </div>
            ) : (
              <div className="flex flex-col items-center justify-center gap-1.5 rounded-lg border border-dashed py-14 text-center">
                <Inbox className="size-7 text-muted-foreground/50" />
                <p className="text-sm font-medium">{files.length ? '没有匹配的文件' : '当前目录没有文件'}</p>
                <p className="text-xs text-muted-foreground">
                  {files.length ? '换个关键词继续过滤当前目录。' : '可从左侧目录树进入其他目录浏览。'}
                </p>
              </div>
            )}
          </div>
        </div>
      </Card>

      <Dialog open={Boolean(selectedImage)} onOpenChange={open => { if (!open) setSelectedImage(null) }}>
        <DialogContent className="sm:max-w-4xl p-4">
          {selectedImage && (
            <>
              <DialogHeader>
                <DialogTitle className="truncate pr-8 text-sm font-medium">{displayName(selectedImage)}</DialogTitle>
                <DialogDescription className="truncate font-mono text-xs">{selectedImage.path}</DialogDescription>
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
                    打开原图
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
