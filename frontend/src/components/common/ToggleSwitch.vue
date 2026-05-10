<template>
  <span
    :class="[
      'inline-flex items-center gap-2',
      disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'
    ]"
  >
    <span v-if="label" class="text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ label }}
    </span>
    <button
      type="button"
      role="switch"
      :aria-checked="checked"
      :aria-label="ariaLabel || label"
      :disabled="disabled || loading"
      :class="[
        'relative inline-flex h-6 w-11 shrink-0 rounded-full border-2 border-transparent transition-colors duration-200 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 focus:ring-offset-white dark:focus:ring-offset-dark-900',
        checked ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600',
        disabled || loading ? 'cursor-not-allowed' : 'cursor-pointer'
      ]"
      @click="emit('toggle')"
    >
      <span
        :class="[
          'pointer-events-none inline-flex h-5 w-5 items-center justify-center rounded-full bg-white text-[10px] shadow-sm transition-transform duration-200',
          checked ? 'translate-x-5' : 'translate-x-0'
        ]"
      >
        <svg
          v-if="loading"
          class="h-3 w-3 animate-spin text-primary-500"
          fill="none"
          viewBox="0 0 24 24"
        >
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" />
        </svg>
      </span>
    </button>
  </span>
</template>

<script setup lang="ts">
defineProps<{
  checked: boolean
  label?: string
  ariaLabel?: string
  disabled?: boolean
  loading?: boolean
}>()

const emit = defineEmits<{ toggle: [] }>()
</script>
