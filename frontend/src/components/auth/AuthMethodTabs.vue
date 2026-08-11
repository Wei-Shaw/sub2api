<template>
  <div class="grid grid-cols-2 rounded-xl bg-gray-100 p-1 dark:bg-dark-800">
    <RouterLink
      :to="emailTarget"
      class="rounded-lg px-3 py-2 text-center text-sm font-medium transition"
      :class="active === 'email' ? activeClass : inactiveClass"
    >
      {{ t('auth.web3.emailTab') }}
    </RouterLink>
    <RouterLink
      :to="web3Target"
      class="rounded-lg px-3 py-2 text-center text-sm font-medium transition"
      :class="active === 'web3' ? activeClass : inactiveClass"
    >
      {{ t('auth.web3.walletTab') }}
    </RouterLink>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  active: 'email' | 'web3'
  context: 'login' | 'register'
}>()

const { t } = useI18n()
const route = useRoute()
const query = computed(() => ({ ...route.query }))
const emailTarget = computed(() => ({ path: `/${props.context}`, query: query.value }))
const web3Target = computed(() => ({ path: `/${props.context}/web3`, query: query.value }))
const activeClass = 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
const inactiveClass = 'text-gray-500 hover:text-gray-900 dark:text-dark-400 dark:hover:text-white'
</script>
