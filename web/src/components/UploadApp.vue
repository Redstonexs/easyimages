<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import type { CapWidgetElement, CaptchaData, ProgressItem, UploadBootstrap, UploadResult } from '../types'
import { fetchJSON } from '../shared/api'
import { extractImageFiles, isTextEntryTarget } from '../shared/clipboardFiles'
import { createSemaphore } from '../shared/concurrency'
import { formatSize } from '../shared/format'
import type { LinkFormat } from '../shared/uploadFormats'
import { formatResults, resultName } from '../shared/uploadFormats'
import { useCopy } from '../shared/useCopy'
import { useNotices } from '../shared/useNotices'
import NoticeStack from './NoticeStack.vue'

const props = defineProps<{
  bootstrap: UploadBootstrap
}>()

const DEFAULT_CHUNK_SIZE = 16 * 1024 * 1024
const MAX_CONCURRENT_CHUNKS = 4
const MAX_CONCURRENT_FILES = 3
// One budget shared by whole-file and chunk requests. Without it the file pool and
// the per-file chunk pool multiply (3 x 4 = 12) past the ~6 connections a browser
// opens per origin; the server runs plain HTTP/1.1, so there is no multiplexing.
const MAX_CONCURRENT_REQUESTS = 6
const MAX_RETRIES = 3

const requestSlots = createSemaphore(MAX_CONCURRENT_REQUESTS)

const fileInput = ref<HTMLInputElement | null>(null)
const isDragging = ref(false)
const isWindowDragging = ref(false)
const isUploading = ref(false)
const progressItems = ref<ProgressItem[]>([])
const results = ref<UploadResult[]>([])
const totalSize = ref(0)
const selectedStorageSource = ref(props.bootstrap.config.default_storage_source || props.bootstrap.config.storage_sources[0]?.id || 'local')

// Upload queue. Files may be appended while a batch is in flight, so workers pull
// from a shared cursor instead of iterating a fixed list. queue[i] pairs with
// progressItems[i]; neither is ever spliced.
const queue: File[] = []
let queueCursor = 0
let activeWorkers = 0
let nextItemId = 0
// dragenter/dragleave fire for every child element, so count depth instead of
// clearing the overlay on the first leave.
let dragDepth = 0

const showLogin = ref(false)
const loginUser = ref('')
const loginPass = ref('')
const loginError = ref('')
const captcha = ref<CaptchaData>(props.bootstrap.captcha)
const captchaAnswer = ref('')
const externalCaptchaToken = ref('')
const turnstileRendered = ref(false)
const capWidget = ref<CapWidgetElement | null>(null)

const { notices, notify } = useNotices()
const copy = useCopy(notify)

const maxSizeLabel = computed(() => formatSize(props.bootstrap.config.max_size))
const chunkSize = computed(() => props.bootstrap.config.chunk_size > 0 ? props.bootstrap.config.chunk_size : DEFAULT_CHUNK_SIZE)
const uploadedTotal = computed(() => progressItems.value.reduce((sum, item) => sum + item.loaded, 0))
const settledCount = computed(() => progressItems.value.filter(item => item.status === 'success' || item.status === 'error').length)
const copyableResults = computed(() => results.value.filter(result => Boolean(result.url)))
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
  return `上传中 (${settledCount.value}/${progressItems.value.length}): ${formatSize(uploadedTotal.value)} / ${formatSize(totalSize.value)}`
})

const copyFormats: Array<{ format: LinkFormat; label: string }> = [
  { format: 'url', label: '全部链接' },
  { format: 'markdown', label: '全部 Markdown' },
  { format: 'html', label: '全部 HTML' },
  { format: 'bbcode', label: '全部 BBCode' }
]

function selectFiles() {
  fileInput.value?.click()
}

function handleFileInput(event: Event) {
  const input = event.target as HTMLInputElement
  if (input.files) handleFiles(input.files)
  // Reset so picking the same file twice in a row still fires change.
  input.value = ''
}

function dragHasFiles(event: DragEvent) {
  return Array.from(event.dataTransfer?.types || []).includes('Files')
}

