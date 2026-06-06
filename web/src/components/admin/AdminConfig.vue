<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { AdminConfig, AdminOverview } from '../../types'
import type { NoticeType } from '../../shared/notify'
import { adminApi } from '../../shared/adminApi'
import { normalizeStorageSources, validateStorageSources } from '../../shared/storageSources'
import StorageSourcesEditor from './StorageSourcesEditor.vue'

const emit = defineEmits<{ notice: [message: string, type?: NoticeType] }>()

const loading = ref(true)
const saving = ref(false)
const converting = ref(false)
const uploadingIcon = ref(false)
const overview = ref<AdminOverview | null>(null)
const config = ref<AdminConfig | null>(null)
const siteIconInput = ref<HTMLInputElement | null>(null)

async function load() {
  loading.value = true
  try {
    const data = await adminApi.overview()
    data.config.storage_sources = normalizeStorageSources(data.config.storage_sources)
    overview.value = data
    config.value = data.config
  } catch (error) {
    emit('notice', error instanceof Error ? error.message : '加载配置失败', 'danger')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!config.value) return
  config.value.storage_sources = normalizeStorageSources(config.value.storage_sources)
  const storageErrors = validateStorageSources(config.value.storage_sources, config.value.default_storage_source)
  if (storageErrors.length) {
    emit('notice', storageErrors[0], 'danger')
    return
  }
  saving.value = true
  try {
    const result = await adminApi.saveConfig(config.value)
    result.config.storage_sources = normalizeStorageSources(result.config.storage_sources)
    config.value = result.config
    emit('notice', result.msg || '保存成功', 'success')
  } catch (error) {
    emit('notice', error instanceof Error ? error.message : '保存失败', 'danger')
  } finally {
    saving.value = false
  }
}

async function batchWebP() {
  converting.value = true
  try {
    const result = await adminApi.batchWebP()
    emit('notice', result.message || '转换完成', result.result === 'success' ? 'success' : 'warning')
  } catch (error) {
    emit('notice', error instanceof Error ? error.message : '转换失败', 'danger')
  } finally {
    converting.value = false
  }
}

