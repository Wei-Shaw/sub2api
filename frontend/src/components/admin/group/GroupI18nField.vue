<template>
  <div>
    <div class="mb-1.5 flex items-center justify-between gap-3">
      <label class="input-label mb-0">{{ label }}</label>
      <div ref="menuRef" class="relative">
        <button
          type="button"
          data-testid="group-i18n-menu"
          class="flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs font-medium transition-colors"
          :class="
            activeLocale
              ? 'bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400'
              : 'text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-gray-200'
          "
          :aria-expanded="menuOpen"
          @click="menuOpen = !menuOpen"
        >
          <Icon name="globe" size="sm" />
          <span>{{ activeLabel }}</span>
          <Icon
            name="chevronDown"
            size="xs"
            class="transition-transform duration-200"
            :class="{ 'rotate-180': menuOpen }"
          />
        </button>
        <div v-if="menuOpen" class="dropdown right-0 mt-1 min-w-[10rem]">
          <button
            v-for="option in options"
            :key="option.code"
            type="button"
            :data-locale="option.code || 'default'"
            class="dropdown-item w-full whitespace-nowrap text-left"
            :class="{
              'bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400':
                option.active
            }"
            @click="selectLocale(option.code)"
          >
            <span>{{ option.label }}</span>
            <Icon v-if="option.active" name="check" size="sm" class="ml-auto text-primary-500" />
          </button>
        </div>
      </div>
    </div>
    <textarea
      v-if="multiline"
      v-bind="$attrs"
      ref="controlRef"
      class="input"
      :value="currentValue"
      :required="required && !activeLocale"
      :placeholder="currentPlaceholder"
      @input="onInput"
    ></textarea>
    <input
      v-else
      v-bind="$attrs"
      ref="controlRef"
      type="text"
      class="input"
      :value="currentValue"
      :required="required && !activeLocale"
      :placeholder="currentPlaceholder"
      @input="onInput"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { availableLocales } from '@/i18n'
import type { GroupI18n } from '@/types'

// 透传的属性（rows、data-tour 等）落在输入控件上，而不是外层容器
defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    label: string
    /** 默认内容，即分组自身的 name / description */
    modelValue: string
    /** 全部语言的译文；本组件只读写其中的 field 字段 */
    i18n: GroupI18n
    field: 'name' | 'description'
    multiline?: boolean
    /** 仅约束默认内容，译文始终可留空 */
    required?: boolean
    placeholder?: string
  }>(),
  { multiline: false, required: false, placeholder: '' }
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'update:i18n': [value: GroupI18n]
}>()

const { t } = useI18n()

// 输入控件当前编辑的版本：空串为默认内容，其余为界面语言代码
const activeLocale = ref('')

const options = computed(() => [
  {
    code: '',
    label: t('admin.groups.i18nField.default'),
    active: activeLocale.value === ''
  },
  ...availableLocales.map((locale) => ({
    code: locale.code as string,
    label: t(
      props.i18n[locale.code]?.[props.field]
        ? 'admin.groups.i18nField.edit'
        : 'admin.groups.i18nField.add',
      { language: locale.name }
    ),
    active: activeLocale.value === locale.code
  }))
])

const activeLocaleName = computed(
  () => availableLocales.find((locale) => locale.code === activeLocale.value)?.name ?? ''
)

const activeLabel = computed(
  () => activeLocaleName.value || t('admin.groups.i18nField.default')
)

const currentValue = computed(() =>
  activeLocale.value ? (props.i18n[activeLocale.value]?.[props.field] ?? '') : props.modelValue
)

const currentPlaceholder = computed(() =>
  activeLocale.value
    ? t('admin.groups.i18nField.placeholder', { language: activeLocaleName.value })
    : props.placeholder
)

function onInput(event: Event) {
  const value = (event.target as HTMLInputElement | HTMLTextAreaElement).value
  if (!activeLocale.value) {
    emit('update:modelValue', value)
    return
  }
  emit('update:i18n', {
    ...props.i18n,
    [activeLocale.value]: { ...props.i18n[activeLocale.value], [props.field]: value }
  })
}

const controlRef = ref<HTMLInputElement | HTMLTextAreaElement | null>(null)

async function selectLocale(code: string) {
  menuOpen.value = false
  activeLocale.value = code
  await nextTick()
  controlRef.value?.focus()
}

const menuOpen = ref(false)
const menuRef = ref<HTMLElement | null>(null)

function handleClickOutside(event: MouseEvent) {
  if (menuRef.value && !menuRef.value.contains(event.target as Node)) {
    menuOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>
