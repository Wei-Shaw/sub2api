<template>
  <div class="ui-theme-switcher" role="group" :aria-label="t('nav.uiTheme')">
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      class="ui-theme-option"
      :class="{ 'ui-theme-option-active': uiTheme === option.value }"
      :aria-pressed="uiTheme === option.value"
      @click="setUiTheme(option.value)"
    >
      {{ option.label }}
    </button>
    <button
      type="button"
      class="ui-theme-option ui-theme-custom-option"
      :class="{ 'ui-theme-option-active': customBackground.image && customBackground.enabled }"
      :aria-pressed="Boolean(customBackground.image && customBackground.enabled)"
      @click="showBackgroundDialog = true"
    >
      {{ t('nav.uiThemeCustom') }}
    </button>
  </div>

  <BaseDialog
    :show="showBackgroundDialog"
    :title="t('nav.customBackgroundTitle')"
    width="normal"
    close-on-click-outside
    @close="showBackgroundDialog = false"
  >
    <p class="mb-5 text-sm text-gray-500 dark:text-dark-400">
      {{ t('nav.customBackgroundDescription') }}
    </p>

    <div class="custom-background-toggle-row">
      <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('nav.customBackgroundVisibility') }}
      </span>
      <button
        type="button"
        class="custom-background-toggle"
        :class="{ 'custom-background-toggle-active': draftEnabled }"
        role="switch"
        :aria-checked="draftEnabled"
        :disabled="!draftImage"
        @click="draftEnabled = !draftEnabled"
      >
        <span class="custom-background-toggle-track" aria-hidden="true">
          <span class="custom-background-toggle-thumb" />
        </span>
        <span>{{ draftEnabled ? t('nav.customBackgroundOn') : t('nav.customBackgroundOff') }}</span>
      </button>
    </div>

    <div class="space-y-5">
      <div
        class="custom-background-preview"
        :class="{ 'custom-background-preview-empty': !draftImage }"
        :style="draftImage ? { backgroundImage: `url(${draftImage})`, opacity: draftOpacity } : undefined"
      >
        <span v-if="!draftImage" class="text-sm text-gray-400 dark:text-dark-500">
          {{ t('nav.customBackgroundNoImage') }}
        </span>
      </div>

      <input
        ref="fileInput"
        type="file"
        accept="image/*"
        class="sr-only"
        @change="handleImageSelected"
      />
      <div class="flex flex-wrap items-center gap-3">
        <button type="button" class="btn btn-secondary btn-sm" @click="fileInput?.click()">
          <Icon name="upload" size="sm" />
          {{ draftImage ? t('nav.customBackgroundReplaceImage') : t('nav.customBackgroundChooseImage') }}
        </button>
        <button
          v-if="draftImage"
          type="button"
          class="btn btn-ghost btn-sm text-red-600 dark:text-red-400"
          @click="clearDraft"
        >
          {{ t('nav.customBackgroundClear') }}
        </button>
      </div>

      <label class="block">
        <span class="mb-2 flex items-center justify-between text-sm font-medium text-gray-700 dark:text-gray-300">
          <span>{{ t('nav.customBackgroundOpacity') }}</span>
          <span class="font-mono text-xs text-gray-500 dark:text-dark-400">{{ Math.round(draftOpacity * 100) }}%</span>
        </span>
        <input v-model.number="draftOpacity" type="range" min="0" max="1" step="0.01" class="custom-background-range" />
      </label>

      <fieldset class="custom-background-effect">
        <legend class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('nav.customBackgroundComponentEffect') }}
        </legend>
        <div class="custom-background-effect-options" role="group" :aria-label="t('nav.customBackgroundComponentEffect')">
          <button
            v-for="effect in componentEffects"
            :key="effect.value"
            type="button"
            class="custom-background-effect-option"
            :class="{ 'custom-background-effect-option-active': draftComponentEffect === effect.value }"
            :aria-pressed="draftComponentEffect === effect.value"
            @click="draftComponentEffect = effect.value"
          >
            {{ effect.label }}
          </button>
        </div>
      </fieldset>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="showBackgroundDialog = false">
        {{ t('common.cancel') }}
      </button>
      <button type="button" class="btn btn-primary" @click="applyBackground">
        {{ t('nav.customBackgroundApply') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useUiTheme, type UiTheme } from '@/composables/useUiTheme'
import { useCustomBackground, type CustomComponentEffect } from '@/composables/useCustomBackground'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const { uiTheme, setUiTheme } = useUiTheme()
const { customBackground, setCustomBackgroundFile, setCustomBackgroundOpacity, clearCustomBackground } = useCustomBackground()
const showBackgroundDialog = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const draftImage = ref('')
const draftFile = ref<File | null>(null)
const draftObjectUrl = ref('')
const draftOpacity = ref(0.2)
const draftComponentEffect = ref<CustomComponentEffect>('frosted')
const draftEnabled = ref(false)

watch(showBackgroundDialog, (isOpen) => {
  if (isOpen) {
    draftImage.value = customBackground.value.image
    draftFile.value = null
    draftOpacity.value = customBackground.value.opacity
    draftComponentEffect.value = customBackground.value.componentEffect
    draftEnabled.value = customBackground.value.enabled && Boolean(customBackground.value.image)
  }
})

function handleImageSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (!file.type.startsWith('image/')) {
    window.alert(t('nav.customBackgroundInvalidImage'))
    return
  }
  if (draftObjectUrl.value) URL.revokeObjectURL(draftObjectUrl.value)
  draftObjectUrl.value = URL.createObjectURL(file)
  draftImage.value = draftObjectUrl.value
  draftFile.value = file
}

