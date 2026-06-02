<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { GalleryBootstrap, GalleryFile } from '../../types'
import type { NoticeType } from '../../shared/notify'
import { adminApi } from '../../shared/adminApi'
import { copyText } from '../../shared/clipboard'

const emit = defineEmits<{ notice: [message: string, type?: NoticeType] }>()

const loading = ref(true)
const gallery = ref<GalleryBootstrap | null>(null)
const selectedImage = ref<GalleryFile | null>(null)
const failedThumbs = ref<Set<string>>(new Set())

const activeDate = computed(() => gallery.value?.date || '')
const activeSearch = computed(() => gallery.value?.search || '')

function query(next: Partial<{ date: string; search: string }>) {
  const params = new URLSearchParams()
  const date = next.date ?? gallery.value?.date
  const search = next.search ?? gallery.value?.search
  if (date && date !== gallery.value?.today) params.set('date', date)
  if (search) params.set('search', search)
  return params
}

async function load(next: Partial<{ date: string; search: string }> = {}) {
  loading.value = true
  try {
    gallery.value = await adminApi.history(query(next))
  } catch (error) {
    emit('notice', error instanceof Error ? error.message : '加载失败', 'danger')
  } finally {
    loading.value = false
  }
}

async function copy(url: string) {
  await copyText(url)
  emit('notice', '复制成功', 'success')
}

async function remove(file: GalleryFile) {
  if (!window.confirm('确认删除此图片？')) return
  const result = await adminApi.deleteHistory(file.path)
  if (result.code === 200 && gallery.value) {
    gallery.value.files = gallery.value.files.filter(item => item.url !== file.url)
    emit('notice', result.msg, 'success')
  } else {
    emit('notice', result.msg || '删除失败', 'danger')
  }
}

function markFailed(url: string) { failedThumbs.value = new Set([...failedThumbs.value, url]) }

onMounted(() => load())
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
        <button class="btn btn-sm" :class="activeSearch === '' ? 'btn-info' : 'btn-default'" @click="load({ search: '' })">全部</button>
        <button v-for="ext in gallery.extensions" :key="ext" class="btn btn-sm" :class="activeSearch === ext ? 'btn-info' : 'btn-default'" @click="load({ search: ext })">{{ ext.toUpperCase() }}</button>
      </div>
    </div>

    <section class="admin-gallery">
      <article v-for="file in gallery.files" :key="file.url" class="admin-card">
        <button class="image-button" @click="selectedImage = file">
          <span v-if="failedThumbs.has(file.url)" class="thumb-fallback">{{ file.name }}</span>
          <img v-else :src="file.thumb_url" :alt="file.name" loading="lazy" decoding="async" fetchpriority="low" @error="markFailed(file.url)">
        </button>
        <div class="admin-card-actions">
          <button class="btn btn-xs btn-primary" @click="copy(file.url)">复制</button>
          <a class="btn btn-xs btn-info" :href="file.info_url" target="_blank">详情</a>
          <button class="btn btn-xs btn-danger" @click="remove(file)">删除</button>
        </div>
      </article>
    </section>
  </template>
  <div v-if="selectedImage" class="lightbox" @click.self="selectedImage = null"><img :src="selectedImage.url" :alt="selectedImage.name"></div>
</template>
