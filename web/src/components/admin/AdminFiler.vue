<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { AdminFileEntry, AdminFiler } from '../../types'
import type { NoticeType } from '../../shared/notify'
import { adminApi } from '../../shared/adminApi'
import { copyText } from '../../shared/clipboard'

const emit = defineEmits<{ notice: [message: string, type?: NoticeType] }>()
const loading = ref(true)
const data = ref<AdminFiler | null>(null)
const failedThumbs = ref<Set<string>>(new Set())
const queryInput = ref('')
const selectedImage = ref<AdminFileEntry | null>(null)

const breadcrumbs = computed(() => {
  if (!data.value) return []
  let cumulative = '/'
  return data.value.path.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean).map(segment => {
    cumulative += `${segment}/`
    return { label: segment, path: cumulative }
  })
})

const filteredFiles = computed(() => {
  const files = data.value?.files || []
  const query = queryInput.value.trim().toLowerCase()
  if (!query) return files
  return files.filter(file => [file.name, file.original_name, file.path, file.url, file.ext].some(value => value?.toLowerCase().includes(query)))
})

const totalItems = computed(() => (data.value?.dirs.length || 0) + (data.value?.files.length || 0))

async function load(path?: string) {
  loading.value = true
  const params = new URLSearchParams()
  if (path) params.set('path', path)
  try {
    data.value = await adminApi.filer(params)
    queryInput.value = ''
  } catch (error) {
    emit('notice', error instanceof Error ? error.message : '加载失败', 'danger')
  } finally { loading.value = false }
}

async function copy(url: string) { await copyText(url); emit('notice', '复制成功', 'success') }

async function remove(file: AdminFileEntry) {
  if (!window.confirm('确认删除此文件？')) return
  const result = await adminApi.deleteFile(file.path)
  if (result.code === 200 && data.value) {
    data.value.files = data.value.files.filter(item => item.url !== file.url)
    if (selectedImage.value?.url === file.url) selectedImage.value = null
    emit('notice', result.msg || '删除成功', 'success')
  } else {
    emit('notice', result.msg || '删除失败', 'danger')
  }
}

function markFailed(url: string) { failedThumbs.value = new Set([...failedThumbs.value, url]) }

function openDir(dir: string) {
  if (!data.value) return
  void load(data.value.path + dir)
}

function displayName(file: AdminFileEntry) { return file.original_name || file.name }

function fileExt(file: AdminFileEntry) { return file.ext || file.name.split('.').pop()?.toUpperCase() || 'IMG' }

function clearSearch() { queryInput.value = '' }

onMounted(() => load())
</script>

