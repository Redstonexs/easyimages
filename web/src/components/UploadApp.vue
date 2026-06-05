<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { CaptchaData, UploadBootstrap, UploadResult } from '../types'
import type { Notice, NoticeType } from '../shared/notify'
import { copyText } from '../shared/clipboard'
import { fetchJSON } from '../shared/api'
import { formatSize } from '../shared/format'
import { createNotice } from '../shared/notify'
import NoticeStack from './NoticeStack.vue'

const props = defineProps<{
  bootstrap: UploadBootstrap
}>()

interface ProgressItem {
  id: number
  name: string
  size: number
  loaded: number
  status: 'waiting' | 'uploading' | 'processing' | 'success' | 'error'
  message: string
}

const CHUNK_SIZE = 16 * 1024 * 1024
const MAX_CONCURRENT_CHUNKS = 4
const MAX_RETRIES = 3

const fileInput = ref<HTMLInputElement | null>(null)
const uploadArea = ref<HTMLElement | null>(null)
const isDragging = ref(false)
const isUploading = ref(false)
const progressItems = ref<ProgressItem[]>([])
const results = ref<UploadResult[]>([])
const uploadedTotal = ref(0)
const totalSize = ref(0)
const activeFileIndex = ref(0)
const notices = ref<Notice[]>([])
const selectedStorageSource = ref(props.bootstrap.config.default_storage_source || props.bootstrap.config.storage_sources[0]?.id || 'local')

const showLogin = ref(false)
const loginUser = ref('')
const loginPass = ref('')
const loginError = ref('')
const captcha = ref<CaptchaData>(props.bootstrap.captcha)
const captchaAnswer = ref('')
const externalCaptchaToken = ref('')
const turnstileRendered = ref(false)

const maxSizeLabel = computed(() => formatSize(props.bootstrap.config.max_size))
const overallPercent = computed(() => {
  if (totalSize.value === 0) return 0
  const percent = Math.round((uploadedTotal.value / totalSize.value) * 100)
  return isUploading.value && percent >= 100 ? 99 : percent
})
const overallInfo = computed(() => {
  if (progressItems.value.length === 0) return '准备上传...'
  if (!isUploading.value) {
    const success = progressItems.value.filter(item => item.status === 'success').length
    const failed = progressItems.value.filter(item => item.status === 'error').length
    if (failed === 0) return `全部完成：${success} 个文件上传成功`
    if (success === 0) return `上传失败：${failed} 个文件`
    return `${success} 成功, ${failed} 失败`
  }
  const processingItem = progressItems.value.find(item => item.status === 'processing')
  if (processingItem) return processingItem.message
  return `上传中 (${activeFileIndex.value}/${progressItems.value.length}): ${formatSize(uploadedTotal.value)} / ${formatSize(totalSize.value)}`
})

function notify(message: string, type: NoticeType = 'info') {
  const notice = createNotice(message, type)
  notices.value.push(notice)
  window.setTimeout(() => {
    notices.value = notices.value.filter(item => item.id !== notice.id)
  }, 3600)
}

function selectFiles() {
  fileInput.value?.click()
}

function handleFileInput(event: Event) {
  const input = event.target as HTMLInputElement
  if (input.files) void handleFiles(input.files)
}

function handleDrop(event: DragEvent) {
  event.preventDefault()
  isDragging.value = false
  if (event.dataTransfer?.files) void handleFiles(event.dataTransfer.files)
}

