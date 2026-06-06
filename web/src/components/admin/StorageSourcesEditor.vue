<script setup lang="ts">
import { computed, ref } from 'vue'
import type { StorageSourceConfig } from '../../types'
import { createS3StorageSource, normalizeStorageSource, normalizeStorageSources, validateStorageSources } from '../../shared/storageSources'

const props = defineProps<{ modelValue: StorageSourceConfig[]; defaultSource: string }>()
const emit = defineEmits<{
  (event: 'update:modelValue', value: StorageSourceConfig[]): void
  (event: 'change-default-source', value: string): void
}>()

type StringField = 'id' | 'name' | 'public_base_url' | 's3_endpoint' | 's3_region' | 's3_bucket' | 's3_prefix' | 's3_access_key_id' | 's3_access_key_secret'

const expandedIndex = ref<number | null>(null)

const sources = computed(() => props.modelValue)
const enabledSources = computed(() => sources.value.filter(source => source.enabled))
const validationMessages = computed(() => validateStorageSources(sources.value, props.defaultSource))

function firstEnabledSourceID(nextSources: StorageSourceConfig[]) {
  return nextSources.find(source => source.enabled)?.id || 'local'
}

function commit(nextSources: StorageSourceConfig[]) {
  const normalized = normalizeStorageSources(nextSources)
  emit('update:modelValue', normalized)
  if (!normalized.some(source => source.id === props.defaultSource && source.enabled)) {
    emit('change-default-source', firstEnabledSourceID(normalized))
  }
}

function updateSource(index: number, patch: Partial<StorageSourceConfig>) {
  const next = sources.value.map((source, sourceIndex) => {
    if (sourceIndex !== index) return source
    return normalizeStorageSource({ ...source, ...patch })
  })
  commit(next)
}

function updateString(index: number, field: StringField, event: Event) {
  updateSource(index, { [field]: (event.target as HTMLInputElement).value })
}

function updateEnabled(index: number, event: Event) {
  updateSource(index, { enabled: (event.target as HTMLInputElement).checked })
}

function updatePathStyle(index: number, event: Event) {
  updateSource(index, { s3_force_path_style: (event.target as HTMLInputElement).checked })
}

function changeDefault(event: Event) {
  emit('change-default-source', (event.target as HTMLSelectElement).value)
}

function addS3Source() {
  const nextSource = createS3StorageSource(sources.value)
  expandedIndex.value = sources.value.length
  commit([...sources.value, nextSource])
}

function removeSource(index: number) {
  const source = sources.value[index]
  if (!source || source.id === 'local') return
  if (!window.confirm(`确认删除存储源 ${source.name || source.id}？已上传到该源的历史图片仍会保留原记录。`)) return

  commit(sources.value.filter((_, sourceIndex) => sourceIndex !== index))
  if (expandedIndex.value === index) expandedIndex.value = null
}

function toggleExpanded(index: number) {
  expandedIndex.value = expandedIndex.value === index ? null : index
}

function isIDLocked(source: StorageSourceConfig) {
  return source.id === 'local' || source.s3_secret_set !== undefined
}

function sourceSummary(source: StorageSourceConfig) {
  if (source.type === 'local') return '使用服务器本地 i/ 目录存储图片'
  return [source.s3_bucket, source.s3_endpoint || 'AWS S3 默认端点'].filter(Boolean).join(' · ')
}
</script>

