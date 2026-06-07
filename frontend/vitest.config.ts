import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import { resolve } from 'node:path'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals:     true,
    setupFiles:  ['./src/test/setup.ts'],
    env: {
      BACKEND_INTERNAL_URL: 'https://backend:8080',
    },
    coverage: {
      provider:         'v8',
      reportsDirectory: './coverage',
      reporter:         ['text', 'json-summary', 'lcov'],
      include: [
        'src/**/*.{ts,tsx}',
      ],
      exclude: [
        'src/test/**',
        'src/**/*.test.{ts,tsx}',
        'src/**/*.d.ts',
        'src/app/**/layout.tsx',
        'src/app/**/page.tsx',
        'src/app/middleware.ts',
        'src/middleware.ts',
        'src/lib/api-types.ts',
        'src/app/webhooks/**',
      ],
      thresholds: {
        lines:      80,
        functions:  80,
        branches:   80,
        statements: 80,
      },
    },
  },
  resolve: {
    alias: {
      '@': resolve(__dirname, './src'),
    },
  },
})
