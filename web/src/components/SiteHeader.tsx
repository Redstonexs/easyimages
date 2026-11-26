import * as React from 'react'
import { Moon, Sun } from 'lucide-react'
import { useTheme } from '@/shared/theme'
import { Button } from '@/components/ui/button'

interface SiteHeaderProps {
  brand: string
  brandHref?: string
  subtitle?: string
  leftExtra?: React.ReactNode
  right?: React.ReactNode
}

export function SiteHeader({ brand, brandHref = '/', subtitle, leftExtra, right }: SiteHeaderProps) {
  const { theme, toggleTheme } = useTheme()

  return (
    <header className="sticky top-0 z-40 border-b bg-background/90 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between gap-4 px-4 sm:px-6">
        <div className="flex min-w-0 items-center gap-3">
          <a
            href={brandHref}
            className="flex items-center gap-2.5 text-sm font-semibold tracking-tight text-foreground transition-opacity hover:opacity-80"
          >
            <span
              aria-hidden="true"
              className="grid size-6 shrink-0 place-items-center rounded-md bg-primary text-[11px] font-bold text-primary-foreground"
            >
              EI
            </span>
            <span className="truncate">{brand}</span>
          </a>
          {subtitle && (
            <span className="hidden text-xs text-muted-foreground sm:inline-block">{subtitle}</span>
          )}
          {leftExtra}
        </div>

        <div className="flex items-center gap-1.5 sm:gap-2">
          {right}
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={toggleTheme}
            aria-label={theme === 'dark' ? '切换为浅色模式' : '切换为深色模式'}
            title={theme === 'dark' ? '浅色模式' : '深色模式'}
          >
            {theme === 'dark' ? <Sun className="size-4" /> : <Moon className="size-4" />}
          </Button>
        </div>
      </div>
    </header>
  )
}
