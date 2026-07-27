<template>
  <div class="relative" ref="containerRef">
    <button
      type="button"
      @click="toggle"
      :disabled="disabled"
      :class="[
        'select-trigger',
        isOpen && 'select-trigger-open',
        disabled && 'select-trigger-disabled'
      ]"
      :data-testid="mode === 'group' ? 'proxy-group-selector' : 'proxy-selector'"
    >
      <span class="select-value">
        {{ selectedLabel }}
      </span>
      <span class="select-icon">
        <Icon
          name="chevronDown"
          size="md"
          :class="['transition-transform duration-200', isOpen && 'rotate-180']"
        />
      </span>
    </button>

    <Transition name="select-dropdown">
      <div v-if="isOpen" class="select-dropdown">
        <!-- Search and Batch Test Header -->
        <div class="select-header">
          <div class="select-search">
            <Icon name="search" size="sm" class="text-gray-400" />
            <input
              ref="searchInputRef"
              v-model="searchQuery"
              type="text"
              :placeholder="
                mode === 'group'
                  ? t('admin.proxyGroups.search')
                  : t('admin.proxies.searchProxies')
              "
              class="select-search-input"
              @click.stop
            />
          </div>
          <button
            v-if="mode === 'proxy' && proxies.length > 0"
            type="button"
            @click.stop="handleBatchTest"
            :disabled="batchTesting"
            class="batch-test-btn"
            :title="t('admin.proxies.batchTest')"
          >
            <svg v-if="batchTesting" class="h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            <Icon v-else name="play" size="sm" />
          </button>
        </div>

        <!-- Options list -->
        <div class="select-options">
          <!-- No selection option -->
          <div
            @click="selectOption(null)"
            :class="['select-option', isSelected(null) && 'select-option-selected']"
            data-testid="proxy-selector-none"
          >
            <span class="select-option-label">{{
              mode === 'group' ? t('admin.accounts.noProxyGroup') : t('admin.accounts.noProxy')
            }}</span>
            <Icon v-if="isSelected(null)" name="check" size="sm" class="text-primary-500" />
          </div>

          <!-- Proxy options -->
          <template v-if="mode === 'proxy'">
            <div
              v-for="proxy in filteredProxies"
              :key="`proxy-${proxy.id}`"
              @click="selectOption(proxy.id)"
              :class="['select-option', isSelected(proxy.id) && 'select-option-selected']"
              :data-testid="`proxy-option-${proxy.id}`"
            >
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <span class="truncate font-medium">{{ proxy.name }}</span>
                  <span
                    v-if="proxy.account_count !== undefined"
                    class="inline-flex flex-shrink-0 items-center rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-dark-600 dark:text-gray-400"
                  >
                    {{ proxy.account_count }}
                  </span>
                  <template v-if="testResults[proxy.id]">
                    <span
                      v-if="testResults[proxy.id].success"
                      class="inline-flex flex-shrink-0 items-center gap-1 rounded bg-emerald-100 px-1.5 py-0.5 text-xs text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400"
                    >
                      <span v-if="testResults[proxy.id].country">{{
                        testResults[proxy.id].country
                      }}</span>
                      <span v-if="testResults[proxy.id].latency_ms"
                        >{{ testResults[proxy.id].latency_ms }}ms</span
                      >
                    </span>
                    <span
                      v-else
                      class="inline-flex flex-shrink-0 items-center rounded bg-red-100 px-1.5 py-0.5 text-xs text-red-700 dark:bg-red-900/30 dark:text-red-400"
                    >
                      {{ t('admin.proxies.testFailed') }}
                    </span>
                  </template>
                </div>
                <div class="truncate text-xs text-gray-500 dark:text-gray-400">
                  {{ proxy.protocol }}://{{ proxy.host }}:{{ proxy.port }}
                </div>
              </div>

              <button
                type="button"
                @click.stop="handleTestProxy(proxy)"
                :disabled="testingProxyIds.has(proxy.id)"
                class="test-btn"
                :title="t('admin.proxies.testConnection')"
              >
                <svg
                  v-if="testingProxyIds.has(proxy.id)"
                  class="h-3.5 w-3.5 animate-spin"
                  fill="none"
                  viewBox="0 0 24 24"
                >
                  <circle
                    class="opacity-25"
                    cx="12"
                    cy="12"
                    r="10"
                    stroke="currentColor"
                    stroke-width="4"
                  ></circle>
                  <path
                    class="opacity-75"
                    fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                  ></path>
                </svg>
                <Icon v-else name="play" size="xs" />
              </button>

              <Icon
                v-if="isSelected(proxy.id)"
                name="check"
                size="sm"
                class="flex-shrink-0 text-primary-500"
              />
            </div>

            <div v-if="filteredProxies.length === 0 && searchQuery" class="select-empty">
              {{ t('common.noOptionsFound') }}
            </div>
          </template>

          <!-- Proxy group options -->
          <template v-else>
            <div
              v-for="group in filteredGroups"
              :key="`group-${group.id}`"
              @click="selectOption(group.id)"
              :class="['select-option', isSelected(group.id) && 'select-option-selected']"
              :data-testid="`proxy-group-option-${group.id}`"
            >
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <span class="truncate font-medium">{{ group.name }}</span>
                  <span
                    v-if="group.proxy_count !== undefined"
                    class="inline-flex flex-shrink-0 items-center rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-dark-600 dark:text-gray-400"
                  >
                    {{ group.proxy_count }}
                  </span>
                  <span
                    v-if="group.sticky_by_account"
                    class="inline-flex flex-shrink-0 items-center rounded bg-amber-100 px-1.5 py-0.5 text-xs text-amber-700 dark:bg-amber-900/30 dark:text-amber-400"
                  >
                    sticky
                  </span>
                </div>
                <div class="truncate text-xs text-gray-500 dark:text-gray-400">
                  {{ group.strategy }}
                  <span v-if="group.description"> · {{ group.description }}</span>
                </div>
              </div>
              <Icon
                v-if="isSelected(group.id)"
                name="check"
                size="sm"
                class="flex-shrink-0 text-primary-500"
              />
            </div>

            <div v-if="filteredGroups.length === 0 && searchQuery" class="select-empty">
              {{ t('common.noOptionsFound') }}
            </div>
            <div v-else-if="groups.length === 0" class="select-empty">
              {{ t('admin.proxyGroups.emptyTitle') }}
            </div>
          </template>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import Icon from '@/components/icons/Icon.vue'