<template>
  <div v-if="loading" class="filer-loading">
    <span class="loading-dot"></span>
    正在加载文件...
  </div>
  <section v-else-if="data" class="filer-shell">
    <header class="filer-header">
      <div class="filer-title-block">
        <span class="filer-kicker">File Browser</span>
        <h3>文件管理</h3>
        <p :title="data.path">{{ data.path }}</p>
      </div>
      <div class="filer-stats" aria-label="当前目录概览">
        <span><strong>{{ data.dirs.length }}</strong> 目录</span>
        <span><strong>{{ data.files.length }}</strong> 文件</span>
        <span><strong>{{ totalItems }}</strong> 项目</span>
      </div>
    </header>

    <div class="filer-toolbar">
      <nav class="filer-breadcrumb" aria-label="文件路径">
        <button class="crumb root" @click="load(data.root_path)">根目录</button>
        <button v-for="crumb in breadcrumbs" :key="crumb.path" class="crumb" :class="{ active: crumb.path === data.path }" @click="load(crumb.path)">{{ crumb.label }}</button>
      </nav>
      <div class="filer-actions">
        <button v-if="data.parent_path" class="btn btn-default" @click="load(data.parent_path)"><i class="icon icon-level-up"></i> 上级</button>
        <button class="btn btn-default" @click="load(data.path)"><i class="icon icon-refresh"></i> 刷新</button>
      </div>
    </div>

    <div class="filer-layout">
      <aside class="filer-sidebar" aria-label="子目录">
        <div class="section-heading">
          <span>子目录</span>
          <strong>{{ data.dirs.length }}</strong>
        </div>
        <div v-if="data.dirs.length" class="dir-list">
          <button v-for="dir in data.dirs" :key="dir" class="dir-row" @click="openDir(dir)">
            <span class="folder-icon"><i class="icon icon-folder-close"></i></span>
            <span class="dir-name" :title="dir">{{ dir }}</span>
          </button>
        </div>
        <div v-else class="empty-compact">当前目录没有子目录</div>
      </aside>

      <main class="filer-main">
        <div class="file-table-top">
          <div>
            <div class="section-heading inline">
              <span>文件</span>
              <strong>{{ filteredFiles.length }} / {{ data.files.length }}</strong>
            </div>
            <p class="table-hint">缩略图、源文件名、存储路径、大小、格式和修改时间集中显示。</p>
          </div>
          <form class="filer-search" role="search" @submit.prevent>
            <input v-model="queryInput" type="search" class="form-control" placeholder="过滤文件名、路径或格式">
            <button v-if="queryInput" type="button" class="btn btn-default" @click="clearSearch">清空</button>
          </form>
        </div>

        <div v-if="filteredFiles.length" class="file-table-wrap">
          <table class="table file-table">
            <thead>
              <tr>
                <th class="thumb-col">预览</th>
                <th>文件信息</th>
                <th>格式</th>
                <th>大小</th>
                <th>修改时间</th>
                <th class="action-col">操作</th>
              </tr>
            </thead>
        <tbody>
          <tr v-for="file in filteredFiles" :key="file.url">
            <td class="thumb-col">
              <button class="thumb-button" :title="`预览 ${displayName(file)}`" @click="selectedImage = file">
                <span v-if="failedThumbs.has(file.url)" class="thumb-fallback"><i class="icon icon-file-image-o"></i></span>
                <img v-else :src="file.thumb_url" class="file-thumb" :alt="displayName(file)" loading="lazy" decoding="async" @error="markFailed(file.url)">
              </button>
            </td>
            <td class="file-info-cell">
              <strong :title="displayName(file)">{{ displayName(file) }}</strong>
              <span v-if="file.original_name" class="stored-name" :title="file.name">{{ file.name }}</span>
              <code :title="file.path">{{ file.path }}</code>
              <a v-if="file.webp_url" class="webp-link" :href="file.webp_url" target="_blank">WebP 已生成</a>
            </td>
            <td><span class="ext-pill">{{ fileExt(file) }}</span></td>
            <td class="nowrap">{{ file.size_human || '-' }}</td>
            <td class="nowrap muted-cell">{{ file.modified_at || '-' }}</td>
            <td class="action-col">
              <div class="row-actions">
                <a :href="file.url" target="_blank" class="btn btn-xs btn-primary">查看</a>
                <button class="btn btn-xs btn-info" @click="copy(file.url)">复制</button>
                <button v-if="file.webp_url" class="btn btn-xs btn-default" @click="copy(file.webp_url)">WebP</button>
                <button class="btn btn-xs btn-danger" @click="remove(file)">删除</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
        </div>
        <div v-else class="empty-state">
          <i class="icon icon-inbox"></i>
          <strong>{{ data.files.length ? '没有匹配的文件' : '当前目录没有文件' }}</strong>
          <span>{{ data.files.length ? '换个关键词继续过滤当前目录。' : '可从左侧目录继续深入浏览。' }}</span>
        </div>
      </main>
    </div>
  </section>
  <div v-if="selectedImage" class="filer-lightbox" @click.self="selectedImage = null">
    <button class="lightbox-close" aria-label="关闭预览" @click="selectedImage = null">×</button>
    <img :src="selectedImage.url" :alt="displayName(selectedImage)">
  </div>
