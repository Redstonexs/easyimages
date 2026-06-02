<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { AdminChart } from '../../types'
import { adminApi } from '../../shared/adminApi'

const loading = ref(true)
const chart = ref<AdminChart | null>(null)

const maxDaily = computed(() => Math.max(1, ...(chart.value?.daily_stats.map(item => item.count) || [1])))

async function load() {
  loading.value = true
  try {
    chart.value = await adminApi.chart()
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="alert alert-info">正在加载统计...</div>
  <template v-else-if="chart">
    <div class="row stat-cards">
      <div class="col-md-3"><div class="panel panel-primary"><div class="panel-heading">总文件数</div><div class="panel-body text-center"><h2>{{ chart.total_files }}</h2></div></div></div>
      <div class="col-md-3"><div class="panel panel-success"><div class="panel-heading">已用空间</div><div class="panel-body text-center"><h2>{{ chart.used_human }}</h2></div></div></div>
      <div class="col-md-3"><div class="panel panel-info"><div class="panel-heading">今日上传</div><div class="panel-body text-center"><h2>{{ chart.daily_stats[chart.daily_stats.length - 1]?.count || 0 }}</h2></div></div></div>
      <div class="col-md-3"><div class="panel panel-warning"><div class="panel-heading">版本</div><div class="panel-body text-center"><h2>v{{ chart.version }}</h2></div></div></div>
    </div>

    <section class="panel panel-default">
      <div class="panel-heading"><h3 class="panel-title">近30天上传趋势</h3></div>
      <div class="panel-body daily-bars">
        <div v-for="item in chart.daily_stats" :key="item.date" class="daily-bar">
          <span>{{ item.count }}</span>
          <div :style="{ height: `${Math.max(4, item.count / maxDaily * 200)}px` }"></div>
          <small>{{ item.date }}</small>
        </div>
      </div>
    </section>

    <section class="panel panel-default">
      <div class="panel-heading"><h3 class="panel-title">格式分布</h3></div>
      <div class="panel-body">
        <table class="table table-hover">
          <thead><tr><th>格式</th><th>数量</th><th>占比</th></tr></thead>
          <tbody>
            <tr v-for="(count, ext) in chart.format_stats" :key="ext">
              <td><span class="label label-primary">{{ ext }}</span></td>
              <td>{{ count }}</td>
              <td><div class="progress"><div class="progress-bar" :style="{ width: `${chart.total_files ? count / chart.total_files * 100 : 0}%` }"></div></div></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </template>
</template>

<style scoped>
.daily-bars { display: flex; align-items: flex-end; height: 280px; gap: 4px; overflow-x: auto; }
.daily-bar { min-width: 28px; flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: flex-end; gap: 4px; }
.daily-bar div { width: 100%; background: #3280fc; border-radius: 4px 4px 0 0; }
.daily-bar span { font-size: 11px; color: #666; }
.daily-bar small { font-size: 10px; color: #999; transform: rotate(-45deg); transform-origin: center; margin-top: 10px; }
.progress { margin-bottom: 0; }
</style>
