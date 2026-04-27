/** @type {import('tailwindcss').Config} */
// Plugin Tailwind config consumes the shared preset from @sub2api/plugin-sdk
// so colors / shadows / animations stay aligned with the host frontend.
// Do NOT redeclare design tokens here — change them in the shared preset.
module.exports = {
  presets: [require('@sub2api/plugin-sdk/tailwind-preset.cjs')],
  content: ['./src/**/*.{vue,js,ts,jsx,tsx}', './index.html'],
  darkMode: 'class'
}
