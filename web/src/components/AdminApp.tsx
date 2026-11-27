import { useEffect, useRef, useState } from 'react'
import {
  BarChart3,
  FolderKanban,
  FolderOpen,
  History,
  Home,
  LogOut,
  Settings
} from 'lucide-react'
import type { AdminBootstrap, AdminView } from '@/types'
import { notifyToast } from '@/shared/useNotices'
import { useConfirmDialog } from '@/components/ConfirmDialog'
import { SiteHeader } from '@/components/SiteHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Toaster } from '@/components/ui/sonner'
import AdminChart from '@/components/admin/AdminChart'
import AdminConfig from '@/components/admin/AdminConfig'
import AdminFiler from '@/components/admin/AdminFiler'
import AdminHistory from '@/components/admin/AdminHistory'
import WorkspaceLibrary from '@/components/admin/WorkspaceLibrary'

const NAV_ITEMS: Array<{ view: AdminView; label: string; description: string; href: string; icon: typeof Settings }> = [
  { view: 'manager', label: '系统配置', description: '站点、上传、水印、安全与存储源参数', href: '/admin/manager', icon: Settings },
  { view: 'workspace', label: '图片库', description: '目录管理、实时转换、多租户配额与旧图浏览', href: '/admin/workspace', icon: FolderOpen },
  { view: 'history', label: '历史上传', description: '按日期与格式检索历史上传并批量管理', href: '/admin/history', icon: History },
  { view: 'chart', label: '数据统计', description: '近 30 天上传趋势与存储格式分布', href: '/admin/chart', icon: BarChart3 },
  { view: 'filer', label: '文件管理', description: '按本地目录树浏览、复制与清理文件', href: '/admin/filer', icon: FolderKanban }
]

export default function AdminApp({ bootstrap }: { bootstrap: AdminBootstrap }) {
  const [currentView, setCurrentView] = useState<AdminView>(bootstrap.view)
  const [configDirty, setConfigDirty] = useState(false)
  const activeNavRef = useRef<HTMLAnchorElement>(null)
  const { confirm, dialog: confirmDialog } = useConfirmDialog()

  const activeNav = NAV_ITEMS.find(item => item.view === currentView)
  const title = activeNav?.label || '管理后台'

  useEffect(() => {
    document.title = `${title} - ${bootstrap.title}`
    activeNavRef.current?.scrollIntoView({ inline: 'nearest', block: 'nearest' })
  }, [title, bootstrap.title, currentView])

  useEffect(() => {
    const onPopState = () => {
      const item = NAV_ITEMS.find(nav => nav.href === window.location.pathname)
      if (item) {
        setConfigDirty(false)
        setCurrentView(item.view)
      }
    }
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  async function navigate(view: AdminView, href: string) {
    if (view === currentView) return
    if (configDirty) {
      const ok = await confirm({
        title: '离开当前页面？',
        description: '系统配置尚未保存，离开后未保存的修改将会丢失。',
        confirmLabel: '放弃修改并离开'
      })
      if (!ok) return
    }
    setConfigDirty(false)
    setCurrentView(view)
    window.history.pushState({ view }, '', href)
    if (window.scrollY > 120) {
      window.scrollTo({ top: 0, behavior: 'smooth' })
    }
  }

  return (
    <div className="min-h-screen flex flex-col bg-background text-foreground">
      <Toaster />
      {confirmDialog}

      <SiteHeader
        brand={bootstrap.is_admin ? 'EasyImage 管理后台' : 'EasyImage 图片库'}
        brandHref={bootstrap.is_admin ? '/admin/manager' : '/workspace'}
        subtitle={`v${bootstrap.version}`}
        right={(
          <div className="flex items-center gap-1.5">
            {bootstrap.is_admin && (
              <Button variant="ghost" size="sm" asChild>
                <a href="/">
                  <Home className="size-4" />
                  <span className="hidden sm:inline">上传首页</span>
                </a>
              </Button>
            )}
            <form method="post" action="/logout">
              <Button variant="outline" size="sm" type="submit">
                <LogOut className="size-4" />
                <span className="hidden sm:inline">退出</span>
              </Button>
            </form>
          </div>
        )}
      />

      <div className="mx-auto flex w-full max-w-6xl flex-1 flex-col gap-6 px-4 py-6 sm:px-6 lg:flex-row lg:gap-8 lg:py-8">
        {bootstrap.is_admin && (
          <aside className="shrink-0 lg:w-52" aria-label="后台导航">
            <nav className="flex gap-1 overflow-x-auto pb-1 lg:sticky lg:top-20 lg:flex-col lg:gap-1 lg:overflow-visible lg:pb-0">
              {NAV_ITEMS.map(item => {
                const Icon = item.icon
                const active = currentView === item.view
                return (
                  <a
                    key={item.view}
                    ref={active ? activeNavRef : undefined}
                    href={item.href}
                    onClick={event => {
                      event.preventDefault()
                      void navigate(item.view, item.href)
                    }}
                    aria-current={active ? 'page' : undefined}
                    className={
                      active
                        ? 'inline-flex shrink-0 items-center justify-between gap-2.5 rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-foreground shadow-xs transition-[background-color,color,transform] duration-150'
                        : 'inline-flex shrink-0 items-center justify-between gap-2.5 rounded-lg px-3 py-2 text-sm font-medium text-muted-foreground transition-[background-color,color,transform] duration-150 hover:bg-muted hover:text-foreground active:scale-[0.98]'
                    }
                  >
                    <span className="inline-flex items-center gap-2.5">
                      <Icon className="size-4 shrink-0" />
                      <span>{item.label}</span>
                    </span>
                    {item.view === 'manager' && configDirty && (
                      <span
                        title="有未保存的更改"
                        className={`size-1.5 rounded-full ${active ? 'bg-primary-foreground' : 'bg-foreground'}`}
                      />
                    )}
                  </a>
                )
              })}
            </nav>
          </aside>
        )}

        <main className="min-w-0 flex-1 space-y-6">
          <header className="flex flex-wrap items-end justify-between gap-2 border-b pb-4">
            <div key={currentView} className="panel-enter">
              <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">{title}</h1>
              <p className="mt-1 text-xs text-muted-foreground">
                {activeNav?.description || bootstrap.title}
              </p>
            </div>
            {configDirty && currentView === 'manager' && (
              <Badge variant="outline" className="panel-enter gap-1.5 font-normal">
                <span className="size-1.5 rounded-full bg-foreground" />
                有未保存的配置更改
              </Badge>
            )}
          </header>

          <div key={currentView} className="view-enter">
            {currentView === 'manager' && (
              <AdminConfig onNotice={notifyToast} onDirtyChange={setConfigDirty} />
            )}
            {currentView === 'history' && <AdminHistory onNotice={notifyToast} />}
            {currentView === 'chart' && <AdminChart onNotice={notifyToast} />}
            {currentView === 'filer' && <AdminFiler onNotice={notifyToast} />}
            {currentView === 'workspace' && (
              <WorkspaceLibrary
                isAdmin={bootstrap.is_admin}
                maxUploadSize={bootstrap.max_upload_size}
                onNotice={notifyToast}
              />
            )}
          </div>
        </main>
      </div>
    </div>
  )
}