async function handleFiles(fileList: FileList) {
  if (fileList.length === 0 || isUploading.value) return

  const files: File[] = []
  for (const file of Array.from(fileList)) {
    if (file.size > props.bootstrap.config.max_size) {
      notify(`"${file.name}" 超过大小限制 (${maxSizeLabel.value})`, 'warning')
      continue
    }
    files.push(file)
  }
  if (files.length === 0) return

  isUploading.value = true
  uploadedTotal.value = 0
  totalSize.value = files.reduce((sum, file) => sum + file.size, 0)
  activeFileIndex.value = 0
  progressItems.value = files.map((file, index) => ({
    id: index,
    name: file.name,
    size: file.size,
    loaded: 0,
    status: 'waiting',
    message: formatSize(file.size)
  }))

  let successCount = 0
  let failCount = 0
  for (let index = 0; index < files.length; index++) {
    const file = files[index]
    const item = progressItems.value[index]
    const baseUploaded = uploadedTotal.value
    activeFileIndex.value = index
    item.status = 'uploading'

    try {
      const result = file.size > CHUNK_SIZE
        ? await uploadFileChunked(
          file,
          loaded => updateFileProgress(item, baseUploaded, loaded),
          message => markFileProcessing(item, baseUploaded, message)
        )
        : await uploadFileWhole(
          file,
          loaded => updateFileProgress(item, baseUploaded, loaded),
          message => markFileProcessing(item, baseUploaded, message)
        )
      item.status = 'success'
      item.loaded = file.size
      item.message = formatSize(file.size)
      uploadedTotal.value = baseUploaded + file.size
      successCount++
      if (result.url) results.value.push(result)
    } catch (error) {
      item.status = 'error'
      item.loaded = file.size
      item.message = error instanceof Error ? error.message : '上传失败'
      uploadedTotal.value = baseUploaded + file.size
      failCount++
    }
    activeFileIndex.value = index + 1
  }

  isUploading.value = false
  if (fileInput.value) fileInput.value.value = ''
  if (failCount === 0) notify('上传成功', 'success')
  else if (successCount > 0) notify(`${successCount} 个文件上传成功，${failCount} 个失败`, 'warning')
  else notify('上传失败', 'danger')
}

function updateFileProgress(item: ProgressItem, baseUploaded: number, loaded: number) {
  item.loaded = Math.min(loaded, item.size)
  item.message = `${formatSize(loaded)} / ${formatSize(item.size)}`
  uploadedTotal.value = baseUploaded + item.loaded
}

function markFileProcessing(item: ProgressItem, baseUploaded: number, message: string) {
  item.status = 'processing'
  item.loaded = item.size
  item.message = message
  uploadedTotal.value = baseUploaded + item.size
}

function progressPercent(item: ProgressItem) {
  if (item.size === 0) return 0
  const percent = Math.round((item.loaded / item.size) * 100)
  return item.status === 'success' ? percent : Math.min(percent, 99)
}

function resultName(result: UploadResult) {
  return result.original_name || result.srcName || ''
}

function uploadFileWhole(
  file: File,
  onProgress: (loaded: number) => void,
  onProcessing: (message: string) => void
): Promise<UploadResult> {
  return new Promise((resolve, reject) => {
    const formData = new FormData()
    formData.append('file', file)
    formData.append('storage_source', selectedStorageSource.value)
    const xhr = new XMLHttpRequest()
    xhr.upload.addEventListener('progress', event => {
      if (!event.lengthComputable) return
      onProgress(Math.min(event.loaded, file.size))
      if (event.loaded >= event.total) onProcessing('文件已传完，服务器处理中...')
    })
    xhr.addEventListener('load', () => resolveUpload(xhr, resolve, reject))
    xhr.addEventListener('error', () => reject(new Error('网络错误')))
    xhr.addEventListener('timeout', () => reject(new Error('上传超时')))
    xhr.timeout = 120000
    xhr.open('POST', '/app/upload')
    xhr.send(formData)
  })
}

