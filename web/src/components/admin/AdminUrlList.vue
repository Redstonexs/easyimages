<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { AdminURLList } from '../../types'
import type { NoticeType } from '../../shared/notify'
import { adminApi } from '../../shared/adminApi'
import { copyText } from '../../shared/clipboard'

const emit = defineEmits<{ notice: [message: string, type?: NoticeType] }>()
const loading = ref(true)
const data = ref<AdminURLList | null>(null)
const page = ref(1)

async function load(nextPage = page.value) {
  loading.value = true
  page.value = nextPage
  const params = new URLSearchParams({ page: String(page.value), page_size: '50' })
  try { data.value = await adminApi.urlList(params) } finally { loading.value = false }
}
async function copy(url: string) { await copyText(url); emit('notice', '复制成功', 'success') }
onMounted(() => load())
</script>

<template>
  <div v-if="loading" class="alert alert-info">正在加载图片列表...</div>
  <section v-else-if="data" class="panel panel-default">
    <div class="panel-heading"><h3 class="panel-title">图片URL列表 <span class="badge">{{ data.total }} 张</span></h3></div>
    <div class="panel-body">
      <p class="text-muted">第 {{ data.page }} / {{ data.total_pages }} 页</p>
      <table class="table table-hover table-bordered url-table">
        <thead><tr><th>预览</th><th>文件名</th><th>图片URL</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="file in data.files" :key="file.url">
            <td><img :src="file.thumb_url" class="preview-thumb" :alt="file.name" loading="lazy" decoding="async"></td>
            <td>{{ file.name }}</td>
            <td class="url-cell"><a :href="file.url" target="_blank">{{ file.url }}</a><template v-if="file.webp_url"><br><small>WebP: </small><a :href="file.webp_url" target="_blank">{{ file.webp_url }}</a></template></td>
            <td><button class="btn btn-xs btn-primary" @click="copy(file.url)">复制</button><button v-if="file.webp_url" class="btn btn-xs btn-info" @click="copy(file.webp_url)">WebP</button></td>
          </tr>
        </tbody>
      </table>
      <div class="pager"><button class="btn btn-default" :disabled="data.page <= 1" @click="load(data.page - 1)">上一页</button><span>第 {{ data.page }} / {{ data.total_pages }} 页</span><button class="btn btn-default" :disabled="data.page >= data.total_pages" @click="load(data.page + 1)">下一页</button></div>
    </div>
  </section>
</template>

<style scoped>
.preview-thumb { width: 60px; height: 60px; object-fit: cover; border-radius: 4px; }
.url-cell { word-break: break-all; max-width: 520px; }
.pager { display: flex; gap: 12px; justify-content: center; align-items: center; }
</style>
