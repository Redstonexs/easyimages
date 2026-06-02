import { createApp } from 'vue'
import UploadApp from '../components/UploadApp.vue'
import { registerServiceWorker } from '../shared/sw'
import '../styles/app.css'

const bootstrap = window.EasyImageUpload
const mount = document.getElementById('upload-app')

if (bootstrap && mount) {
  createApp(UploadApp, { bootstrap }).mount(mount)
  registerServiceWorker()
}
