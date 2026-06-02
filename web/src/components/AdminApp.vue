<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { AdminBootstrap, AdminView } from '../types'
import type { Notice as NoticeItem } from '../shared/notify'
import type { NoticeType } from '../shared/notify'
import { createNotice } from '../shared/notify'
import NoticeStack from './NoticeStack.vue'
import AdminChart from './admin/AdminChart.vue'
import AdminConfig from './admin/AdminConfig.vue'
import AdminFiler from './admin/AdminFiler.vue'
import AdminHistory from './admin/AdminHistory.vue'
import AdminUrlList from './admin/AdminUrlList.vue'

const props = defineProps<{ bootstrap: AdminBootstrap }>()

const notices = ref<NoticeItem[]>([])
const currentView = ref<AdminView>(props.bootstrap.view)

const navItems: Array<{ view: AdminView; label: string; href: string }> = [
  { view: 'manager', label: '配置', href: '/admin/manager' },
  { view: 'history', label: '历史上传', href: '/admin/history' },
  { view: 'urllist', label: '图片列表', href: '/admin/urllist' },
  { view: 'chart', label: '统计', href: '/admin/chart' },
  { view: 'filer', label: '文件管理', href: '/admin/filer' }
]

const title = computed(() => navItems.find(item => item.view === currentView.value)?.label || '管理后台')

function notify(message: string, type: NoticeType = 'info') {
  const notice = createNotice(message, type)
  notices.value.push(notice)
  window.setTimeout(() => {
    notices.value = notices.value.filter(item => item.id !== notice.id)
  }, 3600)
}

function navigate(view: AdminView, href: string) {
  currentView.value = view
  window.history.pushState({ view }, '', href)
}

onMounted(() => {
  window.addEventListener('popstate', () => {
    const item = navItems.find(nav => nav.href === window.location.pathname)
    if (item) currentView.value = item.view
  })
})
</script>

<template>
  <NoticeStack :notices="notices" />
  <nav class="navbar navbar-default admin-nav">
    <div class="container">
      <div class="navbar-header">
        <a class="navbar-brand" href="/admin/manager">EasyImage 管理后台</a>
      </div>
      <ul class="nav navbar-nav navbar-right">
        <li><a href="/">首页</a></li>
        <li v-for="item in navItems" :key="item.view" :class="{ active: currentView === item.view }">
          <a :href="item.href" @click.prevent="navigate(item.view, item.href)">{{ item.label }}</a>
        </li>
      </ul>
    </div>
  </nav>

  <main class="container admin-shell">
    <header class="admin-page-heading">
      <div>
        <p class="text-muted">{{ bootstrap.title }} v{{ bootstrap.version }}</p>
        <h2>{{ title }}</h2>
      </div>
    </header>

    <AdminConfig v-if="currentView === 'manager'" @notice="notify" />
    <AdminHistory v-else-if="currentView === 'history'" @notice="notify" />
    <AdminUrlList v-else-if="currentView === 'urllist'" @notice="notify" />
    <AdminChart v-else-if="currentView === 'chart'" />
    <AdminFiler v-else-if="currentView === 'filer'" @notice="notify" />
  </main>
</template>

<style scoped>
.admin-shell { margin-bottom: 48px; }
.admin-page-heading { display: flex; justify-content: space-between; align-items: center; margin: 20px 0; }
.admin-page-heading h2 { margin: 0; font-weight: 700; }
.admin-page-heading p { margin: 0 0 4px; }
.admin-nav .active > a { color: #3280fc; font-weight: 700; }
</style>
