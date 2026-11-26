import { useEffect, useMemo, useRef, useState } from 'react'
import { AlertCircle, Copy, ExternalLink, Loader2, Sparkles, Trash2 } from 'lucide-react'
import type { NoticeType } from '@/shared/notify'
import { formatSize } from '@/shared/format'
import { useCopy } from '@/shared/useCopy'
import type { WorkspaceAsset, WorkspaceFolder, WorkspaceTransform } from '@/shared/workspaceApi'
import { workspaceApi } from '@/shared/workspaceApi'
import { useConfirmDialog } from '@/components/ConfirmDialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'

const ROOT_SENTINEL = '__root__'

interface WorkspaceImageDetailProps {
  id: string
  folders: WorkspaceFolder[]
  onNotice: (message: string, type?: NoticeType) => void
  onRemoved: () => void
  onChanged: () => void
}

export default function WorkspaceImageDetail({
  id,
  folders,
  onNotice,
  onRemoved,
  onChanged
}: WorkspaceImageDetailProps) {
  const copy = useMemo(() => useCopy(onNotice), [onNotice])
  const { confirm, dialog: confirmDialog } = useConfirmDialog()
  const formRef = useRef<HTMLFormElement>(null)

  const [image, setImage] = useState<WorkspaceAsset | undefined>()
  const [variants, setVariants] = useState<WorkspaceAsset[]>([])
  const [destination, setDestination] = useState('')
  const [sizeMode, setSizeMode] = useState<'pixels' | 'scale' | 'original'>('pixels')
  const [width, setWidth] = useState('800')
  const [height, setHeight] = useState('')
  const [scale, setScale] = useState('0.5')
  const [format, setFormat] = useState<WorkspaceTransform['format']>('png')
  const [fit, setFit] = useState<NonNullable<WorkspaceTransform['fit']>>('scale-down')
  const [quality, setQuality] = useState('85')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      setError('')
      try {
        const data = await workspaceApi.image(id)
        if (cancelled) return
        setImage(data.image)
        setVariants(data.variants)
        setDestination(data.image.folder)
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : '无法加载图片')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [id])

  function buildTransformOptions(): WorkspaceTransform | undefined {
    if (!formRef.current?.reportValidity()) return undefined
    setError('')
    const options: WorkspaceTransform = { format }
    if (format !== 'png') options.quality = Number(quality) || 85
    if (sizeMode === 'scale') options.scale = Number(scale)
    if (sizeMode === 'pixels') {
      const w = width ? Number(width) : 0
      const h = height ? Number(height) : 0
      if ((!w && !h) || (fit === 'cover' && (!w || !h))) {
        setError(fit === 'cover' ? '居中裁剪需要同时填写宽度和高度' : '请至少填写宽度或高度')
        return undefined
      }
      if (w) options.width = w
      if (h) options.height = h
      options.fit = fit
    }
    return options
  }

  function copyDynamicLink() {
    const options = buildTransformOptions()
    if (!options || !image) return
    const url = new URL(image.url, window.location.href)
    for (const [key, value] of Object.entries(options)) url.searchParams.set(key, String(value))
    void copy(url.toString(), '已复制动态转换链接')
  }

  async function saveVariant(event: React.FormEvent) {
    event.preventDefault()
    const options = buildTransformOptions()
    if (!options) return
    setBusy(true)
    try {
      const item = await workspaceApi.createVariant(id, destination, options)
      setVariants(prev => [item, ...prev])
      onNotice('已预生成版本', 'success')
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : '转换失败')
    } finally {
      setBusy(false)
    }
  }

  async function remove(variant?: WorkspaceAsset) {
    const ok = await confirm({
      title: variant ? '删除预生成版本？' : '删除原图及所有版本？',
      description: variant
        ? `确认删除版本「${variant.filename}」？`
        : '删除原图会同时删除它的全部预生成版本，此操作不可恢复。',
      confirmLabel: '确认删除'
    })
    if (!ok) return
    setBusy(true)
    setError('')
    try {
      if (variant) {
        await workspaceApi.deleteVariant(id, variant.id)
        setVariants(prev => prev.filter(item => item.id !== variant.id))
        onChanged()
      } else {
        await workspaceApi.deleteImage(id)
        onRemoved()
      }
      onNotice('已删除', 'success')
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败')
    } finally {
      setBusy(false)
    }
  }

  if (loading) {
    return <Skeleton className="h-80 rounded-xl" />
  }

  return (
    <Card id="workspace-image-detail" tabIndex={-1} aria-label="图片详情">
      {confirmDialog}

      {image && (
        <>
          <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
            <div className="min-w-0 space-y-1">
              <CardTitle className="truncate text-base">{image.filename}</CardTitle>
              <CardDescription className="font-mono text-xs">
                {image.folder || '根目录'} · {formatSize(image.size_bytes)}
                {image.width && image.height ? ` · ${image.width} × ${image.height}` : ''}
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="xs" asChild>
                <a href={image.url} target="_blank" rel="noopener noreferrer">
                  <ExternalLink className="size-3.5" />
                  查看原图
                </a>
              </Button>
              <Button type="button" variant="outline" size="xs" disabled={busy} onClick={() => void remove()}>
                <Trash2 className="size-3.5" />
                删除原图
              </Button>
            </div>
          </CardHeader>

          <CardContent className="space-y-5">
            <div className="grid gap-5 md:grid-cols-2">
              <a
                href={image.url}
                target="_blank"
                rel="noopener noreferrer"
                className="checker-well flex min-h-56 items-center justify-center overflow-hidden rounded-lg border p-3"
              >
                <img
                  src={image.url}
                  alt={image.filename}
                  decoding="async"
                  className="max-h-72 max-w-full rounded object-contain"
                />
              </a>

              <form ref={formRef} onSubmit={saveVariant} className="space-y-3">
                <p className="text-sm font-medium">实时转换与预生成</p>
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1.5">
                    <Label className="text-xs">输出格式</Label>
                    <Select value={format} onValueChange={v => setFormat(v as WorkspaceTransform['format'])} disabled={busy}>
                      <SelectTrigger size="sm">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="png">PNG</SelectItem>
                        <SelectItem value="jpeg">JPEG</SelectItem>
                        <SelectItem value="webp">WebP</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="space-y-1.5">
                    <Label className="text-xs">尺寸模式</Label>
                    <Select value={sizeMode} onValueChange={v => setSizeMode(v as 'pixels' | 'scale' | 'original')} disabled={busy}>
                      <SelectTrigger size="sm">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="pixels">指定像素</SelectItem>
                        <SelectItem value="scale">按比例缩放</SelectItem>
                        <SelectItem value="original">保持原尺寸</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                {sizeMode === 'pixels' && (
                  <>
                    <div className="grid grid-cols-2 gap-3">
                      <div className="space-y-1.5">
                        <Label className="text-xs">宽度（px）</Label>
                        <Input
                          type="number"
                          min={1}
                          max={4096}
                          value={width}
                          onChange={e => setWidth(e.target.value)}
                          placeholder="自动"
                          disabled={busy}
                          className="h-8"
                        />
                      </div>
                      <div className="space-y-1.5">
                        <Label className="text-xs">高度（px）</Label>
                        <Input
                          type="number"
                          min={1}
                          max={4096}
                          value={height}
                          onChange={e => setHeight(e.target.value)}
                          placeholder="自动"
                          disabled={busy}
                          className="h-8"
                        />
                      </div>
                    </div>
                    <div className="space-y-1.5">
                      <Label className="text-xs">适配方式</Label>
                      <Select value={fit} onValueChange={v => setFit(v as NonNullable<WorkspaceTransform['fit']>)} disabled={busy}>
                        <SelectTrigger size="sm">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="scale-down">按比例缩小</SelectItem>
                          <SelectItem value="contain">按比例缩放</SelectItem>
                          <SelectItem value="cover">居中裁剪填满</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </>
                )}

                {sizeMode === 'scale' && (
                  <div className="space-y-1.5">
                    <Label className="text-xs">缩放倍数（0.01–4）</Label>
                    <Input
                      type="number"
                      min={0.01}
                      max={4}
                      step="any"
                      required
                      value={scale}
                      onChange={e => setScale(e.target.value)}
                      disabled={busy}
                      className="h-8"
                    />
                  </div>
                )}

                {format !== 'png' && (
                  <div className="space-y-1.5">
                    <Label className="text-xs">画质（1–100）</Label>
                    <Input
                      type="number"
                      min={1}
                      max={100}
                      required
                      value={quality}
                      onChange={e => setQuality(e.target.value)}
                      disabled={busy}
                      className="h-8"
                    />
                  </div>
                )}

                <div className="space-y-1.5">
                  <Label className="text-xs">保存目录</Label>
                  <Select
                    value={destination || ROOT_SENTINEL}
                    onValueChange={v => setDestination(v === ROOT_SENTINEL ? '' : v)}
                    disabled={busy}
                  >
                    <SelectTrigger size="sm">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={ROOT_SENTINEL}>根目录</SelectItem>
                      {folders.map(folder => (
                        <SelectItem key={folder.path} value={folder.path}>{folder.path}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                <div className="flex flex-wrap gap-2 pt-1">
                  <Button type="button" variant="outline" size="sm" disabled={busy} onClick={copyDynamicLink}>
                    <Copy className="size-3.5" />
                    复制动态链接
                  </Button>
                  <Button type="submit" size="sm" disabled={busy}>
                    {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Sparkles className="size-3.5" />}
                    {busy ? '处理中...' : '预生成版本'}
                  </Button>
                </div>
              </form>
            </div>

            <Separator />

            <div className="space-y-2.5">
              <p className="text-sm font-medium">预生成版本（{variants.length}）</p>
              {variants.length === 0 ? (
                <p className="text-xs text-muted-foreground">暂无预生成版本</p>
              ) : (
                <ul className="divide-y rounded-lg border">
                  {variants.map(item => (
                    <li key={item.id} className="flex flex-wrap items-center justify-between gap-2 px-3 py-2 text-sm">
                      <div className="min-w-0">
                        <a
                          href={item.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="block truncate font-medium underline-offset-4 hover:underline"
                        >
                          {item.filename}
                        </a>
                        <p className="font-mono text-xs text-muted-foreground">
                          {item.folder || '根目录'} · {formatSize(item.size_bytes)}
                        </p>
                      </div>
                      <div className="flex items-center gap-1.5">
                        <Button type="button" size="xs" variant="outline" onClick={() => void copy(item.url)}>
                          <Copy className="size-3" />
                          复制
                        </Button>
                        <Button type="button" size="xs" variant="ghost" disabled={busy} onClick={() => void remove(item)}>
                          <Trash2 className="size-3" />
                          删除
                        </Button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </CardContent>
        </>
      )}

      {error && (
        <CardContent className="pt-0">
          <Alert variant="emphasis">
            <AlertCircle className="size-4" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        </CardContent>
      )}
    </Card>
  )
}
