<template>
  <div class="empty-state">
    <!-- Icon -->
    <div class="w-20 h-20 mb-5 rounded-2xl bg-gray-100 dark:bg-dark-800 flex items-center justify-center">
      <slot name="icon">
        <component
          v-if="icon"
          :is="icon"
          class="empty-state-icon w-10 h-10"
          aria-hidden="true"
        />
        <svg
          v-else
          class="empty-state-icon w-10 h-10"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
          stroke-width="1.5"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"
          />
        </svg>
      </slot>
    </div>

    <!-- Title -->
    <h3 class="empty-state-title">
      {{ title REDACTEDREDACTED
    </h3>

    <!-- Description -->
    <p class="empty-state-description">
      {{ description REDACTEDREDACTED
    </p>

    <!-- Action -->
    <div v-if="actionText || $slots.action" class="mt-6">
      <slot name="action">
        <component
          :is="actionTo ? 'RouterLink' : 'button'"
          v-if="actionText"
          :to="actionTo"
          @click="!actionTo && $emit('action')"
          class="btn btn-primary"
        >
          <svg
            v-if="actionIcon"
            class="w-5 h-5 mr-2"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            stroke-width="1.5"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M12 4.5v15m7.5-7.5h-15"
            />
          </svg>
          {{ actionText REDACTEDREDACTED
        </component>
      </slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Component REDACTED from 'vue'
import { RouterLink REDACTED from 'vue-router'

interface Props {
  icon?: Component | string
  title?: string
  description?: string
  actionText?: string
  actionTo?: string | object
  actionIcon?: boolean
  message?: string
REDACTED

const props = withDefaults(defineProps<Props>(), {
  title: 'No data found',
  description: '',
  actionIcon: true
REDACTED)

defineEmits(['action'])
</script>
