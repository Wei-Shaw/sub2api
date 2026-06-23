/**
 * Setup API endpoints
 */
import axios from 'axios'
import { buildGatewayUrl REDACTED from './url'

// Create a separate client for setup endpoints (not under /api/v1)
const setupClient = axios.create({
  baseURL: buildGatewayUrl('/').replace(/\/+$/, ''),
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  REDACTED
REDACTED)

export interface SetupStatus {
  needs_setup: boolean
  step: string
REDACTED

export interface DatabaseConfig {
  host: string
  port: number
  user: string
  password: string
  dbname: string
  sslmode: string
REDACTED

export interface RedisConfig {
  host: string
  port: number
  password: string
  db: number
  enable_tls: boolean
REDACTED

export interface AdminConfig {
  email: string
  password: string
REDACTED

export interface ServerConfig {
  host: string
  port: number
  mode: string
REDACTED

export interface InstallRequest {
  database: DatabaseConfig
  redis: RedisConfig
  admin: AdminConfig
  server: ServerConfig
REDACTED

export interface InstallResponse {
  message: string
  restart: boolean
REDACTED

/**
 * Get setup status
 */
export async function getSetupStatus(): Promise<SetupStatus> {
  const response = await setupClient.get('/setup/status')
  return response.data.data
REDACTED

/**
 * Test database connection
 */
export async function testDatabase(config: DatabaseConfig): Promise<void> {
  await setupClient.post('/setup/test-db', config)
REDACTED

/**
 * Test Redis connection
 */
export async function testRedis(config: RedisConfig): Promise<void> {
  await setupClient.post('/setup/test-redis', config)
REDACTED

/**
 * Perform installation
 */
export async function install(config: InstallRequest): Promise<InstallResponse> {
  const response = await setupClient.post('/setup/install', config)
  return response.data.data
REDACTED
