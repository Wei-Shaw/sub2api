<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { createPluginApi, loadImageKeys, pluginApiBase } from './api/client'
import ChatThread from './components/ChatThread.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'
import HistorySidebar from './components/HistorySidebar.vue'
import ImagePreviewDialog from './components/ImagePreviewDialog.vue'
import PromptComposer from './components/PromptComposer.vue'
import SidebarToggleButton from './components/SidebarToggleButton.vue'
import { useImageGeneration } from './composables/useImageGeneration'
import type { Conversation, GeneratedImage, ImageReference } from './types'

const state = useImageGeneration({ api: createPluginApi(pluginApiBase()), loadKeys: loadImageKeys })
const drawerOpen = ref(false)
const sidebarCollapsed = ref(false)
const deleteTarget = ref<Conversation | null>(null)
const imagePreview = ref({ open: false, src: '', alt: '' })

onMounted(async () => {
  try { await state.initialize() }
  catch (error) { state.errorMessage.value = error instanceof Error ? error.message : String(error) }
})
onBeforeUnmount(state.dispose)

function useAsReference(image: GeneratedImage) {
  const reference: ImageReference = {
    id: `${image.id}-reference`, dataUrl: image.src, originalDataUrl: image.originalSrc || image.src, fileName: `${image.id}.png`, mimeType: 'image/png',
  }
  state.setReference(reference)
}

function openImagePreview(src: string, alt: string) { imagePreview.value = { open: true, src, alt } }

async function confirmDelete() {
  if (deleteTarget.value) await state.deleteConversation(deleteTarget.value)
  deleteTarget.value = null
}
</script>

<template>
  <main class="plugin-shell" data-testid="image-workspace">
    <SidebarToggleButton
      v-if="sidebarCollapsed || !drawerOpen"
      direction="expand"
      class="drawer-toggle"
      :class="{ 'collapsed-handle': sidebarCollapsed }"
      data-testid="history-drawer-toggle"
      :aria-expanded="drawerOpen"
      @click="sidebarCollapsed = false; drawerOpen = true"
    />
    <div v-if="drawerOpen" class="drawer-scrim" @click="drawerOpen = false" />
    <div class="sidebar-wrap" :class="{ open: drawerOpen, collapsed: sidebarCollapsed }">
      <HistorySidebar
        :conversations="state.conversations.value"
        :active-id="state.activeConversationId.value"
        :keys="state.keys.value"
        :selected-key-id="state.selectedKeyId.value"
        @update:selected-key-id="state.selectedKeyId.value = $event"
        @select="state.selectConversation($event); drawerOpen = false"
        @create="state.createConversation(); drawerOpen = false"
        @delete="deleteTarget = $event"
        @collapse="sidebarCollapsed = true; drawerOpen = false"
      />
    </div>
    <section class="chat-panel" data-testid="image-chat-panel">
      <ChatThread
        :conversation="state.activeConversation.value"
        :loading="state.loadingConversation.value"
        :has-older="state.hasOlderMessages.value"
        @reference="useAsReference"
        @reference-image="state.setReference"
        @refine="state.refineFromImage"
        @refine-prompt="state.prompt.value = $event"
        @repeat="state.repeatFromImage($event, state.prompt.value)"
        @retry="state.retryMessage"
        @view="openImagePreview"
        @load-older="state.loadOlderMessages"
      />
      <p v-if="state.errorMessage.value" class="inline-error" role="alert">{{ state.errorMessage.value }}</p>
      <PromptComposer
        :prompt="state.prompt.value"
        :model="state.model.value"
        :size="state.size.value"
        :size-options="state.availableSizes.value"
        :aspect-ratio="state.aspectRatio.value"
        :aspect-ratio-options="state.availableAspectRatios.value"
        :resolution="state.resolution.value"
        :resolution-options="state.availableResolutions.value"
        :quality="state.quality.value"
        :quality-options="state.availableQualities.value"
        :output-format="state.outputFormat.value"
        :output-format-options="state.availableOutputFormats.value"
        :output-compression="state.outputCompression.value"
        :compression-min="state.compressionCapability.value?.min ?? 0"
        :compression-max="state.compressionCapability.value?.max ?? 100"
        :supports-output-compression="state.supportsOutputCompression.value"
        :background="state.background.value"
        :background-options="state.availableBackgrounds.value"
        :input-fidelity="state.inputFidelity.value"
        :input-fidelity-options="state.availableInputFidelities.value"
        :output-count="state.outputCount.value"
        :max-output-images="state.maxOutputImages.value"
        :models="state.availableModels.value"
        :prompt-optimizer-model="state.promptOptimizerModel.value"
        :prompt-optimizer-models="state.promptOptimizerModels.value"
        :optimizing-prompt="state.optimizingPrompt.value"
        :preset-selection="state.presetSelection.value"
        :busy="state.generationStatus.value !== 'idle'"
        :references="state.activeConversation.value?.referenceImages ?? []"
        :max-reference-images="state.maxReferenceImages.value"
        :reference-limit-exceeded="state.referenceLimitExceeded.value"
        @update:prompt="state.prompt.value = $event"
        @update:model="state.model.value = $event"
        @update:prompt-optimizer-model="state.promptOptimizerModel.value = $event"
        @update:preset-selection="state.presetSelection.value = $event"
        @apply-preset-selection="state.applyPresetSelection($event)"
        @update:size="state.size.value = $event"
        @update:aspect-ratio="state.aspectRatio.value = $event"
        @update:resolution="state.resolution.value = $event"
        @update:quality="state.quality.value = $event"
        @update:output-format="state.outputFormat.value = $event"
        @update:output-compression="state.outputCompression.value = $event"
        @update:background="state.background.value = $event"
        @update:input-fidelity="state.inputFidelity.value = $event"
        @update:output-count="state.outputCount.value = $event"
        @reference-files="state.uploadReference($event)"
        @remove-reference="state.removeReference($event)"
        @clear-references="state.clearReferences()"
        @optimize-prompt="state.optimizePrompt($event)"
        @cancel-prompt-optimization="state.cancelPromptOptimization()"
        @submit="state.submit()"
        @stop="state.cancelGeneration()"
      />
    </section>
    <ConfirmDialog
      :open="Boolean(deleteTarget)"
      title="删除历史记录"
      message="确认删除当前历史记录及其生成图片吗？"
      @cancel="deleteTarget = null"
      @confirm="confirmDelete"
    />
    <ImagePreviewDialog
      :open="imagePreview.open"
      :src="imagePreview.src"
      :alt="imagePreview.alt"
      @close="imagePreview.open = false"
    />
  </main>
</template>
