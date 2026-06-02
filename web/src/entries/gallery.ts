import { createApp } from 'vue'
import GalleryApp from '../components/GalleryApp.vue'
import { registerServiceWorker } from '../shared/sw'
import '../styles/app.css'

const bootstrap = window.EasyImageGallery
const mount = document.getElementById('gallery-app')

if (bootstrap && mount) {
  createApp(GalleryApp, { bootstrap }).mount(mount)
  registerServiceWorker()
}
