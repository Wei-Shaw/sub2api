import { apiClient REDACTED from './client'
import type { AuthResponse REDACTED from '@/types'

export interface PasskeyCredentialSummary {
  id: number
  name: string
  created_at: string
  last_used_at?: string
  backup: boolean
REDACTED

interface CeremonyOptionsResponse {
  session_token: string
  options: {
    publicKey: Record<string, unknown>
  REDACTED
REDACTED

function requirePasskeySupport(): void {
  if (!window.PublicKeyCredential || !navigator.credentials) {
    throw new Error('Passkeys are not supported by this browser')
  REDACTED
REDACTED

function base64URLToBuffer(value: string): ArrayBuffer {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
  const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4)
  const binary = atob(padded)
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0))
  return bytes.buffer
REDACTED

function bufferToBase64URL(value: ArrayBuffer | null): string | null {
  if (value === null) return null
  const bytes = new Uint8Array(value)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
REDACTED

function creationOptionsFromJSON(
  value: Record<string, unknown>
): PublicKeyCredentialCreationOptions {
  const options = { ...value REDACTED as Record<string, unknown>
  options.challenge = base64URLToBuffer(String(options.challenge))

  const user = { ...(options.user as Record<string, unknown>) REDACTED
  user.id = base64URLToBuffer(String(user.id))
  options.user = user

  if (Array.isArray(options.excludeCredentials)) {
    options.excludeCredentials = options.excludeCredentials.map((descriptor) => ({
      ...(descriptor as Record<string, unknown>),
      id: base64URLToBuffer(String((descriptor as Record<string, unknown>).id))
    REDACTED))
  REDACTED
  return options as unknown as PublicKeyCredentialCreationOptions
REDACTED

function requestOptionsFromJSON(
  value: Record<string, unknown>
): PublicKeyCredentialRequestOptions {
  const options = { ...value REDACTED as Record<string, unknown>
  options.challenge = base64URLToBuffer(String(options.challenge))
  if (Array.isArray(options.allowCredentials)) {
    options.allowCredentials = options.allowCredentials.map((descriptor) => ({
      ...(descriptor as Record<string, unknown>),
      id: base64URLToBuffer(String((descriptor as Record<string, unknown>).id))
    REDACTED))
  REDACTED
  return options as unknown as PublicKeyCredentialRequestOptions
REDACTED

function serializeRegistrationCredential(credential: PublicKeyCredential): Record<string, unknown> {
  const response = credential.response as AuthenticatorAttestationResponse
  return {
    id: credential.id,
    rawId: bufferToBase64URL(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment,
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      attestationObject: bufferToBase64URL(response.attestationObject),
      clientDataJSON: bufferToBase64URL(response.clientDataJSON),
      transports: typeof response.getTransports === 'function' ? response.getTransports() : []
    REDACTED
  REDACTED
REDACTED

function serializeAssertionCredential(credential: PublicKeyCredential): Record<string, unknown> {
  const response = credential.response as AuthenticatorAssertionResponse
  return {
    id: credential.id,
    rawId: bufferToBase64URL(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment,
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      authenticatorData: bufferToBase64URL(response.authenticatorData),
      clientDataJSON: bufferToBase64URL(response.clientDataJSON),
      signature: bufferToBase64URL(response.signature),
      userHandle: bufferToBase64URL(response.userHandle)
    REDACTED
  REDACTED
REDACTED

async function login(): Promise<AuthResponse> {
  requirePasskeySupport()
  const { data: begin REDACTED = await apiClient.post<CeremonyOptionsResponse>(
    '/auth/passkey/login/begin'
  )
  const credential = await navigator.credentials.get({
    publicKey: requestOptionsFromJSON(begin.options.publicKey)
  REDACTED)
  if (!(credential instanceof PublicKeyCredential)) {
    throw new Error('Passkey sign-in was cancelled')
  REDACTED
  const { data REDACTED = await apiClient.post<AuthResponse>('/auth/passkey/login/finish', {
    session_token: begin.session_token,
    credential: serializeAssertionCredential(credential)
  REDACTED)
  return data
REDACTED

async function register(name: string): Promise<PasskeyCredentialSummary> {
  requirePasskeySupport()
  const { data: begin REDACTED = await apiClient.post<CeremonyOptionsResponse>(
    '/user/passkeys/register/begin'
  )
  const credential = await navigator.credentials.create({
    publicKey: creationOptionsFromJSON(begin.options.publicKey)
  REDACTED)
  if (!(credential instanceof PublicKeyCredential)) {
    throw new Error('Passkey creation was cancelled')
  REDACTED
  const { data REDACTED = await apiClient.post<PasskeyCredentialSummary>(
    '/user/passkeys/register/finish',
    {
      session_token: begin.session_token,
      name,
      credential: serializeRegistrationCredential(credential)
    REDACTED
  )
  return data
REDACTED

async function list(): Promise<PasskeyCredentialSummary[]> {
  const { data REDACTED = await apiClient.get<PasskeyCredentialSummary[]>('/user/passkeys')
  return data
REDACTED

async function rename(id: number, name: string): Promise<void> {
  await apiClient.patch(`/user/passkeys/${idREDACTED`, { name REDACTED)
REDACTED

async function remove(id: number): Promise<void> {
  await apiClient.delete(`/user/passkeys/${idREDACTED`)
REDACTED

export const passkeyAPI = {
  isSupported: () => Boolean(window.PublicKeyCredential && navigator.credentials),
  login,
  register,
  list,
  rename,
  remove
REDACTED
