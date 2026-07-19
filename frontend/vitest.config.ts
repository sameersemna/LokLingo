import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// Separate vitest config so `vite build` doesn't have to bundle vitest/types.
// This is a sibling to vite.config.ts and is picked up only by `vitest`.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: false,
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['./src/test/setup.ts'],
  },
})
