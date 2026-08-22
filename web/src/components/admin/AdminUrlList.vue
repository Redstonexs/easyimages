<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import type { AdminURLList } from '../../types'
import type { NoticeType } from '../../shared/notify'
import { adminApi } from '../../shared/adminApi'
import { useCopy } from '../../shared/useCopy'

const emit = defineEmits<{ notice: [message: string, type?: NoticeType] }>()
const loading = ref(true)
const data = ref<AdminURLList | null>(null)
const page = ref(1)
const queryInput = ref('')

const notify = (message: string, type?: NoticeType) => emit('notice', message, type)
const copy = useCopy(notify)
let inflight: AbortController | null = null

async function load(nextPage = page.value) {
  inflight?.abort()
  const controller = new AbortController()
  inflight = controller

  loading.value = true
  const params = new URLSearchParams({ page: String(nextPage), page_size: '50' })
  if (queryInput.value.trim()) params.set('q', queryInput.value.trim())
  const typedQuery = queryInput.value
  try {
    data.value = await adminApi.urlList(params, { signal: controller.signal })
    // Commit the page only once the matching response lands.
    page.value = nextPage
    if (queryInput.value === typedQuery) queryInput.value = data.value.q || ''
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') return
    notify(error instanceof Error ? error.message : '加载图片列表失败', 'danger')
  } finally {
    if (inflight === controller) {
      inflight = null
      loading.value = false
    }
  }
}
function search() { void load(1) }
function clearSearch() { queryInput.value = ''; void load(1) }
function displayName(file: { name: string; original_name?: string }) { return file.original_name || file.name }
onMounted(() => load())
onUnmounted(() => inflight?.abort())
</script>

<template>
  <div v-if="loading" class="alert alert-info">正在加载图片列表...</div>
  <section v-else-if="data" class="panel panel-default">
    <div class="panel-heading"><h3 class="panel-title">图片URL列表 <span class="badge">{{ data.total }} 张</span></h3></div>
    <div class="panel-body">
      <form class="url-search" role="search" @submit.prevent="search">
        <input v-model="queryInput" type="search" class="form-control" placeholder="搜索源文件名、存储文件名或路径">
        <button type="submit" class="btn btn-primary">搜索</button>
        <button v-if="data.q" type="button" class="btn btn-default" @click="clearSearch">清空</button>
      </form>
      <p class="text-muted">第 {{ data.page }} / {{ data.total_pages }} 页<span v-if="data.q">，搜索：{{ data.q }}</span></p>
      <table v-if="data.files.length" class="table table-hover table-bordered url-table">
        <thead><tr><th>预览</th><th>文件名</th><th>图片URL</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="file in data.files" :key="file.url">
            <td><img :src="file.thumb_url" class="preview-thumb" :alt="displayName(file)" loading="lazy" decoding="async"></td>
            <td><strong :title="displayName(file)">{{ displayName(file) }}</strong><br><small v-if="file.original_name" :title="file.name">{{ file.name }}</small></td>
            <td class="url-cell"><a :href="file.url" target="_blank">{{ file.url }}</a><template v-if="file.webp_url"><br><small>WebP: </small><a :href="file.webp_url" target="_blank">{{ file.webp_url }}</a></template></td>
            <td><button class="btn btn-xs btn-primary" @click="copy(file.url)">复制</button><button v-if="file.webp_url" class="btn btn-xs btn-info" @click="copy(file.webp_url)">WebP</button></td>
          </tr>
        </tbody>
      </table>
      <div v-else class="alert alert-info">{{ data.q ? '没有匹配的图片。' : '暂无图片。' }}</div>
      <div class="pager"><button class="btn btn-default" :disabled="data.page <= 1" @click="load(data.page - 1)">上一页</button><span>第 {{ data.page }} / {{ data.total_pages }} 页</span><button class="btn btn-default" :disabled="data.page >= data.total_pages" @click="load(data.page + 1)">下一页</button></div>
    </div>
  </section>
</template>

<style scoped>
.preview-thumb { width: 60px; height: 60px; object-fit: cover; border-radius: 4px; }
.url-search { display: flex; gap: 8px; margin-bottom: 14px; }
.url-search .form-control { max-width: 420px; }
.url-cell { word-break: break-all; max-width: 520px; }
.url-table small { color: #777; }
.pager { display: flex; gap: 12px; justify-content: center; align-items: center; }
@media (max-width: 640px) { .url-search { flex-direction: column; } }
</style>
