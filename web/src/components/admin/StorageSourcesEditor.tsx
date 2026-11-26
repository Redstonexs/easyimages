import { useMemo, useState } from 'react'
import { AlertCircle, ChevronDown, ChevronUp, Plus, Trash2 } from 'lucide-react'
import type { StorageSourceConfig } from '@/types'
import {
  createS3StorageSource,
  normalizeStorageSource,
  normalizeStorageSources,
  validateStorageSources
} from '@/shared/storageSources'
import { useConfirmDialog } from '@/components/ConfirmDialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

interface StorageSourcesEditorProps {
  sources: StorageSourceConfig[]
  defaultSource: string
  onChangeSources: (sources: StorageSourceConfig[]) => void
  onChangeDefaultSource: (id: string) => void
}

function sourceSummary(source: StorageSourceConfig): string {
  if (source.type === 'local') return '使用服务器本地 i/ 目录存储图片'
  return [source.s3_bucket, source.s3_endpoint || 'AWS S3 默认端点'].filter(Boolean).join(' · ') || '尚未填写 Bucket'
}

export default function StorageSourcesEditor({
  sources,
  defaultSource,
  onChangeSources,
  onChangeDefaultSource
}: StorageSourcesEditorProps) {
  const [expandedIndex, setExpandedIndex] = useState<number | null>(null)
  const { confirm, dialog: confirmDialog } = useConfirmDialog()

  const enabledSources = useMemo(() => sources.filter(source => source.enabled), [sources])
  const validationMessages = useMemo(
    () => validateStorageSources(sources, defaultSource),
    [sources, defaultSource]
  )

  function commit(nextSources: StorageSourceConfig[]) {
    const normalized = normalizeStorageSources(nextSources)
    onChangeSources(normalized)
    if (!normalized.some(source => source.id === defaultSource && source.enabled)) {
      onChangeDefaultSource(normalized.find(source => source.enabled)?.id || 'local')
    }
  }

  function updateSource(index: number, patch: Partial<StorageSourceConfig>) {
    const next = sources.map((source, idx) =>
      idx === index ? normalizeStorageSource({ ...source, ...patch }) : source
    )
    commit(next)
  }

  function addS3Source() {
    const nextSource = createS3StorageSource(sources)
    setExpandedIndex(sources.length)
    commit([...sources, nextSource])
  }

  async function removeSource(index: number) {
    const source = sources[index]
    if (!source || source.id === 'local') return
    const ok = await confirm({
      title: `删除存储源「${source.name || source.id}」？`,
      description: '已上传到该存储源的历史图片记录仍会保留，但无法继续向其上传新文件。',
      confirmLabel: '确认删除'
    })
    if (!ok) return
    commit(sources.filter((_, idx) => idx !== index))
    if (expandedIndex === index) setExpandedIndex(null)
  }

  return (
    <div className="space-y-4">
      {confirmDialog}

      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="flex-1 space-y-2 sm:max-w-xs">
          <Label>默认上传源</Label>
          <Select value={defaultSource} onValueChange={onChangeDefaultSource}>
            <SelectTrigger>
              <SelectValue placeholder="选择默认存储源" />
            </SelectTrigger>
            <SelectContent>
              {enabledSources.map(source => (
                <SelectItem key={source.id} value={source.id}>
                  {source.name} ({source.type})
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={addS3Source}>
          <Plus className="size-4" />
          添加 S3 存储源
        </Button>
      </div>

      {validationMessages.length > 0 && (
        <Alert variant="muted">
          <AlertCircle className="size-4" />
          <AlertTitle>存储源配置需要调整</AlertTitle>
          <AlertDescription>
            <ul className="list-disc pl-4 space-y-0.5 text-xs">
              {validationMessages.map(msg => <li key={msg}>{msg}</li>)}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      <div className="space-y-3">
        {sources.map((source, index) => {
          const isLocal = source.id === 'local'
          const isIDLocked = isLocal || source.s3_secret_set !== undefined
          const expanded = expandedIndex === index

          return (
            <article
              key={`${source.id || 'source'}-${index}`}
              className={`rounded-xl border bg-card transition-opacity ${source.enabled ? '' : 'opacity-70'}`}
            >
              <header className="flex flex-wrap items-center justify-between gap-3 p-4">
                <div className="min-w-0 space-y-1">
                  <div className="flex items-center gap-2">
                    <Badge variant={source.enabled ? 'default' : 'secondary'} className="font-mono uppercase text-[10px]">
                      {source.type}
                    </Badge>
                    <h4 className="truncate text-sm font-semibold">{source.name || source.id || '未命名存储源'}</h4>
                    {defaultSource === source.id && (
                      <Badge variant="outline" className="text-[10px]">默认</Badge>
                    )}
                  </div>
                  <p className="truncate text-xs text-muted-foreground">{sourceSummary(source)}</p>
                </div>

                <div className="flex items-center gap-2.5">
                  <div className="flex items-center gap-1.5">
                    <Switch
                      checked={source.enabled}
                      disabled={isLocal}
                      aria-label={`启用 ${source.name || source.id}`}
                      onCheckedChange={checked => updateSource(index, { enabled: checked })}
                    />
                    <span className="text-xs text-muted-foreground">
                      {source.enabled ? '已启用' : '已停用'}
                    </span>
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    size="xs"
                    onClick={() => setExpandedIndex(expanded ? null : index)}
                  >
                    {expanded ? <ChevronUp className="size-3.5" /> : <ChevronDown className="size-3.5" />}
                    {expanded ? '收起' : '编辑'}
                  </Button>
                  {!isLocal && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      className="size-7"
                      title="删除存储源"
                      onClick={() => void removeSource(index)}
                    >
                      <Trash2 className="size-3.5" />
                      <span className="sr-only">删除</span>
                    </Button>
                  )}
                </div>
              </header>

              {expanded && (
                <div className="space-y-4 border-t p-4">
                  <div className="grid gap-3 sm:grid-cols-2">
                    <div className="space-y-1.5">
                      <Label>显示名称</Label>
                      <Input
                        value={source.name}
                        onChange={e => updateSource(index, { name: e.target.value })}
                      />
                    </div>
                    <div className="space-y-1.5">
                      <Label>存储源 ID</Label>
                      <Input
                        value={source.id}
                        disabled={isIDLocked}
                        onChange={e => updateSource(index, { id: e.target.value })}
                        className="font-mono text-xs"
                      />
                    </div>
                    <div className="space-y-1.5 sm:col-span-2">
                      <Label>公开访问地址（Public Base URL）</Label>
                      <Input
                        value={source.public_base_url}
                        placeholder="https://cdn.example.com/images"
                        onChange={e => updateSource(index, { public_base_url: e.target.value })}
                      />
                    </div>
                  </div>

                  {source.type === 's3' && (
                    <>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <div className="space-y-1.5">
                          <Label>Endpoint</Label>
                          <Input
                            value={source.s3_endpoint}
                            placeholder="https://s3.example.com"
                            onChange={e => updateSource(index, { s3_endpoint: e.target.value })}
                          />
                        </div>
                        <div className="space-y-1.5">
                          <Label>Region</Label>
                          <Input
                            value={source.s3_region}
                            placeholder="auto"
                            onChange={e => updateSource(index, { s3_region: e.target.value })}
                          />
                        </div>
                        <div className="space-y-1.5">
                          <Label>Bucket</Label>
                          <Input
                            value={source.s3_bucket}
                            onChange={e => updateSource(index, { s3_bucket: e.target.value })}
                          />
                        </div>
                        <div className="space-y-1.5">
                          <Label>Prefix（可选）</Label>
                          <Input
                            value={source.s3_prefix}
                            placeholder="uploads"
                            onChange={e => updateSource(index, { s3_prefix: e.target.value })}
                          />
                        </div>
                        <div className="space-y-1.5">
                          <Label>Access Key ID</Label>
                          <Input
                            autoComplete="off"
                            value={source.s3_access_key_id}
                            onChange={e => updateSource(index, { s3_access_key_id: e.target.value })}
                            className="font-mono text-xs"
                          />
                        </div>
                        <div className="space-y-1.5">
                          <Label>Access Key Secret</Label>
                          <Input
                            type="password"
                            autoComplete="new-password"
                            value={source.s3_access_key_secret || ''}
                            placeholder={source.s3_secret_set ? '已设置，留空保留原密钥' : '请输入访问密钥'}
                            onChange={e => updateSource(index, { s3_access_key_secret: e.target.value })}
                            className="font-mono text-xs"
                          />
                        </div>
                      </div>

                      <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
                        <div className="space-y-0.5">
                          <Label>使用 Path-Style 地址</Label>
                          <p className="text-xs text-muted-foreground">
                            MinIO 等自建对象存储通常需要开启。密钥不会在接口回显，留空即保持不变。
                          </p>
                        </div>
                        <Switch
                          checked={source.s3_force_path_style}
                          onCheckedChange={checked => updateSource(index, { s3_force_path_style: checked })}
                        />
                      </div>
                    </>
                  )}
                </div>
              )}
            </article>
          )
        })}
      </div>
    </div>
  )
}
