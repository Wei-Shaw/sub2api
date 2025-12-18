<template>
  <AuthLayout>
    <div class="space-y-6">
      <!-- Title -->
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.createAccount') REDACTEDREDACTED
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('auth.signUpToStart', { siteName REDACTED) REDACTEDREDACTED
        </p>
      </div>

      <!-- Registration Disabled Message -->
      <div v-if="!registrationEnabled && settingsLoaded" class="p-4 rounded-xl bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800/50">
        <div class="flex items-start gap-3">
          <div class="flex-shrink-0">
            <svg class="w-5 h-5 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
            </svg>
          </div>
          <p class="text-sm text-amber-700 dark:text-amber-400">
            {{ t('auth.registrationDisabled') REDACTEDREDACTED
          </p>
        </div>
      </div>

      <!-- Registration Form -->
      <form v-else @submit.prevent="handleRegister" class="space-y-5">
        <!-- Email Input -->
        <div>
          <label for="email" class="input-label">
            {{ t('auth.emailLabel') REDACTEDREDACTED
          </label>
          <div class="relative">
            <div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none">
              <svg class="w-5 h-5 text-gray-400 dark:text-dark-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75" />
              </svg>
            </div>
            <input
              id="email"
              v-model="formData.email"
              type="email"
              required
              autocomplete="email"
              :disabled="isLoading"
              class="input pl-11"
              :class="{ 'input-error': errors.email REDACTED"
              :placeholder="t('auth.emailPlaceholder')"
            />
          </div>
          <p v-if="errors.email" class="input-error-text">
            {{ errors.email REDACTEDREDACTED
          </p>
        </div>

        <!-- Password Input -->
        <div>
          <label for="password" class="input-label">
            {{ t('auth.passwordLabel') REDACTEDREDACTED
          </label>
          <div class="relative">
            <div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none">
              <svg class="w-5 h-5 text-gray-400 dark:text-dark-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
              </svg>
            </div>
            <input
              id="password"
              v-model="formData.password"
              :type="showPassword ? 'text' : 'password'"
              required
              autocomplete="new-password"
              :disabled="isLoading"
              class="input pl-11 pr-11"
              :class="{ 'input-error': errors.password REDACTED"
              :placeholder="t('auth.createPasswordPlaceholder')"
            />
            <button
              type="button"
              @click="showPassword = !showPassword"
              class="absolute inset-y-0 right-0 pr-3.5 flex items-center text-gray-400 hover:text-gray-600 dark:hover:text-dark-300 transition-colors"
            >
              <svg v-if="showPassword" class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M3.98 8.223A10.477 10.477 0 001.934 12C3.226 16.338 7.244 19.5 12 19.5c.993 0 1.953-.138 2.863-.395M6.228 6.228A10.45 10.45 0 0112 4.5c4.756 0 8.773 3.162 10.065 7.498a10.523 10.523 0 01-4.293 5.774M6.228 6.228L3 3m3.228 3.228l3.65 3.65m7.894 7.894L21 21m-3.228-3.228l-3.65-3.65m0 0a3 3 0 10-4.243-4.243m4.242 4.242L9.88 9.88" />
              </svg>
              <svg v-else class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M2.036 12.322a1.012 1.012 0 010-.639C3.423 7.51 7.36 4.5 12 4.5c4.638 0 8.573 3.007 9.963 7.178.07.207.07.431 0 .639C20.577 16.49 16.64 19.5 12 19.5c-4.638 0-8.573-3.007-9.963-7.178z" />
                <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
              </svg>
            </button>
          </div>
          <p v-if="errors.password" class="input-error-text">
            {{ errors.password REDACTEDREDACTED
          </p>
          <p v-else class="input-hint">
            {{ t('auth.passwordHint') REDACTEDREDACTED
          </p>
        </div>

        <!-- Turnstile Widget -->
        <div v-if="turnstileEnabled && turnstileSiteKey">
          <TurnstileWidget
            ref="turnstileRef"
            :site-key="turnstileSiteKey"
            @verify="onTurnstileVerify"
            @expire="onTurnstileExpire"
            @error="onTurnstileError"
          />
          <p v-if="errors.turnstile" class="input-error-text text-center mt-2">
            {{ errors.turnstile REDACTEDREDACTED
          </p>
        </div>

        <!-- Error Message -->
        <transition name="fade">
          <div
            v-if="errorMessage"
            class="p-4 rounded-xl bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800/50"
          >
            <div class="flex items-start gap-3">
              <div class="flex-shrink-0">
                <svg class="w-5 h-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
                </svg>
              </div>
              <p class="text-sm text-red-700 dark:text-red-400">
                {{ errorMessage REDACTEDREDACTED
              </p>
            </div>
          </div>
        </transition>

        <!-- Submit Button -->
        <button
          type="submit"
          :disabled="isLoading || (turnstileEnabled && !turnstileToken)"
          class="btn btn-primary w-full"
        >
          <svg
            v-if="isLoading"
            class="animate-spin -ml-1 mr-2 h-4 w-4 text-white"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          <svg v-else class="w-5 h-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M19 7.5v3m0 0v3m0-3h3m-3 0h-3m-2.25-4.125a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zM4 19.235v-.11a6.375 6.375 0 0112.75 0v.109A12.318 12.318 0 0110.374 21c-2.331 0-4.512-.645-6.374-1.766z" />
          </svg>
          {{ isLoading ? t('auth.processing') : (emailVerifyEnabled ? t('auth.continue') : t('auth.createAccount')) REDACTEDREDACTED
        </button>
      </form>
    </div>

    <!-- Footer -->
    <template #footer>
      <p class="text-gray-500 dark:text-dark-400">
        {{ t('auth.alreadyHaveAccount') REDACTEDREDACTED
        <router-link
          to="/login"
          class="font-medium text-primary-600 hover:text-primary-500 dark:text-primary-400 dark:hover:text-primary-300 transition-colors"
        >
          {{ t('auth.signIn') REDACTEDREDACTED
        </router-link>
      </p>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted REDACTED from 'vue';
import { useRouter REDACTED from 'vue-router';
import { useI18n REDACTED from 'vue-i18n';
import { AuthLayout REDACTED from '@/components/layout';
import TurnstileWidget from '@/components/TurnstileWidget.vue';
import { useAuthStore, useAppStore REDACTED from '@/stores';
import { getPublicSettings REDACTED from '@/api/auth';

const { t REDACTED = useI18n();

// ==================== Router & Stores ====================

const router = useRouter();
const authStore = useAuthStore();
const appStore = useAppStore();

// ==================== State ====================

const isLoading = ref<boolean>(false);
const settingsLoaded = ref<boolean>(false);
const errorMessage = ref<string>('');
const showPassword = ref<boolean>(false);

// Public settings
const registrationEnabled = ref<boolean>(true);
const emailVerifyEnabled = ref<boolean>(false);
const turnstileEnabled = ref<boolean>(false);
const turnstileSiteKey = ref<string>('');
const siteName = ref<string>('Sub2API');

// Turnstile
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null);
const turnstileToken = ref<string>('');

const formData = reactive({
  email: '',
  password: '',
REDACTED);

const errors = reactive({
  email: '',
  password: '',
  turnstile: '',
REDACTED);

// ==================== Lifecycle ====================

onMounted(async () => {
  try {
    const settings = await getPublicSettings();
    registrationEnabled.value = settings.registration_enabled;
    emailVerifyEnabled.value = settings.email_verify_enabled;
    turnstileEnabled.value = settings.turnstile_enabled;
    turnstileSiteKey.value = settings.turnstile_site_key || '';
    siteName.value = settings.site_name || 'Sub2API';
  REDACTED catch (error) {
    console.error('Failed to load public settings:', error);
  REDACTED finally {
    settingsLoaded.value = true;
  REDACTED
REDACTED);

// ==================== Turnstile Handlers ====================

function onTurnstileVerify(token: string): void {
  turnstileToken.value = token;
  errors.turnstile = '';
REDACTED

function onTurnstileExpire(): void {
  turnstileToken.value = '';
  errors.turnstile = 'Verification expired, please try again';
REDACTED

function onTurnstileError(): void {
  turnstileToken.value = '';
  errors.turnstile = 'Verification failed, please try again';
REDACTED

// ==================== Validation ====================

function validateEmail(email: string): boolean {
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  return emailRegex.test(email);
REDACTED

function validateForm(): boolean {
  // Reset errors
  errors.email = '';
  errors.password = '';
  errors.turnstile = '';

  let isValid = true;

  // Email validation
  if (!formData.email.trim()) {
    errors.email = 'Email is required';
    isValid = false;
  REDACTED else if (!validateEmail(formData.email)) {
    errors.email = 'Please enter a valid email address';
    isValid = false;
  REDACTED

  // Password validation
  if (!formData.password) {
    errors.password = 'Password is required';
    isValid = false;
  REDACTED else if (formData.password.length < 6) {
    errors.password = 'Password must be at least 6 characters';
    isValid = false;
  REDACTED

  // Turnstile validation
  if (turnstileEnabled.value && !turnstileToken.value) {
    errors.turnstile = 'Please complete the verification';
    isValid = false;
  REDACTED

  return isValid;
REDACTED

// ==================== Form Handlers ====================

async function handleRegister(): Promise<void> {
  // Clear previous error
  errorMessage.value = '';

  // Validate form
  if (!validateForm()) {
    return;
  REDACTED

  isLoading.value = true;

  try {
    // If email verification is enabled, redirect to verification page
    if (emailVerifyEnabled.value) {
      // Store registration data in sessionStorage
      sessionStorage.setItem('register_data', JSON.stringify({
        email: formData.email,
        password: formData.password,
        turnstile_token: turnstileToken.value,
      REDACTED));

      // Navigate to email verification page
      await router.push('/email-verify');
      return;
    REDACTED

    // Otherwise, directly register
    await authStore.register({
      email: formData.email,
      password: formData.password,
      turnstile_token: turnstileEnabled.value ? turnstileToken.value : undefined,
    REDACTED);

    // Show success toast
    appStore.showSuccess('Account created successfully! Welcome to ' + siteName.value + '.');

    // Redirect to dashboard
    await router.push('/dashboard');
  REDACTED catch (error: unknown) {
    // Reset Turnstile on error
    if (turnstileRef.value) {
      turnstileRef.value.reset();
      turnstileToken.value = '';
    REDACTED

    // Handle registration error
    const err = error as { message?: string; response?: { data?: { detail?: string REDACTED REDACTED REDACTED;

    if (err.response?.data?.detail) {
      errorMessage.value = err.response.data.detail;
    REDACTED else if (err.message) {
      errorMessage.value = err.message;
    REDACTED else {
      errorMessage.value = 'Registration failed. Please try again.';
    REDACTED

    // Also show error toast
    appStore.showError(errorMessage.value);
  REDACTED finally {
    isLoading.value = false;
  REDACTED
REDACTED
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: all 0.3s ease;
REDACTED

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
REDACTED
</style>
