import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertCircle,
  ChevronRight,
  Copy,
  ExternalLink,
  Folder,
  FolderOpen,
  FolderPlus,
  ImagePlus,
  KeyRound,
  LayoutGrid,
  List,
  Loader2,
  Upload,
  X
} from 'lucide-react'
import type { NoticeType } from '@/shared/notify'
import { formatSize } from '@/shared/format'
import { fetchCached, getCached, invalidateCached } from '@/shared/swrCache'
import { useCopy } from '@/shared/useCopy'
import { useDelayedLoading } from '@/shared/useDelayedLoading'
import type { WorkspaceAsset, WorkspaceFolder, WorkspaceUser } from '@/shared/workspaceApi'
import { workspaceApi } from '@/shared/workspaceApi'
import { useConfirmDialog } from '@/components/ConfirmDialog'
import { FadeImage } from '@/components/FadeImage'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import WorkspaceAdmin from '@/components/admin/WorkspaceAdmin'
import WorkspaceImageDetail from '@/components/admin/WorkspaceImageDetail'

const ROOT_SENTINEL = '__root__'
const VIEW_MODE_KEY = 'easyimage-workspace-view'
const MAX_TRANSFORM_PIXELS = 20_000_000
const MAX_TRANSFORM_BYTES = 32 * 1024 * 1024

interface WorkspaceLibraryProps {
  isAdmin: boolean
  maxUploadSize: number
  onNotice: (message: string, type?: NoticeType) => void
}

interface FolderTreeNode {
  name: string
  path: string
  browseOnly: boolean
  children: FolderTreeNode[]
}

function buildFolderTree(folders: WorkspaceFolder[]): FolderTreeNode[] {
  const byPath = new Map<string, FolderTreeNode>()
  const roots: FolderTreeNode[] = []

  const ensureNode = (fullPath: string, browseOnly: boolean): FolderTreeNode => {
    const existing = byPath.get(fullPath)
    if (existing) {
      if (!browseOnly) existing.browseOnly = false
      return existing
    }
    const idx = fullPath.lastIndexOf('/')
    const name = idx >= 0 ? fullPath.slice(idx + 1) : fullPath
    const node: FolderTreeNode = { name, path: fullPath, browseOnly, children: [] }
    byPath.set(fullPath, node)
    if (idx >= 0) {
      const parent = ensureNode(fullPath.slice(0, idx), browseOnly)
      parent.children.push(node)
    } else {
      roots.push(node)
    }
    return node
  }

  for (const folder of folders) {
    ensureNode(folder.path, Boolean(folder.browse_only))
  }
  return roots
}

function workspaceThumbURL(image: WorkspaceAsset): string {
  if (!image.url || image.content_type === 'image/svg+xml' || image.size_bytes <= 16_384) {
    return image.url
  }
  if (!image.legacy) {
    if (
      image.size_bytes <= MAX_TRANSFORM_BYTES &&
      (!image.width || !image.height || image.width * image.height <= MAX_TRANSFORM_PIXELS)
    ) {
      return `${image.url}?width=240&height=240&fit=cover&format=jpeg&quality=75`
    }
    return image.url
  }
  try {
    const parsed = new URL(image.url, window.location.href)
    if (parsed.origin === window.location.origin && parsed.pathname.startsWith('/i/')) {
      return `/app/thumb?img=${encodeURIComponent(decodeURIComponent(parsed.pathname))}`
    }
  } catch {
    // Fall back to direct URL if parsing/decoding fails.
  }
  return image.url
}

function imagesCacheKey(targetOwner: string, targetFolder: string | null) {
  return `ws:images:${targetOwner}:${targetFolder === null ? '__all__' : targetFolder}`
}

function readInitialViewMode(): 'list' | 'grid' {
  if (typeof window === 'undefined') return 'grid'
  try {
    const stored = window.localStorage.getItem(VIEW_MODE_KEY)
    if (stored === 'list' || stored === 'grid') return stored
  } catch {
    // Ignore storage restrictions.
  }
  return 'grid'
}

