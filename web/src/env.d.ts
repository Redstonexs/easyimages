/// <reference types="vite/client" />

interface Window {
  EasyImageUpload?: import('./types').UploadBootstrap
  EasyImageGallery?: import('./types').GalleryBootstrap
  turnstile?: {
    render: (selector: string, options: { sitekey: string; callback: (token: string) => void }) => unknown
    reset: (selector: string) => void
  }
  grecaptcha?: {
    ready: (callback: () => void) => void
    execute: (siteKey: string, options: { action: string }) => Promise<string>
  }
}
