import { StrictMode, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import GalleryApp from '@/components/GalleryApp'
import { registerServiceWorker } from '@/shared/sw'
import { applyTheme, readInitialTheme } from '@/shared/theme'
import '@/styles/globals.css'

applyTheme(readInitialTheme())

const bootstrap = window.EasyImageGallery
const mount = document.getElementById('gallery-app')

if (bootstrap && mount) {
  createRoot(mount).render(createElement(StrictMode, null, createElement(GalleryApp, { bootstrap })))
  registerServiceWorker()
}