export default function WorkspaceLibrary({ isAdmin, maxUploadSize, onNotice }: WorkspaceLibraryProps) {
  const copy = useMemo(() => useCopy(onNotice), [onNotice])
  const { confirm, dialog: confirmDialog } = useConfirmDialog()

  const cachedMe = getCached<WorkspaceUser>('ws:me')
  const initialOwner = cachedMe?.id || (isAdmin ? 'admin' : '')
  const cachedUsers = getCached<{ items: WorkspaceUser[] }>('ws:users')?.items
  const cachedFolders = initialOwner
    ? getCached<{ items: WorkspaceFolder[] }>(`ws:folders:${initialOwner}`)?.items
    : undefined
  const cachedImagesData = initialOwner
    ? getCached<{ items: WorkspaceAsset[]; next_cursor: string }>(imagesCacheKey(initialOwner, null))
    : undefined

  const [me, setMe] = useState<WorkspaceUser | undefined>(cachedMe)
  const [users, setUsers] = useState<WorkspaceUser[]>(cachedUsers ?? [])
  const [owner, setOwner] = useState(initialOwner)
  const [section, setSection] = useState<'library' | 'admin'>('library')
  const [folderFilter, setFolderFilter] = useState<string | null>(null)
  const [folders, setFolders] = useState<WorkspaceFolder[]>(cachedFolders ?? [])
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(() => new Set())
  const [showNewFolder, setShowNewFolder] = useState(false)
  const [folderPath, setFolderPath] = useState('')
  const [uploadFolder, setUploadFolder] = useState('')
  const [queuedFiles, setQueuedFiles] = useState<File[]>([])
  const uploadInputRef = useRef<HTMLInputElement>(null)
  const [images, setImages] = useState<WorkspaceAsset[]>(cachedImagesData?.items ?? [])
  const [cursor, setCursor] = useState(cachedImagesData?.next_cursor ?? '')
  const [selected, setSelected] = useState('')
  const [viewMode, setViewMode] = useState<'list' | 'grid'>(readInitialViewMode)
  const [loadingImages, setLoadingImages] = useState(!cachedImagesData)
  const [hasLoadedImages, setHasLoadedImages] = useState(Boolean(cachedImagesData))
  const [dragActive, setDragActive] = useState(false)
  const [action, setAction] = useState<'upload' | 'folder' | ''>('')
  const [token, setToken] = useState('')
  const [tokenBusy, setTokenBusy] = useState(false)
  const [accountError, setAccountError] = useState('')
  const [libraryError, setLibraryError] = useState('')
  const showInitialSkeleton = useDelayedLoading(!me, 180)

  const imageRequestRef = useRef(0)
  const accountRequestRef = useRef(0)
  const ownerRef = useRef(owner)
  ownerRef.current = owner
  const cursorRef = useRef(cursor)
  cursorRef.current = cursor

  const folderTree = useMemo(() => buildFolderTree(folders), [folders])

  const activeAccount = useMemo(
    () => (owner === me?.id ? me : users.find(user => user.id === owner)),
    [owner, me, users]
  )
  const hasLimit = Boolean(activeAccount?.quota_bytes)
  const usagePercent = useMemo(() => {
    if (!activeAccount?.quota_bytes) return 0
    return Math.min(100, Math.round((activeAccount.used_bytes / activeAccount.quota_bytes) * 100))
  }, [activeAccount])

  function changeViewMode(next: 'list' | 'grid') {
    setViewMode(next)
    try {
      window.localStorage.setItem(VIEW_MODE_KEY, next)
    } catch {
      // Ignore storage restrictions.
    }
  }

  const loadAccounts = useCallback(async (force = false) => {
    const request = ++accountRequestRef.current
    try {
      const [account, tenants] = await Promise.all([
        force ? workspaceApi.me() : fetchCached('ws:me', () => workspaceApi.me(), 5_000),
        isAdmin
          ? (force ? workspaceApi.users() : fetchCached('ws:users', () => workspaceApi.users(), 5_000))
          : Promise.resolve({ items: [] as WorkspaceUser[] })
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

  const loadFolders = useCallback(async (targetOwner: string, force = false) => {
    if (!targetOwner) return
    const key = `ws:folders:${targetOwner}`
    const cached = getCached<{ items: WorkspaceFolder[] }>(key)
    if (cached) setFolders(cached.items)
    try {
      const data = force
        ? await workspaceApi.folders(targetOwner)
        : await fetchCached(key, () => workspaceApi.folders(targetOwner), 5_000)
      if (ownerRef.current === targetOwner) setFolders(data.items)
    } catch (err) {
      if (ownerRef.current === targetOwner) {
        onNotice(err instanceof Error ? err.message : '无法加载目录', 'danger')
      }
    }
  }, [onNotice])

  const loadImages = useCallback(async (targetOwner: string, targetFolder: string | null, more = false, force = false) => {
    if (!targetOwner) return
    const request = ++imageRequestRef.current
    const reqCursor = more ? cursorRef.current : ''
    const key = imagesCacheKey(targetOwner, targetFolder)
    if (!more) {
      setCursor('')
      const cached = !force ? getCached<{ items: WorkspaceAsset[]; next_cursor: string }>(key) : undefined
      if (cached) {
        setImages(cached.items)
        setCursor(cached.next_cursor)
        setHasLoadedImages(true)
      }
    }
    setLoadingImages(true)
    setLibraryError('')
    try {
      const data = more || force
        ? await workspaceApi.images(targetOwner, targetFolder, reqCursor)
        : await fetchCached(key, () => workspaceApi.images(targetOwner, targetFolder, ''), 5_000)
      if (request !== imageRequestRef.current) return
      setImages(prev => (more ? [...prev, ...data.items] : data.items))
      setCursor(data.next_cursor)
      setHasLoadedImages(true)
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
    setUploadFolder(path && !folders.some(folder => folder.path === path && folder.browse_only) ? path : '')
    if (path) {
      setExpandedFolders(prev => {
        const next = new Set(prev)
        const parts = path.split('/')
        let acc = ''
        for (const part of parts) {
          acc = acc ? `${acc}/${part}` : part
          next.add(acc)
        }
        return next
      })
    }
  }

  function toggleFolderExpand(path: string) {
    setExpandedFolders(prev => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }

  function selectImage(asset: WorkspaceAsset) {
    const originalID = asset.image_id || asset.id
    setSelected(originalID)
  }

  async function createFolder(event: React.FormEvent) {
    event.preventDefault()
    if (!folderPath.trim()) return
    setAction('folder')
    try {
      const folder = await workspaceApi.createFolder(owner, folderPath.trim())
      setFolderPath('')
      setShowNewFolder(false)
      invalidateCached(`ws:folders:${owner}`)
      await loadFolders(owner, true)
      selectFolder(folder.path)
      onNotice('已创建目录', 'success')
    } catch (err) {
      onNotice(err instanceof Error ? err.message : '目录创建失败', 'danger')
    } finally {
      setAction('')
    }
  }

  async function uploadFiles(files: File[]) {
    if (!files.length || action) return
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
        if (uploadInputRef.current) uploadInputRef.current.value = ''
        setQueuedFiles([])
        setSelected('')
        invalidateCached(`ws:images:${owner}:`)
        invalidateCached(`ws:folders:${owner}`)
        invalidateCached('ws:me')
        if (folderFilter !== null && folderFilter !== destination) {
          selectFolder(destination)
        } else {
          await loadImages(owner, folderFilter, false, true)
        }
        await loadAccounts(true)
        onNotice(`已上传 ${completed} 张原图`, 'success')
      }
    } finally {
      setAction('')
    }
  }

  async function upload(event: React.FormEvent) {
    event.preventDefault()
    if (!queuedFiles.length) {
      uploadInputRef.current?.click()
      return
    }
    await uploadFiles(queuedFiles)
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
      invalidateCached('ws:me')
      await loadAccounts(true)
    } catch (err) {
      onNotice(err instanceof Error ? err.message : '令牌操作失败', 'danger')
    } finally {
      setTokenBusy(false)
    }
  }

  function renderTreeNodes(nodes: FolderTreeNode[], depth = 0): React.ReactNode {
    return nodes.map(node => {
      const hasChildren = node.children.length > 0
      const expanded = expandedFolders.has(node.path)
      const active = folderFilter === node.path
      return (
        <div key={node.path}>
          <div
            style={{ paddingLeft: `${depth * 14 + 4}px` }}
            className={`group relative flex items-center gap-1 rounded-md pr-2 py-1 text-xs transition-colors duration-150 ${
              active
                ? 'bg-secondary font-medium text-foreground ring-1 ring-border/80'
                : 'text-muted-foreground hover:bg-muted/70 hover:text-foreground'
            }`}
          >
            {hasChildren ? (
              <button
                type="button"
                aria-label={expanded ? `收起 ${node.name}` : `展开 ${node.name}`}
                onClick={e => {
                  e.stopPropagation()
                  toggleFolderExpand(node.path)
                }}
                className="grid size-5 shrink-0 place-items-center rounded text-muted-foreground hover:text-foreground cursor-pointer"
              >
                <ChevronRight
                  className={`size-3.5 transition-transform duration-150 ${expanded ? 'rotate-90' : ''}`}
                />
              </button>
            ) : (
              <span className="size-5 shrink-0" aria-hidden="true" />
            )}
            <button
              type="button"
              onClick={() => selectFolder(node.path)}
              className="flex min-w-0 flex-1 items-center gap-1.5 py-0.5 text-left font-mono cursor-pointer"
            >
              {active || expanded ? (
                <FolderOpen className="size-3.5 shrink-0 text-foreground/80" />
              ) : (
                <Folder className="size-3.5 shrink-0 text-muted-foreground" />
              )}
              <span className="truncate" title={node.path}>{node.name}</span>
            </button>
          </div>
          {hasChildren && expanded && (
            <div className="mt-0.5 space-y-0.5">
              {renderTreeNodes(node.children, depth + 1)}
            </div>
          )}
        </div>
      )
    })
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
    if (!showInitialSkeleton) return <div className="min-h-[420px]" aria-busy="true" />
    return (
      <div className="space-y-4">
        <Skeleton className="h-16 rounded-xl" />
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
          onUsersChanged={() => {
            invalidateCached('ws:users')
            invalidateCached('ws:me')
            void loadAccounts(true)
          }}
          onLinksChanged={() => {
            setSelected('')
            invalidateCached(`ws:images:${owner}:`)
            void loadImages(owner, folderFilter, false, true)
          }}
        />
      ) : (
        <>
          {activeAccount && (
            <Card className="py-3.5">
              <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                {isAdmin ? (
                  <div className="flex items-center gap-3">
                    <Label className="shrink-0 text-xs text-muted-foreground">查看账号</Label>
                    <Select value={owner} onValueChange={setOwner} disabled={Boolean(action)}>
                      <SelectTrigger className="h-8 w-52 text-xs">
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

                <div className="flex flex-col gap-1.5 sm:min-w-56 sm:items-end">
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

          <div className="grid gap-5 lg:grid-cols-[232px_minmax(0,1fr)]">
            <Card className="h-fit gap-3 py-4 lg:sticky lg:top-20">
              <CardHeader className="flex flex-row items-center justify-between gap-2 pb-0">
                <CardTitle className="text-sm">目录</CardTitle>
                <Button
                  type="button"
                  variant={showNewFolder ? 'secondary' : 'ghost'}
                  size="xs"
                  onClick={() => setShowNewFolder(prev => !prev)}
                  className="h-6 px-2 text-xs"
                >
                  <FolderPlus className="size-3.5" />
                  新建
                </Button>
              </CardHeader>
              <CardContent className="space-y-3">
                {showNewFolder && (
                  <form onSubmit={createFolder} className="panel-enter space-y-2 rounded-lg border bg-muted/30 p-2.5">
                    <Label htmlFor="ws-new-folder" className="text-[11px] text-muted-foreground">
                      输入目录路径
                    </Label>
                    <Input
                      id="ws-new-folder"
                      value={folderPath}
                      onChange={e => setFolderPath(e.target.value)}
                      placeholder="例如：项目/素材"
                      maxLength={200}
                      disabled={Boolean(action)}
                      autoFocus
                      required
                      className="h-7 text-xs"
                    />
                    <div className="flex items-center gap-1.5">
                      <Button type="submit" size="xs" className="flex-1" disabled={Boolean(action)}>
                        创建
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="xs"
                        onClick={() => {
                          setShowNewFolder(false)
                          setFolderPath('')
                        }}
                      >
                        取消
                      </Button>
                    </div>
                  </form>
                )}

                <nav className="max-h-[420px] space-y-0.5 overflow-y-auto pr-1" aria-label="目录列表">
                  <button
                    type="button"
                    onClick={() => selectFolder(null)}
                    className={`flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-xs transition-colors duration-150 cursor-pointer ${
                      folderFilter === null
                        ? 'bg-secondary font-medium text-foreground ring-1 ring-border/80'
                        : 'text-muted-foreground hover:bg-muted/70 hover:text-foreground'
                    }`}
                  >
                    <FolderOpen className="size-3.5 shrink-0" />
                    <span>全部图片</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => selectFolder('')}
                    className={`flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-xs transition-colors duration-150 cursor-pointer ${
                      folderFilter === ''
                        ? 'bg-secondary font-medium text-foreground ring-1 ring-border/80'
                        : 'text-muted-foreground hover:bg-muted/70 hover:text-foreground'
                    }`}
                  >
                    <Folder className="size-3.5 shrink-0" />
                    <span>根目录</span>
                  </button>
                  {folderTree.length > 0 && (
                    <div className="mt-1.5 border-t pt-1.5 space-y-0.5">
                      {renderTreeNodes(folderTree)}
                    </div>
                  )}
                </nav>
              </CardContent>
            </Card>

            <div className="min-w-0 space-y-4">
              <Card className="relative overflow-hidden">
                {loadingImages && <div className="loading-bar absolute inset-x-0 top-0" aria-hidden="true" />}
                <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <CardTitle className="text-base">
                      {folderFilter === null ? '全部图片' : folderFilter || '根目录'}
                    </CardTitle>
                    {hasLoadedImages && (
                      <Badge variant="outline" className="font-mono text-[11px] font-normal tabular-nums">
                        {images.length} 张
                      </Badge>
                    )}
                  </div>

                  <div className="inline-flex items-center rounded-md border bg-muted/40 p-0.5" role="group" aria-label="视图模式">
                    <button
                      type="button"
                      onClick={() => changeViewMode('grid')}
                      aria-pressed={viewMode === 'grid'}
                      title="网格视图"
                      className={`inline-flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors cursor-pointer ${
                        viewMode === 'grid'
                          ? 'bg-background font-medium text-foreground shadow-2xs'
                          : 'text-muted-foreground hover:text-foreground'
                      }`}
                    >
                      <LayoutGrid className="size-3.5" />
                      <span className="hidden sm:inline">网格</span>
                    </button>
                    <button
                      type="button"
                      onClick={() => changeViewMode('list')}
                      aria-pressed={viewMode === 'list'}
                      title="列表视图"
                      className={`inline-flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors cursor-pointer ${
                        viewMode === 'list'
                          ? 'bg-background font-medium text-foreground shadow-2xs'
                          : 'text-muted-foreground hover:text-foreground'
                      }`}
                    >
                      <List className="size-3.5" />
                      <span className="hidden sm:inline">列表</span>
                    </button>
                  </div>
                </CardHeader>

                <CardContent className="space-y-4">
                  <form
                    onSubmit={upload}
                    onDragOver={event => {
                      event.preventDefault()
                      if (!action) setDragActive(true)
                    }}
                    onDragLeave={event => {
                      const next = event.relatedTarget as Node | null
                      if (next && event.currentTarget.contains(next)) return
                      setDragActive(false)
                    }}
                    onDrop={event => {
                      event.preventDefault()
                      setDragActive(false)
                      if (action || !event.dataTransfer?.files?.length) return
                      void uploadFiles(Array.from(event.dataTransfer.files))
                    }}
                    className={`flex flex-col gap-2.5 rounded-xl border border-dashed p-3 transition-[border-color,background-color] duration-150 sm:flex-row sm:items-center ${
                      dragActive ? 'border-foreground bg-muted/70' : 'border-border bg-muted/20'
                    }`}
                  >
                    <input
                      id="ws-upload-file"
                      ref={uploadInputRef}
                      type="file"
                      accept="image/*,.tif,.tiff"
                      multiple
                      disabled={Boolean(action)}
                      className="sr-only"
                      onChange={e => {
                        const list = e.target.files ? Array.from(e.target.files) : []
                        setQueuedFiles(list)
                      }}
                    />

                    <div className="shrink-0 sm:w-40">
                      <Select
                        value={uploadFolder || ROOT_SENTINEL}
                        onValueChange={v => setUploadFolder(v === ROOT_SENTINEL ? '' : v)}
                        disabled={Boolean(action)}
                      >
                        <SelectTrigger size="sm" aria-label="目标目录" className="w-full text-xs">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value={ROOT_SENTINEL}>根目录</SelectItem>
                          {folders.filter(folder => !folder.browse_only).map(folder => (
                            <SelectItem key={folder.path} value={folder.path}>{folder.path}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>

                    <button
                      type="button"
                      disabled={Boolean(action)}
                      onClick={() => uploadInputRef.current?.click()}
                      className="flex min-w-0 flex-1 items-center gap-2 rounded-lg border bg-background/80 px-3 py-1.5 text-left text-xs text-muted-foreground transition-colors hover:border-foreground/30 hover:text-foreground cursor-pointer"
                    >
                      <ImagePlus className="size-3.5 shrink-0" />
                      <span className="truncate">
                        {dragActive
                          ? '松开即可上传到当前目录'
                          : queuedFiles.length > 0
                            ? `已选 ${queuedFiles.length} 张：${queuedFiles.map(f => f.name).join('、')}`
                            : '点击选择原图，或直接拖入此处上传'}
                      </span>
                    </button>

                    <div className="flex shrink-0 items-center gap-1.5">
                      {queuedFiles.length > 0 && !action && (
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon-sm"
                          aria-label="清空已选文件"
                          onClick={() => {
                            setQueuedFiles([])
                            if (uploadInputRef.current) uploadInputRef.current.value = ''
                          }}
                        >
                          <X className="size-3.5" />
                        </Button>
                      )}
                      <Button type="submit" size="sm" disabled={Boolean(action)}>
                        {action === 'upload' ? <Loader2 className="size-3.5 animate-spin" /> : <Upload className="size-3.5" />}
                        {action === 'upload' ? '上传中...' : '上传原图'}
                      </Button>
                    </div>
                  </form>

                  {libraryError && (
                    <Alert variant="emphasis">
                      <AlertCircle className="size-4" />
                      <AlertDescription>{libraryError}</AlertDescription>
                    </Alert>
                  )}

                  {images.length === 0 ? (
                    <p className="py-12 text-center text-sm text-muted-foreground" role="status">
                      {loadingImages ? '正在加载图片...' : '此目录还没有图片，可拖拽或点击上方上传'}
                    </p>
                  ) : viewMode === 'grid' ? (
                    <div className={`grid grid-cols-2 gap-3 transition-opacity duration-150 sm:grid-cols-3 ${loadingImages ? 'opacity-65' : 'opacity-100'}`}>
                      {images.map(image => {
                        const webpURL = image.webp_url
                        return (
                          <article
                            key={image.id}
                            className="group flex flex-col overflow-hidden rounded-xl border bg-card transition-[border-color,box-shadow,transform] duration-200 hover:-translate-y-0.5 hover:border-foreground/25 hover:shadow-sm"
                          >
                            <button
                              type="button"
                              onClick={() => selectImage(image)}
                              aria-label={`查看 ${image.filename} 详情`}
                              className="checker-well relative block aspect-[4/3] w-full overflow-hidden border-b cursor-pointer"
                            >
                              <FadeImage
                                src={workspaceThumbURL(image)}
                                fallbackSrc={image.url}
                                alt={image.filename}
                                className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.04]"
                              />
                              <Badge
                                variant="secondary"
                                className="absolute top-2 left-2 bg-background/85 px-1.5 py-0 text-[10px] backdrop-blur-xs"
                              >
                                {image.legacy ? '旧图' : image.image_id ? '变体' : '原图'}
                              </Badge>
                            </button>
                            <div className="flex flex-1 flex-col justify-between gap-2 p-2.5">
                              <div className="min-w-0">
                                <button
                                  type="button"
                                  onClick={() => selectImage(image)}
                                  title={image.filename}
                                  className="block max-w-full truncate text-left text-xs font-medium text-foreground underline-offset-4 hover:underline cursor-pointer"
                                >
                                  {image.filename}
                                </button>
                                <p className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground tabular-nums">
                                  {image.folder || '根目录'} · {formatSize(image.size_bytes)}
                                </p>
                              </div>
                              <div className="flex flex-wrap items-center gap-1 border-t pt-2">
                                {webpURL !== image.url && (
                                  <Button type="button" size="xs" variant="ghost" className="h-6 px-2 text-[11px]" onClick={() => void copy(image.url)}>
                                    <Copy className="size-3" />
                                    链接
                                  </Button>
                                )}
                                {webpURL && (
                                  <Button
                                    type="button"
                                    size="xs"
                                    variant="ghost"
                                    className="h-6 px-2 text-[11px]"
                                    onClick={() => void copy(new URL(webpURL, window.location.href).href, '已复制 WebP 链接')}
                                  >
                                    <Copy className="size-3" />
                                    WebP
                                  </Button>
                                )}
                              </div>
                            </div>
                          </article>
                        )
                      })}
                    </div>
                  ) : (
                    <ul className={`divide-y rounded-xl border transition-opacity duration-150 ${loadingImages ? 'opacity-65' : 'opacity-100'}`}>
                      {images.map(image => {
                        const webpURL = image.webp_url
                        return (
                          <li
                            key={image.id}
                            className="flex flex-wrap items-center justify-between gap-3 px-3.5 py-2.5 transition-colors duration-150 hover:bg-muted/35"
                          >
                            <div className="flex min-w-0 flex-1 items-center gap-3">
                              <button
                                type="button"
                                onClick={() => selectImage(image)}
                                aria-label={`查看 ${image.filename} 详情`}
                                className="checker-well size-11 shrink-0 overflow-hidden rounded-md border transition-transform duration-150 hover:scale-105 cursor-pointer"
                              >
                                <FadeImage
                                  src={workspaceThumbURL(image)}
                                  fallbackSrc={image.url}
                                  alt={image.filename}
                                  className="h-full w-full object-cover"
                                />
                              </button>
                              <div className="min-w-0 flex-1">
                                <button
                                  type="button"
                                  onClick={() => selectImage(image)}
                                  className="block max-w-full truncate text-left text-sm font-medium text-foreground underline-offset-4 hover:underline cursor-pointer"
                                >
                                  {image.filename}
                                </button>
                                <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                                  <Badge variant={image.image_id ? 'outline' : 'secondary'} className="text-[10px] py-0">
                                    {image.legacy ? '旧图' : image.image_id ? '预生成版本' : '原图'}
                                  </Badge>
                                  <span className="font-mono">{image.folder || '根目录'}</span>
                                  <span>·</span>
                                  <span className="font-mono tabular-nums">{formatSize(image.size_bytes)}</span>
                                </div>
                              </div>
                            </div>
                            <div className="flex flex-wrap gap-1.5">
                              {webpURL !== image.url && (
                                <Button type="button" size="xs" variant="outline" onClick={() => void copy(image.url)}>
                                  <Copy className="size-3" />
                                  复制链接
                                </Button>
                              )}
                              {webpURL && (
                                <Button type="button" size="xs" variant="outline" onClick={() => void copy(new URL(webpURL, window.location.href).href, '已复制 WebP 链接')}>
                                  <Copy className="size-3" />
                                  复制 WebP
                                </Button>
                              )}
                            </div>
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
            </div>
          </div>
        </>
      )}

      {/* On-screen Image Inspector Dialog */}
      <Dialog open={Boolean(selected)} onOpenChange={open => { if (!open) setSelected('') }}>
        <DialogContent className="sm:max-w-4xl max-h-[90vh] overflow-y-auto p-5">
          <DialogHeader className="sr-only">
            <DialogTitle>图片详情</DialogTitle>
            <DialogDescription>查看原图、动态转换参数与预生成版本</DialogDescription>
          </DialogHeader>
          {selected && (
            <WorkspaceImageDetail
              key={selected}
              id={selected}
              folders={folders}
              embedded
              onNotice={onNotice}
              onRemoved={() => {
                setSelected('')
                invalidateCached(`ws:images:${owner}:`)
                invalidateCached('ws:me')
                void Promise.all([loadImages(owner, folderFilter, false, true), loadAccounts(true)])
              }}
              onChanged={() => {
                invalidateCached(`ws:images:${owner}:`)
                invalidateCached('ws:me')
                void Promise.all([loadImages(owner, folderFilter, false, true), loadAccounts(true)])
              }}
            />
          )}
        </DialogContent>
      </Dialog>

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
