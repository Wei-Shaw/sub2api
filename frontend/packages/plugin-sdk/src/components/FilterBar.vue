<template>
  <div class="filter-bar">
    <div v-if="$slots.left" class="filter-bar-left">
      <slot name="left" />
    </div>
    <div v-if="$slots.right" class="page-actions">
      <slot name="right" />
    </div>
  </div>
</template>

<script setup lang="ts">
// FilterBar: 标准的「左侧搜索/筛选 + 右侧操作按钮」横向布局
// 等价于 plugins/channel-management ChannelsView 中 inline 的 flex 容器,
// 抽到 SDK 后避免 plugin 各自重复写 Tailwind class 串.
</script>
