<template>
  <div>
    <button type="button" class="sidebar-link mb-1 w-full" :class="{ 'sidebar-link-active': active && !expanded, 'sidebar-link-collapsed': collapsed }" :title="collapsed ? item.label : undefined" :aria-label="item.label" :aria-expanded="!collapsed && expanded" @click="$emit('toggle')">
      <component :is="item.icon" class="h-5 w-5 flex-shrink-0" />
      <span class="sidebar-label sidebar-label-flex" :class="{ 'sidebar-label-collapsed': collapsed }" :aria-hidden="collapsed ? 'true' : 'false'">
        <span class="min-w-0 truncate">{{ item.label }}</span>
        <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" class="h-4 w-4 shrink-0 transition-transform" :class="expanded ? 'rotate-180' : ''" aria-hidden="true"><path d="m5 7.5 5 5 5-5" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" /></svg>
      </span>
    </button>
    <div v-if="!collapsed && expanded" class="mb-1 ml-4 border-l border-gray-200 pl-2 dark:border-dark-600">
      <router-link v-for="child in item.children" :key="child.path" :to="child.path" class="sidebar-link mb-0.5 py-1.5 text-sm" :class="{ 'sidebar-link-active': activePath === child.path }" :aria-current="activePath === child.path ? 'page' : undefined" @click="$emit('navigate', child.path)">
        <component :is="child.icon" class="h-4 w-4 shrink-0" /><span>{{ child.label }}</span>
      </router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  item: { label: string; icon: unknown; children?: { path: string; label: string; icon: unknown }[] }
  collapsed: boolean
  expanded: boolean
  active: boolean
  activePath: string
}>()
defineEmits<{ (event: 'toggle'): void; (event: 'navigate', path: string): void }>()
</script>
