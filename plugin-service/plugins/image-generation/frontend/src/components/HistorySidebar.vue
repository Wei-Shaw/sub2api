<script setup lang="ts">
import type { Conversation, ImageApiKey } from '../types'
import SidebarToggleButton from './SidebarToggleButton.vue'

defineProps<{
  conversations: Conversation[]
  activeId: string
  keys: ImageApiKey[]
  selectedKeyId: number | null
}>()

const emit = defineEmits<{
  select: [id: string]
  delete: [conversation: Conversation]
  create: []
  collapse: []
  'update:selectedKeyId': [id: number]
}>()

function changeKey(event: Event) {
  emit('update:selectedKeyId', Number((event.target as HTMLSelectElement).value))
}
</script>

<template>
  <aside class="history-sidebar" data-testid="image-history" aria-label="History">
    <div class="key-picker">
      <label for="image-key-select">可用 Key</label>
      <select id="image-key-select" :value="selectedKeyId ?? ''" data-testid="image-key-select" @change="changeKey">
        <option v-for="key in keys" :key="key.id" :value="key.id">{{ key.name }}</option>
      </select>
    </div>
    <button type="button" class="new-conversation" data-testid="new-image-session" @click="emit('create')">
      <span aria-hidden="true">+</span> 新建会话
    </button>
    <h2>历史记录</h2>
    <ul class="history-list" data-testid="history-list">
      <li v-for="conversation in conversations" :key="conversation.id" class="history-row" :class="{ active: conversation.id === activeId }">
        <button type="button" class="history-select" data-testid="history-select" @click="emit('select', conversation.id)">
          <strong>{{ conversation.title }}</strong>
          <span>{{ conversation.preview || '暂无内容' }}</span>
          <time>{{ conversation.lastUsedAt }}</time>
        </button>
        <button
          type="button"
          class="icon-button delete-button"
          data-testid="history-delete-button"
          aria-label="删除历史记录"
          @click="emit('delete', conversation)"
        >
          删除
        </button>
      </li>
    </ul>
    <div class="sidebar-footer" data-testid="sidebar-footer">
      <SidebarToggleButton direction="collapse" data-testid="history-inline-collapse" @click="emit('collapse')" />
    </div>
  </aside>
</template>