function resetDragState() {
  dragDepth = 0
  isDragging.value = false
  isWindowDragging.value = false
}

function handleWindowDragEnter(event: DragEvent) {
  if (!dragHasFiles(event)) return
  dragDepth++
  isWindowDragging.value = true
}

function handleWindowDragLeave(event: DragEvent) {
  if (!dragHasFiles(event)) return
  dragDepth = Math.max(0, dragDepth - 1)
  if (dragDepth === 0) {
    isWindowDragging.value = false
    isDragging.value = false
  }
}

function handleWindowDragOver(event: DragEvent) {
  // Required: without it the browser opens the dropped image and discards all
  // in-flight upload state.
  event.preventDefault()
}

function handleWindowDrop(event: DragEvent) {
  event.preventDefault()
  resetDragState()
  if (event.dataTransfer?.files?.length) handleFiles(event.dataTransfer.files)
}

function handleAreaDragOver(event: DragEvent) {
  event.preventDefault()
  if (dragHasFiles(event)) isDragging.value = true
}

function handleAreaDragLeave(event: DragEvent) {
  const next = event.relatedTarget as Node | null
  const area = event.currentTarget as Node | null
  // Moving onto a child still counts as being over the dropzone.
  if (next && area && area.contains(next)) return
  isDragging.value = false
}

function handlePaste(event: ClipboardEvent) {
  // Leave text fields (the login form) alone.
  if (isTextEntryTarget(event.target)) return
  const files = extractImageFiles(event.clipboardData?.items)
  if (files.length === 0) return
  event.preventDefault()
  handleFiles(files)
}

function handleFiles(fileList: FileList | File[]) {
  const accepted: File[] = []
  for (const file of Array.from(fileList)) {
    if (file.size > props.bootstrap.config.max_size) {
      notify(`"${file.name}" 超过大小限制 (${maxSizeLabel.value})`, 'warning')
      continue
    }
    accepted.push(file)
  }
  if (accepted.length === 0) return

  if (!isUploading.value) {
    // Starting fresh: clear the finished batch before queueing the new one.
    queue.length = 0
    queueCursor = 0
    progressItems.value = []
    totalSize.value = 0
  }

  for (const file of accepted) {
    queue.push(file)
    progressItems.value.push({
      id: nextItemId++,
      name: file.name,
      size: file.size,
      loaded: 0,
      status: 'waiting',
      message: formatSize(file.size)
    })
  }
  totalSize.value += accepted.reduce((sum, file) => sum + file.size, 0)
  isUploading.value = true
  ensureWorkers()
}

function ensureWorkers() {
  // runWorker claims its index synchronously, so re-checking the cursor is safe.
  while (activeWorkers < MAX_CONCURRENT_FILES && queueCursor < queue.length) {
    activeWorkers++
    void runWorker()
  }
}

async function runWorker() {
  while (queueCursor < queue.length) {
    const index = queueCursor++
    const file = queue[index]
    const item = progressItems.value[index]
    item.status = 'uploading'

    try {
      const result = file.size > chunkSize.value
        ? await uploadFileChunked(
          file,
          loaded => updateFileProgress(item, loaded),
          message => markFileProcessing(item, message)
        )
        : await uploadFileWhole(
          file,
          loaded => updateFileProgress(item, loaded),
          message => markFileProcessing(item, message)
        )
      item.status = 'success'
      item.loaded = file.size
      item.message = formatSize(file.size)
      if (result.url) results.value.push(result)
    } catch (error) {
      item.status = 'error'
      // Count failures as complete so the overall bar still reaches 100%.
      item.loaded = file.size
      item.message = error instanceof Error ? error.message : '上传失败'
    }
  }

  activeWorkers--
  if (activeWorkers === 0) finishBatch()
}

function finishBatch() {
  isUploading.value = false
  if (fileInput.value) fileInput.value.value = ''
  const success = progressItems.value.filter(item => item.status === 'success').length
  const failed = progressItems.value.filter(item => item.status === 'error').length
  if (failed === 0) notify('上传成功', 'success')
  else if (success > 0) notify(`${success} 个文件上传成功，${failed} 个失败`, 'warning')
  else notify('上传失败', 'danger')
}

