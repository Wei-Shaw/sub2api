<template>
  <div class="space-y-6">
    <!-- Active Theme Banner -->
    <div v-if="activeTheme" class="card">
      <div class="card-body flex items-center justify-between">
        <div class="flex items-center gap-4">
          <div
            v-if="activeTheme.metadata.preview"
            class="h-16 w-16 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700"
          >
            <img
              :src="`/api/v1/themes/assets/${activeTheme.metadata.short}/${activeTheme.metadata.preview}`"
              :alt="activeTheme.metadata.name"
              class="h-full w-full object-cover"
            />
          </div>
          <div
            v-else
            class="flex h-16 w-16 items-center justify-center rounded-lg bg-primary-100 dark:bg-primary-900/30"
          >
            <Icon name="paintBrush" size="lg" class="text-primary-600 dark:text-primary-400" />
          </div>
          <div>
            <div class="flex items-center gap-2">
              <span class="badge badge-success">{{ t('admin.settings.theme.activeTheme') }}</span>
              <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ activeTheme.metadata.name }}
              </h3>
            </div>
            <p class="mt-0.5 text-sm text-gray-500 dark:text-dark-400">
              {{ t('admin.settings.theme.version') }} {{ activeTheme.metadata.version }}
              <span v-if="activeTheme.metadata.author">
                · {{ activeTheme.metadata.author }}
              </span>
            </p>
            <p
              v-if="activeTheme.metadata.description"
              class="mt-1 text-xs text-gray-400 dark:text-dark-500"
            >
              {{ activeTheme.metadata.description }}
            </p>
          </div>
        </div>
        <button @click="handleDeactivate" class="btn btn-secondary btn-sm">
          {{ t('admin.settings.theme.deactivate') }}
        </button>
      </div>
    </div>

    <!-- No Active Theme -->
    <div v-else class="card">
      <div class="card-body text-center py-8">
        <Icon name="paintBrush" size="xl" class="mx-auto mb-3 text-gray-300 dark:text-dark-600" />
        <p class="text-sm text-gray-500 dark:text-dark-400">
          {{ t('admin.settings.theme.noActiveTheme') }}
        </p>
      </div>
    </div>

    <!-- Install Section -->
    <div class="card">
      <div class="card-header">
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.theme.install') }}
        </h3>
      </div>
      <div class="card-body space-y-4">
        <!-- Zip Upload -->
        <div>
          <label class="input-label">{{ t('admin.settings.theme.installFromZip') }}</label>
          <div
            @dragover.prevent="dragOver = true"
            @dragleave="dragOver = false"
            @drop.prevent="handleDrop"
            @click="fileInputRef?.click()"
            :class="[
              'mt-1 flex cursor-pointer flex-col items-center justify-center rounded-xl border-2 border-dashed p-6 transition-colors',
              dragOver
                ? 'border-primary-400 bg-primary-50 dark:border-primary-500 dark:bg-primary-900/20'
                : 'border-gray-300 hover:border-gray-400 dark:border-dark-600 dark:hover:border-dark-500',
            ]"
          >
            <Icon
              name="cloud"
              size="lg"
              class="mb-2 text-gray-400 dark:text-dark-500"
            />
            <p class="text-sm text-gray-600 dark:text-dark-300">
              {{ installing ? t('admin.settings.theme.installing') : t('admin.settings.theme.dragDropHint') }}
            </p>
            <input
              ref="fileInputRef"
              type="file"
              accept=".zip"
              class="hidden"
              @change="handleFileSelect"
            />
          </div>
        </div>

        <!-- GitHub URL -->
        <div>
          <label class="input-label">{{ t('admin.settings.theme.installFromGithub') }}</label>
          <div class="mt-1 flex gap-2">
            <input
              v-model="githubUrl"
              type="url"
              class="input flex-1"
              :placeholder="t('admin.settings.theme.githubUrlPlaceholder')"
              :disabled="installing"
            />
            <button
              @click="handleInstallFromGitHub"
              class="btn btn-primary btn-md"
              :disabled="!githubUrl.trim() || installing"
            >
              <Icon v-if="installing" name="refresh" size="sm" class="animate-spin" />
              {{ t('admin.settings.theme.install') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Installed Themes -->
    <div v-if="themes.length > 0" class="card">
      <div class="card-header">
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.theme.tabs.theme') }}
        </h3>
      </div>
      <div class="card-body">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
          <div
            v-for="theme in themes"
            :key="theme.metadata.short"
            class="group relative overflow-hidden rounded-xl border border-gray-200 transition-all hover:shadow-md dark:border-dark-700"
          >
            <!-- Preview Image -->
            <div
              v-if="theme.metadata.preview"
              class="aspect-video overflow-hidden bg-gray-100 dark:bg-dark-800"
            >
              <img
                :src="`/api/v1/themes/assets/${theme.metadata.short}/${theme.metadata.preview}`"
                :alt="theme.metadata.name"
                class="h-full w-full object-cover"
              />
            </div>
            <div
              v-else
              class="flex aspect-video items-center justify-center bg-gradient-to-br from-gray-100 to-gray-200 dark:from-dark-800 dark:to-dark-700"
            >
              <Icon name="paintBrush" size="xl" class="text-gray-300 dark:text-dark-600" />
            </div>

            <!-- Theme Info -->
            <div class="p-4">
              <div class="flex items-start justify-between">
                <div>
                  <h4 class="font-semibold text-gray-900 dark:text-white">
                    {{ theme.metadata.name }}
                  </h4>
                  <p class="text-xs text-gray-500 dark:text-dark-400">
                    {{ theme.metadata.version }}
                    <span v-if="theme.metadata.author"> · {{ theme.metadata.author }}</span>
                  </p>
                </div>
                <span v-if="theme.is_active" class="badge badge-success">
                  {{ t('admin.settings.theme.activeTheme') }}
                </span>
              </div>
              <p
                v-if="theme.metadata.description"
                class="mt-2 line-clamp-2 text-xs text-gray-400 dark:text-dark-500"
              >
                {{ theme.metadata.description }}
              </p>

              <!-- Actions -->
              <div class="mt-3 flex gap-2">
                <button
                  v-if="!theme.is_active"
                  @click="handleActivate(theme.metadata.short)"
                  class="btn btn-primary btn-sm flex-1"
                >
                  {{ t('admin.settings.theme.activate') }}
                </button>
                <button
                  @click="handleDelete(theme.metadata.short)"
                  class="btn btn-ghost btn-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                >
                  {{ t('admin.settings.theme.delete') }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- No Themes -->
    <div v-else-if="!loading" class="card">
      <div class="card-body text-center py-8">
        <p class="text-sm text-gray-500 dark:text-dark-400">
          {{ t('admin.settings.theme.noThemes') }}
        </p>
      </div>
    </div>

    <!-- Theme Config -->
    <div v-if="activeTheme?.metadata.config?.length" class="card">
      <div class="card-header">
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.theme.config') }}
        </h3>
      </div>
      <div class="card-body space-y-4">
        <div v-for="item in activeTheme.metadata.config" :key="item.key">
          <!-- Section title -->
          <template v-if="item.type === 'title'">
            <h4 class="mt-4 mb-2 text-sm font-semibold text-gray-700 dark:text-gray-300">
              {{ item.label }}
            </h4>
          </template>
          <!-- Config fields -->
          <template v-else>
            <!-- Boolean: checkbox with inline label, no separate label above -->
            <template v-if="item.type === 'boolean'">
              <label class="flex cursor-pointer items-center gap-2">
                <input
                  v-model="themeConfig[item.key]"
                  type="checkbox"
                  class="h-4 w-4 rounded text-primary-600"
                  true-value="true"
                  false-value="false"
                />
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ item.label }}</span>
              </label>
              <p v-if="item.description" class="input-hint ml-6">{{ item.description }}</p>
            </template>

            <!-- All other types: label + input -->
            <template v-else>
              <label class="input-label">{{ item.label }}</label>
              <p v-if="item.description" class="input-hint mb-1.5">{{ item.description }}</p>

              <!-- Color -->
              <input
                v-if="item.type === 'color'"
                v-model="themeConfig[item.key]"
                type="color"
                class="h-10 w-20 rounded-lg border border-gray-200 dark:border-dark-600"
              />

              <!-- Text -->
              <input
                v-else-if="item.type === 'text'"
                v-model="themeConfig[item.key]"
                type="text"
                class="input"
                :placeholder="item.default"
              />

              <!-- Number -->
              <input
                v-else-if="item.type === 'number'"
                v-model="themeConfig[item.key]"
                type="number"
                class="input"
                :placeholder="item.default"
              />

              <!-- Select -->
              <Select
                v-else-if="item.type === 'select'"
                :modelValue="themeConfig[item.key]"
                @update:modelValue="themeConfig[item.key] = $event as string"
                :options="(item.options || []).map(o => ({ value: o.value, label: o.label }))"
              />
            </template>
          </template>
        </div>

        <div class="flex justify-end pt-2">
          <button @click="handleSaveConfig" class="btn btn-primary btn-md">
            {{ t('admin.settings.theme.saveConfig') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('admin.settings.theme.delete')"
      :message="t('admin.settings.theme.deleteConfirm')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import type { Theme } from '@/api/admin/themes'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const { t } = useI18n()

const loading = ref(false)
const installing = ref(false)
const dragOver = ref(false)
const themes = ref<Theme[]>([])
const activeTheme = ref<Theme | null>(null)
const githubUrl = ref('')
const themeConfig = reactive<Record<string, string>>({})
const showDeleteConfirm = ref(false)
const deleteTarget = ref('')
const fileInputRef = ref<HTMLInputElement | null>(null)

async function loadThemes() {
  loading.value = true
  try {
    const [listRes, activeRes] = await Promise.all([
      adminAPI.themes.list(),
      adminAPI.themes.getActive(),
    ])
    themes.value = listRes.data.themes || []
    if (activeRes.data.active && activeRes.data.theme) {
      activeTheme.value = activeRes.data.theme
      // Initialize config with defaults
      if (activeRes.data.theme.metadata.config) {
        for (const item of activeRes.data.theme.metadata.config) {
          const existing = activeRes.data.theme.config?.[item.key]
          themeConfig[item.key] = existing ?? item.default ?? ''
        }
      }
    } else {
      activeTheme.value = null
    }
  } catch (e: any) {
    console.error('Failed to load themes:', e)
  } finally {
    loading.value = false
  }
}

function handleDrop(e: DragEvent) {
  dragOver.value = false
  const file = e.dataTransfer?.files?.[0]
  if (file && file.name.endsWith('.zip')) {
    installFile(file)
  }
}

function handleFileSelect(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) {
    installFile(file)
  }
  input.value = ''
}

async function installFile(file: File) {
  installing.value = true
  try {
    await adminAPI.themes.install(file)
    await loadThemes()
  } catch (e: any) {
    console.error('Failed to install theme:', e)
    alert(e.response?.data?.error || e.message)
  } finally {
    installing.value = false
  }
}

async function handleInstallFromGitHub() {
  if (!githubUrl.value.trim()) return
  installing.value = true
  try {
    await adminAPI.themes.installFromGitHub(githubUrl.value.trim())
    githubUrl.value = ''
    await loadThemes()
  } catch (e: any) {
    console.error('Failed to install theme from GitHub:', e)
    alert(e.response?.data?.error || e.message)
  } finally {
    installing.value = false
  }
}

async function handleActivate(short: string) {
  try {
    await adminAPI.themes.activate(short)
    await loadThemes()
  } catch (e: any) {
    console.error('Failed to activate theme:', e)
    alert(e.response?.data?.error || e.message)
  }
}

async function handleDeactivate() {
  try {
    await adminAPI.themes.deactivate()
    await loadThemes()
  } catch (e: any) {
    console.error('Failed to deactivate theme:', e)
    alert(e.response?.data?.error || e.message)
  }
}

function handleDelete(short: string) {
  deleteTarget.value = short
  showDeleteConfirm.value = true
}

async function confirmDelete() {
  showDeleteConfirm.value = false
  try {
    await adminAPI.themes.delete(deleteTarget.value)
    await loadThemes()
  } catch (e: any) {
    console.error('Failed to delete theme:', e)
    alert(e.response?.data?.error || e.message)
  }
}

async function handleSaveConfig() {
  if (!activeTheme.value) return
  try {
    await adminAPI.themes.updateConfig(activeTheme.value.metadata.short, { ...themeConfig })
    // Force reload to pick up new CSS
    window.location.reload()
  } catch (e: any) {
    console.error('Failed to save theme config:', e)
    alert(e.response?.data?.error || e.message)
  }
}

onMounted(loadThemes)
</script>
