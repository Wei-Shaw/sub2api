/** @type {import('tailwindcss').Config} */
const path = require('path')
const sdkPath = path.dirname(require.resolve('@sub2api/plugin-sdk/package.json'))

module.exports = {
  presets: [require('@sub2api/plugin-sdk/tailwind-preset.cjs')],
  content: [
    './src/**/*.{vue,js,ts,jsx,tsx}',
    './index.html',
    path.join(sdkPath, 'src/**/*.{vue,js,ts,jsx,tsx}'),
  ],
  darkMode: 'class',
}
