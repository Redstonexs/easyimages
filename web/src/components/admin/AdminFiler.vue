<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { AdminFileEntry, AdminFiler } from '../../types'
import type { NoticeType } from '../../shared/notify'
import { adminApi } from '../../shared/adminApi'
import { copyText } from '../../shared/clipboard'

const emit = defineEmits<{ notice: [message: string, type?: NoticeType] }>()
const loading = ref(true)
const data = ref<AdminFiler | null>(null)
const failedThumbs = ref<Set<string>>(new Set())

async function load(path?: string) {
  loading.value = true
  const params = new URLSearchParams()
  if (path) params.set('path', path)
  try { data.value = await adminApi.filer(params) } finally { loading.value = false }
}

async function copy(url: string) { await copyText(url); emit('notice', '复制成功', 'success') }

async function remove(file: AdminFileEntry) {
  if (!window.confirm('确认删除此文件？')) return
  const result = await adminApi.deleteFile(file.path)
  if (result.code === 200 && data.value) {
    data.value.files = data.value.files.filter(item => item.url !== file.url)
    emit('notice', result.msg || '删除成功', 'success')
  } else {
    emit('notice', result.msg || '删除失败', 'danger')
  }
}

function markFailed(url: string) { failedThumbs.value = new Set([...failedThumbs.value, url]) }
onMounted(() => load())
</script>

<template>
  <div v-if="loading" class="alert alert-info">正在加载文件...</div>
  <section v-else-if="data" class="panel panel-default">
    <div class="panel-heading"><h3 class="panel-title">文件管理</h3></div>
    <div class="panel-body">
      <ol class="breadcrumb">
        <li><button class="btn-link" @click="load('/i/')">根目录</button></li>
        <li v-if="data.parent_path"><button class="btn-link" @click="load(data.parent_path)">上级</button></li>
        <li class="active">{{ data.path }}</li>
      </ol>

      <h4>子目录</h4>
      <div v-if="data.dirs.length" class="dir-grid">
        <button v-for="dir in data.dirs" :key="dir" class="btn btn-default" @click="load(data.path + dir)"><i class="icon icon-folder-close text-warning"></i> {{ dir }}</button>
      </div>
      <div v-else class="text-muted">当前目录没有子目录</div>

      <h4>文件</h4>
      <table v-if="data.files.length" class="table table-hover table-striped">
        <thead><tr><th>文件名</th><th>预览</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="file in data.files" :key="file.url">
            <td><i class="icon icon-file-image-o text-primary"></i> {{ file.name }}</td>
            <td>
              <span v-if="failedThumbs.has(file.url)" class="text-muted">无预览</span>
              <img v-else :src="file.thumb_url" class="file-thumb" :alt="file.name" loading="lazy" decoding="async" @error="markFailed(file.url)">
            </td>
            <td>
              <a :href="file.url" target="_blank" class="btn btn-xs btn-primary">查看</a>
              <button class="btn btn-xs btn-info" @click="copy(file.url)">复制</button>
              <button class="btn btn-xs btn-danger" @click="remove(file)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else class="alert alert-info">当前目录没有文件</div>
    </div>
  </section>
</template>

<style scoped>
.dir-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 8px; margin-bottom: 20px; }
.file-thumb { max-width: 100px; max-height: 60px; object-fit: cover; border-radius: 4px; }
.btn-link { border: 0; background: transparent; color: #3280fc; padding: 0; }
</style>