function clearDraft() {
  if (draftObjectUrl.value) URL.revokeObjectURL(draftObjectUrl.value)
  draftObjectUrl.value = ''
  draftImage.value = ''
  draftFile.value = null
  draftEnabled.value = false
}

async function applyBackground() {
  if (!draftImage.value) {
    clearCustomBackground()
  } else if (draftFile.value) {
    await setCustomBackgroundFile(draftFile.value, draftOpacity.value, draftComponentEffect.value, draftEnabled.value)
  } else {
    await setCustomBackgroundOpacity(draftOpacity.value, draftComponentEffect.value, draftEnabled.value)
  }
  if (draftObjectUrl.value && draftObjectUrl.value !== customBackground.value.image) {
    URL.revokeObjectURL(draftObjectUrl.value)
    draftObjectUrl.value = ''
  }
  showBackgroundDialog.value = false
}

const options = computed<Array<{ value: UiTheme; label: string }>>(() => [
  { value: 'original', label: t('nav.uiThemeOriginal') },
  { value: 'simple', label: t('nav.uiThemeSimple') },
])

const componentEffects = computed<Array<{ value: CustomComponentEffect; label: string }>>(() => [
  { value: 'frosted', label: t('nav.customBackgroundEffectFrosted') },
  { value: 'transparent', label: t('nav.customBackgroundEffectTransparent') },
])
</script>

<style scoped>
.ui-theme-switcher {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  min-width: 116px;
  height: 38px;
  padding: 3px;
  border: 1px solid #dfe3e7;
  border-radius: 12px;
  background: #ffffff;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.ui-theme-option {
  display: inline-flex;
  flex: 1 1 0;
  align-items: center;
  justify-content: center;
  height: 30px;
  padding: 0 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: #68717a;
  font-size: 13px;
  font-weight: 500;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
  transition: background-color 160ms ease, color 160ms ease, box-shadow 160ms ease;
}

.ui-theme-option:hover {
  color: #343a40;
  background: #f5f7f8;
}

.ui-theme-option:focus-visible {
  outline: 2px solid rgba(0, 123, 255, 0.45);
  outline-offset: 1px;
}

.ui-theme-option-active {
  color: #0069d9;
  background: #eef7ff;
  box-shadow: inset 0 0 0 1px rgba(0, 123, 255, 0.24);
}

.ui-theme-custom-option {
  flex: 0 0 auto;
  padding-inline: 10px;
}

.custom-background-preview {
  display: flex;
  min-height: 156px;
  align-items: center;
  justify-content: center;
  border: 1px solid #dfe3e7;
  border-radius: 12px;
  background-color: #f5f7f8;
  background-position: center;
  background-size: cover;
  background-repeat: no-repeat;
  transition: opacity 160ms ease;
}

