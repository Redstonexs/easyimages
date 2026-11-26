/// <reference types="vite/client" />
import type { DetailedHTMLProps, HTMLAttributes } from 'react'

declare global {
  interface Window {
    EasyImageUpload?: import('./types').UploadBootstrap
    EasyImageGallery?: import('./types').GalleryBootstrap
    EasyImageAdmin?: import('./types').AdminBootstrap
    turnstile?: {
      render: (selector: string, options: { sitekey: string; callback: (token: string) => void }) => unknown
      reset: (selector: string) => void
    }
    grecaptcha?: {
      ready: (callback: () => void) => void
      execute: (siteKey: string, options: { action: string }) => Promise<string>
    }
  }
}

declare module 'react' {
  namespace JSX {
    interface IntrinsicElements {
      'cap-widget': DetailedHTMLProps<HTMLAttributes<HTMLElement>, HTMLElement> & {
        'data-cap-api-endpoint'?: string
      }
    }
  }
}

export {}
