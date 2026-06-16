/**
 * Plugin frontend bundle config (gateway-openai).
 *
 * Output:
 *   - dist/entry.js  — single-file ES bundle, default export { install(sdk) }
 *   - dist/entry.css — bundled CSS
 *
 * Externals (provided by host importmap):
 *   - vue / vue-router / vue-i18n / pinia / axios
 *   - @sub2api/plugin-sdk
 */
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  define: {
    __INTLIFY_JIT_COMPILATION__: true,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    cssCodeSplit: false,
    cssMinify: true,
    minify: 'esbuild',
    target: 'es2020',
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      formats: ['es'],
      fileName: () => 'entry.js',
    },
    rollupOptions: {
      external: [
        'vue',
        'vue-router',
        'vue-i18n',
        'pinia',
        'axios',
        '@sub2api/plugin-sdk',
      ],
      output: {
        inlineDynamicImports: true,
        assetFileNames: (assetInfo) => {
          if (assetInfo.name && assetInfo.name.endsWith('.css')) {
            return 'entry.css'
          }
          return 'assets/[name][extname]'
        },
      },
    },
  },
})
