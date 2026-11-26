import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ChevronLeft, ChevronRight, Copy, ExternalLink, Loader2, Search, X } from 'lucide-react'
import type { AdminURLList } from '@/types'
import type { NoticeType } from '@/shared/notify'
import { adminApi } from '@/shared/adminApi'
import { useCopy } from '@/shared/useCopy'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

export default function AdminUrlList({ onNotice }: { onNotice: (message: string, type?: NoticeType) => void }) {
  const copy = useMemo(() => useCopy(onNotice), [onNotice])
  const [loading, setLoading] = useState(true)
  const [data, setData] = useState<AdminURLList | null>(null)
  const [queryInput, setQueryInput] = useState('')
  const inflightRef = useRef<AbortController | null>(null)
  const queryRef = useRef(queryInput)
  queryRef.current = queryInput

  const load = useCallback(async (nextPage = 1, explicitQuery?: string) => {
    inflightRef.current?.abort()
    const controller = new AbortController()
    inflightRef.current = controller

    const q = (explicitQuery !== undefined ? explicitQuery : queryRef.current).trim()
    const params = new URLSearchParams({ page: String(nextPage), page_size: '50' })
    if (q) params.set('q', q)

    const typedQuery = queryRef.current
    setLoading(true)
    try {
      const payload = await adminApi.urlList(params, { signal: controller.signal })
      setData(payload)
      if (queryRef.current === typedQuery) setQueryInput(payload.q || '')
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') return
      onNotice(error instanceof Error ? error.message : '加载图片列表失败', 'danger')
    } finally {
      if (inflightRef.current === controller) {
        inflightRef.current = null
        setLoading(false)
      }
    }
  }, [onNotice])

  useEffect(() => {
    void load(1)
    return () => inflightRef.current?.abort()
  }, [load])

  if (loading && !data) {
    return <Skeleton className="h-96 rounded-xl" />
  }

  if (!data) return null

  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <CardTitle className="flex items-center gap-2 text-base">
            图片 URL 列表
            <Badge variant="secondary" className="font-mono tabular-nums">{data.total} 张</Badge>
          </CardTitle>
          <CardDescription>
            第 {data.page} / {Math.max(1, data.total_pages)} 页{data.q ? ` · 搜索「${data.q}」` : ''}
          </CardDescription>
        </div>

        <form
          role="search"
          onSubmit={event => {
            event.preventDefault()
            void load(1)
          }}
          className="flex w-full gap-2 sm:w-auto"
        >
          <div className="relative flex-1 sm:w-72">
            <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="search"
              value={queryInput}
              onChange={e => setQueryInput(e.target.value)}
              placeholder="搜索源文件名、存储名或路径"
              className="pl-9"
            />
          </div>
          <Button type="submit" disabled={loading}>
            {loading && <Loader2 className="size-4 animate-spin" />}
            搜索
          </Button>
          {data.q && (
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setQueryInput('')
                void load(1, '')
              }}
            >
              <X className="size-4" />
            </Button>
          )}
        </form>
      </CardHeader>

      <CardContent className="space-y-4">
        {data.files.length > 0 ? (
          <div className="rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-20">预览</TableHead>
                  <TableHead>文件名</TableHead>
                  <TableHead>图片链接</TableHead>
                  <TableHead className="w-36 text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.files.map(file => {
                  const name = file.original_name || file.name
                  return (
                    <TableRow key={file.url}>
                      <TableCell>
                        <img
                          src={file.thumb_url}
                          alt={name}
                          loading="lazy"
                          decoding="async"
                          className="size-12 rounded-md border object-cover bg-muted"
                        />
                      </TableCell>
                      <TableCell className="max-w-[220px]">
                        <p className="truncate font-medium" title={name}>{name}</p>
                        {file.original_name && (
                          <p className="truncate font-mono text-xs text-muted-foreground" title={file.name}>
                            {file.name}
                          </p>
                        )}
                      </TableCell>
                      <TableCell className="max-w-[380px] space-y-1">
                        <a
                          href={file.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="block truncate font-mono text-xs text-foreground underline-offset-4 hover:underline"
                          title={file.url}
                        >
                          {file.url}
                        </a>
                        {file.webp_url && (
                          <a
                            href={file.webp_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="block truncate font-mono text-[11px] text-muted-foreground underline-offset-4 hover:underline"
                            title={file.webp_url}
                          >
                            WebP: {file.webp_url}
                          </a>
                        )}
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="inline-flex items-center justify-end gap-1.5">
                          <Button type="button" size="xs" onClick={() => void copy(file.url)}>
                            <Copy className="size-3" />
                            复制
                          </Button>
                          {file.webp_url && (
                            <Button type="button" size="xs" variant="outline" onClick={() => void copy(file.webp_url!)}>
                              WebP
                            </Button>
                          )}
                          <Button size="icon-sm" variant="ghost" className="size-7" asChild title="打开">
                            <a href={file.url} target="_blank" rel="noopener noreferrer">
                              <ExternalLink className="size-3.5" />
                            </a>
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
          <p className="py-12 text-center text-sm text-muted-foreground">
            {data.q ? '没有匹配的图片。' : '暂无图片。'}
          </p>
        )}

        <div className="flex items-center justify-center gap-3 pt-1">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={loading || data.page <= 1}
            onClick={() => void load(data.page - 1)}
          >
            <ChevronLeft className="size-4" />
            上一页
          </Button>
          <span className="font-mono text-xs text-muted-foreground tabular-nums">
            {data.page} / {Math.max(1, data.total_pages)}
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={loading || data.page >= data.total_pages}
            onClick={() => void load(data.page + 1)}
          >
            下一页
            <ChevronRight className="size-4" />
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
