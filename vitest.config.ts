import { defineConfig } from 'vitest/config'

// Kept separate from vite.config.ts so the PWA/service-worker plugin and the
// island build inputs do not run during unit tests.
export default defineConfig({
  test: {
    environment: 'happy-dom',
    include: ['web/src/**/*.test.ts']
  }
})