async function uploadFileChunked(
  file: File,
  onProgress: (loaded: number) => void,
  onProcessing: (message: string) => void
): Promise<UploadResult> {
  const totalChunks = Math.ceil(file.size / CHUNK_SIZE)
  const uploadId = crypto.randomUUID ? crypto.randomUUID() : `upload-${Date.now()}-${Math.random().toString(16).slice(2)}`
  const chunkProgress = new Array<number>(totalChunks).fill(0)
  let nextChunk = 0
  let failed = false

  const updateChunkProgress = () => onProgress(chunkProgress.reduce((sum, value) => sum + value, 0))
  const workers = Array.from({ length: Math.min(MAX_CONCURRENT_CHUNKS, totalChunks) }, async () => {
    while (!failed) {
      const chunkIndex = nextChunk++
      if (chunkIndex >= totalChunks) break

      const start = chunkIndex * CHUNK_SIZE
      const end = Math.min(start + CHUNK_SIZE, file.size)
      let lastError: Error | null = null

      for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
        if (failed) break
        if (attempt > 0) await new Promise(resolve => window.setTimeout(resolve, 500 * 2 ** (attempt - 1)))

        const formData = new FormData()
        formData.append('chunk', file.slice(start, end), file.name)
        formData.append('uploadId', uploadId)
        formData.append('chunkIndex', chunkIndex.toString())
        formData.append('totalChunks', totalChunks.toString())
        formData.append('filename', file.name)
        formData.append('storage_source', selectedStorageSource.value)

        try {
          await sendChunk(formData, loaded => {
            chunkProgress[chunkIndex] = Math.min(loaded, end - start)
            updateChunkProgress()
          })
          chunkProgress[chunkIndex] = end - start
          updateChunkProgress()
          lastError = null
          break
        } catch (error) {
          lastError = error instanceof Error ? error : new Error('分片上传失败')
          chunkProgress[chunkIndex] = 0
          updateChunkProgress()
        }
      }

      if (lastError) {
        failed = true
        throw lastError
      }
    }
  })

  try {
    await Promise.all(workers)
  } catch (error) {
    const cleanup = new FormData()
    cleanup.append('uploadId', uploadId)
    cleanup.append('storage_source', selectedStorageSource.value)
    navigator.sendBeacon?.('/app/upload/chunk/cleanup', cleanup)
    throw error
  }

  const mergeData = new FormData()
  mergeData.append('uploadId', uploadId)
  mergeData.append('totalChunks', totalChunks.toString())
  mergeData.append('filename', file.name)
  mergeData.append('merge', 'true')
  mergeData.append('storage_source', selectedStorageSource.value)
  onProgress(file.size)
  onProcessing('文件已传完，服务器合并处理中...')
  return await sendChunk(mergeData, () => undefined, '分片合并失败')
}

function sendChunk(formData: FormData, onProgress: (loaded: number) => void, fallback = '分片上传失败'): Promise<UploadResult> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.upload.addEventListener('progress', event => {
      if (event.lengthComputable) onProgress(event.loaded)
    })
    xhr.addEventListener('load', () => resolveUpload(xhr, resolve, reject, fallback))
    xhr.addEventListener('error', () => reject(new Error('网络错误')))
    xhr.addEventListener('timeout', () => reject(new Error('分片上传超时')))
    xhr.timeout = 120000
    xhr.open('POST', '/app/upload/chunk')
    xhr.send(formData)
  })
}

function resolveUpload(
  xhr: XMLHttpRequest,
  resolve: (value: UploadResult) => void,
  reject: (reason: Error) => void,
  fallback = '服务器拒绝'
) {
  if (xhr.status >= 200 && xhr.status < 300) {
    try {
      const data = JSON.parse(xhr.responseText) as UploadResult
      if (data.result === 'success' || data.code === 200 || data.url) resolve(data)
      else reject(new Error(data.message || fallback))
    } catch {
      reject(new Error('响应解析错误'))
    }
    return
  }

  let message = `HTTP ${xhr.status}`
  try {
    const data = JSON.parse(xhr.responseText) as UploadResult
    message = data.message || message
  } catch {
    // Keep status fallback.
  }
  reject(new Error(message))
}

async function copy(value: string) {
  try {
    await copyText(value)
    notify('复制成功', 'success')
  } catch {
    notify('复制失败，请手动复制', 'danger')
  }
}

function openAdmin() {
  if (props.bootstrap.is_admin) {
    window.location.href = '/admin/manager'
    return
  }
  showLogin.value = true
  loginError.value = ''
  window.setTimeout(renderCaptcha, 0)
}

async function refreshCaptcha() {
  captcha.value = await fetchJSON<CaptchaData>('/api/captcha')
  captchaAnswer.value = ''
}

function renderCaptcha() {
  if (captcha.value.type === 'turnstile' && captcha.value.site_key) {
    const render = () => {
      if (!window.turnstile) {
        window.setTimeout(render, 100)
        return
      }
      if (turnstileRendered.value) {
        window.turnstile.reset('#login-turnstile-container')
        return
      }
      window.turnstile.render('#login-turnstile-container', {
        sitekey: captcha.value.site_key || '',
        callback: token => {
          externalCaptchaToken.value = token
        }
      })
      turnstileRendered.value = true
    }
    render()
  } else if (captcha.value.type === 'recaptcha' && captcha.value.site_key) {
    const execute = () => {
      if (!window.grecaptcha?.ready) {
        window.setTimeout(execute, 100)
        return
      }
      window.grecaptcha.ready(() => {
        void window.grecaptcha?.execute(captcha.value.site_key || '', { action: 'login' }).then(token => {
          externalCaptchaToken.value = token
        })
      })
    }
    execute()
  } else if (captcha.value.type === 'builtin') {
    void refreshCaptcha().catch(() => undefined)
  }
}

