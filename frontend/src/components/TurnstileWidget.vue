<template>
  <div v-if="siteKey" class="turnstile-wrapper">
    <div ref="containerRef" class="turnstile-container"></div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch REDACTED from 'vue';

interface TurnstileRenderOptions {
  sitekey: string;
  callback: (token: string) => void;
  'expired-callback'?: () => void;
  'error-callback'?: () => void;
  theme?: 'light' | 'dark' | 'auto';
  size?: 'normal' | 'compact' | 'flexible';
REDACTED

interface TurnstileAPI {
  render: (container: HTMLElement, options: TurnstileRenderOptions) => string;
  reset: (widgetId?: string) => void;
  remove: (widgetId?: string) => void;
REDACTED

declare global {
  interface Window {
    turnstile?: TurnstileAPI;
    onTurnstileLoad?: () => void;
  REDACTED
REDACTED

const props = withDefaults(defineProps<{
  siteKey: string;
  theme?: 'light' | 'dark' | 'auto';
  size?: 'normal' | 'compact' | 'flexible';
REDACTED>(), {
  theme: 'auto',
  size: 'flexible',
REDACTED);

const emit = defineEmits<{
  (e: 'verify', token: string): void;
  (e: 'expire'): void;
  (e: 'error'): void;
REDACTED>();

const containerRef = ref<HTMLElement | null>(null);
const widgetId = ref<string | null>(null);
const scriptLoaded = ref(false);

const loadScript = (): Promise<void> => {
  return new Promise((resolve, reject) => {
    if (window.turnstile) {
      scriptLoaded.value = true;
      resolve();
      return;
    REDACTED

    // Check if script is already loading
    const existingScript = document.querySelector('script[src*="turnstile"]');
    if (existingScript) {
      window.onTurnstileLoad = () => {
        scriptLoaded.value = true;
        resolve();
      REDACTED;
      return;
    REDACTED

    const script = document.createElement('script');
    script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?onload=onTurnstileLoad';
    script.async = true;
    script.defer = true;

    window.onTurnstileLoad = () => {
      scriptLoaded.value = true;
      resolve();
    REDACTED;

    script.onerror = () => {
      reject(new Error('Failed to load Turnstile script'));
    REDACTED;

    document.head.appendChild(script);
  REDACTED);
REDACTED;

const renderWidget = () => {
  if (!window.turnstile || !containerRef.value || !props.siteKey) {
    return;
  REDACTED

  // Remove existing widget if any
  if (widgetId.value) {
    try {
      window.turnstile.remove(widgetId.value);
    REDACTED catch {
      // Ignore errors when removing
    REDACTED
    widgetId.value = null;
  REDACTED

  // Clear container
  containerRef.value.innerHTML = '';

  widgetId.value = window.turnstile.render(containerRef.value, {
    sitekey: props.siteKey,
    callback: (token: string) => {
      emit('verify', token);
    REDACTED,
    'expired-callback': () => {
      emit('expire');
    REDACTED,
    'error-callback': () => {
      emit('error');
    REDACTED,
    theme: props.theme,
    size: props.size,
  REDACTED);
REDACTED;

const reset = () => {
  if (window.turnstile && widgetId.value) {
    window.turnstile.reset(widgetId.value);
  REDACTED
REDACTED;

// Expose reset method to parent
defineExpose({ reset REDACTED);

onMounted(async () => {
  if (!props.siteKey) {
    return;
  REDACTED

  try {
    await loadScript();
    renderWidget();
  REDACTED catch (error) {
    console.error('Failed to initialize Turnstile:', error);
    emit('error');
  REDACTED
REDACTED);

onUnmounted(() => {
  if (window.turnstile && widgetId.value) {
    try {
      window.turnstile.remove(widgetId.value);
    REDACTED catch {
      // Ignore errors when removing
    REDACTED
  REDACTED
REDACTED);

// Re-render when siteKey changes
watch(() => props.siteKey, (newKey) => {
  if (newKey && scriptLoaded.value) {
    renderWidget();
  REDACTED
REDACTED);
</script>

<style scoped>
.turnstile-wrapper {
  width: 100%;
REDACTED

.turnstile-container {
  width: 100%;
  min-height: 65px;
REDACTED

/* Make the Turnstile iframe fill the container width */
.turnstile-container :deep(iframe) {
  width: 100% !important;
REDACTED
</style>
