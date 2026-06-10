/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL: string
  readonly BASE_URL: string
REDACTED

interface ImportMeta {
  readonly env: ImportMetaEnv
REDACTED

declare module '*.vue' {
  import type { DefineComponent REDACTED from 'vue'
  const component: DefineComponent<{REDACTED, {REDACTED, any>
  export default component
REDACTED

declare module '*.md?raw' {
  const content: string
  export default content
REDACTED
