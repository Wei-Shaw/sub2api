<template>
  <div :class="labelPosition === 'inline' ? 'flex items-start gap-3' : 'flex flex-col'">
    <label
      v-if="label || $slots.label"
      :for="fieldId"
      class="mb-1.5 block text-xs font-medium text-ink-secondary"
      :class="labelPosition === 'inline' && 'mb-0 w-40 shrink-0 pt-1.5'"
    >
      <slot name="label">{{ label }}</slot>
      <span v-if="required" class="ml-0.5 text-danger" aria-hidden="true">*</span>
      <span v-else-if="optional" class="ml-1 font-normal text-ink-disabled">
        {{ optionalText }}
      </span>
    </label>

    <div class="min-w-0 flex-1">
      <slot :id="fieldId" :described-by="describedBy" :invalid="Boolean(error)" />

      <!--
        THE POINT OF THIS COMPONENT.
        The message row reserves its line box whether or not there is a message,
        so a validation error does not push everything below it down. The old
        Input toggled `<p>` elements in and out, which made the whole form jump
        on every blur — and in a settings page with sixty fields that is not a
        cosmetic problem, it is a "the control moved out from under my cursor"
        problem.
      -->
      <p
        v-if="!hideMessage"
        :id="messageId"
        class="mt-1 text-2xs"
        :class="error ? 'text-danger' : 'text-ink-tertiary'"
        style="min-height: var(--ds-lh-2xs)"
      >
        <slot name="message">{{ error || hint }}</slot>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'

/**
 * The composition root for a labelled control.
 *
 * It owns three things that are easy to get wrong once per form and impossible
 * to audit afterwards:
 *   1. id generation and `for`/`id` pairing
 *   2. `aria-describedby` / `aria-invalid` wiring, exposed via slot props so
 *      any control can consume them
 *   3. reserved vertical space for the hint/error line (see the template)
 *
 * It does NOT render an input. Passing the control through the default slot
 * keeps this usable with the existing `Input`, `Select`, `TextArea`, the
 * third-party pickers, and anything added later.
 */
const props = withDefaults(
  defineProps<{
    label?: string
    hint?: string
    error?: string
    required?: boolean
    /** Renders a muted "optional" marker instead of the required asterisk. */
    optional?: boolean
    optionalText?: string
    /** Supply to control the id; otherwise one is generated. */
    id?: string
    labelPosition?: 'top' | 'inline'
    /**
     * Opt out of the reserved message row. Only for fields in a dense grid
     * where the caller has already reserved space at the row level.
     */
    hideMessage?: boolean
  }>(),
  { labelPosition: 'top', optionalText: 'optional' }
)

const generated = useId()

const fieldId = computed(() => props.id ?? `field-${generated}`)
const messageId = computed(() => `${fieldId.value}-message`)

/** Only advertise a description when there is actually text to read. */
const describedBy = computed(() =>
  !props.hideMessage && (props.error || props.hint) ? messageId.value : undefined
)
</script>