async function login() {
  loginError.value = ''
  if (!loginUser.value || !loginPass.value) {
    loginError.value = '请输入用户名和密码'
    return
  }

  const body = new URLSearchParams()
  body.set('user', loginUser.value)
  body.set('password', loginPass.value)
  if (captcha.value.type === 'builtin') {
    body.set('captcha_answer', captchaAnswer.value)
    body.set('captcha_token', captcha.value.token || '')
  } else if (captcha.value.type === 'turnstile') {
    body.set('cf_turnstile_response', externalCaptchaToken.value)
  } else if (captcha.value.type === 'recaptcha') {
    body.set('g_recaptcha_response', externalCaptchaToken.value)
  }

  try {
    const result = await fetchJSON<{ result: string; message?: string }>('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body
    })
    if (result.result === 'success') window.location.href = '/admin/manager'
    else loginError.value = result.message || '登录失败'
  } catch (error) {
    loginError.value = error instanceof Error ? error.message : '登录失败'
    if (captcha.value.type === 'builtin') void refreshCaptcha().catch(() => undefined)
  }
}

onMounted(() => {
  if (props.bootstrap.must_login === 1 && captcha.value.type === 'builtin') {
    void refreshCaptcha().catch(() => undefined)
  }
})
</script>

<template>
  <NoticeStack :notices="notices" />
  <main class="ei-shell upload-shell">
    <header class="ei-title">
      <h1>{{ bootstrap.config.title }}</h1>
      <p>{{ bootstrap.config.description }}</p>
    </header>

    <section
      ref="uploadArea"
      class="upload-area"
      :class="{ dragover: isDragging, uploading: isUploading }"
      role="button"
      tabindex="0"
      @click="selectFiles"
      @keydown.enter.prevent="selectFiles"
      @keydown.space.prevent="selectFiles"
      @dragover.prevent="isDragging = !isUploading"
      @dragleave="isDragging = false"
      @drop="handleDrop"
    >
      <i class="icon icon-cloud-upload icon-3x" aria-hidden="true"></i>
      <h3>拖拽文件到此处或点击选择</h3>
      <p class="text-muted">支持 jpg, png, gif, webp 等格式，单文件最大 {{ maxSizeLabel }}</p>
      <input ref="fileInput" type="file" multiple accept="image/*" class="sr-only" @change="handleFileInput">
      <button type="button" class="btn btn-primary btn-lg upload-btn" :disabled="isUploading" @click.stop="selectFiles">
        <i class="icon icon-upload" aria-hidden="true"></i> 选择文件
      </button>
    </section>

    <section v-if="bootstrap.config.storage_sources.length > 1" class="storage-source-panel">
      <label for="storage-source">存储位置</label>
      <select id="storage-source" v-model="selectedStorageSource" class="form-control" :disabled="isUploading">
        <option v-for="source in bootstrap.config.storage_sources" :key="source.id" :value="source.id">
          {{ source.name }} ({{ source.type === 's3' ? 'S3' : '本地' }})
        </option>
      </select>
      <p class="text-muted">选择 S3 源时，文件会直接写入对应存储桶，不保存本地原图。</p>
    </section>

    <section v-if="progressItems.length" class="upload-progress-container active" aria-live="polite">
      <div class="file-progress-list">
        <article v-for="item in progressItems" :key="item.id" class="file-progress-item">
          <div class="file-progress-header">
            <span class="file-progress-name" :title="item.name">
              <span class="upload-status-icon" :class="`status-${item.status}`"></span>
              {{ item.name }}
            </span>
            <span class="file-progress-info">{{ item.message }}</span>
          </div>
          <div class="progress">
            <div class="progress-bar" :class="item.status === 'error' ? 'progress-bar-danger' : item.status === 'success' ? 'progress-bar-success' : item.status === 'processing' ? 'progress-bar-warning' : 'progress-bar-info'" :style="{ width: `${progressPercent(item)}%` }"></div>
          </div>
        </article>
      </div>
      <div class="overall-progress">
        <div class="overall-stats">
          <span>{{ overallInfo }}</span>
          <span class="percentage">{{ overallPercent }}%</span>
        </div>
        <div class="progress">
          <div class="progress-bar" :class="progressItems.some(item => item.status === 'error') && !isUploading ? 'progress-bar-danger' : progressItems.some(item => item.status === 'processing') ? 'progress-bar-warning' : 'progress-bar-info'" :style="{ width: `${overallPercent}%` }"></div>
        </div>
      </div>
    </section>

    <section v-if="results.length" class="result-area">
      <h3>上传结果</h3>
      <article v-for="(result, index) in results" :key="`${result.url}-${index}`" class="link-box">
        <div class="row">
          <div class="col-md-3">
            <img :src="result.thumb || result.url" class="preview-img" alt="preview" loading="lazy">
          </div>
          <div class="col-md-9 result-links">
            <label>源文件名</label>
            <div class="result-name" :title="resultName(result)">{{ resultName(result) }}</div>
            <label>直链</label>
            <div class="input-group">
              <input type="text" class="form-control" :value="result.url" readonly>
              <span class="input-group-btn"><button type="button" class="btn btn-primary" @click="copy(result.url || '')">复制</button></span>
            </div>
            <label>Markdown</label>
            <div class="input-group">
              <input type="text" class="form-control" :value="`![${resultName(result)}](${result.url})`" readonly>
              <span class="input-group-btn"><button type="button" class="btn btn-primary" @click="copy(`![${resultName(result)}](${result.url})`)">复制</button></span>
            </div>
            <label>HTML</label>
            <div class="input-group">
              <input type="text" class="form-control" :value="`<img src=&quot;${result.url}&quot; />`" readonly>
              <span class="input-group-btn"><button type="button" class="btn btn-primary" @click="copy(`<img src=&quot;${result.url}&quot; />`)">复制</button></span>
            </div>
            <a v-if="result.del" :href="result.del" class="btn btn-danger btn-sm delete-link">
              <i class="icon icon-trash" aria-hidden="true"></i> 删除
            </a>
          </div>
        </div>
      </article>
    </section>

    <nav class="upload-actions text-center" aria-label="站点操作">
      <a v-if="bootstrap.is_admin" href="/admin/history" class="btn btn-default"><i class="icon icon-time"></i> 历史上传</a>
      <a v-if="bootstrap.is_admin" href="/admin/manager" class="btn btn-default"><i class="icon icon-cog"></i> 管理后台</a>
      <a v-if="bootstrap.config.api_status" href="/api/" class="btn btn-default"><i class="icon icon-code"></i> API接口</a>
    </nav>

    <footer class="text-center text-muted upload-footer">
      <p>Powered by <a href="https://github.com/Redstonexs/easyimages">EasyImage</a> v{{ bootstrap.version }}</p>
    </footer>
  </main>

  <div class="ei-fab">
    <a href="/app/list" class="btn btn-default" title="图床广场"><i class="icon icon-list"></i></a>
    <button type="button" class="btn btn-primary" title="管理员登录" @click="openAdmin"><i class="icon icon-user"></i></button>
  </div>

  <div v-if="showLogin" class="login-backdrop" @click.self="showLogin = false">
    <section class="login-dialog" role="dialog" aria-modal="true" aria-labelledby="login-title">
      <header class="login-header">
        <h4 id="login-title"><i class="icon icon-sign-in"></i> 管理员登录</h4>
        <button type="button" class="close" aria-label="关闭" @click="showLogin = false">&times;</button>
      </header>
      <div class="login-body">
        <div v-if="loginError" class="alert alert-danger">{{ loginError }}</div>
        <div class="form-group">
          <label for="login-user">用户名</label>
          <input id="login-user" v-model="loginUser" type="text" class="form-control" autocomplete="username">
        </div>
        <div class="form-group">
          <label for="login-pass">密码</label>
          <input id="login-pass" v-model="loginPass" type="password" class="form-control" autocomplete="current-password" @keydown.enter="login">
        </div>
        <div v-if="captcha.type === 'builtin'" class="form-group">
          <label for="captcha-answer">验证码</label>
          <div class="captcha-question">{{ captcha.question }}</div>
          <input id="captcha-answer" v-model="captchaAnswer" type="text" class="form-control" autocomplete="off" @keydown.enter="login">
          <button type="button" class="btn btn-link btn-sm" @click="refreshCaptcha">换一题</button>
        </div>
        <div v-else-if="captcha.type === 'turnstile'" id="login-turnstile-container" class="form-group"></div>
      </div>
      <footer class="login-footer">
        <button type="button" class="btn btn-default" @click="showLogin = false">取消</button>
        <button type="button" class="btn btn-primary" @click="login"><i class="icon icon-ok"></i> 登录</button>
      </footer>
    </section>
  </div>
