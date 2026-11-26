import { StrictMode, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import UploadApp from '@/components/UploadApp'
import { registerServiceWorker } from '@/shared/sw'
import { applyTheme, readInitialTheme } from '@/shared/theme'
import '@/styles/globals.css'

applyTheme(readInitialTheme())

const bootstrap = window.EasyImageUpload
const mount = document.getElementById('upload-app')

if (bootstrap && mount) {
  createRoot(mount).render(createElement(StrictMode, null, createElement(UploadApp, { bootstrap })))
  registerServiceWorker()
}
