<template>
	<HelpTooltip :content="content" width-class="w-80">
		<template #trigger>
			<span
				class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-gray-400/70 text-[10px] font-semibold text-gray-400 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-gray-500 dark:text-gray-500 dark:hover:border-primary-400 dark:hover:text-primary-400"
				:aria-label="content"
			>
				?
			</span>
		</template>
	</HelpTooltip>
</template>

<script setup lang="ts">
import HelpTooltip from '@/components/common/HelpTooltip.vue'

defineProps<{ content: string }>()
</script>
