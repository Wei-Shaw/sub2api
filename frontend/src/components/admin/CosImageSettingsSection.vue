<template>
  <div class="space-y-6">
    <div>
      <h3 class="text-base font-semibold text-gray-900 dark:text-gray-100">
        {{ t('admin.settings.cosImage.title') }}
      </h3>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.cosImage.description') }}
      </p>
    </div>

    <div v-if="loading" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('common.loading') }}
    </div>

    <template v-else>
      <!-- 主开关 -->
      <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
        <label class="flex items-start gap-3">
          <input v-model="form.enabled" type="checkbox" class="mt-1 h-4 w-4" />
          <span>
            <span class="block text-sm font-medium text-gray-900 dark:text-gray-100">
              {{ t('admin.settings.cosImage.enabled') }}
            </span>
            <span class="block text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.cosImage.enabledHint') }}
            </span>
          </span>
        </label>
      </div>

      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div>
          <label class="form-label">{{ t('admin.settings.cosImage.endpoint') }}</label>
          <input
            v-model="form.endpoint"
            type="text"
            class="input"
            placeholder="https://cos.ap-guangzhou.myqcloud.com"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.cosImage.endpointHint') }}
          </p>
        </div>
        <div>
          <label class="form-label">{{ t('admin.settings.cosImage.region') }}</label>
          <input v-model="form.region" type="text" class="input" placeholder="ap-guangzhou" />
        </div>
        <div>
          <label class="form-label">{{ t('admin.settings.cosImage.bucket') }}</label>
          <input v-model="form.bucket" type="text" class="input" placeholder="mybucket-1250000000" />
        </div>
        <div>
          <label class="form-label">{{ t('admin.settings.cosImage.accessKeyId') }}</label>
          <input v-model="form.access_key_id" type="text" class="input" autocomplete="off" />
        </div>
        <div>
          <label class="form-label">{{ t('admin.settings.cosImage.secretAccessKey') }}</label>
          <input
            v-model="form.secret_access_key"
            type="password"
            class="input"
            autocomplete="new-password"
            :placeholder="secretKeySet
              ? t('admin.settings.cosImage.secretAccessKeySetPlaceholder')
              : t('admin.settings.cosImage.secretAccessKeyPlaceholder')"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ secretKeySet
              ? t('admin.settings.cosImage.secretAccessKeyKeepHint')
              : t('admin.settings.cosImage.secretAccessKeyHint') }}
          </p>
        </div>
        <div>
          <label class="form-label">{{ t('admin.settings.cosImage.prefix') }}</label>
          <input v-model="form.prefix" type="text" class="input" placeholder="images/" />
        </div>
        <div class="sm:col-span-2">
          <label class="form-label">{{ t('admin.settings.cosImage.publicBaseUrl') }}</label>
          <input
            v-model="form.public_base_url"
            type="text"
            class="input"
            placeholder="https://cdn.example.com"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.cosImage.publicBaseUrlHint') }}
          </p>
        </div>
      </div>

      <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
        <label class="flex items-start gap-3">
          <input v-model="form.force_path_style" type="checkbox" class="mt-1 h-4 w-4" />
          <span>
            <span class="block text-sm font-medium text-gray-900 dark:text-gray-100">
              {{ t('admin.settings.cosImage.forcePathStyle') }}
            </span>
            <span class="block text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.cosImage.forcePathStyleHint') }}
            </span>
          </span>
        </label>
      </div>

      <div class="flex justify-end">
        <button type="button" class="btn btn-primary" :disabled="saving" @click="onSave">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import {
  getCOSImageConfig,
  updateCOSImageConfig,
  type COSImageConfig,
} from '@/api/admin/cosImage'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)
const secretKeySet = ref(false)

const form = reactive<COSImageConfig>({
  enabled: false,
  endpoint: '',
  region: '',
  bucket: '',
  access_key_id: '',
  secret_access_key: '',
  prefix: '',
  force_path_style: false,
  public_base_url: '',
})

async function load() {
  loading.value = true
  try {
    const resp = await getCOSImageConfig()
    Object.assign(form, resp.config)
    // 后端不回显明文，输入框保持空；仅用 secretKeySet 标识是否已配置。
    form.secret_access_key = ''
    secretKeySet.value = resp.secret_access_key_set
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('admin.settings.cosImage.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function onSave() {
  saving.value = true
  try {
    // secret_access_key 留空表示保持原值，避免覆盖为空。
    const payload: COSImageConfig = { ...form }
    if (!payload.secret_access_key) {
      delete payload.secret_access_key
    }
    const updated = await updateCOSImageConfig(payload)
    Object.assign(form, updated)
    form.secret_access_key = ''
    // 保存成功后，若本次填了新密钥或原本已设置，则标记已配置。
    if (payload.secret_access_key || secretKeySet.value) {
      secretKeySet.value = true
    }
    appStore.showSuccess(t('admin.settings.cosImage.saveSuccess'))
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('admin.settings.cosImage.saveFailed'))
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