</template>

<style scoped>
.filer-loading {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 18px;
  border: 1px solid #dbeafe;
  border-radius: 14px;
  background: #eff6ff;
  color: #1d4ed8;
}

.loading-dot {
  width: 10px;
  height: 10px;
  border-radius: 999px;
  background: #3280fc;
  box-shadow: 0 0 0 6px rgba(50, 128, 252, 0.12);
}

.filer-shell {
  overflow: hidden;
  border: 1px solid #dbe3ef;
  border-radius: 16px;
  background: #f8fafc;
  box-shadow: 0 18px 44px rgba(15, 23, 42, 0.08);
}

.filer-header {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  padding: 18px 20px;
  border-bottom: 1px solid #dbe3ef;
  background:
    radial-gradient(circle at top left, rgba(50, 128, 252, 0.16), transparent 34%),
    linear-gradient(135deg, #ffffff, #f1f5f9);
}

.filer-kicker {
  color: #3280fc;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.filer-title-block h3 {
  margin: 4px 0;
  color: #0f172a;
  font-weight: 800;
}

.filer-title-block p {
  overflow: hidden;
  max-width: 720px;
  margin: 0;
  color: #64748b;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.filer-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(72px, 1fr));
  gap: 8px;
  min-width: 260px;
}

.filer-stats span {
  padding: 10px 12px;
  border: 1px solid rgba(148, 163, 184, 0.28);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.8);
  color: #64748b;
  text-align: center;
}

.filer-stats strong {
  display: block;
  color: #0f172a;
  font-size: 20px;
  line-height: 1.1;
}

.filer-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 14px;
  border-bottom: 1px solid #e2e8f0;
  background: #fff;
}

.filer-breadcrumb {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: center;
  gap: 6px;
  overflow-x: auto;
}

.crumb {
  position: relative;
  flex: 0 0 auto;
  padding: 5px 10px;
  border: 1px solid #dbe3ef;
  border-radius: 999px;
  background: #f8fafc;
  color: #334155;
  font-size: 12px;
  font-weight: 700;
}

.crumb:hover,
.crumb.active {
  border-color: #3280fc;
  background: #eff6ff;
  color: #1d4ed8;
}

.crumb.root {
  background: #0f172a;
  color: #fff;
}

.filer-actions {
  display: flex;
  flex: 0 0 auto;
  gap: 8px;
}

.filer-layout {
  display: grid;
  grid-template-columns: minmax(210px, 260px) minmax(0, 1fr);
  min-height: 460px;
}

.filer-sidebar {
  padding: 14px;
  border-right: 1px solid #e2e8f0;
  background: #f8fafc;
}

.section-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
  color: #475569;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.section-heading.inline {
  justify-content: flex-start;
  gap: 8px;
  margin-bottom: 2px;
}

.section-heading strong {
  padding: 2px 8px;
  border-radius: 999px;
  background: #e2e8f0;
  color: #0f172a;
  letter-spacing: 0;
}

.dir-list {
  display: grid;
  gap: 7px;
}

.dir-row {
  display: grid;
  grid-template-columns: 30px minmax(0, 1fr);
  align-items: center;
  width: 100%;
  padding: 8px;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  background: #fff;
  color: #0f172a;
  text-align: left;
  transition: border-color 0.15s ease, transform 0.15s ease, box-shadow 0.15s ease;
}

.dir-row:hover {
  border-color: #93c5fd;
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.08);
  transform: translateY(-1px);
}

.folder-icon {
  display: grid;
  width: 26px;
  height: 26px;
  place-items: center;
  border-radius: 8px;
  background: #fef3c7;
  color: #b45309;
}

