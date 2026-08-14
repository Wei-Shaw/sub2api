<template>
  <div
    class='fixed inset-0 pointer-events-none z-0 overflow-hidden transition-colors duration-500'
    :class='[isDark ? "dark bg-[#050810]" : "bg-gray-50"]'
  >
    <!-- 主光斑 -->
    <div class='absolute -top-40 -left-40 h-[700px] w-[700px] rounded-full bg-indigo-500/15 blur-[140px] dark:bg-indigo-900/20'></div>
    <div class='absolute top-1/4 -right-40 h-[600px] w-[600px] rounded-full bg-purple-500/12 blur-[130px] dark:bg-purple-900/18'></div>
    <div class='absolute bottom-0 left-1/3 h-[500px] w-[500px] rounded-full bg-cyan-500/10 blur-[110px] dark:bg-cyan-900/15'></div>
    <!-- 网格纹理 -->
    <div class='absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.03)_1px,transparent_1px)] bg-[size:48px_48px] opacity-40'></div>
    <!-- 顶部光晕 -->
    <div class='absolute top-0 left-1/2 -translate-x-1/2 h-64 w-full max-w-4xl rounded-full bg-gradient-to-b from-indigo-500/10 via-transparent to-transparent blur-3xl'></div>
  </div>
</template>

<script setup lang='ts'>
interface Props {
  isDark?: boolean
}

defineProps<Props>()
</script>
