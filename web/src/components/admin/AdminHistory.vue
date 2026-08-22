<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import type { GalleryBootstrap, GalleryFile } from '../../types'
import type { NoticeType } from '../../shared/notify'
import { adminApi } from '../../shared/adminApi'
import { runBatchDelete, summarizeBatchDelete } from '../../shared/batchDelete'
import { useCopy } from '../../shared/useCopy'

const emit = defineEmits<{ notice: [message: string, type?: NoticeType] }>()

const loading = ref(true)
const gallery = ref<GalleryBootstrap | null>(null)
const selectedImage = ref<GalleryFile | null>(null)
const failedThumbs = ref<Set<string>>(new Set())
const queryInput = ref('')
const selected = ref<Set<string>>(new Set())

const notify = (message: string, type?: NoticeType) => emit('notice', message, type)
const copy = useCopy(notify)
// Filter buttons stay enabled while loading, so overlapping requests are easy to
// trigger; cancel the previous one rather than letting it land last.
let inflight: AbortController | null = null

const activeDate = computed(() => gallery.value?.date || '')
const activeExt = computed(() => gallery.value?.ext || gallery.value?.search || '')
const files = computed(() => gallery.value?.files || [])
const selectedFiles = computed(() => files.value.filter(file => selected.value.has(file.url)))
const allSelected = computed(() => files.value.length > 0 && selectedFiles.value.length === files.value.length)

function toggleFile(file: GalleryFile) {
  const next = new Set(selected.value)
  if (next.has(file.url)) next.delete(file.url)
  else next.add(file.url)
  selected.value = next
}

function toggleAll() {
  selected.value = allSelected.value ? new Set() : new Set(files.value.map(file => file.url))
}

async function removeSelected() {
  const targets = selectedFiles.value
  if (targets.length === 0) return
  if (!window.confirm(`确认删除选中的 ${targets.length} 个图片？此操作不可恢复。`)) return

  const byUrl = new Map(targets.map(file => [file.url, file]))
  const outcome = await runBatchDelete(
    targets.map(file => file.url),
    async url => adminApi.deleteHistory(byUrl.get(url)!.path)
  )
  const deleted = new Set(outcome.succeeded)
  if (gallery.value) gallery.value.files = gallery.value.files.filter(item => !deleted.has(item.url))
  selected.value = new Set([...selected.value].filter(url => !deleted.has(url)))
  const summary = summarizeBatchDelete(outcome)
  notify(summary.message, summary.type)
}

function copySelected() {
  void copy(selectedFiles.value.map(file => file.url).join('\n'), `已复制 ${selectedFiles.value.length} 条链接`)
}

function query(next: Partial<{ date: string; ext: string; q: string }>) {
  const params = new URLSearchParams()
  const date = next.date ?? gallery.value?.date
  const ext = next.ext ?? (gallery.value?.ext || gallery.value?.search)
  const q = next.q ?? gallery.value?.q
  if (date && date !== gallery.value?.today) params.set('date', date)
  if (ext) params.set('ext', ext)
  if (q) params.set('q', q)
  return params
}

async function load(next: Partial<{ date: string; ext: string; q: string }> = {}) {
  inflight?.abort()
  const controller = new AbortController()
  inflight = controller

  loading.value = true
  const typedQuery = queryInput.value
  try {
    gallery.value = await adminApi.history(query(next), { signal: controller.signal })
    selected.value = new Set()
    if (queryInput.value === typedQuery) queryInput.value = gallery.value.q || ''
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') return
    notify(error instanceof Error ? error.message : '加载失败', 'danger')
  } finally {
    if (inflight === controller) {
      inflight = null
      loading.value = false
    }
  }
}

async function remove(file: GalleryFile) {
  if (!window.confirm('确认删除此图片？')) return
  try {
    const result = await adminApi.deleteHistory(file.path)
    if (result.code === 200 && gallery.value) {
      gallery.value.files = gallery.value.files.filter(item => item.url !== file.url)
      notify(result.msg, 'success')
    } else {
      notify(result.msg || '删除失败', 'danger')
    }
  } catch (error) {
    notify(error instanceof Error ? error.message : '删除失败', 'danger')
  }
}

function markFailed(url: string) { failedThumbs.value = new Set([...failedThumbs.value, url]) }

function displayName(file: GalleryFile) { return file.original_name || file.name }

function submitSearch() { void load({ q: queryInput.value.trim() }) }

function clearSearch() {
  queryInput.value = ''
  void load({ q: '' })
}

onMounted(() => load())
onUnmounted(() => inflight?.abort())
</script>

