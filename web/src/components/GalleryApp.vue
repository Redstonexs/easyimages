<script setup lang="ts">
import { computed, ref } from 'vue'
import type { GalleryBootstrap, GalleryFile } from '../types'
import type { Notice, NoticeType } from '../shared/notify'
import { copyText } from '../shared/clipboard'
import { fetchJSON } from '../shared/api'
import { createNotice } from '../shared/notify'
import NoticeStack from './NoticeStack.vue'

const props = defineProps<{
  bootstrap: GalleryBootstrap
}>()

const gallery = ref<GalleryBootstrap>({ ...props.bootstrap })
const loading = ref(false)
const selectedImage = ref<GalleryFile | null>(null)
const notices = ref<Notice[]>([])
const failedThumbs = ref<Set<string>>(new Set())
const queryInput = ref(props.bootstrap.q || '')

const activeDate = computed(() => gallery.value.date)
const activeExt = computed(() => gallery.value.ext || gallery.value.search)

function notify(message: string, type: NoticeType = 'info') {
  const notice = createNotice(message, type)
  notices.value.push(notice)
  window.setTimeout(() => {
    notices.value = notices.value.filter(item => item.id !== notice.id)
  }, 3200)
}

function buildQuery(next: Partial<{ date: string; ext: string; q: string; num: number }>) {
  const params = new URLSearchParams()
  const date = next.date ?? gallery.value.date
  const ext = next.ext ?? (gallery.value.ext || gallery.value.search)
  const q = next.q ?? gallery.value.q
  const num = next.num ?? gallery.value.limit
  if (date && date !== gallery.value.today) params.set('date', date)
  if (ext) params.set('ext', ext)
  if (q) params.set('q', q)
  if (num !== gallery.value.limit) params.set('num', String(num))
  return params
}

async function loadGallery(next: Partial<{ date: string; ext: string; q: string; num: number }>) {
  const pageParams = buildQuery(next)
  const apiParams = new URLSearchParams(pageParams)
  apiParams.set('num', String(next.num ?? gallery.value.limit))

  loading.value = true
  try {
    const nextGallery = await fetchJSON<GalleryBootstrap>(`/api/list?${apiParams.toString()}`)
    gallery.value = nextGallery
    queryInput.value = nextGallery.q || ''
    const pageQuery = pageParams.toString()
    window.history.pushState(null, '', pageQuery ? `/app/list?${pageQuery}` : '/app/list')
  } catch (error) {
    notify(error instanceof Error ? error.message : '加载失败', 'danger')
  } finally {
    loading.value = false
  }
}

function submitSearch() {
  void loadGallery({ q: queryInput.value.trim() })
}

function clearSearch() {
  queryInput.value = ''
  void loadGallery({ q: '' })
}

async function copy(url: string) {
  try {
    await copyText(url)
    notify('复制成功', 'success')
  } catch {
    notify('复制失败，请手动复制', 'danger')
  }
}

function openImage(file: GalleryFile) {
  selectedImage.value = file
}

function closeImage() {
  selectedImage.value = null
}

function markThumbFailed(url: string) {
  failedThumbs.value = new Set([...failedThumbs.value, url])
}

function displayName(file: GalleryFile) {
  return file.original_name || file.name
}
</script>

<template>
  <NoticeStack :notices="notices" />
  <main class="gallery-shell">
    <header class="gallery-title">
      <h1>图床广场</h1>
      <p class="text-muted">当前日期上传: {{ gallery.total }} 张</p>
    </header>

    <section class="gallery-toolbar" aria-label="图库筛选">
      <div class="btn-group">
        <button type="button" class="btn" :class="activeDate === gallery.today ? 'btn-primary' : 'btn-default'" :disabled="loading" @click="loadGallery({ date: gallery.today })">今日</button>
        <button type="button" class="btn" :class="activeDate === gallery.yesterday ? 'btn-primary' : 'btn-default'" :disabled="loading" @click="loadGallery({ date: gallery.yesterday })">昨日</button>
        <button
          v-for="link in gallery.date_links"
          :key="link.date"
          type="button"
          class="btn hidden-xs"
          :class="activeDate === link.date ? 'btn-primary' : 'btn-default'"
          :disabled="loading"
          @click="loadGallery({ date: link.date })"
        >
          {{ link.label }}
        </button>
      </div>
      <div class="btn-group">
        <button type="button" class="btn btn-sm" :class="activeExt === '' ? 'btn-info' : 'btn-default'" :disabled="loading" @click="loadGallery({ ext: '' })">全部</button>
        <button
          v-for="extension in gallery.extensions"
          :key="extension"
          type="button"
          class="btn btn-sm"
          :class="activeExt === extension ? 'btn-info' : 'btn-default'"
          :disabled="loading"
          @click="loadGallery({ ext: extension })"
        >
          {{ extension.toUpperCase() }}
        </button>
      </div>
      <form class="gallery-search" role="search" @submit.prevent="submitSearch">
        <input v-model="queryInput" type="search" class="form-control" placeholder="搜索原文件名或存储文件名" :disabled="loading">
        <button type="submit" class="btn btn-primary" :disabled="loading">搜索</button>
        <button v-if="gallery.q" type="button" class="btn btn-default" :disabled="loading" @click="clearSearch">清空</button>
      </form>
    </section>

    <div v-if="loading" class="gallery-loading">正在加载...</div>
    <section v-if="gallery.files.length" class="gallery-grid" aria-live="polite">
      <article v-for="file in gallery.files" :key="file.url" class="gallery-card">
        <button type="button" class="image-button" @click="openImage(file)">
          <span v-if="failedThumbs.has(file.url)" class="thumb-fallback">
            <i class="icon icon-picture" aria-hidden="true"></i>
            <span>{{ displayName(file) }}</span>
          </span>
          <img
            v-else
            :src="file.thumb_url"
            :alt="displayName(file)"
            width="320"
            height="240"
            loading="lazy"
            decoding="async"
            fetchpriority="low"
            @error="markThumbFailed(file.url)"
          >
        </button>
        <div class="gallery-card-meta">
          <strong :title="displayName(file)">{{ displayName(file) }}</strong>
          <small v-if="file.original_name" :title="file.name">{{ file.name }}</small>
        </div>
        <div class="gallery-card-actions">
          <a :href="file.url" target="_blank" rel="noopener" title="打开"><i class="icon icon-picture"></i></a>
          <button type="button" title="复制链接" @click="copy(file.url)"><i class="icon icon-copy"></i></button>
          <a :href="file.info_url" target="_blank" rel="noopener" title="详细信息"><i class="icon icon-info-sign"></i></a>
          <a :href="file.down_url" target="_blank" rel="noopener" title="下载"><i class="icon icon-cloud-download"></i></a>
        </div>
      </article>
    </section>
    <section v-else-if="!loading" class="alert alert-info">{{ gallery.q ? '没有匹配的图片。' : '该日期还没有上传的图片。' }}</section>

    <footer class="gallery-footer">
      <a href="/" class="btn btn-primary"><i class="icon icon-upload"></i> 上传图片</a>
    </footer>
  </main>

  <div v-if="selectedImage" class="lightbox" @click.self="closeImage">
    <figure>
      <img :src="selectedImage.url" :alt="displayName(selectedImage)">
      <figcaption>
        <span>{{ displayName(selectedImage) }}</span>
        <button type="button" class="btn btn-primary btn-sm" @click="copy(selectedImage.url)">复制链接</button>
        <button type="button" class="btn btn-default btn-sm" @click="closeImage">关闭</button>
      </figcaption>
    </figure>
  </div>
