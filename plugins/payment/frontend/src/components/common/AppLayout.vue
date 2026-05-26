<!--
  AppLayout shim for the payment plugin.

  The host SPA's <AppLayout> wraps every page in the sidebar+header chrome.
  Inside the plugin runtime the host's PluginView already mounts us inside
  that chrome (the plugin sees a slot), so we just render the default slot
  without any extra wrapper. Keeping the component name lets the migrated
  SFCs stay verbatim — Phase 3 cleanup can replace usages with PluginPageLayout.
-->
<template>
  <div class="space-y-4">
    <slot />
  </div>
</template>

<script setup lang="ts">
// No-op: host owns the chrome.
</script>