import type { Proxy, ProxyGroup } from '@/types'

const { t } = useI18n()

interface ProxyTestResult {
  success: boolean
  message: string
  latency_ms?: number
  ip_address?: string
  city?: string
  region?: string
  country?: string
}

export type ProxySelectorMode = 'proxy' | 'group'

interface Props {
  /** 选中的代理/组 ID；支持 number 与数字字符串的宽松相等（避免 v-model 类型漂移导致高亮丢失） */
  modelValue: number | string | null
  proxies?: Proxy[]
  groups?: ProxyGroup[]
  /** proxy=单代理选择（默认）；group=代理池选择 */
  mode?: ProxySelectorMode
  disabled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  proxies: () => [],
  groups: () => [],
  mode: 'proxy',
  disabled: false
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

const isOpen = ref(false)
const searchQuery = ref('')
const containerRef = ref<HTMLElement | null>(null)
const searchInputRef = ref<HTMLInputElement | null>(null)

const testResults = reactive<Record<number, ProxyTestResult>>({})
const testingProxyIds = reactive(new Set<number>())
const batchTesting = ref(false)

/** 宽松 ID 比较：null 仅匹配 null/undefined；否则 Number 相等（"12" === 12） */
function sameId(a: number | string | null | undefined, b: number | string | null | undefined): boolean {
  if (a === null || a === undefined) {
    return b === null || b === undefined
  }
  if (b === null || b === undefined) {
    return false
  }
  const na = Number(a)
  const nb = Number(b)
  if (Number.isNaN(na) || Number.isNaN(nb)) {
    return String(a) === String(b)
  }
  return na === nb
}

function isSelected(id: number | null): boolean {
  return sameId(props.modelValue, id)
}

const selectedProxy = computed(() => {
  if (props.mode !== 'proxy' || props.modelValue === null || props.modelValue === undefined) {
    return null
  }
  return props.proxies.find((p) => sameId(p.id, props.modelValue)) || null
})

const selectedGroup = computed(() => {
  if (props.mode !== 'group' || props.modelValue === null || props.modelValue === undefined) {
    return null
  }
  return props.groups.find((g) => sameId(g.id, props.modelValue)) || null
})

const selectedLabel = computed(() => {
  if (props.mode === 'group') {
    if (!selectedGroup.value) {
      return t('admin.accounts.noProxyGroup')
    }
    const g = selectedGroup.value
    const sticky = g.sticky_by_account ? ' · sticky' : ''
    return `${g.name}${sticky} (${g.strategy})`
  }
  if (!selectedProxy.value) {
    return t('admin.accounts.noProxy')
  }
  const proxy = selectedProxy.value
  return `${proxy.name} (${proxy.protocol}://${proxy.host}:${proxy.port})`
})

const filteredProxies = computed(() => {
  if (!searchQuery.value) {
    return props.proxies
  }
  const query = searchQuery.value.toLowerCase()
  return props.proxies.filter((proxy) => {
    const name = proxy.name.toLowerCase()
    const host = proxy.host.toLowerCase()
    return name.includes(query) || host.includes(query)
  })
})

const filteredGroups = computed(() => {
  if (!searchQuery.value) {
    return props.groups
  }
  const query = searchQuery.value.toLowerCase()
  return props.groups.filter((group) => {
    const name = group.name.toLowerCase()
    const desc = (group.description || '').toLowerCase()
    const strategy = (group.strategy || '').toLowerCase()
    return name.includes(query) || desc.includes(query) || strategy.includes(query)
  })
})

const toggle = () => {
  if (props.disabled) return
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    nextTick(() => {
      searchInputRef.value?.focus()
    })
  }
}

