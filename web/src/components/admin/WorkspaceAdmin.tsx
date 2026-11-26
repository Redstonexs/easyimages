import { useEffect, useMemo, useState } from 'react'
import { AlertCircle, Link2, Loader2, Save, UserPlus, Users } from 'lucide-react'
import type { NoticeType } from '@/shared/notify'
import { formatSize } from '@/shared/format'
import type { WorkspaceSettings, WorkspaceUser } from '@/shared/workspaceApi'
import { workspaceApi } from '@/shared/workspaceApi'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

const BYTES_PER_MIB = 1024 * 1024

interface WorkspaceAdminProps {
  users: WorkspaceUser[]
  onNotice: (message: string, type?: NoticeType) => void
  onUsersChanged: () => void
  onLinksChanged: () => void
}

export default function WorkspaceAdmin({
  users,
  onNotice,
  onUsersChanged,
  onLinksChanged
}: WorkspaceAdminProps) {
  const [editing, setEditing] = useState<WorkspaceUser | undefined>()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [quota, setQuota] = useState('0')
  const [disabled, setDisabled] = useState(false)
  const [savingUser, setSavingUser] = useState(false)
  const [userError, setUserError] = useState('')

  const [settings, setSettings] = useState<WorkspaceSettings | undefined>()
  const [savingLinks, setSavingLinks] = useState(false)
  const [settingsError, setSettingsError] = useState('')

  useEffect(() => {
    let cancelled = false
    async function load() {
      try {
        const res = await workspaceApi.settings()
        if (!cancelled) setSettings(res)
      } catch (err) {
        if (!cancelled) setSettingsError(err instanceof Error ? err.message : '无法加载公开链接设置')
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [])

  const exampleURL = useMemo(() => {
    if (!settings) return ''
    const origin = settings.public_origin || '站点域名'
    return `${origin}${settings.public_prefix}/{image_id}`
  }, [settings])

  function startEdit(user?: WorkspaceUser) {
    setEditing(user)
    setUsername(user?.username || '')
    setQuota(String((user?.quota_bytes || 0) / BYTES_PER_MIB))
    setDisabled(Boolean(user?.disabled))
    setPassword('')
    setUserError('')
  }

  async function saveUser(event: React.FormEvent) {
    event.preventDefault()
    setSavingUser(true)
    setUserError('')
    const quota_bytes = Math.round(Number(quota) * BYTES_PER_MIB)
    try {
      if (editing) {
        await workspaceApi.updateUser(editing.id, {
          quota_bytes,
          disabled,
          password: password || undefined
        })
        onNotice('已更新租户账号', 'success')
      } else {
        await workspaceApi.createUser({ username: username.trim(), password, quota_bytes })
        onNotice('已创建租户账号', 'success')
      }
      startEdit(undefined)
      onUsersChanged()
    } catch (err) {
      setUserError(err instanceof Error ? err.message : '账号保存失败')
    } finally {
      setSavingUser(false)
    }
  }

  async function saveSettings(event: React.FormEvent) {
    event.preventDefault()
    if (!settings) return
    setSavingLinks(true)
    setSettingsError('')
    try {
      const updated = await workspaceApi.updateSettings({
        public_origin: settings.public_origin.trim(),
        public_prefix: settings.public_prefix.trim()
      })
      setSettings(updated)
      onNotice('已保存公开链接设置', 'success')
      onLinksChanged()
    } catch (err) {
      setSettingsError(err instanceof Error ? err.message : '链接设置保存失败')
    } finally {
      setSavingLinks(false)
    }
  }

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Users className="size-4" />
            租户账号
          </CardTitle>
          <CardDescription>为团队成员分配独立图片空间与存储配额。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          {users.length === 0 ? (
            <p className="rounded-lg border border-dashed py-6 text-center text-xs text-muted-foreground">
              暂无租户账号
            </p>
          ) : (
            <ul className="divide-y rounded-lg border">
              {users.map(user => (
                <li key={user.id} className="flex items-center justify-between gap-3 px-3.5 py-2.5">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium">{user.username}</span>
                      {user.disabled && <Badge variant="outline" className="text-[10px]">已停用</Badge>}
                    </div>
                    <p className="font-mono text-xs text-muted-foreground tabular-nums">
                      {formatSize(user.used_bytes)} / {user.quota_bytes ? formatSize(user.quota_bytes) : '不限'}
                    </p>
                  </div>
                  <Button type="button" size="xs" variant="outline" disabled={savingUser} onClick={() => startEdit(user)}>
                    编辑
                  </Button>
                </li>
              ))}
            </ul>
          )}

          <form onSubmit={saveUser} className="space-y-3 border-t pt-4">
            <h4 className="text-sm font-medium">
              {editing ? `编辑账号：${editing.username}` : '创建新租户账号'}
            </h4>

            {!editing && (
              <div className="space-y-1.5">
                <Label htmlFor="ws-username">用户名</Label>
                <Input
                  id="ws-username"
                  value={username}
                  onChange={e => setUsername(e.target.value)}
                  minLength={3}
                  maxLength={32}
                  required
                  autoComplete="off"
                  disabled={savingUser}
                />
              </div>
            )}

            <div className="space-y-1.5">
              <Label htmlFor="ws-password">{editing ? '新密码（留空则不修改）' : '初始密码'}</Label>
              <Input
                id="ws-password"
                type="password"
                value={password}
                onChange={e => setPassword(e.target.value)}
                minLength={8}
                maxLength={72}
                required={!editing}
                autoComplete="new-password"
                disabled={savingUser}
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="ws-quota">存储配额（MiB，0 表示不限）</Label>
              <Input
                id="ws-quota"
                type="number"
                min={0}
                max={1073741824}
                step="any"
                required
                value={quota}
                onChange={e => setQuota(e.target.value)}
                disabled={savingUser}
              />
              <p className="text-xs text-muted-foreground">原图和预生成版本均计入配额。</p>
            </div>

            {editing && (
              <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
                <Label htmlFor="ws-disabled" className="text-xs">停用登录与 API 访问</Label>
                <Switch id="ws-disabled" checked={disabled} onCheckedChange={setDisabled} disabled={savingUser} />
              </div>
            )}

            <div className="flex items-center gap-2 pt-1">
              <Button type="submit" size="sm" disabled={savingUser}>
                {savingUser ? <Loader2 className="size-3.5 animate-spin" /> : editing ? <Save className="size-3.5" /> : <UserPlus className="size-3.5" />}
                {editing ? '保存账号' : '创建账号'}
              </Button>
              {editing && (
                <Button type="button" variant="outline" size="sm" onClick={() => startEdit(undefined)}>
                  取消
                </Button>
              )}
            </div>

            {userError && (
              <Alert variant="emphasis">
                <AlertCircle className="size-4" />
                <AlertDescription>{userError}</AlertDescription>
              </Alert>
            )}
          </form>
        </CardContent>
      </Card>

      <Card className="h-fit">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Link2 className="size-4" />
            公开图片链接
          </CardTitle>
          <CardDescription>自定义对外分发的域名和路径前缀，历史前缀继续保持可访问。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {settings && (
            <form onSubmit={saveSettings} className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="ws-origin">公开域名（可选）</Label>
                <Input
                  id="ws-origin"
                  type="url"
                  value={settings.public_origin}
                  onChange={e => setSettings({ ...settings, public_origin: e.target.value })}
                  placeholder="留空使用站点域名"
                  disabled={savingLinks}
                />
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="ws-prefix">链接前缀</Label>
                <Input
                  id="ws-prefix"
                  value={settings.public_prefix}
                  onChange={e => setSettings({ ...settings, public_prefix: e.target.value })}
                  maxLength={80}
                  placeholder="/media"
                  required
                  disabled={savingLinks}
                  className="font-mono text-xs"
                />
              </div>

              <div className="rounded-lg border bg-muted/40 px-3 py-2 font-mono text-xs text-muted-foreground break-all">
                {exampleURL}
              </div>

              <Button type="submit" size="sm" disabled={savingLinks}>
                {savingLinks ? <Loader2 className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
                保存链接设置
              </Button>
            </form>
          )}

          {settingsError && (
            <Alert variant="emphasis">
              <AlertCircle className="size-4" />
              <AlertDescription>{settingsError}</AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
