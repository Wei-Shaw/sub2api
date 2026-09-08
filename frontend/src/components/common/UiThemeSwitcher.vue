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
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useUiTheme, type UiTheme } from '@/composables/useUiTheme'

const { t } = useI18n()
const { uiTheme, setUiTheme } = useUiTheme()

const options = computed<Array<{ value: UiTheme; label: string }>>(() => [
  { value: 'original', label: t('nav.uiThemeOriginal') },
  { value: 'simple', label: t('nav.uiThemeSimple') },
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
}
</style>
