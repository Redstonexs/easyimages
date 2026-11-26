import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertCircle,
  Copy,
  ExternalLink,
  Folder,
  FolderPlus,
  KeyRound,
  Loader2,
  Upload
} from 'lucide-react'
import type { NoticeType } from '@/shared/notify'
import { formatSize } from '@/shared/format'
import { useCopy } from '@/shared/useCopy'
import type { WorkspaceAsset, WorkspaceFolder, WorkspaceUser } from '@/shared/workspaceApi'
import { workspaceApi } from '@/shared/workspaceApi'
import { useConfirmDialog } from '@/components/ConfirmDialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import WorkspaceAdmin from '@/components/admin/WorkspaceAdmin'
import WorkspaceImageDetail from '@/components/admin/WorkspaceImageDetail'

const ROOT_SENTINEL = '__root__'

interface WorkspaceLibraryProps {
  isAdmin: boolean
  maxUploadSize: number
  onNotice: (message: string, type?: NoticeType) => void
}

export default function WorkspaceLibrary({ isAdmin, maxUploadSize, onNotice }: WorkspaceLibraryProps) {
  const copy = useMemo(() => useCopy(onNotice), [onNotice])
  const { confirm, dialog: confirmDialog } = useConfirmDialog()

  const [me, setMe] = useState<WorkspaceUser | undefined>()
  const [users, setUsers] = useState<WorkspaceUser[]>([])
  const [owner, setOwner] = useState('')
  const [section, setSection] = useState<'library' | 'admin'>('library')
  const [folderFilter, setFolderFilter] = useState<string | null>(null)
  const [folders, setFolders] = useState<WorkspaceFolder[]>([])
  const [folderPath, setFolderPath] = useState('')
  const [uploadFolder, setUploadFolder] = useState('')
  const uploadInputRef = useRef<HTMLInputElement>(null)
  const [images, setImages] = useState<WorkspaceAsset[]>([])
  const [cursor, setCursor] = useState('')
  const [selected, setSelected] = useState('')
  const [loadingImages, setLoadingImages] = useState(true)
  const [action, setAction] = useState<'upload' | 'folder' | ''>('')
  const [token, setToken] = useState('')
  const [tokenBusy, setTokenBusy] = useState(false)
  const [accountError, setAccountError] = useState('')
  const [libraryError, setLibraryError] = useState('')

  const imageRequestRef = useRef(0)
  const accountRequestRef = useRef(0)
  const ownerRef = useRef(owner)
  ownerRef.current = owner
  const cursorRef = useRef(cursor)
  cursorRef.current = cursor

  const activeAccount = useMemo(
    () => (owner === me?.id ? me : users.find(user => user.id === owner)),
    [owner, me, users]
  )
  const hasLimit = Boolean(activeAccount?.quota_bytes)
  const usagePercent = useMemo(() => {
    if (!activeAccount?.quota_bytes) return 0
    return Math.min(100, Math.round((activeAccount.used_bytes / activeAccount.quota_bytes) * 100))
  }, [activeAccount])

  const loadAccounts = useCallback(async () => {
    const request = ++accountRequestRef.current
    try {
      const [account, tenants] = await Promise.all([
        workspaceApi.me(),
        isAdmin ? workspaceApi.users() : Promise.resolve({ items: [] })
      ])
      if (request !== accountRequestRef.current) return
      setMe(account)
      setUsers(tenants.items)
      setOwner(prev => prev || account.id)
      setAccountError('')
    } catch (err) {
      if (request === accountRequestRef.current) {
        setAccountError(err instanceof Error ? err.message : '无法加载图片库')
      }
    }
  }, [isAdmin])

  const loadFolders = useCallback(async (targetOwner: string) => {
    if (!targetOwner) return
    try {
      const data = await workspaceApi.folders(targetOwner)
      if (ownerRef.current === targetOwner) setFolders(data.items)
    } catch (err) {
      if (ownerRef.current === targetOwner) {
        onNotice(err instanceof Error ? err.message : '无法加载目录', 'danger')
      }
    }
  }, [onNotice])

  const loadImages = useCallback(async (targetOwner: string, targetFolder: string | null, more = false) => {
    if (!targetOwner) return
    const request = ++imageRequestRef.current
    const reqCursor = more ? cursorRef.current : ''
    if (!more) {
      setImages([])
      setCursor('')
    }
    setLoadingImages(true)
    setLibraryError('')
    try {
      const data = await workspaceApi.images(targetOwner, targetFolder, reqCursor)
      if (request !== imageRequestRef.current) return
      setImages(prev => (more ? [...prev, ...data.items] : data.items))
      setCursor(data.next_cursor)
    } catch (err) {
      if (request === imageRequestRef.current) {
        setLibraryError(err instanceof Error ? err.message : '无法加载图片')
      }
    } finally {
      if (request === imageRequestRef.current) setLoadingImages(false)
    }
  }, [])

  useEffect(() => {
    void loadAccounts()
  }, [loadAccounts])

  useEffect(() => {
    if (!owner) return
    setFolders([])
    setFolderFilter(null)
    setUploadFolder('')
    setFolderPath('')
    setSelected('')
    void loadFolders(owner)
  }, [owner, loadFolders])

  useEffect(() => {
    if (!owner) return
    setSelected('')
    void loadImages(owner, folderFilter, false)
  }, [owner, folderFilter, loadImages])

  function selectFolder(path: string | null) {
    setFolderFilter(path)
    setUploadFolder(path || '')
  }

  function selectImage(asset: WorkspaceAsset) {
    const originalID = asset.image_id || asset.id
    setSelected(prev => (prev === originalID ? '' : originalID))
  }

  async function createFolder(event: React.FormEvent) {
    event.preventDefault()
    if (!folderPath.trim()) return
    setAction('folder')
    try {
      const folder = await workspaceApi.createFolder(owner, folderPath.trim())
      setFolderPath('')
      await loadFolders(owner)
      selectFolder(folder.path)
      onNotice('已创建目录', 'success')
    } catch (err) {
      onNotice(err instanceof Error ? err.message : '目录创建失败', 'danger')
    } finally {
      setAction('')
    }
  }

  async function upload(event: React.FormEvent) {
    event.preventDefault()
    const input = uploadInputRef.current
    if (!input?.files?.length) return
    const files = Array.from(input.files)
    const destination = uploadFolder
    setAction('upload')
    let completed = 0
    try {
      for (const file of files) {
        try {
          if (file.size > maxUploadSize) throw new Error(`单文件最大 ${formatSize(maxUploadSize)}`)
          await workspaceApi.upload(owner, destination, file)
          completed++
        } catch (err) {
          onNotice(`${file.name} · ${err instanceof Error ? err.message : '上传失败'}`, 'danger')
        }
      }
      if (completed > 0) {
        input.value = ''
        setSelected('')
        if (folderFilter !== null && folderFilter !== destination) {
          selectFolder(destination)
        } else {
          await loadImages(owner, folderFilter, false)
        }
        await loadAccounts()
        onNotice(`已上传 ${completed} 张原图`, 'success')
      }
    } finally {
      setAction('')
    }
  }

  async function changeToken(revoke: boolean) {
    if (me?.has_token) {
      const ok = await confirm({
        title: revoke ? '撤销当前 API 令牌？' : '重新生成 API 令牌？',
        description: revoke
          ? '撤销后使用该令牌的外部脚本将立即失去访问权限。'
          : '重新生成将使当前 API 令牌立即失效，且新令牌只显示一次。',
        confirmLabel: revoke ? '确认撤销' : '重新生成'
      })
      if (!ok) return
    }
    setTokenBusy(true)
    try {
      if (revoke) {
        await workspaceApi.revokeToken()
        setToken('')
        onNotice('已撤销 API 令牌', 'success')
      } else {
        const res = await workspaceApi.rotateToken()
        setToken(res.token)
        onNotice('已生成新 API 令牌，请妥善保存', 'success')
      }
      await loadAccounts()
    } catch (err) {
      onNotice(err instanceof Error ? err.message : '令牌操作失败', 'danger')
    } finally {
      setTokenBusy(false)
    }
  }

  if (accountError) {
    return (
      <Alert variant="emphasis">
        <AlertCircle className="size-4" />
        <AlertDescription>{accountError}</AlertDescription>
      </Alert>
    )
  }

  if (!me) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-20 rounded-xl" />
        <Skeleton className="h-96 rounded-xl" />
      </div>
    )
  }

  return (
    <div className="space-y-5">
      {confirmDialog}

      {isAdmin && (
        <Tabs value={section} onValueChange={v => { if (!action) setSection(v as 'library' | 'admin') }}>
          <TabsList>
            <TabsTrigger value="library">图片与目录</TabsTrigger>
            <TabsTrigger value="admin">租户与公开链接</TabsTrigger>
          </TabsList>
        </Tabs>
      )}

      {section === 'admin' ? (
        <WorkspaceAdmin
          users={users}
          onNotice={onNotice}
          onUsersChanged={() => void loadAccounts()}
          onLinksChanged={() => {
            setSelected('')
            void loadImages(owner, folderFilter, false)
          }}
        />
      ) : (
        <>
          {activeAccount && (
            <Card className="py-4">
              <CardContent className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                {isAdmin ? (
                  <div className="flex items-center gap-3">
                    <Label className="shrink-0 text-xs text-muted-foreground">查看账号</Label>
                    <Select value={owner} onValueChange={setOwner} disabled={Boolean(action)}>
                      <SelectTrigger className="w-56">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={me.id}>{me.username}（管理员）</SelectItem>
                        {users.map(user => (
                          <SelectItem key={user.id} value={user.id}>
                            {user.username}{user.disabled ? '（停用）' : ''}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                ) : (
                  <div className="flex items-center gap-2">
                    <span className="font-semibold">{me.username}</span>
                    <Badge variant="secondary">成员</Badge>
                  </div>
                )}

                <div className="flex flex-col gap-1.5 sm:min-w-60 sm:items-end">
                  <div className="flex items-center gap-2 text-xs">
                    <span className="text-muted-foreground">存储用量</span>
                    <span className="font-mono font-medium tabular-nums">
                      {formatSize(activeAccount.used_bytes)} / {hasLimit ? formatSize(activeAccount.quota_bytes) : '不限'}
                    </span>
                  </div>
                  {hasLimit && <Progress value={usagePercent} className="h-1.5 w-full sm:w-56" />}
                </div>
              </CardContent>
            </Card>
          )}

          <div className="grid gap-5 lg:grid-cols-[240px_minmax(0,1fr)]">
            <Card className="h-fit py-4">
              <CardHeader className="pb-2">
                <CardTitle className="text-sm">目录</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <nav className="flex flex-col gap-1" aria-label="目录列表">
                  <button
                    type="button"
                    onClick={() => selectFolder(null)}
                    className={`flex items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-sm transition-colors cursor-pointer ${
                      folderFilter === null ? 'bg-primary text-primary-foreground font-medium' : 'hover:bg-muted text-foreground'
                    }`}
                  >
                    <Folder className="size-3.5 shrink-0" />
                    <span>全部图片</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => selectFolder('')}
                    className={`flex items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-sm transition-colors cursor-pointer ${
                      folderFilter === '' ? 'bg-primary text-primary-foreground font-medium' : 'hover:bg-muted text-foreground'
                    }`}
                  >
                    <Folder className="size-3.5 shrink-0" />
                    <span>根目录</span>
                  </button>
                  {folders.map(folder => (
                    <button
                      key={folder.path}
                      type="button"
                      onClick={() => selectFolder(folder.path)}
                      className={`flex items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-xs font-mono transition-colors cursor-pointer ${
                        folderFilter === folder.path ? 'bg-primary text-primary-foreground font-medium' : 'hover:bg-muted text-foreground'
                      }`}
                    >
                      <Folder className="size-3.5 shrink-0" />
                      <span className="truncate" title={folder.path}>{folder.path}</span>
                    </button>
                  ))}
                </nav>

                <form onSubmit={createFolder} className="space-y-2 border-t pt-3">
                  <Label htmlFor="ws-new-folder" className="text-xs text-muted-foreground">新建目录</Label>
                  <Input
                    id="ws-new-folder"
                    value={folderPath}
                    onChange={e => setFolderPath(e.target.value)}
                    placeholder="项目/素材"
                    maxLength={200}
                    disabled={Boolean(action)}
                    required
                    className="h-8 text-xs"
                  />
                  <Button type="submit" variant="outline" size="xs" className="w-full" disabled={Boolean(action)}>
                    <FolderPlus className="size-3.5" />
                    创建目录
                  </Button>
                </form>
              </CardContent>
            </Card>

            <div className="min-w-0 space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">
                    {folderFilter === null ? '全部图片' : folderFilter || '根目录'}
                  </CardTitle>
                  <CardDescription>点击列表中的图片可查看详情、动态转换链接与预生成版本。</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <form onSubmit={upload} className="grid gap-3 rounded-lg border bg-muted/30 p-3 sm:grid-cols-[160px_1fr_auto] sm:items-end">
                    <div className="space-y-1.5">
                      <Label className="text-xs">目标目录</Label>
                      <Select
                        value={uploadFolder || ROOT_SENTINEL}
                        onValueChange={v => setUploadFolder(v === ROOT_SENTINEL ? '' : v)}
                        disabled={Boolean(action)}
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
                    <div className="space-y-1.5">
                      <Label htmlFor="ws-upload-file" className="text-xs">选择原图</Label>
                      <Input
                        id="ws-upload-file"
                        ref={uploadInputRef}
                        type="file"
                        accept="image/*,.tif,.tiff"
                        multiple
                        required
                        disabled={Boolean(action)}
                        className="h-8 text-xs"
                      />
                    </div>
                    <Button type="submit" size="sm" disabled={Boolean(action)}>
                      {action === 'upload' ? <Loader2 className="size-3.5 animate-spin" /> : <Upload className="size-3.5" />}
                      {action === 'upload' ? '上传中...' : '上传原图'}
                    </Button>
                  </form>

                  {libraryError && (
                    <Alert variant="emphasis">
                      <AlertCircle className="size-4" />
                      <AlertDescription>{libraryError}</AlertDescription>
                    </Alert>
                  )}

                  {images.length === 0 ? (
                    <p className="py-10 text-center text-sm text-muted-foreground" role="status">
                      {loadingImages ? '正在加载图片...' : '此目录还没有图片'}
                    </p>
                  ) : (
                    <ul className="divide-y rounded-lg border">
                      {images.map(image => {
                        const originalID = image.image_id || image.id
                        const isSelected = selected === originalID
                        return (
                          <li
                            key={image.id}
                            className={`flex flex-wrap items-center justify-between gap-3 px-3.5 py-2.5 transition-colors ${
                              isSelected ? 'bg-muted/70' : 'hover:bg-muted/30'
                            }`}
                          >
                            <div className="min-w-0 flex-1">
                              <button
                                type="button"
                                aria-expanded={isSelected}
                                onClick={() => selectImage(image)}
                                className="block max-w-full truncate text-left text-sm font-medium text-foreground underline-offset-4 hover:underline cursor-pointer"
                              >
                                {image.filename}
                              </button>
                              <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                                <Badge variant={image.image_id ? 'outline' : 'secondary'} className="text-[10px] py-0">
                                  {image.image_id ? '预生成版本' : '原图'}
                                </Badge>
                                <span className="font-mono">{image.folder || '根目录'}</span>
                                <span>·</span>
                                <span className="font-mono tabular-nums">{formatSize(image.size_bytes)}</span>
                              </div>
                            </div>
                            <Button type="button" size="xs" variant="outline" onClick={() => void copy(image.url)}>
                              <Copy className="size-3" />
                              复制链接
                            </Button>
                          </li>
                        )
                      })}
                    </ul>
                  )}

                  {cursor && (
                    <div className="text-center">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={loadingImages}
                        onClick={() => void loadImages(owner, folderFilter, true)}
                      >
                        {loadingImages && <Loader2 className="size-3.5 animate-spin" />}
                        加载更多
                      </Button>
                    </div>
                  )}
                </CardContent>
              </Card>

              {selected && (
                <WorkspaceImageDetail
                  key={selected}
                  id={selected}
                  folders={folders}
                  onNotice={onNotice}
                  onRemoved={() => {
                    setSelected('')
                    void Promise.all([loadImages(owner, folderFilter, false), loadAccounts()])
                  }}
                  onChanged={() => {
                    void Promise.all([loadImages(owner, folderFilter, false), loadAccounts()])
                  }}
                />
              )}
            </div>
          </div>
        </>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <KeyRound className="size-4" />
            API 访问令牌
          </CardTitle>
          <CardDescription>Bearer 令牌拥有当前账号权限，生成后仅显示一次。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            <Button type="button" variant="outline" size="sm" disabled={tokenBusy} onClick={() => void changeToken(false)}>
              {me.has_token ? '重新生成令牌' : '生成令牌'}
            </Button>
            {me.has_token && (
              <Button type="button" variant="ghost" size="sm" disabled={tokenBusy} onClick={() => void changeToken(true)}>
                撤销令牌
              </Button>
            )}
            <Button variant="link" size="sm" asChild>
              <a href={workspaceApi.openapiURL} target="_blank" rel="noopener noreferrer">
                OpenAPI 规范
                <ExternalLink className="size-3.5" />
              </a>
            </Button>
          </div>

          {token && (
            <div className="flex items-center gap-2">
              <Input
                value={token}
                readOnly
                aria-label="新 API 令牌"
                className="font-mono text-xs bg-muted/40"
                onFocus={e => e.currentTarget.select()}
              />
              <Button type="button" size="sm" onClick={() => void copy(token, '已复制令牌')}>
                <Copy className="size-3.5" />
                复制令牌
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
