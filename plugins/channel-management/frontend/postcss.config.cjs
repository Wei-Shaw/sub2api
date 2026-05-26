/* eslint-env node */
// Vite auto-detects this file; running `pnpm build` will pipe CSS through
// tailwindcss + autoprefixer so plugin-side primary-/accent-/dark-/shadow-
// classes are emitted into dist/entry.css.
module.exports = {
  plugins: {
    tailwindcss: {},
    autoprefixer: {}
  }
}
