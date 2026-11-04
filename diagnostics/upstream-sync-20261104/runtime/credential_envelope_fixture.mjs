import { readFileSync } from 'node:fs'
import { randomBytes, publicEncrypt, createPublicKey, createCipheriv, constants } from 'node:crypto'

const { server, email, password } = JSON.parse(readFileSync(0, 'utf8'))
const publicKey = createPublicKey({ key: Buffer.from(server.public_key, 'base64url'), format: 'der', type: 'spki' })
const aes = randomBytes(32)
const iv = randomBytes(12)
const cipher = createCipheriv('aes-256-gcm', aes, iv)
cipher.setAAD(Buffer.from(server.key_id))
const plaintext = JSON.stringify({ email, password, issued_at: server.server_time })
const ciphertext = Buffer.concat([cipher.update(plaintext), cipher.final(), cipher.getAuthTag()])
const encryptedKey = publicEncrypt({ key: publicKey, padding: constants.RSA_PKCS1_OAEP_PADDING, oaepHash: 'sha256' }, aes)
process.stdout.write(JSON.stringify({ algorithm: server.algorithm, key_id: server.key_id, encrypted_key: encryptedKey.toString('base64url'), iv: iv.toString('base64url'), ciphertext: ciphertext.toString('base64url') }))
