import { useEffect, useMemo, useState } from 'react'
import { Files, HardDrive, Tag, TrendingUp } from 'lucide-react'
import type { AdminChart as AdminChartData } from '@/types'
import type { NoticeType } from '@/shared/notify'
import { adminApi } from '@/shared/adminApi'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

export default function AdminChart({ onNotice }: { onNotice: (message: string, type?: NoticeType) => void }) {
  const [loading, setLoading] = useState(true)
  const [chart, setChart] = useState<AdminChartData | null>(null)
  const [hoveredDay, setHoveredDay] = useState<{ date: string; count: number } | null>(null)

  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      try {
        const data = await adminApi.chart()
        if (!cancelled) setChart(data)
      } catch (error) {
        if (!cancelled) onNotice(error instanceof Error ? error.message : '加载统计失败', 'danger')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [onNotice])

  const maxDaily = useMemo(() => {
    if (!chart?.daily_stats.length) return 1
    return Math.max(1, ...chart.daily_stats.map(item => item.count))
  }, [chart])

  const formatRows = useMemo(() => {
    if (!chart?.format_stats) return []
    return Object.entries(chart.format_stats).sort((a, b) => b[1] - a[1])
  }, [chart])

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-24 rounded-xl" />)}
        </div>
        <Skeleton className="h-72 rounded-xl" />
        <Skeleton className="h-56 rounded-xl" />
      </div>
    )
  }

  if (!chart) return null

  const todayCount = chart.daily_stats[chart.daily_stats.length - 1]?.count || 0

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Card className="gap-1 py-4">
          <CardContent className="flex items-center justify-between">
            <div>
              <p className="text-xs text-muted-foreground">总文件数</p>
              <p className="mt-1 font-mono text-2xl font-semibold tabular-nums">{chart.total_files}</p>
            </div>
            <Files className="size-4 text-muted-foreground" />
          </CardContent>
        </Card>
        <Card className="gap-1 py-4">
          <CardContent className="flex items-center justify-between">
            <div>
              <p className="text-xs text-muted-foreground">已用空间</p>
              <p className="mt-1 font-mono text-2xl font-semibold tabular-nums">{chart.used_human}</p>
            </div>
            <HardDrive className="size-4 text-muted-foreground" />
          </CardContent>
        </Card>
        <Card className="gap-1 py-4">
          <CardContent className="flex items-center justify-between">
            <div>
              <p className="text-xs text-muted-foreground">今日上传</p>
              <p className="mt-1 font-mono text-2xl font-semibold tabular-nums">{todayCount}</p>
            </div>
            <TrendingUp className="size-4 text-muted-foreground" />
          </CardContent>
        </Card>
        <Card className="gap-1 py-4">
          <CardContent className="flex items-center justify-between">
            <div>
              <p className="text-xs text-muted-foreground">系统版本</p>
              <p className="mt-1 font-mono text-2xl font-semibold tabular-nums">v{chart.version}</p>
            </div>
            <Tag className="size-4 text-muted-foreground" />
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2">
          <div>
            <CardTitle className="text-base">近 30 天上传趋势</CardTitle>
            <CardDescription>按日期统计本地存储目录新增图片数量。</CardDescription>
          </div>
          <div className="text-right font-mono text-xs text-muted-foreground tabular-nums">
            {hoveredDay ? `${hoveredDay.date} · ${hoveredDay.count} 张` : `峰值 ${maxDaily} 张/日`}
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex h-60 items-end gap-1.5 overflow-x-auto pb-2 pt-4">
            {chart.daily_stats.map(item => {
              const heightPct = item.count === 0 ? 2 : Math.max(6, Math.round((item.count / maxDaily) * 100))
              return (
                <div
                  key={item.date}
                  onMouseEnter={() => setHoveredDay(item)}
                  onMouseLeave={() => setHoveredDay(null)}
                  className="group flex h-full min-w-[20px] flex-1 flex-col items-center justify-end gap-1.5"
                >
                  <span className="font-mono text-[10px] text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 tabular-nums">
                    {item.count}
                  </span>
                  <div className="flex w-full flex-1 items-end">
                    <div
                      style={{ height: `${heightPct}%` }}
                      className={`w-full rounded-t-sm transition-colors ${
                        item.count > 0
                          ? 'bg-foreground/80 group-hover:bg-foreground'
                          : 'bg-muted'
                      }`}
                    />
                  </div>
                  <span className="font-mono text-[10px] text-muted-foreground select-none">
                    {item.date.slice(3)}
                  </span>
                </div>
              )
            })}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">格式分布</CardTitle>
          <CardDescription>各扩展名文件数量与占比。</CardDescription>
        </CardHeader>
        <CardContent>
          {formatRows.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">暂无格式统计数据</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-28">格式</TableHead>
                  <TableHead className="w-24">数量</TableHead>
                  <TableHead>占比</TableHead>
                  <TableHead className="w-20 text-right">比例</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {formatRows.map(([ext, count]) => {
                  const pct = chart.total_files ? (count / chart.total_files) * 100 : 0
                  return (
                    <TableRow key={ext}>
                      <TableCell>
                        <Badge variant="secondary" className="font-mono uppercase">{ext.replace(/^\./, '')}</Badge>
                      </TableCell>
                      <TableCell className="font-mono tabular-nums">{count}</TableCell>
                      <TableCell>
                        <Progress value={pct} className="h-1.5" />
                      </TableCell>
                      <TableCell className="text-right font-mono text-xs text-muted-foreground tabular-nums">
                        {pct.toFixed(1)}%
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