</template>

<style scoped>
.upload-area {
  border: 2px dashed #c7d2e1;
  border-radius: 16px;
  padding: 42px;
  text-align: center;
  cursor: pointer;
  transition: border-color 0.2s ease, background 0.2s ease, transform 0.2s ease;
  background: #fff;
  box-shadow: 0 20px 45px rgba(15, 23, 42, 0.08);
}

.upload-area:hover,
.upload-area.dragover {
  border-color: #3280fc;
  background: #f0f7ff;
  transform: translateY(-1px);
}

.upload-area.uploading {
  border-color: #f39c12;
  background: #fffaf0;
  cursor: default;
}

.upload-btn { margin-top: 18px; }

.storage-source-panel { margin: 16px 0; padding: 14px; border: 1px solid #e8eef6; border-radius: 12px; background: #fff; }
.storage-source-panel label { display: block; margin-bottom: 6px; font-weight: 700; }

.upload-progress-container { margin-top: 20px; }

.file-progress-item {
  margin-bottom: 8px;
  padding: 10px 14px;
  background: #fff;
  border: 1px solid #e8eef6;
  border-radius: 8px;
}

.file-progress-header,
.overall-stats {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 6px;
  font-size: 13px;
}

.file-progress-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
}

.file-progress-info { color: #777; }

.upload-status-icon {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 6px;
  background: #bbb;
}

.status-uploading { background: #f39c12; animation: pulse 1s infinite; }
.status-processing { background: #f39c12; animation: pulse 1s infinite; }
.status-success { background: #06b96f; }
.status-error { background: #e74c3c; }

.progress { margin-bottom: 0; height: 7px; border-radius: 999px; overflow: hidden; }
.progress-bar { transition: width 0.15s ease; }
.overall-progress { margin-top: 14px; padding: 14px; border-radius: 10px; background: #f8fafc; }
.percentage { font-size: 18px; font-weight: 700; color: #3280fc; }

.result-area { margin-top: 24px; }
.link-box { margin: 12px 0; padding: 14px; background: #fff; border-radius: 10px; border: 1px solid #e8eef6; }
.preview-img { max-width: 200px; max-height: 200px; margin: 10px; border-radius: 8px; }
.result-links label { margin-top: 8px; color: #666; font-weight: 500; }
.result-name { padding: 7px 0; word-break: break-all; }
.delete-link { margin-top: 10px; }
.upload-actions { margin-top: 30px; }
.upload-footer { margin-top: 50px; }

.login-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1500;
  display: grid;
  place-items: center;
  padding: 16px;
  background: rgba(15, 23, 42, 0.45);
}

.login-dialog {
  width: min(420px, 100%);
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 30px 80px rgba(15, 23, 42, 0.25);
  overflow: hidden;
}

.login-header,
.login-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 18px;
  border-bottom: 1px solid #edf2f7;
}

.login-footer {
  justify-content: flex-end;
  gap: 8px;
  border-top: 1px solid #edf2f7;
  border-bottom: 0;
}

.login-header h4 { margin: 0; }
.login-body { padding: 18px; }
.captcha-question {
  margin-bottom: 10px;
  padding: 12px 15px;
  border: 1px solid #dee2e6;
  border-radius: 6px;
  background: #f8f9fa;
  text-align: center;
  font-size: 16px;
  font-weight: 700;
  letter-spacing: 2px;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

@media (max-width: 640px) {
  .upload-area { padding: 28px 18px; }
  .file-progress-header { align-items: flex-start; flex-direction: column; }
}
</style>