.dir-name,
.file-info-cell strong,
.stored-name,
.file-info-cell code {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.filer-main {
  min-width: 0;
  padding: 14px;
  background: #fff;
}

.file-table-top {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
  margin-bottom: 12px;
}

.table-hint {
  margin: 0;
  color: #64748b;
  font-size: 12px;
}

.filer-search {
  display: flex;
  flex: 0 1 360px;
  gap: 8px;
}

.file-table-wrap {
  overflow-x: auto;
  border: 1px solid #e2e8f0;
  border-radius: 14px;
}

.file-table {
  min-width: 860px;
  margin-bottom: 0;
  color: #1e293b;
  font-size: 13px;
}

.file-table > thead > tr > th {
  border-bottom: 1px solid #dbe3ef;
  background: #f8fafc;
  color: #64748b;
  font-size: 11px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.file-table > tbody > tr > td {
  vertical-align: middle;
}

.thumb-col {
  width: 74px;
}

.thumb-button {
  display: block;
  width: 54px;
  height: 54px;
  padding: 0;
  overflow: hidden;
  border: 1px solid #dbe3ef;
  border-radius: 12px;
  background: #f1f5f9;
}

.file-thumb,
.thumb-fallback {
  display: grid;
  width: 100%;
  height: 100%;
  place-items: center;
  object-fit: cover;
  color: #64748b;
}

.file-info-cell {
  max-width: 360px;
}

.file-info-cell strong,
.stored-name,
.file-info-cell code,
.webp-link {
  display: block;
}

.file-info-cell strong {
  color: #0f172a;
  font-weight: 800;
}

.stored-name {
  margin-top: 2px;
  color: #64748b;
  font-size: 12px;
}

.file-info-cell code {
  max-width: 100%;
  margin-top: 4px;
  padding: 2px 6px;
  border-radius: 6px;
  background: #f1f5f9;
  color: #475569;
  font-size: 11px;
}

.webp-link {
  margin-top: 3px;
  color: #0f766e;
  font-size: 12px;
}

.ext-pill {
  display: inline-flex;
  min-width: 42px;
  justify-content: center;
  padding: 3px 8px;
  border-radius: 999px;
  background: #eef2ff;
  color: #3730a3;
  font-size: 11px;
  font-weight: 800;
}

.nowrap {
  white-space: nowrap;
}

.muted-cell {
  color: #64748b;
}

.action-col {
  width: 190px;
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
}

.empty-compact,
.empty-state {
  border: 1px dashed #cbd5e1;
  border-radius: 12px;
  background: #fff;
  color: #64748b;
}

.empty-compact {
  padding: 14px;
  text-align: center;
}

.empty-state {
  display: grid;
  min-height: 220px;
  place-items: center;
  align-content: center;
  gap: 6px;
  padding: 28px;
  text-align: center;
}

.empty-state i {
  color: #94a3b8;
  font-size: 30px;
}

.empty-state strong {
  color: #334155;
}

.filer-lightbox {
  position: fixed;
  inset: 0;
  z-index: 1600;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgba(15, 23, 42, 0.86);
}

.filer-lightbox img {
  max-width: 100%;
  max-height: 90vh;
  border-radius: 14px;
  box-shadow: 0 24px 80px rgba(0, 0, 0, 0.36);
}

.lightbox-close {
  position: fixed;
  top: 18px;
  right: 18px;
  width: 38px;
  height: 38px;
  border: 0;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.92);
  color: #0f172a;
  font-size: 24px;
  line-height: 1;
}

@media (max-width: 900px) {
  .filer-header,
  .filer-toolbar,
  .file-table-top {
    flex-direction: column;
    align-items: stretch;
  }

  .filer-stats {
    min-width: 0;
  }

  .filer-layout {
    grid-template-columns: 1fr;
  }

  .filer-sidebar {
    border-right: 0;
    border-bottom: 1px solid #e2e8f0;
  }

  .dir-list {
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  }

  .filer-search {
    flex: 1;
  }
}

@media (max-width: 560px) {
  .filer-stats {
    grid-template-columns: 1fr;
  }

  .filer-actions,
  .filer-search {
    flex-direction: column;
  }
}
</style>