<template>
  <div class="storage-editor">
    <div class="storage-editor-toolbar">
      <label>默认上传源
        <select class="form-control" :value="props.defaultSource" @change="changeDefault">
          <option v-for="source in enabledSources" :key="source.id" :value="source.id">{{ source.name }} ({{ source.type }})</option>
        </select>
      </label>
      <button type="button" class="btn btn-info" @click="addS3Source"><i class="icon icon-plus"></i> 添加 S3 源</button>
    </div>

    <div v-if="validationMessages.length" class="alert alert-warning storage-validation">
      <strong>存储源配置需要调整：</strong>
      <ul>
        <li v-for="message in validationMessages" :key="message">{{ message }}</li>
      </ul>
    </div>

    <div class="storage-source-list">
      <article v-for="(source, index) in sources" :key="`${source.id || 'source'}-${index}`" class="storage-source-card" :class="{ disabled: !source.enabled }">
        <header class="storage-source-head">
          <div>
            <span class="source-type">{{ source.type }}</span>
            <h5>{{ source.name || source.id || '未命名存储源' }}</h5>
            <p>{{ sourceSummary(source) }}</p>
          </div>
          <div class="source-actions">
            <label class="source-switch" :class="{ locked: source.id === 'local' }">
              <input type="checkbox" :checked="source.enabled" :disabled="source.id === 'local'" @change="updateEnabled(index, $event)">
              <span>{{ source.enabled ? '已启用' : '已停用' }}</span>
            </label>
            <button type="button" class="btn btn-xs btn-default" @click="toggleExpanded(index)">{{ expandedIndex === index ? '收起' : '编辑' }}</button>
            <button type="button" class="btn btn-xs btn-danger" :disabled="source.id === 'local'" @click="removeSource(index)">删除</button>
          </div>
        </header>

        <div v-if="expandedIndex === index" class="storage-source-form">
          <div class="storage-form-grid">
            <label>名称<input class="form-control" :value="source.name" @input="updateString(index, 'name', $event)"></label>
            <label>ID<input class="form-control" :value="source.id" :disabled="isIDLocked(source)" @input="updateString(index, 'id', $event)"></label>
            <label>类型<input class="form-control" :value="source.type" disabled></label>
            <label>公开访问地址<input class="form-control" placeholder="https://cdn.example.com/images" :value="source.public_base_url" @input="updateString(index, 'public_base_url', $event)"></label>
          </div>

          <template v-if="source.type === 's3'">
            <div class="storage-form-grid s3-grid">
              <label>Endpoint<input class="form-control" placeholder="https://s3.example.com" :value="source.s3_endpoint" @input="updateString(index, 's3_endpoint', $event)"></label>
              <label>Region<input class="form-control" placeholder="auto" :value="source.s3_region" @input="updateString(index, 's3_region', $event)"></label>
              <label>Bucket<input class="form-control" :value="source.s3_bucket" @input="updateString(index, 's3_bucket', $event)"></label>
              <label>Prefix<input class="form-control" placeholder="uploads" :value="source.s3_prefix" @input="updateString(index, 's3_prefix', $event)"></label>
              <label>Access Key ID<input class="form-control" autocomplete="off" :value="source.s3_access_key_id" @input="updateString(index, 's3_access_key_id', $event)"></label>
              <label>Access Key Secret<input class="form-control" type="password" autocomplete="new-password" :placeholder="source.s3_secret_set ? '已设置，留空保留原密钥' : '请输入访问密钥'" :value="source.s3_access_key_secret || ''" @input="updateString(index, 's3_access_key_secret', $event)"></label>
            </div>
            <label class="checkbox-inline path-style-toggle">
              <input type="checkbox" :checked="source.s3_force_path_style" @change="updatePathStyle(index, $event)"> 使用 Path-Style 地址
            </label>
            <p class="help-block">密钥不会在接口中回显；已有密钥的存储源留空即可保留原密钥。</p>
          </template>
        </div>
      </article>
    </div>
  </div>
</template>

<style scoped>
.storage-editor { display: grid; gap: 14px; }
.storage-editor-toolbar { display: flex; align-items: end; justify-content: space-between; gap: 12px; }
.storage-editor-toolbar label { flex: 1 1 280px; margin: 0; }
.storage-validation { margin: 0; }
.storage-validation ul { margin: 8px 0 0; padding-left: 18px; }
.storage-source-list { display: grid; gap: 12px; }
.storage-source-card { border: 1px solid #dbe3ef; border-radius: 12px; background: #fff; box-shadow: 0 10px 26px rgba(15, 23, 42, 0.05); }
.storage-source-card.disabled { opacity: 0.72; }
.storage-source-head { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 14px; }
.storage-source-head h5 { margin: 4px 0; font-weight: 700; }
.storage-source-head p { margin: 0; color: #64748b; }
.source-type { display: inline-flex; padding: 2px 8px; border-radius: 999px; background: #eef6ff; color: #2563eb; font-size: 11px; font-weight: 800; letter-spacing: 0.08em; text-transform: uppercase; }
.source-actions { display: flex; align-items: center; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.source-switch { display: inline-flex; align-items: center; gap: 6px; margin: 0; color: #334155; font-weight: 600; }
.source-switch.locked { color: #64748b; }
.storage-source-form { padding: 0 14px 14px; border-top: 1px solid #edf2f7; }
.storage-form-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px; padding-top: 14px; }
.s3-grid { grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); }
.path-style-toggle { margin-top: 12px; }
@media (max-width: 640px) {
  .storage-editor-toolbar, .storage-source-head, .source-actions { align-items: stretch; flex-direction: column; }
  .storage-editor-toolbar .btn, .source-actions .btn { width: 100%; }
}
</style>
