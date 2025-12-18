<template>
  <div class="min-h-screen bg-gradient-to-br from-gray-50 to-gray-100 dark:from-dark-900 dark:to-dark-800 flex items-center justify-center p-4">
    <div class="w-full max-w-2xl">
      <!-- Logo & Title -->
      <div class="text-center mb-8">
        <div class="inline-flex items-center justify-center w-16 h-16 rounded-2xl bg-gradient-to-br from-primary-500 to-primary-600 shadow-lg mb-4">
          <svg class="w-8 h-8 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z" />
            <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
          </svg>
        </div>
        <h1 class="text-3xl font-bold text-gray-900 dark:text-white">Sub2API Setup</h1>
        <p class="mt-2 text-gray-500 dark:text-dark-400">Configure your Sub2API instance</p>
      </div>

      <!-- Progress Steps -->
      <div class="mb-8">
        <div class="flex items-center justify-center">
          <template v-for="(step, index) in steps" :key="step.id">
            <div class="flex items-center">
              <div
                :class="[
                  'w-10 h-10 rounded-full flex items-center justify-center font-semibold text-sm transition-all',
                  currentStep > index
                    ? 'bg-primary-500 text-white'
                    : currentStep === index
                    ? 'bg-primary-500 text-white ring-4 ring-primary-100 dark:ring-primary-900'
                    : 'bg-gray-200 dark:bg-dark-700 text-gray-500 dark:text-dark-400'
                ]"
              >
                <svg v-if="currentStep > index" class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                </svg>
                <span v-else>{{ index + 1 REDACTEDREDACTED</span>
              </div>
              <span class="ml-2 text-sm font-medium" :class="currentStep >= index ? 'text-gray-900 dark:text-white' : 'text-gray-400 dark:text-dark-500'">
                {{ step.title REDACTEDREDACTED
              </span>
            </div>
            <div v-if="index < steps.length - 1" class="w-12 h-0.5 mx-3" :class="currentStep > index ? 'bg-primary-500' : 'bg-gray-200 dark:bg-dark-700'"></div>
          </template>
        </div>
      </div>

      <!-- Step Content -->
      <div class="bg-white dark:bg-dark-800 rounded-2xl shadow-xl p-8">
        <!-- Step 1: Database -->
        <div v-if="currentStep === 0" class="space-y-6">
          <div class="text-center mb-6">
            <h2 class="text-xl font-semibold text-gray-900 dark:text-white">Database Configuration</h2>
            <p class="text-sm text-gray-500 dark:text-dark-400 mt-1">Connect to your PostgreSQL database</p>
          </div>

          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="input-label">Host</label>
              <input v-model="formData.database.host" type="text" class="input" placeholder="localhost" />
            </div>
            <div>
              <label class="input-label">Port</label>
              <input v-model.number="formData.database.port" type="number" class="input" placeholder="5432" />
            </div>
          </div>

          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="input-label">Username</label>
              <input v-model="formData.database.user" type="text" class="input" placeholder="postgres" />
            </div>
            <div>
              <label class="input-label">Password</label>
              <input v-model="formData.database.password" type="password" class="input" placeholder="Password" />
            </div>
          </div>

          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="input-label">Database Name</label>
              <input v-model="formData.database.dbname" type="text" class="input" placeholder="sub2api" />
            </div>
            <div>
              <label class="input-label">SSL Mode</label>
              <select v-model="formData.database.sslmode" class="input">
                <option value="disable">Disable</option>
                <option value="require">Require</option>
                <option value="verify-ca">Verify CA</option>
                <option value="verify-full">Verify Full</option>
              </select>
            </div>
          </div>

          <button
            @click="testDatabaseConnection"
            :disabled="testingDb"
            class="btn btn-secondary w-full"
          >
            <svg v-if="testingDb" class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            <svg v-else-if="dbConnected" class="w-5 h-5 mr-2 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
            </svg>
            {{ testingDb ? 'Testing...' : dbConnected ? 'Connection Successful' : 'Test Connection' REDACTEDREDACTED
          </button>
        </div>

        <!-- Step 2: Redis -->
        <div v-if="currentStep === 1" class="space-y-6">
          <div class="text-center mb-6">
            <h2 class="text-xl font-semibold text-gray-900 dark:text-white">Redis Configuration</h2>
            <p class="text-sm text-gray-500 dark:text-dark-400 mt-1">Connect to your Redis server</p>
          </div>

          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="input-label">Host</label>
              <input v-model="formData.redis.host" type="text" class="input" placeholder="localhost" />
            </div>
            <div>
              <label class="input-label">Port</label>
              <input v-model.number="formData.redis.port" type="number" class="input" placeholder="6379" />
            </div>
          </div>

          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="input-label">Password (optional)</label>
              <input v-model="formData.redis.password" type="password" class="input" placeholder="Password" />
            </div>
            <div>
              <label class="input-label">Database</label>
              <input v-model.number="formData.redis.db" type="number" class="input" placeholder="0" />
            </div>
          </div>

          <button
            @click="testRedisConnection"
            :disabled="testingRedis"
            class="btn btn-secondary w-full"
          >
            <svg v-if="testingRedis" class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            <svg v-else-if="redisConnected" class="w-5 h-5 mr-2 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
            </svg>
            {{ testingRedis ? 'Testing...' : redisConnected ? 'Connection Successful' : 'Test Connection' REDACTEDREDACTED
          </button>
        </div>

        <!-- Step 3: Admin -->
        <div v-if="currentStep === 2" class="space-y-6">
          <div class="text-center mb-6">
            <h2 class="text-xl font-semibold text-gray-900 dark:text-white">Admin Account</h2>
            <p class="text-sm text-gray-500 dark:text-dark-400 mt-1">Create your administrator account</p>
          </div>

          <div>
            <label class="input-label">Email</label>
            <input v-model="formData.admin.email" type="email" class="input" placeholder="admin@example.com" />
          </div>

          <div>
            <label class="input-label">Password</label>
            <input v-model="formData.admin.password" type="password" class="input" placeholder="Min 6 characters" />
          </div>

          <div>
            <label class="input-label">Confirm Password</label>
            <input v-model="confirmPassword" type="password" class="input" placeholder="Confirm password" />
            <p v-if="confirmPassword && formData.admin.password !== confirmPassword" class="input-error-text">
              Passwords do not match
            </p>
          </div>
        </div>

        <!-- Step 4: Complete -->
        <div v-if="currentStep === 3" class="space-y-6">
          <div class="text-center mb-6">
            <h2 class="text-xl font-semibold text-gray-900 dark:text-white">Ready to Install</h2>
            <p class="text-sm text-gray-500 dark:text-dark-400 mt-1">Review your configuration and complete setup</p>
          </div>

          <div class="space-y-4">
            <div class="p-4 bg-gray-50 dark:bg-dark-700 rounded-xl">
              <h3 class="text-sm font-medium text-gray-500 dark:text-dark-400 mb-2">Database</h3>
              <p class="text-gray-900 dark:text-white">{{ formData.database.user REDACTEDREDACTED@{{ formData.database.host REDACTEDREDACTED:{{ formData.database.port REDACTEDREDACTED/{{ formData.database.dbname REDACTEDREDACTED</p>
            </div>

            <div class="p-4 bg-gray-50 dark:bg-dark-700 rounded-xl">
              <h3 class="text-sm font-medium text-gray-500 dark:text-dark-400 mb-2">Redis</h3>
              <p class="text-gray-900 dark:text-white">{{ formData.redis.host REDACTEDREDACTED:{{ formData.redis.port REDACTEDREDACTED</p>
            </div>

            <div class="p-4 bg-gray-50 dark:bg-dark-700 rounded-xl">
              <h3 class="text-sm font-medium text-gray-500 dark:text-dark-400 mb-2">Admin Email</h3>
              <p class="text-gray-900 dark:text-white">{{ formData.admin.email REDACTEDREDACTED</p>
            </div>
          </div>
        </div>

        <!-- Error Message -->
        <div v-if="errorMessage" class="mt-6 p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800/50 rounded-xl">
          <div class="flex items-start gap-3">
            <svg class="w-5 h-5 text-red-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
            </svg>
            <p class="text-sm text-red-700 dark:text-red-400">{{ errorMessage REDACTEDREDACTED</p>
          </div>
        </div>

        <!-- Success Message -->
        <div v-if="installSuccess" class="mt-6 p-4 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800/50 rounded-xl">
          <div class="flex items-start gap-3">
            <svg class="w-5 h-5 text-green-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <div>
              <p class="text-sm font-medium text-green-700 dark:text-green-400">Installation completed!</p>
              <p class="text-sm text-green-600 dark:text-green-500 mt-1">Please restart the service to apply changes.</p>
            </div>
          </div>
        </div>

        <!-- Navigation Buttons -->
        <div class="mt-8 flex justify-between">
          <button
            v-if="currentStep > 0 && !installSuccess"
            @click="currentStep--"
            class="btn btn-secondary"
          >
            <svg class="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
            </svg>
            Previous
          </button>
          <div v-else></div>

          <button
            v-if="currentStep < 3"
            @click="nextStep"
            :disabled="!canProceed"
            class="btn btn-primary"
          >
            Next
            <svg class="w-4 h-4 ml-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
            </svg>
          </button>

          <button
            v-else-if="!installSuccess"
            @click="performInstall"
            :disabled="installing"
            class="btn btn-primary"
          >
            <svg v-if="installing" class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            {{ installing ? 'Installing...' : 'Complete Installation' REDACTEDREDACTED
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed REDACTED from 'vue';
import { testDatabase, testRedis, install, type InstallRequest REDACTED from '@/api/setup';

const steps = [
  { id: 'database', title: 'Database' REDACTED,
  { id: 'redis', title: 'Redis' REDACTED,
  { id: 'admin', title: 'Admin' REDACTED,
  { id: 'complete', title: 'Complete' REDACTED,
];

const currentStep = ref(0);
const errorMessage = ref('');
const installSuccess = ref(false);

// Connection test states
const testingDb = ref(false);
const testingRedis = ref(false);
const dbConnected = ref(false);
const redisConnected = ref(false);
const installing = ref(false);
const confirmPassword = ref('');

// Get current server port from browser location (set by install.sh)
const getCurrentPort = (): number => {
  const port = window.location.port;
  if (port) {
    return parseInt(port, 10);
  REDACTED
  // Default port based on protocol
  return window.location.protocol === 'https:' ? 443 : 80;
REDACTED;

const formData = reactive<InstallRequest>({
  database: {
    host: 'localhost',
    port: 5432,
    user: 'postgres',
    password: '',
    dbname: 'sub2api',
    sslmode: 'disable',
  REDACTED,
  redis: {
    host: 'localhost',
    port: 6379,
    password: '',
    db: 0,
  REDACTED,
  admin: {
    email: '',
    password: '',
  REDACTED,
  server: {
    host: '0.0.0.0',
    port: getCurrentPort(),  // Use current port from browser
    mode: 'release',
  REDACTED,
REDACTED);

const canProceed = computed(() => {
  switch (currentStep.value) {
    case 0:
      return dbConnected.value;
    case 1:
      return redisConnected.value;
    case 2:
      return (
        formData.admin.email &&
        formData.admin.password.length >= 6 &&
        formData.admin.password === confirmPassword.value
      );
    default:
      return true;
  REDACTED
REDACTED);

async function testDatabaseConnection() {
  testingDb.value = true;
  errorMessage.value = '';
  dbConnected.value = false;

  try {
    await testDatabase(formData.database);
    dbConnected.value = true;
  REDACTED catch (error: unknown) {
    const err = error as { response?: { data?: { detail?: string REDACTED REDACTED; message?: string REDACTED;
    errorMessage.value = err.response?.data?.detail || err.message || 'Connection failed';
  REDACTED finally {
    testingDb.value = false;
  REDACTED
REDACTED

async function testRedisConnection() {
  testingRedis.value = true;
  errorMessage.value = '';
  redisConnected.value = false;

  try {
    await testRedis(formData.redis);
    redisConnected.value = true;
  REDACTED catch (error: unknown) {
    const err = error as { response?: { data?: { detail?: string REDACTED REDACTED; message?: string REDACTED;
    errorMessage.value = err.response?.data?.detail || err.message || 'Connection failed';
  REDACTED finally {
    testingRedis.value = false;
  REDACTED
REDACTED

function nextStep() {
  if (canProceed.value) {
    errorMessage.value = '';
    currentStep.value++;
  REDACTED
REDACTED

async function performInstall() {
  installing.value = true;
  errorMessage.value = '';

  try {
    await install(formData);
    installSuccess.value = true;
  REDACTED catch (error: unknown) {
    const err = error as { response?: { data?: { detail?: string REDACTED REDACTED; message?: string REDACTED;
    errorMessage.value = err.response?.data?.detail || err.message || 'Installation failed';
  REDACTED finally {
    installing.value = false;
  REDACTED
REDACTED
</script>
