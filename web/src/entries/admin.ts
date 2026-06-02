import { createApp } from 'vue'
import AdminApp from '../components/AdminApp.vue'
import { registerServiceWorker } from '../shared/sw'
import '../styles/app.css'
import '../styles/admin.css'

const bootstrap = window.EasyImageAdmin
const mount = document.getElementById('admin-app')

if (bootstrap && mount) {
  createApp(AdminApp, { bootstrap }).mount(mount)
  registerServiceWorker()
}
