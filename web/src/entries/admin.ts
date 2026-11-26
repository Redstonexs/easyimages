import { StrictMode, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import AdminApp from '@/components/AdminApp'
import { applyTheme, readInitialTheme } from '@/shared/theme'
import { registerServiceWorker } from '@/shared/sw'
import '@/styles/globals.css'

applyTheme(readInitialTheme())

const bootstrap = window.EasyImageAdmin
const mount = document.getElementById('admin-app')

if (bootstrap && mount) {
  createRoot(mount).render(createElement(StrictMode, null, createElement(AdminApp, { bootstrap })))
  registerServiceWorker()
}
