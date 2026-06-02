import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import { resolve } from 'path'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals:     true,
    setupFiles:  ['./src/test/setup.ts'],
    coverage: {
      provider:         'v8',
      reportsDirectory: './coverage',
      reporter:         ['text', 'json-summary', 'lcov'],
      // Scope: only files with unit tests. api.ts and api-types.ts are
      // integration-level (require network mocks) and are excluded here;
      // they are covered by E2E tests (Playwright).
      include: [
        'src/lib/utils.ts',
        'src/lib/sse.ts',
        'src/components/shared/**/*.tsx',
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
