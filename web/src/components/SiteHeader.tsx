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
    <header className="sticky top-0 z-40 border-b bg-background/85 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between gap-4 px-4 sm:px-6">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <a
            href={brandHref}
            className="group flex min-w-0 items-center gap-2.5 text-sm font-semibold tracking-tight text-foreground transition-opacity hover:opacity-85"
          >
            <span
              aria-hidden="true"
              className="grid size-7 shrink-0 place-items-center rounded-md bg-primary text-[11px] font-bold tracking-tighter text-primary-foreground shadow-2xs transition-transform duration-200 group-hover:scale-105"
            >
              EI
            </span>
            <span className="truncate">{brand}</span>
          </a>
          {subtitle && (
            <span className="hidden shrink-0 rounded-md border bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground sm:inline-block">
              {subtitle}
            </span>
          )}
          {leftExtra}
        </div>

        <div className="flex shrink-0 items-center gap-1 sm:gap-2">
          {right}
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={toggleTheme}
            aria-label={theme === 'dark' ? '切换为浅色模式' : '切换为深色模式'}
            title={theme === 'dark' ? '切换浅色模式' : '切换深色模式'}
            className="relative overflow-hidden"
          >
            <Sun
              className={`size-4 transition-all duration-200 ${
                theme === 'dark' ? 'rotate-0 scale-100 opacity-100' : 'rotate-90 scale-0 opacity-0'
              }`}
            />
            <Moon
              className={`absolute size-4 transition-all duration-200 ${
                theme === 'dark' ? '-rotate-90 scale-0 opacity-0' : 'rotate-0 scale-100 opacity-100'
              }`}
            />
          </Button>
        </div>
      </div>
    </header>
  )
}