async function uploadSiteIcon() {
  if (!config.value) return
  const file = siteIconInput.value?.files?.[0]
  if (!file) {
    emit('notice', '请选择图标文件', 'warning')
    return
  }

  const formData = new FormData()
  formData.append('icon', file)
  uploadingIcon.value = true
  try {
    const result = await adminApi.uploadSiteIcon(formData)
    config.value.site_icon = result.site_icon
    if (siteIconInput.value) siteIconInput.value.value = ''
    emit('notice', result.message || '图标已更新', 'success')
  } catch (error) {
    emit('notice', error instanceof Error ? error.message : '上传图标失败', 'danger')
  } finally {
    uploadingIcon.value = false
  }
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="alert alert-info">正在加载配置...</div>
  <div v-else-if="config" class="admin-config-grid">
    <aside class="panel panel-primary">
      <div class="panel-heading"><h3 class="panel-title">系统信息</h3></div>
      <div class="panel-body">
        <p>版本: v{{ overview?.version }}</p>
        <p>文件数: {{ overview?.total_files }}</p>
        <p>已用空间: {{ overview?.used_human }}</p>
      </div>
    </aside>

    <section class="panel panel-default">
      <div class="panel-heading"><h3 class="panel-title">基本配置</h3></div>
      <div class="panel-body">
        <form @submit.prevent="save">
          <div class="form-section">
            <h4>站点</h4>
            <div class="form-row">
              <label>网站名称<input v-model="config.title" class="form-control"></label>
              <label>域名<input v-model="config.domain" class="form-control"></label>
              <label>图片域名<input v-model="config.imgurl" class="form-control"></label>
            </div>
            <div class="site-icon-editor">
              <div class="site-icon-preview">
                <img :src="config.site_icon" alt="当前网站图标">
              </div>
              <div class="site-icon-fields">
                <label>网站图标<input ref="siteIconInput" type="file" class="form-control" accept=".ico,.png,.svg,image/x-icon,image/png,image/svg+xml"></label>
                <p class="help-block">支持 ICO、PNG、SVG，文件大小不超过 512KB。上传后会自动刷新浏览器图标缓存。</p>
                <button type="button" class="btn btn-default" :disabled="uploadingIcon" @click="uploadSiteIcon">
                  <i class="icon" :class="uploadingIcon ? 'icon-spin icon-spinner' : 'icon-upload'"></i> {{ uploadingIcon ? '上传中...' : '上传图标' }}
                </button>
              </div>
            </div>
          </div>

          <div class="form-section">
            <h4>上传</h4>
            <div class="form-row">
              <label>最大上传大小<input v-model.number="config.maxSize" type="number" class="form-control"></label>
              <label>允许扩展名<input v-model="config.extensions" class="form-control"></label>
              <label>私有模式<select v-model.number="config.mustLogin" class="form-control"><option :value="1">开启：仅登录用户可上传</option><option :value="0">关闭：允许访客上传</option></select></label>
              <label>图片质量<input v-model.number="config.compress_ratio" type="number" min="1" max="100" class="form-control"></label>
            </div>
          </div>

          <div class="form-section">
            <h4>缩略图与 WebP</h4>
            <div class="form-row">
              <label>缩略图模式<select v-model.number="config.thumbnail" class="form-control"><option :value="0">不生成</option><option :value="1">访问时生成</option><option :value="2">上传时生成</option></select></label>
              <label>缩略图宽度<input v-model.number="config.thumbnail_w" type="number" class="form-control"></label>
              <label>缩略图高度<input v-model.number="config.thumbnail_h" type="number" class="form-control"></label>
              <label>自动 WebP<select v-model.number="config.webp_convert" class="form-control"><option :value="0">关闭</option><option :value="1">开启</option></select></label>
              <label>WebP质量<input v-model.number="config.webp_quality" type="number" min="1" max="100" class="form-control"></label>
            </div>
            <button type="button" class="btn btn-success" :disabled="converting" @click="batchWebP">
              <i class="icon" :class="converting ? 'icon-spin icon-spinner' : 'icon-refresh'"></i> {{ converting ? '转换中...' : '批量生成WebP' }}
            </button>
          </div>

          <div class="form-section">
            <h4>水印</h4>
            <div class="form-row">
              <label>水印类型<select v-model.number="config.watermark" class="form-control"><option :value="0">无水印</option><option :value="1">文字水印</option><option :value="2">图片水印</option></select></label>
              <label>水印文字<input v-model="config.waterText" class="form-control"></label>
              <label>水印位置<input v-model.number="config.waterPosition" type="number" min="1" max="9" class="form-control"></label>
              <label>文字颜色<input v-model="config.textColor" class="form-control"></label>
              <label>文字大小<input v-model.number="config.textSize" type="number" class="form-control"></label>
              <label>字体文件路径<input v-model="config.textFont" class="form-control"></label>
              <label>水印图片路径<input v-model="config.waterImg" class="form-control"></label>
            </div>
          </div>

          <div class="form-section">
            <h4>安全</h4>
            <div class="form-row">
              <label>登录验证码<select v-model.number="config.captcha" class="form-control"><option :value="0">关闭</option><option :value="1">开启</option></select></label>
              <label v-if="config.captcha === 1">验证码类型<select v-model.number="config.captcha_type" class="form-control"><option :value="0">内置数学题</option><option :value="1">Turnstile</option><option :value="2">reCAPTCHA</option></select></label>
              <label v-if="config.captcha_type === 1">Turnstile Site Key<input v-model="config.turnstile_site_key" class="form-control"></label>
              <label v-if="config.captcha_type === 2">reCAPTCHA Site Key<input v-model="config.recaptcha_site_key" class="form-control"></label>
              <label>防盗链<select v-model.number="config.hotlink_protect" class="form-control"><option :value="0">关闭</option><option :value="1">开启</option></select></label>
            </div>
            <label v-if="config.hotlink_protect === 1">白名单域名<textarea v-model="config.hotlink_domains" class="form-control" rows="4"></textarea></label>
          </div>

          <div class="form-section">
            <h4>其他</h4>
            <div class="form-row">
              <label>MIME 类型<input v-model="config.mime" class="form-control"></label>
              <label>路径规则<input v-model="config.storage_path" class="form-control"></label>
              <label>时间格式<input v-model="config.time_format" class="form-control"></label>
              <label>自动删除旧图<select v-model.number="config.auto_delete" class="form-control"><option :value="0">关闭</option><option :value="1">开启</option></select></label>
            </div>
          </div>

          <div class="form-section">
            <h4>存储源</h4>
            <StorageSourcesEditor
              v-model="config.storage_sources"
              :default-source="config.default_storage_source"
              @change-default-source="config.default_storage_source = $event"
            />
          </div>

          <button type="submit" class="btn btn-primary" :disabled="saving"><i class="icon icon-save"></i> {{ saving ? '保存中...' : '保存配置' }}</button>
        </form>
      </div>
    </section>
  </div>
</template>

<style scoped>
.admin-config-grid { display: grid; grid-template-columns: 260px 1fr; gap: 20px; align-items: start; }
.form-section { padding: 14px 0; border-bottom: 1px solid #edf2f7; }
.form-section h4 { margin-top: 0; }
.form-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 14px; }
label { font-weight: 500; }
label .form-control, label textarea { margin-top: 6px; font-weight: 400; }
.site-icon-editor { display: flex; gap: 14px; align-items: center; margin-top: 14px; padding: 12px; border: 1px solid #edf2f7; border-radius: 10px; background: #f8fafc; }
.site-icon-preview { display: grid; flex: 0 0 58px; width: 58px; height: 58px; place-items: center; border: 1px solid #dbe3ef; border-radius: 14px; background: #fff; }
.site-icon-preview img { max-width: 36px; max-height: 36px; object-fit: contain; }
.site-icon-fields { flex: 1; min-width: 0; }
.site-icon-fields .help-block { margin: 6px 0 8px; }
@media (max-width: 900px) { .admin-config-grid { grid-template-columns: 1fr; } }
@media (max-width: 560px) { .site-icon-editor { align-items: stretch; flex-direction: column; } }
</style>