function updateFileProgress(item: ProgressItem, loaded: number) {
  item.loaded = Math.min(loaded, item.size)
  item.message = `${formatSize(loaded)} / ${formatSize(item.size)}`
}

function markFileProcessing(item: ProgressItem, message: string) {
  item.status = 'processing'
  item.loaded = item.size
  item.message = message
}

function progressPercent(item: ProgressItem) {
  if (item.size === 0) return 0
  const percent = Math.round((item.loaded / item.size) * 100)
  return item.status === 'success' ? percent : Math.min(percent, 99)
}

function copyAll(format: LinkFormat) {
  void copy(formatResults(results.value, format), `已复制 ${copyableResults.value.length} 条链接`)
}

function clearResults() {
  results.value = []
}

function uploadFileWhole(
  file: File,
  onProgress: (loaded: number) => void,
  onProcessing: (message: string) => void
): Promise<UploadResult> {
  return requestSlots.run(() => new Promise<UploadResult>((resolve, reject) => {
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
  }))
}

async function uploadFileChunked(
  file: File,
  onProgress: (loaded: number) => void,
  onProcessing: (message: string) => void
): Promise<UploadResult> {
  const currentChunkSize = chunkSize.value
  const totalChunks = Math.ceil(file.size / currentChunkSize)
  const uploadId = crypto.randomUUID ? crypto.randomUUID() : `upload-${Date.now()}-${Math.random().toString(16).slice(2)}`
  const chunkProgress = new Array<number>(totalChunks).fill(0)
  let nextChunk = 0
  let failed = false

  const updateChunkProgress = () => onProgress(chunkProgress.reduce((sum, value) => sum + value, 0))
  const workers = Array.from({ length: Math.min(MAX_CONCURRENT_CHUNKS, totalChunks) }, async () => {
    while (!failed) {
      const chunkIndex = nextChunk++
      if (chunkIndex >= totalChunks) break

      const start = chunkIndex * currentChunkSize
      const end = Math.min(start + currentChunkSize, file.size)
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
  return requestSlots.run(() => new Promise<UploadResult>((resolve, reject) => {
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
  }))
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

// The Cap widget renders and solves itself once it is in the DOM; all we do is
// collect the token it emits.
function handleCapSolve(event: Event) {
  externalCaptchaToken.value = (event as CustomEvent<{ token?: string }>).detail?.token || ''
}

function handleCapError(event: Event) {
  externalCaptchaToken.value = ''
  const detail = (event as CustomEvent<{ message?: string }>).detail
  loginError.value = detail?.message || '人机验证加载失败，请刷新页面重试'
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
  } else if (captcha.value.type === 'cap') {
    // Matches the hidden input the widget injects inside a plain <form>.
    body.set('cap-token', externalCaptchaToken.value)
  }

  try {
    const result = await fetchJSON<{ result: string; message?: string }>('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body
    })
    if (result.result === 'success') window.location.href = '/admin/manager'
    else {
      loginError.value = result.message || '登录失败'
      resetCaptchaAfterFailure()
    }
  } catch (error) {
    loginError.value = error instanceof Error ? error.message : '登录失败'
    resetCaptchaAfterFailure()
  }
}

// Every captcha here is single-use: the builtin token is consumed by answering,
// and Cap's instance deletes the token during /siteverify. After a failed login
// the old token is worthless, so issue a fresh challenge.
function resetCaptchaAfterFailure() {
  if (captcha.value.type === 'builtin') {
    void refreshCaptcha().catch(() => undefined)
  } else if (captcha.value.type === 'cap') {
    externalCaptchaToken.value = ''
    capWidget.value?.reset()
  }
}

onMounted(() => {
  if (props.bootstrap.must_login === 1 && captcha.value.type === 'builtin') {
    void refreshCaptcha().catch(() => undefined)
  }
  window.addEventListener('dragenter', handleWindowDragEnter)
  window.addEventListener('dragover', handleWindowDragOver)
  window.addEventListener('dragleave', handleWindowDragLeave)
  window.addEventListener('drop', handleWindowDrop)
  document.addEventListener('paste', handlePaste)
})

onUnmounted(() => {
  window.removeEventListener('dragenter', handleWindowDragEnter)
  window.removeEventListener('dragover', handleWindowDragOver)
  window.removeEventListener('dragleave', handleWindowDragLeave)
  window.removeEventListener('drop', handleWindowDrop)
  document.removeEventListener('paste', handlePaste)
})
</script>

<template>
  <NoticeStack :notices="notices" />
  <div v-if="isWindowDragging" class="drop-overlay" aria-hidden="true">
    <div class="drop-overlay-inner">
      <i class="icon icon-cloud-upload icon-3x" aria-hidden="true"></i>
      <p>松开即可上传到本站</p>
    </div>
  </div>
  <main class="ei-shell upload-shell">
    <header class="ei-title">
      <h1>{{ bootstrap.config.title }}</h1>
      <p>{{ bootstrap.config.description }}</p>
    </header>

    <section
      class="upload-area"
      :class="{ dragover: isDragging, uploading: isUploading }"
      role="button"
      tabindex="0"
      @click="selectFiles"
      @keydown.enter.prevent="selectFiles"
      @keydown.space.prevent="selectFiles"
      @dragover="handleAreaDragOver"
      @dragleave="handleAreaDragLeave"
    >
      <i class="icon icon-cloud-upload icon-3x" aria-hidden="true"></i>
      <h3>拖拽文件到此处或点击选择</h3>
      <p class="text-muted">支持 jpg, png, gif, webp 等格式，单文件最大 {{ maxSizeLabel }}</p>
      <input ref="fileInput" type="file" multiple accept="image/*" class="sr-only" @change="handleFileInput">
      <button type="button" class="btn btn-primary btn-lg upload-btn" @click.stop="selectFiles">
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
      <div v-if="copyableResults.length > 1" class="batch-actions">
        <span class="batch-actions-count">共 {{ copyableResults.length }} 条链接</span>
        <div class="batch-actions-buttons">
          <button
            v-for="option in copyFormats"
            :key="option.format"
            type="button"
            class="btn btn-primary btn-sm"
            @click="copyAll(option.format)"
          >{{ option.label }}</button>
          <button type="button" class="btn btn-default btn-sm" @click="clearResults">清空结果</button>
        </div>
      </div>
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
        <div v-else-if="captcha.type === 'cap'" class="form-group">
          <cap-widget
            ref="capWidget"
            :data-cap-api-endpoint="captcha.api_endpoint"
            @solve="handleCapSolve"
            @error="handleCapError"
          ></cap-widget>
        </div>
      </div>
      <footer class="login-footer">
        <button type="button" class="btn btn-default" @click="showLogin = false">取消</button>
        <button type="button" class="btn btn-primary" @click="login"><i class="icon icon-ok"></i> 登录</button>
      </footer>
    </section>
  </div>
</template>

<style scoped>
.drop-overlay {
  position: fixed;
  inset: 0;
  z-index: 1080;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(50, 128, 252, 0.14);
  backdrop-filter: blur(2px);
  pointer-events: none;
}
.drop-overlay-inner {
  border: 3px dashed #3280fc;
  border-radius: 20px;
  padding: 48px 72px;
  background: rgba(255, 255, 255, 0.94);
  color: #3280fc;
  text-align: center;
  box-shadow: 0 18px 48px rgba(15, 42, 90, 0.18);
}
.drop-overlay-inner p {
  margin: 12px 0 0;
  font-size: 18px;
  font-weight: 700;
}
.batch-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 16px;
  padding: 12px 16px;
  border-radius: 10px;
  background: #f2f6fd;
  border: 1px solid #dbe6f8;
}
.batch-actions-count {
  font-weight: 700;
  color: #35507a;
}
.batch-actions-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
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