.custom-background-toggle-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
  padding: 12px 14px;
  border: 1px solid #dfe3e7;
  border-radius: 10px;
  background: rgba(245, 247, 248, 0.72);
}

.custom-background-toggle {
  display: inline-flex;
  min-height: 36px;
  align-items: center;
  gap: 8px;
  padding: 0 10px 0 6px;
  border: 1px solid #dfe3e7;
  border-radius: 999px;
  background: #ffffff;
  color: #68717a;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background-color 160ms ease, border-color 160ms ease, color 160ms ease;
}

.custom-background-toggle:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.custom-background-toggle:focus-visible {
  outline: 2px solid rgba(0, 123, 255, 0.45);
  outline-offset: 1px;
}

.custom-background-toggle-track {
  display: inline-flex;
  width: 30px;
  height: 18px;
  align-items: center;
  padding: 2px;
  border-radius: 999px;
  background: #c5cbd1;
  transition: background-color 160ms ease;
}

.custom-background-toggle-thumb {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: #ffffff;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.18);
  transition: transform 160ms ease;
}

.custom-background-toggle-active {
  border-color: rgba(0, 123, 255, 0.45);
  background: #eef7ff;
  color: #0069d9;
}

.custom-background-toggle-active .custom-background-toggle-track {
  background: #007bff;
}

.custom-background-toggle-active .custom-background-toggle-thumb {
  transform: translateX(12px);
}

.custom-background-preview-empty {
  border-style: dashed;
}

.custom-background-range {
  width: 100%;
  accent-color: #007bff;
}

.custom-background-effect {
  margin: 0;
  padding: 0;
  border: 0;
}

.custom-background-effect-options {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.custom-background-effect-option {
  min-height: 40px;
  padding: 0 12px;
  border: 1px solid #dfe3e7;
  border-radius: 10px;
  background: #ffffff;
  color: #68717a;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: background-color 160ms ease, border-color 160ms ease, color 160ms ease;
}

.custom-background-effect-option:hover {
  border-color: rgba(0, 123, 255, 0.45);
  color: #0069d9;
}

.custom-background-effect-option:focus-visible {
  outline: 2px solid rgba(0, 123, 255, 0.45);
  outline-offset: 1px;
}

.custom-background-effect-option-active {
  border-color: #007bff;
  background: #eef7ff;
  color: #0069d9;
  box-shadow: inset 0 0 0 1px rgba(0, 123, 255, 0.18);
}

.dark .custom-background-preview {
  border-color: #334155;
  background-color: #1e293b;
}

.dark .custom-background-toggle-row {
  border-color: #334155;
  background: rgba(30, 41, 59, 0.72);
}

.dark .custom-background-toggle {
  border-color: #334155;
  background: #1e293b;
  color: #cbd5e1;
}

.dark .custom-background-toggle-active {
  border-color: rgba(75, 158, 255, 0.65);
  background: rgba(0, 123, 255, 0.18);
  color: #8dc5ff;
}

.dark .custom-background-effect-option {
  border-color: #334155;
  background: #1e293b;
  color: #cbd5e1;
}

.dark .custom-background-effect-option:hover {
  border-color: rgba(75, 158, 255, 0.65);
  color: #8dc5ff;
}

.dark .custom-background-effect-option-active {
  border-color: #4b9eff;
  background: rgba(0, 123, 255, 0.18);
  color: #8dc5ff;
}

.dark .ui-theme-switcher {
  border-color: #334155;
  background: #1e293b;
}

.dark .ui-theme-option {
  color: #cbd5e1;
}

.dark .ui-theme-option:hover {
  color: #ffffff;
  background: #334155;
}

.dark .ui-theme-option-active {
  color: #8dc5ff;
  background: rgba(0, 123, 255, 0.18);
  box-shadow: inset 0 0 0 1px rgba(75, 158, 255, 0.38);
}

@media (prefers-reduced-motion: reduce) {
  .ui-theme-option {
    transition-duration: 1ms;
  }

  .custom-background-preview {
    transition-duration: 1ms;
  }

  .custom-background-effect-option {
    transition-duration: 1ms;
  }

  .custom-background-toggle,
  .custom-background-toggle-track,
  .custom-background-toggle-thumb {
    transition-duration: 1ms;
  }
}
</style>
