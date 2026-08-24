<template>
  <div>
    <div class="mb-2 flex items-center justify-between gap-3">
      <label :for="`${idPrefix}-slider`" class="input-label mb-0">
        {{ label }}
      </label>
      <div class="relative w-24 flex-shrink-0">
        <input
          :id="`${idPrefix}-input`"
          data-testid="ratio-input"
          type="number"
          min="0"
          max="100"
          step="1"
          inputmode="numeric"
          class="input h-9 px-2 pr-7 text-right tabular-nums"
          :value="percentageValue"
          :aria-label="`${label} (%)`"
          @input="handleNumberInput"
          @blur="restoreNumberInput"
        />
        <span
          class="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-sm text-gray-500 dark:text-dark-400"
          aria-hidden="true"
        >
          %
        </span>
      </div>
    </div>

    <div class="flex items-center gap-3">
      <span class="w-7 flex-shrink-0 text-xs text-gray-500 dark:text-dark-400">0%</span>
      <input
        :id="`${idPrefix}-slider`"
        data-testid="ratio-slider"
        type="range"
        min="0"
        max="100"
        step="1"
        class="h-2 min-w-0 flex-1 cursor-pointer accent-primary-600"
        :value="percentageValue"
        :aria-describedby="hint ? `${idPrefix}-hint` : undefined"
        :aria-valuetext="`${percentageValue}%`"
        @input="handleRangeInput"
      />
      <span class="w-9 flex-shrink-0 text-right text-xs text-gray-500 dark:text-dark-400">100%</span>
    </div>

    <p v-if="hint" :id="`${idPrefix}-hint`" class="input-hint">
      {{ hint }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  idPrefix: string;
  modelValue: number;
  label: string;
  hint?: string;
}>();

const emit = defineEmits<{
  (event: "update:modelValue", value: number): void;
}>();

const clampPercentage = (value: number): number =>
  Math.min(100, Math.max(0, Math.round(value)));

const percentageValue = computed(() => {
  const ratio = Number(props.modelValue);
  if (!Number.isFinite(ratio)) return 0;
  return clampPercentage(ratio * 100);
});

const emitPercentage = (value: number, target: HTMLInputElement) => {
  const percentage = clampPercentage(value);
  target.value = String(percentage);
  emit("update:modelValue", percentage / 100);
};

const handleRangeInput = (event: Event) => {
  const target = event.target as HTMLInputElement;
  emitPercentage(Number(target.value), target);
};

const handleNumberInput = (event: Event) => {
  const target = event.target as HTMLInputElement;
  if (target.value.trim() === "") return;

  const value = Number(target.value);
  if (!Number.isFinite(value)) return;
  emitPercentage(value, target);
};

const restoreNumberInput = (event: FocusEvent) => {
  const target = event.target as HTMLInputElement;
  if (target.value.trim() === "" || !Number.isFinite(Number(target.value))) {
    target.value = String(percentageValue.value);
  }
};
</script>
