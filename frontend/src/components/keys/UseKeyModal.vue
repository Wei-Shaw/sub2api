<template>
  <BaseDialog
    :show="show"
    :title="t('keys.useKeyModal.title')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <!-- No Group Assigned Warning -->
      <div v-if="!platform" class="flex items-start gap-3 p-4 rounded-lg bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800">
        <svg class="w-5 h-5 text-yellow-500 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
        </svg>
        <div>
          <p class="text-sm font-medium text-yellow-800 dark:text-yellow-200">
            {{ t('keys.useKeyModal.noGroupTitle') REDACTEDREDACTED
          </p>
          <p class="text-sm text-yellow-700 dark:text-yellow-300 mt-1">
            {{ t('keys.useKeyModal.noGroupDescription') REDACTEDREDACTED
          </p>
        </div>
      </div>

      <!-- Platform-specific content -->
      <template v-else>
        <!-- Description -->
        <p class="text-sm text-gray-600 dark:text-gray-400">
          {{ platformDescription REDACTEDREDACTED
        </p>

        <!-- Client Tabs (only for Antigravity platform) -->
        <div v-if="platform === 'antigravity'" class="border-b border-gray-200 dark:border-dark-700">
          <nav class="-mb-px flex space-x-6" aria-label="Client">
            <button
              v-for="tab in clientTabs"
              :key="tab.id"
              @click="activeClientTab = tab.id"
              :class="[
                'whitespace-nowrap py-2.5 px-1 border-b-2 font-medium text-sm transition-colors',
                activeClientTab === tab.id
                  ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300'
              ]"
            >
              <span class="flex items-center gap-2">
                <component :is="tab.icon" class="w-4 h-4" />
                {{ tab.label REDACTEDREDACTED
              </span>
            </button>
          </nav>
        </div>

        <!-- OS/Shell Tabs -->
        <div class="border-b border-gray-200 dark:border-dark-700">
          <nav class="-mb-px flex space-x-4" aria-label="Tabs">
            <button
              v-for="tab in currentTabs"
              :key="tab.id"
              @click="activeTab = tab.id"
              :class="[
                'whitespace-nowrap py-2.5 px-1 border-b-2 font-medium text-sm transition-colors',
                activeTab === tab.id
                  ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300'
              ]"
            >
              <span class="flex items-center gap-2">
                <component :is="tab.icon" class="w-4 h-4" />
                {{ tab.label REDACTEDREDACTED
              </span>
            </button>
          </nav>
        </div>

        <!-- Code Blocks (Stacked for multi-file platforms) -->
        <div class="space-y-4">
          <div
            v-for="(file, index) in currentFiles"
            :key="index"
            class="relative"
          >
            <!-- File Hint (if exists) -->
            <p v-if="file.hint" class="text-xs text-amber-600 dark:text-amber-400 mb-1.5 flex items-center gap-1">
              <svg class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
              </svg>
              {{ file.hint REDACTEDREDACTED
            </p>
            <div class="bg-gray-900 dark:bg-dark-900 rounded-xl overflow-hidden">
              <!-- Code Header -->
              <div class="flex items-center justify-between px-4 py-2 bg-gray-800 dark:bg-dark-800 border-b border-gray-700 dark:border-dark-700">
                <span class="text-xs text-gray-400 font-mono">{{ file.path REDACTEDREDACTED</span>
                <button
                  @click="copyContent(file.content, index)"
                  class="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-lg transition-colors"
                  :class="copiedIndex === index
                    ? 'bg-green-500/20 text-green-400'
                    : 'bg-gray-700 hover:bg-gray-600 text-gray-300 hover:text-white'"
                >
                  <svg v-if="copiedIndex === index" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                  <svg v-else class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
                  </svg>
                  {{ copiedIndex === index ? t('keys.useKeyModal.copied') : t('keys.useKeyModal.copy') REDACTEDREDACTED
                </button>
              </div>
              <!-- Code Content -->
              <pre class="p-4 text-sm font-mono text-gray-100 overflow-x-auto"><code v-text="file.content"></code></pre>
            </div>
          </div>
        </div>

        <!-- Usage Note -->
        <div class="flex items-start gap-3 p-3 rounded-lg bg-blue-50 dark:bg-blue-900/20 border border-blue-100 dark:border-blue-800">
          <svg class="w-5 h-5 text-blue-500 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M11.25 11.25l.041-.02a.75.75 0 011.063.852l-.708 2.836a.75.75 0 001.063.853l.041-.021M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3.75h.008v.008H12V8.25z" />
          </svg>
          <p class="text-sm text-blue-700 dark:text-blue-300">
            {{ platformNote REDACTEDREDACTED
          </p>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button
          @click="emit('close')"
          class="btn btn-secondary"
        >
          {{ t('common.close') REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, h, watch, type Component REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useClipboard REDACTED from '@/composables/useClipboard'
import type { GroupPlatform REDACTED from '@/types'

interface Props {
  show: boolean
  apiKey: string
  baseUrl: string
  platform: GroupPlatform | null
REDACTED

interface Emits {
  (e: 'close'): void
REDACTED

interface TabConfig {
  id: string
  label: string
  icon: Component
REDACTED

interface FileConfig {
  path: string
  content: string
  hint?: string  // Optional hint message for this file
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t REDACTED = useI18n()
const { copyToClipboard: clipboardCopy REDACTED = useClipboard()

const copiedIndex = ref<number | null>(null)
const activeTab = ref<string>('unix')
const activeClientTab = ref<string>('claude')  // Level 1 tab for antigravity platform

// Reset tabs when platform changes
watch(() => props.platform, (newPlatform) => {
  activeTab.value = 'unix'
  if (newPlatform === 'antigravity') {
    activeClientTab.value = 'claude'
  REDACTED
REDACTED)

// Reset shell tab when client changes (for antigravity)
watch(activeClientTab, () => {
  activeTab.value = 'unix'
REDACTED)

// Icon components
const AppleIcon = {
  render() {
    return h('svg', {
      fill: 'currentColor',
      viewBox: '0 0 24 24',
      class: 'w-4 h-4'
    REDACTED, [
      h('path', { d: 'M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z' REDACTED)
    ])
  REDACTED
REDACTED

const WindowsIcon = {
  render() {
    return h('svg', {
      fill: 'currentColor',
      viewBox: '0 0 24 24',
      class: 'w-4 h-4'
    REDACTED, [
      h('path', { d: 'M3 12V6.75l6-1.32v6.48L3 12zm17-9v8.75l-10 .15V5.21L20 3zM3 13l6 .09v6.81l-6-1.15V13zm7 .25l10 .15V21l-10-1.91v-5.84z' REDACTED)
    ])
  REDACTED
REDACTED

// Terminal icon for Claude Code
const TerminalIcon = {
  render() {
    return h('svg', {
      fill: 'none',
      stroke: 'currentColor',
      viewBox: '0 0 24 24',
      'stroke-width': '1.5',
      class: 'w-4 h-4'
    REDACTED, [
      h('path', {
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round',
        d: 'm6.75 7.5 3 2.25-3 2.25m4.5 0h3m-9 8.25h13.5A2.25 2.25 0 0 0 21 17.25V6.75A2.25 2.25 0 0 0 18.75 4.5H5.25A2.25 2.25 0 0 0 3 6.75v10.5A2.25 2.25 0 0 0 5.25 20.25Z'
      REDACTED)
    ])
  REDACTED
REDACTED

// Sparkle icon for Gemini
const SparkleIcon = {
  render() {
    return h('svg', {
      fill: 'none',
      stroke: 'currentColor',
      viewBox: '0 0 24 24',
      'stroke-width': '1.5',
      class: 'w-4 h-4'
    REDACTED, [
      h('path', {
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round',
        d: 'M9.813 15.904 9 18.75l-.813-2.846a4.5 4.5 0 0 0-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 0 0 3.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 0 0 3.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 0 0-3.09 3.09ZM18.259 8.715 18 9.75l-.259-1.035a3.375 3.375 0 0 0-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 0 0 2.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 0 0 2.456 2.456L21.75 6l-1.035.259a3.375 3.375 0 0 0-2.456 2.456ZM16.894 20.567 16.5 21.75l-.394-1.183a2.25 2.25 0 0 0-1.423-1.423L13.5 18.75l1.183-.394a2.25 2.25 0 0 0 1.423-1.423l.394-1.183.394 1.183a2.25 2.25 0 0 0 1.423 1.423l1.183.394-1.183.394a2.25 2.25 0 0 0-1.423 1.423Z'
      REDACTED)
    ])
  REDACTED
REDACTED

// Client tabs for Antigravity platform (Level 1)
const clientTabs = computed((): TabConfig[] => [
  { id: 'claude', label: t('keys.useKeyModal.antigravity.claudeCode'), icon: TerminalIcon REDACTED,
  { id: 'gemini', label: t('keys.useKeyModal.antigravity.geminiCli'), icon: SparkleIcon REDACTED
])

// Shell tabs (3 types for environment variable based configs)
const shellTabs: TabConfig[] = [
  { id: 'unix', label: 'macOS / Linux', icon: AppleIcon REDACTED,
  { id: 'cmd', label: 'Windows CMD', icon: WindowsIcon REDACTED,
  { id: 'powershell', label: 'PowerShell', icon: WindowsIcon REDACTED
]

// OpenAI tabs (2 OS types)
const openaiTabs: TabConfig[] = [
  { id: 'unix', label: 'macOS / Linux', icon: AppleIcon REDACTED,
  { id: 'windows', label: 'Windows', icon: WindowsIcon REDACTED
]

const currentTabs = computed(() => {
  if (props.platform === 'openai') {
    return openaiTabs  // 2 tabs: unix, windows
  REDACTED
  // All other platforms (anthropic, gemini, antigravity) use shell tabs
  return shellTabs
REDACTED)

const platformDescription = computed(() => {
  switch (props.platform) {
    case 'openai':
      return t('keys.useKeyModal.openai.description')
    case 'gemini':
      return t('keys.useKeyModal.gemini.description')
    case 'antigravity':
      return t('keys.useKeyModal.antigravity.description')
    default:
      return t('keys.useKeyModal.description')
  REDACTED
REDACTED)

const platformNote = computed(() => {
  switch (props.platform) {
    case 'openai':
      return activeTab.value === 'windows'
        ? t('keys.useKeyModal.openai.noteWindows')
        : t('keys.useKeyModal.openai.note')
    case 'gemini':
      return t('keys.useKeyModal.gemini.note')
    case 'antigravity':
      return activeClientTab.value === 'claude'
        ? t('keys.useKeyModal.antigravity.claudeNote')
        : t('keys.useKeyModal.antigravity.geminiNote')
    default:
      return t('keys.useKeyModal.note')
  REDACTED
REDACTED)

// Syntax highlighting helpers
// Generate file configs based on platform and active tab
const currentFiles = computed((): FileConfig[] => {
  const baseUrl = props.baseUrl || window.location.origin
  const apiKey = props.apiKey

  switch (props.platform) {
    case 'openai':
      return generateOpenAIFiles(baseUrl, apiKey)
    case 'gemini':
      return [generateGeminiCliContent(baseUrl, apiKey)]
    case 'antigravity':
      // Both Claude Code and Gemini CLI need /antigravity suffix for antigravity platform
      if (activeClientTab.value === 'claude') {
        return generateAnthropicFiles(`${baseUrlREDACTED/antigravity`, apiKey)
      REDACTED
      return [generateGeminiCliContent(`${baseUrlREDACTED/antigravity`, apiKey)]
    default: // anthropic
      return generateAnthropicFiles(baseUrl, apiKey)
  REDACTED
REDACTED)

function generateAnthropicFiles(baseUrl: string, apiKey: string): FileConfig[] {
  let path: string
  let content: string

  switch (activeTab.value) {
    case 'unix':
      path = 'Terminal'
      content = `export ANTHROPIC_BASE_URL="${baseUrlREDACTED"
export ANTHROPIC_AUTH_TOKEN="${apiKeyREDACTED"`
      break
    case 'cmd':
      path = 'Command Prompt'
      content = `set ANTHROPIC_BASE_URL=${baseUrlREDACTED
set ANTHROPIC_AUTH_TOKEN=${apiKeyREDACTED`
      break
    case 'powershell':
      path = 'PowerShell'
      content = `$env:ANTHROPIC_BASE_URL="${baseUrlREDACTED"
$env:ANTHROPIC_AUTH_TOKEN="${apiKeyREDACTED"`
      break
    default:
      path = 'Terminal'
      content = ''
  REDACTED

  return [{ path, content REDACTED]
REDACTED

function generateGeminiCliContent(baseUrl: string, apiKey: string): FileConfig {
  const model = 'gemini-2.5-pro'
  const modelComment = t('keys.useKeyModal.gemini.modelComment')
  let path: string
  let content: string
  let highlighted: string

  switch (activeTab.value) {
    case 'unix':
      path = 'Terminal'
      content = `export GOOGLE_GEMINI_BASE_URL="${baseUrlREDACTED"
export GEMINI_API_KEY="${apiKeyREDACTED"
export GEMINI_MODEL="${modelREDACTED"  # ${modelCommentREDACTED`
      highlighted = `${keyword('export')REDACTED ${variable('GOOGLE_GEMINI_BASE_URL')REDACTED${operator('=')REDACTED${string(`"${baseUrlREDACTED"`)REDACTED
${keyword('export')REDACTED ${variable('GEMINI_API_KEY')REDACTED${operator('=')REDACTED${string(`"${apiKeyREDACTED"`)REDACTED
${keyword('export')REDACTED ${variable('GEMINI_MODEL')REDACTED${operator('=')REDACTED${string(`"${modelREDACTED"`)REDACTED  ${comment(`# ${modelCommentREDACTED`)REDACTED`
      break
    case 'cmd':
      path = 'Command Prompt'
      content = `set GOOGLE_GEMINI_BASE_URL=${baseUrlREDACTED
set GEMINI_API_KEY=${apiKeyREDACTED
set GEMINI_MODEL=${modelREDACTED`
      highlighted = `${keyword('set')REDACTED ${variable('GOOGLE_GEMINI_BASE_URL')REDACTED${operator('=')REDACTED${baseUrlREDACTED
${keyword('set')REDACTED ${variable('GEMINI_API_KEY')REDACTED${operator('=')REDACTED${apiKeyREDACTED
${keyword('set')REDACTED ${variable('GEMINI_MODEL')REDACTED${operator('=')REDACTED${modelREDACTED
${comment(`REM ${modelCommentREDACTED`)REDACTED`
      break
    case 'powershell':
      path = 'PowerShell'
      content = `$env:GOOGLE_GEMINI_BASE_URL="${baseUrlREDACTED"
$env:GEMINI_API_KEY="${apiKeyREDACTED"
$env:GEMINI_MODEL="${modelREDACTED"  # ${modelCommentREDACTED`
      highlighted = `${keyword('$env:')REDACTED${variable('GOOGLE_GEMINI_BASE_URL')REDACTED${operator('=')REDACTED${string(`"${baseUrlREDACTED"`)REDACTED
${keyword('$env:')REDACTED${variable('GEMINI_API_KEY')REDACTED${operator('=')REDACTED${string(`"${apiKeyREDACTED"`)REDACTED
${keyword('$env:')REDACTED${variable('GEMINI_MODEL')REDACTED${operator('=')REDACTED${string(`"${modelREDACTED"`)REDACTED  ${comment(`# ${modelCommentREDACTED`)REDACTED`
      break
    default:
      path = 'Terminal'
      content = ''
      highlighted = ''
  REDACTED

  return { path, content, highlighted REDACTED
REDACTED

function generateOpenAIFiles(baseUrl: string, apiKey: string): FileConfig[] {
  const isWindows = activeTab.value === 'windows'
  const configDir = isWindows ? '%userprofile%\\.codex' : '~/.codex'

  // config.toml content
  const configContent = `model_provider = "sub2api"
model = "gpt-5.2-codex"
model_reasoning_effort = "high"
network_access = "enabled"
disable_response_storage = true
windows_wsl_setup_acknowledged = true
model_verbosity = "high"

[model_providers.sub2api]
name = "sub2api"
base_url = "${baseUrlREDACTED"
wire_api = "responses"
requires_openai_auth = true`

  // auth.json content
  const authContent = `{
  "OPENAI_API_KEY": "${apiKeyREDACTED"
REDACTED`

  return [
    {
      path: `${configDirREDACTED/config.toml`,
      content: configContent,
      hint: t('keys.useKeyModal.openai.configTomlHint')
    REDACTED,
    {
      path: `${configDirREDACTED/auth.json`,
      content: authContent
    REDACTED
  ]
REDACTED

const copyContent = async (content: string, index: number) => {
  const success = await clipboardCopy(content, t('keys.copied'))
  if (success) {
    copiedIndex.value = index
    setTimeout(() => {
      copiedIndex.value = null
    REDACTED, 2000)
  REDACTED
REDACTED
</script>
