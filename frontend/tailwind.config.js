/** @type {import('tailwindcss').Config} */
// Host Tailwind configuration. All design tokens live in the shared preset
// (frontend/packages/plugin-sdk/tailwind-preset.cjs) so plugin frontends pull
// from the same source of truth. Do NOT add token overrides here without also
// updating the preset, otherwise host and plugins will visually drift.
import preset from './packages/plugin-sdk/tailwind-preset.cjs'

export default {
  presets: [preset],
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class'
}