</template>

<style scoped>
.gallery-shell {
  max-width: 1180px;
  margin: 28px auto 40px;
  padding: 0 16px;
}

.gallery-title {
  text-align: center;
  margin-bottom: 22px;
}

.gallery-title h1 {
  font-weight: 700;
}

.gallery-toolbar {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  padding: 14px;
  margin-bottom: 18px;
  border: 1px solid #e8eef6;
  border-radius: 12px;
  background: #fff;
  box-shadow: 0 12px 30px rgba(15, 23, 42, 0.06);
}

.gallery-search {
  display: flex;
  flex: 1 1 320px;
  gap: 8px;
}

.gallery-search .form-control {
  min-width: 180px;
}

.gallery-loading {
  margin-bottom: 14px;
  color: #666;
}

.gallery-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 18px;
}

.gallery-card {
  position: relative;
  overflow: hidden;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 16px 40px rgba(15, 23, 42, 0.09);
}

.gallery-card-meta {
  padding: 10px 12px 42px;
  background: #fff;
}

.gallery-card-meta strong,
.gallery-card-meta small {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gallery-card-meta small {
  margin-top: 4px;
  color: #718096;
}

.image-button {
  display: block;
  width: 100%;
  padding: 0;
  border: 0;
  background: transparent;
}

.image-button img {
  display: block;
  width: 100%;
  aspect-ratio: 4 / 3;
  object-fit: cover;
  transition: transform 0.25s ease;
}

.thumb-fallback {
  display: grid;
  width: 100%;
  aspect-ratio: 4 / 3;
  place-items: center;
  gap: 8px;
  padding: 18px;
  color: #718096;
  background: linear-gradient(135deg, #f8fafc, #e8eef6);
  word-break: break-all;
}

.thumb-fallback .icon {
  font-size: 28px;
}

.gallery-card:hover img {
  transform: scale(1.04);
}

.gallery-card-actions {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  justify-content: center;
  gap: 14px;
  padding: 10px;
  background: linear-gradient(to top, rgba(15, 23, 42, 0.78), rgba(15, 23, 42, 0));
  opacity: 0;
  transition: opacity 0.2s ease;
}

.gallery-card:hover .gallery-card-actions,
.gallery-card:focus-within .gallery-card-actions {
  opacity: 1;
}

.gallery-card-actions a,
.gallery-card-actions button {
  color: #fff;
  border: 0;
  background: transparent;
  line-height: 1;
}

.gallery-footer {
  margin: 32px 0;
  text-align: center;
}

.lightbox {
  position: fixed;
  inset: 0;
  z-index: 1600;
  display: grid;
  place-items: center;
  padding: 18px;
  background: rgba(15, 23, 42, 0.86);
}

.lightbox figure {
  max-width: min(100%, 1100px);
  max-height: 92vh;
  margin: 0;
}

.lightbox img {
  display: block;
  max-width: 100%;
  max-height: 82vh;
  margin: 0 auto;
  border-radius: 10px;
  object-fit: contain;
}

.lightbox figcaption {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-top: 12px;
  color: #fff;
}

@media (max-width: 640px) {
  .gallery-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .gallery-search {
    flex-direction: column;
  }

  .gallery-grid {
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 12px;
  }

  .gallery-card-actions {
    opacity: 1;
  }
}
</style>