<template>
  <div v-if="loading" class="alert alert-info">正在加载历史图片...</div>
  <template v-else-if="gallery">
    <div class="admin-toolbar">
      <div class="btn-group">
        <button class="btn" :class="activeDate === gallery.today ? 'btn-primary' : 'btn-default'" @click="load({ date: gallery.today })">今日</button>
        <button class="btn" :class="activeDate === gallery.yesterday ? 'btn-primary' : 'btn-default'" @click="load({ date: gallery.yesterday })">昨日</button>
        <button v-for="link in gallery.date_links" :key="link.date" class="btn btn-default hidden-xs" @click="load({ date: link.date })">{{ link.label }}</button>
      </div>
      <div class="btn-group">
        <button class="btn btn-sm" :class="activeExt === '' ? 'btn-info' : 'btn-default'" @click="load({ ext: '' })">全部</button>
        <button v-for="ext in gallery.extensions" :key="ext" class="btn btn-sm" :class="activeExt === ext ? 'btn-info' : 'btn-default'" @click="load({ ext })">{{ ext.toUpperCase() }}</button>
      </div>
      <form class="admin-search" role="search" @submit.prevent="submitSearch">
        <input v-model="queryInput" type="search" class="form-control" placeholder="搜索原文件名或存储文件名">
        <button type="submit" class="btn btn-primary">搜索</button>
        <button v-if="gallery.q" type="button" class="btn btn-default" @click="clearSearch">清空</button>
      </form>
    </div>

    <div v-if="gallery.files.length" class="batch-bar">
      <label class="batch-select-all">
        <input type="checkbox" :checked="allSelected" @change="toggleAll">
        全选（{{ gallery.files.length }}）
      </label>
      <template v-if="selectedFiles.length">
        <span class="batch-count">已选 {{ selectedFiles.length }} 个</span>
        <button type="button" class="btn btn-xs btn-primary" @click="copySelected">复制链接</button>
        <button type="button" class="btn btn-xs btn-danger" @click="removeSelected">批量删除</button>
      </template>
    </div>

    <section v-if="gallery.files.length" class="admin-gallery">
      <article v-for="file in gallery.files" :key="file.url" class="admin-card" :class="{ 'is-selected': selected.has(file.url) }">
        <label class="admin-card-select" @click.stop>
          <input type="checkbox" :checked="selected.has(file.url)" @change="toggleFile(file)">
        </label>
        <button class="image-button" @click="selectedImage = file">
          <span v-if="failedThumbs.has(file.url)" class="thumb-fallback">{{ displayName(file) }}</span>
          <img v-else :src="file.thumb_url" :alt="displayName(file)" loading="lazy" decoding="async" fetchpriority="low" @error="markFailed(file.url)">
        </button>
        <div class="admin-card-meta">
          <strong :title="displayName(file)">{{ displayName(file) }}</strong>
          <small v-if="file.original_name" :title="file.name">{{ file.name }}</small>
        </div>
        <div class="admin-card-actions">
          <button class="btn btn-xs btn-primary" @click="copy(file.url)">复制</button>
          <a class="btn btn-xs btn-info" :href="file.info_url" target="_blank">详情</a>
          <button class="btn btn-xs btn-danger" @click="remove(file)">删除</button>
        </div>
      </article>
    </section>
    <div v-else class="alert alert-info">{{ gallery.q ? '没有匹配的图片。' : '该日期还没有上传的图片。' }}</div>
  </template>
  <div v-if="selectedImage" class="lightbox" @click.self="selectedImage = null"><img :src="selectedImage.url" :alt="displayName(selectedImage)"></div>
</template>

<style scoped>
.batch-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  padding: 10px 14px;
  border: 1px solid #dbe6f8;
  border-radius: 8px;
  background: #f2f6fd;
}
.batch-select-all {
  margin: 0;
  font-weight: 700;
  color: #35507a;
  cursor: pointer;
}
.batch-select-all input {
  margin-right: 6px;
  vertical-align: middle;
}
.batch-count {
  color: #35507a;
}
.admin-card {
  position: relative;
}
.admin-card.is-selected {
  outline: 2px solid #3280fc;
  outline-offset: 2px;
}
.admin-card-select {
  position: absolute;
  top: 6px;
  left: 6px;
  z-index: 2;
  margin: 0;
  padding: 3px 5px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
  cursor: pointer;
}
.admin-card-select input {
  margin: 0;
  vertical-align: middle;
  cursor: pointer;
}
.admin-search {
  display: flex;
  flex: 1 1 320px;
  gap: 8px;
}

.admin-card-meta {
  padding: 8px 10px 42px;
  background: #fff;
}

.admin-card-meta strong,
.admin-card-meta small {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.admin-card-meta small {
  margin-top: 3px;
  color: #777;
}

@media (max-width: 640px) {
  .admin-search { flex-direction: column; }
}
</style>