const selectOption = (value: number | null) => {
  emit('update:modelValue', value)
  isOpen.value = false
  searchQuery.value = ''
}

const handleTestProxy = async (proxy: Proxy) => {
  if (testingProxyIds.has(proxy.id)) return

  testingProxyIds.add(proxy.id)
  try {
    const result = await adminAPI.proxies.testProxy(proxy.id)
    testResults[proxy.id] = result
  } catch (error: any) {
    testResults[proxy.id] = {
      success: false,
      message: error.response?.data?.detail || 'Test failed'
    }
  } finally {
    testingProxyIds.delete(proxy.id)
  }
}

const handleBatchTest = async () => {
  if (batchTesting.value || props.proxies.length === 0) return

  batchTesting.value = true

  const testPromises = props.proxies.map(async (proxy) => {
    testingProxyIds.add(proxy.id)
    try {
      const result = await adminAPI.proxies.testProxy(proxy.id)
      testResults[proxy.id] = result
    } catch (error: any) {
      testResults[proxy.id] = {
        success: false,
        message: error.response?.data?.detail || 'Test failed'
      }
    } finally {
      testingProxyIds.delete(proxy.id)
    }
  })

  await Promise.all(testPromises)
  batchTesting.value = false
}

const handleClickOutside = (event: MouseEvent) => {
  if (containerRef.value && !containerRef.value.contains(event.target as Node)) {
    isOpen.value = false
    searchQuery.value = ''
  }
}

const handleEscape = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && isOpen.value) {
    isOpen.value = false
    searchQuery.value = ''
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleEscape)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleEscape)
})
</script>

<style scoped>
.select-trigger {
  @apply flex w-full items-center justify-between gap-2;
  @apply rounded-xl px-4 py-2.5 text-sm;
  @apply bg-white dark:bg-dark-800;
  @apply border border-gray-200 dark:border-dark-600;
  @apply text-gray-900 dark:text-gray-100;
  @apply transition-all duration-200;
  @apply hover:border-gray-300 dark:hover:border-dark-500;
  @apply focus:outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500;
}

.select-trigger-open {
  @apply border-primary-500 ring-2 ring-primary-500/20;
}

.select-trigger-disabled {
  @apply cursor-not-allowed opacity-50;
}

.select-value {
  @apply truncate text-left;
}

.select-icon {
  @apply flex-shrink-0 text-gray-400 dark:text-dark-400;
}

.select-dropdown {
  @apply absolute z-[100] mt-2 w-full;
  @apply bg-white dark:bg-dark-800;
  @apply rounded-xl;
  @apply border border-gray-200 dark:border-dark-700;
  @apply shadow-lg shadow-black/10 dark:shadow-black/30;
  @apply overflow-hidden;
}

.select-header {
  @apply flex items-center gap-2 px-3 py-2;
  @apply border-b border-gray-100 dark:border-dark-700;
}

.select-search {
  @apply flex flex-1 items-center gap-2;
}

.select-search-input {
  @apply flex-1 bg-transparent text-sm;
  @apply text-gray-900 dark:text-gray-100;
  @apply placeholder:text-gray-400 dark:placeholder:text-dark-400;
  @apply focus:outline-none;
}

.batch-test-btn {
  @apply flex-shrink-0 rounded-lg p-1.5;
  @apply text-gray-500 hover:text-emerald-600 dark:hover:text-emerald-400;
  @apply hover:bg-emerald-50 dark:hover:bg-emerald-900/20;
  @apply transition-colors disabled:cursor-not-allowed disabled:opacity-50;
}

.select-options {
  @apply max-h-60 overflow-y-auto py-1;
}

.select-option {
  @apply flex items-center justify-between gap-2;
  @apply px-4 py-2.5 text-sm;
  @apply text-gray-700 dark:text-gray-300;
  @apply cursor-pointer transition-colors duration-150;
  @apply hover:bg-gray-50 dark:hover:bg-dark-700;
}

.select-option-selected {
  @apply bg-primary-50 dark:bg-primary-900/20;
  @apply text-primary-700 dark:text-primary-300;
}

.select-option-label {
  @apply truncate;
}

.select-empty {
  @apply px-4 py-8 text-center text-sm;
  @apply text-gray-500 dark:text-dark-400;
}

.test-btn {
  @apply flex-shrink-0 rounded p-1;
  @apply text-gray-400 hover:text-emerald-600 dark:hover:text-emerald-400;
  @apply hover:bg-emerald-50 dark:hover:bg-emerald-900/20;
  @apply transition-colors disabled:cursor-not-allowed disabled:opacity-50;
}

.select-dropdown-enter-active,
.select-dropdown-leave-active {
  transition: all 0.2s ease;
}

.select-dropdown-enter-from,
.select-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
