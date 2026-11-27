import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Database,
  Files,
  HardDrive,
  Image as ImageIcon,
  Loader2,
  RefreshCw,
  RotateCcw,
  Save,
  Tag,
  Upload
} from 'lucide-react'
import type { AdminConfig as AdminConfigType, AdminOverview } from '@/types'
import type { NoticeType } from '@/shared/notify'
import { adminApi } from '@/shared/adminApi'
import { normalizeStorageSources, validateStorageSources } from '@/shared/storageSources'
import { useDelayedLoading } from '@/shared/useDelayedLoading'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import StorageSourcesEditor from '@/components/admin/StorageSourcesEditor'

interface AdminConfigProps {
  onNotice: (message: string, type?: NoticeType) => void
  onDirtyChange: (dirty: boolean) => void
}

export default function AdminConfig({ onNotice, onDirtyChange }: AdminConfigProps) {
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [converting, setConverting] = useState(false)
  const [uploadingIcon, setUploadingIcon] = useState(false)
  const [overview, setOverview] = useState<AdminOverview | null>(null)
  const [config, setConfig] = useState<AdminConfigType | null>(null)
  const [snapshot, setSnapshot] = useState('')
  const siteIconInputRef = useRef<HTMLInputElement>(null)
  const showSkeleton = useDelayedLoading(loading)

  const isDirty = useMemo(() => {
    if (!config || !snapshot) return false
    return JSON.stringify(config) !== snapshot
  }, [config, snapshot])

  useEffect(() => {
    onDirtyChange(isDirty)
    if (!isDirty) return
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', warn)
    return () => {
      window.removeEventListener('beforeunload', warn)
    }
  }, [isDirty, onDirtyChange])

  useEffect(() => {
    return () => onDirtyChange(false)
  }, [onDirtyChange])

  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      try {
        const data = await adminApi.overview()
        if (cancelled) return
        data.config.storage_sources = normalizeStorageSources(data.config.storage_sources)
        setOverview(data)
        setConfig(data.config)
        setSnapshot(JSON.stringify(data.config))
      } catch (error) {
        if (!cancelled) onNotice(error instanceof Error ? error.message : '加载配置失败', 'danger')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [onNotice])

  function updateField<K extends keyof AdminConfigType>(key: K, value: AdminConfigType[K]) {
    setConfig(prev => (prev ? { ...prev, [key]: value } : prev))
  }

  const save = useCallback(async (event?: React.FormEvent) => {
    event?.preventDefault()
    if (!config || saving) return
    const normalizedSources = normalizeStorageSources(config.storage_sources)
    const nextConfig: AdminConfigType = { ...config, storage_sources: normalizedSources }
    const storageErrors = validateStorageSources(normalizedSources, nextConfig.default_storage_source)
    if (storageErrors.length) {
      onNotice(storageErrors[0], 'danger')
      return
    }
    setSaving(true)
    try {
      const result = await adminApi.saveConfig(nextConfig)
      result.config.storage_sources = normalizeStorageSources(result.config.storage_sources)
      setConfig(result.config)
      setSnapshot(JSON.stringify(result.config))
      onNotice(result.msg || '配置已保存', 'success')
    } catch (error) {
      onNotice(error instanceof Error ? error.message : '保存失败', 'danger')
    } finally {
      setSaving(false)
    }
  }, [config, saving, onNotice])

  function resetChanges() {
    if (!snapshot) return
    try {
      setConfig(JSON.parse(snapshot) as AdminConfigType)
      onNotice('已还原未保存的更改', 'info')
    } catch {
      // Ignore malformed snapshot.
    }
  }

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
        event.preventDefault()
        void save()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [save])

  async function batchWebP() {
    setConverting(true)
    try {
      const result = await adminApi.batchWebP()
      onNotice(result.message || '转换完成', result.result === 'success' ? 'success' : 'warning')
    } catch (error) {
      onNotice(error instanceof Error ? error.message : '转换失败', 'danger')
    } finally {
      setConverting(false)
    }
  }

  async function uploadSiteIcon() {
    if (!config) return
    const file = siteIconInputRef.current?.files?.[0]
    if (!file) {
      onNotice('请选择图标文件', 'warning')
      return
    }
    const formData = new FormData()
    formData.append('icon', file)
    setUploadingIcon(true)
    try {
      const result = await adminApi.uploadSiteIcon(formData)
      setConfig(prev => (prev ? { ...prev, site_icon: result.site_icon } : prev))
      setSnapshot(prev => {
        if (!prev) return prev
        try {
          const parsed = JSON.parse(prev) as AdminConfigType
          parsed.site_icon = result.site_icon
          return JSON.stringify(parsed)
        } catch {
          return prev
        }
      })
      if (siteIconInputRef.current) siteIconInputRef.current.value = ''
      onNotice(result.message || '图标已更新', 'success')
    } catch (error) {
      onNotice(error instanceof Error ? error.message : '上传图标失败', 'danger')
    } finally {
      setUploadingIcon(false)
    }
  }

  if (loading) {
    if (!showSkeleton) return <div className="min-h-[420px]" aria-busy="true" />
    return (
      <div className="panel-enter space-y-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <Skeleton className="h-24 rounded-xl" />
          <Skeleton className="h-24 rounded-xl" />
          <Skeleton className="h-24 rounded-xl" />
        </div>
        <Skeleton className="h-96 rounded-xl" />
      </div>
    )
  }

  if (!config) return null

  return (
    <form onSubmit={save} className="panel-enter space-y-6 pb-20">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Card className="gap-1 py-4">
          <CardContent className="flex items-center justify-between">
            <div>
              <p className="text-xs text-muted-foreground">当前版本</p>
              <p className="mt-1 font-mono text-xl font-semibold tabular-nums">v{overview?.version}</p>
            </div>
            <Tag className="size-4 text-muted-foreground" />
          </CardContent>
        </Card>
        <Card className="gap-1 py-4">
          <CardContent className="flex items-center justify-between">
            <div>
              <p className="text-xs text-muted-foreground">本地图片总数</p>
              <p className="mt-1 font-mono text-xl font-semibold tabular-nums">{overview?.total_files ?? 0}</p>
            </div>
            <Files className="size-4 text-muted-foreground" />
          </CardContent>
        </Card>
        <Card className="gap-1 py-4">
          <CardContent className="flex items-center justify-between">
            <div>
              <p className="text-xs text-muted-foreground">已用空间</p>
              <p className="mt-1 font-mono text-xl font-semibold tabular-nums">{overview?.used_human || '0 B'}</p>
            </div>
            <HardDrive className="size-4 text-muted-foreground" />
          </CardContent>
        </Card>
      </div>

      <Tabs defaultValue="site" className="space-y-4">
        <TabsList className="w-full sm:w-fit">
          <TabsTrigger value="site">站点与图标</TabsTrigger>
          <TabsTrigger value="upload">上传与 WebP</TabsTrigger>
          <TabsTrigger value="watermark">水印</TabsTrigger>
          <TabsTrigger value="security">安全与防盗链</TabsTrigger>
          <TabsTrigger value="storage">存储源</TabsTrigger>
          <TabsTrigger value="advanced">路径与高级</TabsTrigger>
        </TabsList>

        <TabsContent value="site" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">站点信息</CardTitle>
              <CardDescription>配置站点名称、公开域名和对外图片链接前缀。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="cfg-title">网站名称</Label>
                <Input
                  id="cfg-title"
                  value={config.title}
                  onChange={e => updateField('title', e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="cfg-domain">站点域名</Label>
                <Input
                  id="cfg-domain"
                  value={config.domain}
                  onChange={e => updateField('domain', e.target.value)}
                  placeholder="https://img.example.com"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="cfg-imgurl">图片域名</Label>
                <Input
                  id="cfg-imgurl"
                  value={config.imgurl}
                  onChange={e => updateField('imgurl', e.target.value)}
                  placeholder="https://cdn.example.com"
                />
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">网站图标</CardTitle>
              <CardDescription>支持 ICO、PNG 或 SVG，不超过 512KB，上传后立即生效。</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4 sm:flex-row sm:items-center">
              <div className="checker-well grid size-16 shrink-0 place-items-center rounded-xl border">
                <img src={config.site_icon} alt="当前网站图标" className="size-9 object-contain" />
              </div>
              <div className="flex flex-1 flex-col gap-2 sm:flex-row sm:items-center">
                <Input
                  ref={siteIconInputRef}
                  type="file"
                  accept=".ico,.png,.svg,image/x-icon,image/png,image/svg+xml"
                  aria-label="选择网站图标"
                />
                <Button type="button" variant="outline" disabled={uploadingIcon} onClick={() => void uploadSiteIcon()} className="shrink-0">
                  {uploadingIcon ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
                  {uploadingIcon ? '上传中...' : '上传图标'}
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="upload" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">上传限制与访问策略</CardTitle>
              <CardDescription>控制单文件体积上限、允许扩展名与私有上传模式。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="flex items-center justify-between gap-4 rounded-lg border p-3.5">
                <div className="space-y-0.5">
                  <Label htmlFor="cfg-must-login">私有上传模式</Label>
                  <p className="text-xs text-muted-foreground">开启后仅已登录用户可访问首页与上传图片。</p>
                </div>
                <Switch
                  id="cfg-must-login"
                  checked={config.mustLogin === 1}
                  onCheckedChange={checked => updateField('mustLogin', checked ? 1 : 0)}
                />
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="cfg-max-size">最大上传大小（字节）</Label>
                  <Input
                    id="cfg-max-size"
                    type="number"
                    value={config.maxSize}
                    onChange={e => updateField('maxSize', Number(e.target.value) || 0)}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="cfg-quality">压缩质量（1-100）</Label>
                  <Input
                    id="cfg-quality"
                    type="number"
                    min={1}
                    max={100}
                    value={config.compress_ratio}
                    onChange={e => updateField('compress_ratio', Number(e.target.value) || 80)}
                  />
                </div>
                <div className="space-y-2 sm:col-span-2">
                  <Label htmlFor="cfg-ext">允许扩展名（逗号分隔）</Label>
                  <Input
                    id="cfg-ext"
                    value={config.extensions}
                    onChange={e => updateField('extensions', e.target.value)}
                    className="font-mono text-xs"
                  />
                </div>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">缩略图与 WebP</CardTitle>
              <CardDescription>配置缩略图生成时机、尺寸，以及使用 cwebp 自动生成 WebP 副本。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-4 sm:grid-cols-3">
                <div className="space-y-2">
                  <Label>缩略图模式</Label>
                  <Select
                    value={String(config.thumbnail)}
                    onValueChange={v => updateField('thumbnail', Number(v))}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="0">不生成</SelectItem>
                      <SelectItem value="1">访问时生成</SelectItem>
                      <SelectItem value="2">上传时生成</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="cfg-thumb-w">缩略图宽度（px）</Label>
                  <Input
                    id="cfg-thumb-w"
                    type="number"
                    value={config.thumbnail_w}
                    onChange={e => updateField('thumbnail_w', Number(e.target.value) || 0)}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="cfg-thumb-h">缩略图高度（px）</Label>
                  <Input
                    id="cfg-thumb-h"
                    type="number"
                    value={config.thumbnail_h}
                    onChange={e => updateField('thumbnail_h', Number(e.target.value) || 0)}
                  />
                </div>
              </div>

              <div className="flex flex-col gap-4 rounded-lg border p-3.5 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-center gap-3">
                  <Switch
                    id="cfg-webp"
                    checked={config.webp_convert === 1}
                    onCheckedChange={checked => updateField('webp_convert', checked ? 1 : 0)}
                  />
                  <div>
                    <Label htmlFor="cfg-webp">自动生成 WebP</Label>
                    <p className="text-xs text-muted-foreground">跳过现有 WebP 和动态 GIF，依赖服务器安装 cwebp。</p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Label htmlFor="cfg-webp-q" className="text-xs text-muted-foreground whitespace-nowrap">质量</Label>
                  <Input
                    id="cfg-webp-q"
                    type="number"
                    min={1}
                    max={100}
                    value={config.webp_quality}
                    onChange={e => updateField('webp_quality', Number(e.target.value) || 80)}
                    className="w-20"
                  />
                  <Button type="button" variant="outline" size="sm" disabled={converting} onClick={() => void batchWebP()}>
                    {converting ? <Loader2 className="size-3.5 animate-spin" /> : <RefreshCw className="size-3.5" />}
                    {converting ? '转换中...' : '批量生成 WebP'}
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="watermark">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">水印设置</CardTitle>
              <CardDescription>为本地上传的新图片叠加文字或图片水印。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label>水印类型</Label>
                <Select
                  value={String(config.watermark)}
                  onValueChange={v => updateField('watermark', Number(v))}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="0">无水印</SelectItem>
                    <SelectItem value="1">文字水印</SelectItem>
                    <SelectItem value="2">图片水印</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="cfg-water-pos">水印位置（1-9 九宫格）</Label>
                <Input
                  id="cfg-water-pos"
                  type="number"
                  min={1}
                  max={9}
                  value={config.waterPosition}
                  onChange={e => updateField('waterPosition', Number(e.target.value) || 9)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="cfg-water-text">水印文字</Label>
                <Input
                  id="cfg-water-text"
                  value={config.waterText}
                  onChange={e => updateField('waterText', e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="cfg-water-color">文字颜色</Label>
                <Input
                  id="cfg-water-color"
                  value={config.textColor}
                  onChange={e => updateField('textColor', e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="cfg-water-size">文字大小</Label>
                <Input
                  id="cfg-water-size"
                  type="number"
                  value={config.textSize}
                  onChange={e => updateField('textSize', Number(e.target.value) || 16)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="cfg-water-font">字体文件路径</Label>
                <Input
                  id="cfg-water-font"
                  value={config.textFont}
                  onChange={e => updateField('textFont', e.target.value)}
                  className="font-mono text-xs"
                />
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="cfg-water-img">水印图片路径</Label>
                <Input
                  id="cfg-water-img"
                  value={config.waterImg}
                  onChange={e => updateField('waterImg', e.target.value)}
                  className="font-mono text-xs"
                />
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="security" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">登录验证码</CardTitle>
              <CardDescription>支持内置数学题、Cloudflare Turnstile、reCAPTCHA 与自托管 Cap。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between gap-4 rounded-lg border p-3.5">
                <div className="space-y-0.5">
                  <Label htmlFor="cfg-captcha">启用登录验证码</Label>
                  <p className="text-xs text-muted-foreground">防止暴力破解登录表单。</p>
                </div>
                <Switch
                  id="cfg-captcha"
                  checked={config.captcha === 1}
                  onCheckedChange={checked => updateField('captcha', checked ? 1 : 0)}
                />
              </div>

              {config.captcha === 1 && (
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2 sm:col-span-2">
                    <Label>验证码类型</Label>
                    <Select
                      value={String(config.captcha_type)}
                      onValueChange={v => updateField('captcha_type', Number(v))}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="0">内置数学题</SelectItem>
                        <SelectItem value="1">Cloudflare Turnstile</SelectItem>
                        <SelectItem value="2">Google reCAPTCHA</SelectItem>
                        <SelectItem value="3">Cap（自托管）</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  {config.captcha_type === 1 && (
                    <div className="space-y-2 sm:col-span-2">
                      <Label htmlFor="cfg-turnstile">Turnstile Site Key</Label>
                      <Input
                        id="cfg-turnstile"
                        value={config.turnstile_site_key}
                        onChange={e => updateField('turnstile_site_key', e.target.value)}
                        className="font-mono text-xs"
                      />
                    </div>
                  )}

                  {config.captcha_type === 2 && (
                    <div className="space-y-2 sm:col-span-2">
                      <Label htmlFor="cfg-recaptcha">reCAPTCHA Site Key</Label>
                      <Input
                        id="cfg-recaptcha"
                        value={config.recaptcha_site_key}
                        onChange={e => updateField('recaptcha_site_key', e.target.value)}
                        className="font-mono text-xs"
                      />
                    </div>
                  )}

                  {config.captcha_type === 3 && (
                    <>
                      <div className="space-y-2">
                        <Label htmlFor="cfg-cap-url">Cap 实例地址</Label>
                        <Input
                          id="cfg-cap-url"
                          value={config.cap_instance_url}
                          onChange={e => updateField('cap_instance_url', e.target.value)}
                          placeholder="https://cap.example.com"
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="cfg-cap-site">Cap Site Key</Label>
                        <Input
                          id="cfg-cap-site"
                          value={config.cap_site_key}
                          onChange={e => updateField('cap_site_key', e.target.value)}
                          placeholder="10 位十六进制"
                          className="font-mono text-xs"
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="cfg-cap-secret">Cap Secret Key</Label>
                        <Input
                          id="cfg-cap-secret"
                          type="password"
                          autoComplete="new-password"
                          value={config.cap_secret_key || ''}
                          onChange={e => updateField('cap_secret_key', e.target.value)}
                          placeholder={config.cap_secret_set ? '已配置，留空保持不变' : 'sk- 开头，仅服务端保存'}
                          className="font-mono text-xs"
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="cfg-cap-widget">Cap 组件地址（可选）</Label>
                        <Input
                          id="cfg-cap-widget"
                          value={config.cap_widget_url}
                          onChange={e => updateField('cap_widget_url', e.target.value)}
                          placeholder="留空则使用实例的 /assets/widget.js"
                        />
                      </div>
                      <p className="text-xs text-muted-foreground sm:col-span-2 leading-relaxed">
                        Cap Secret Key 仅在保存时上传，不会回显到浏览器。留空组件地址时要求实例开启{' '}
                        <code className="rounded bg-muted px-1 py-0.5 font-mono">ENABLE_ASSETS_SERVER=true</code>。
                      </p>
                    </>
                  )}
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">防盗链保护</CardTitle>
              <CardDescription>仅放行空白 Referer、站点自身域名及下方白名单中的域名和子域名。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between gap-4 rounded-lg border p-3.5">
                <div className="space-y-0.5">
                  <Label htmlFor="cfg-hotlink">开启防盗链</Label>
                  <p className="text-xs text-muted-foreground">未命中白名单的外部来源将返回 403。</p>
                </div>
                <Switch
                  id="cfg-hotlink"
                  checked={config.hotlink_protect === 1}
                  onCheckedChange={checked => updateField('hotlink_protect', checked ? 1 : 0)}
                />
              </div>

              {config.hotlink_protect === 1 && (
                <div className="space-y-2">
                  <Label htmlFor="cfg-hotlink-domains">白名单域名（逗号分隔）</Label>
                  <Textarea
                    id="cfg-hotlink-domains"
                    rows={3}
                    value={config.hotlink_domains}
                    onChange={e => updateField('hotlink_domains', e.target.value)}
                    placeholder="example.com, blog.example.org"
                    className="font-mono text-xs"
                  />
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="storage">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Database className="size-4" />
                存储源管理
              </CardTitle>
              <CardDescription>配置本地存储和兼容 S3 的对象存储源，并指定默认上传目的地。</CardDescription>
            </CardHeader>
            <CardContent>
              <StorageSourcesEditor
                sources={config.storage_sources}
                defaultSource={config.default_storage_source}
                onChangeSources={sources => updateField('storage_sources', sources)}
                onChangeDefaultSource={value => updateField('default_storage_source', value)}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="advanced">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <ImageIcon className="size-4" />
                存储路径与杂项
              </CardTitle>
              <CardDescription>调整目录生成规则、MIME 白名单、时间格式和旧图清理策略。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="cfg-mime">MIME 类型</Label>
                  <Input
                    id="cfg-mime"
                    value={config.mime}
                    onChange={e => updateField('mime', e.target.value)}
                    className="font-mono text-xs"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="cfg-storage-path">路径规则</Label>
                  <Input
                    id="cfg-storage-path"
                    value={config.storage_path}
                    onChange={e => updateField('storage_path', e.target.value)}
                    className="font-mono text-xs"
                  />
                </div>
                <div className="space-y-2 sm:col-span-2">
                  <Label htmlFor="cfg-time-format">时间格式</Label>
                  <Input
                    id="cfg-time-format"
                    value={config.time_format}
                    onChange={e => updateField('time_format', e.target.value)}
                    className="font-mono text-xs"
                  />
                </div>
              </div>

              <div className="flex items-center justify-between gap-4 rounded-lg border p-3.5">
                <div className="space-y-0.5">
                  <Label htmlFor="cfg-auto-delete">自动删除旧图</Label>
                  <p className="text-xs text-muted-foreground">按系统策略自动清理过期的历史图片。</p>
                </div>
                <Switch
                  id="cfg-auto-delete"
                  checked={config.auto_delete === 1}
                  onCheckedChange={checked => updateField('auto_delete', checked ? 1 : 0)}
                />
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Sticky action bar */}
      <div
        className={`sticky bottom-4 z-30 flex flex-wrap items-center justify-between gap-3 rounded-xl border px-4 py-3 backdrop-blur-md transition-[border-color,box-shadow,background-color] duration-200 ${
          isDirty
            ? 'border-foreground/30 bg-card/95 shadow-lg'
            : 'border-border/80 bg-card/90 shadow-sm'
        }`}
      >
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          {isDirty && <span className="size-1.5 rounded-full bg-foreground" />}
          <span>{isDirty ? '检测到配置已修改，按 Ctrl+S 或点击保存生效。' : '当前所有配置均已同步保存。'}</span>
        </div>
        <div className="flex items-center gap-2">
          {isDirty && (
            <Button type="button" variant="ghost" size="sm" disabled={saving} onClick={resetChanges}>
              <RotateCcw className="size-3.5" />
              还原更改
            </Button>
          )}
          <Button type="submit" size="sm" disabled={saving}>
            {saving ? <Loader2 className="size-4 animate-spin" /> : <Save className="size-4" />}
            {saving ? '保存中...' : '保存配置'}
          </Button>
        </div>
      </div>
    </form>
  )
}
