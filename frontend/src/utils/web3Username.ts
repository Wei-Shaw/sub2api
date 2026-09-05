export const WEB3_USERNAME_MIN_LENGTH = 2
export const WEB3_USERNAME_MAX_LENGTH = 100

export type Web3UsernameValidationError = 'required' | 'tooShort' | 'tooLong' | 'invalid'

export interface Web3UsernameValidationResult {
  username: string
  error: Web3UsernameValidationError | null
}

export function validateWeb3Username(value: string): Web3UsernameValidationResult {
  const username = value.trim()
  const length = Array.from(username).length

  if (length === 0) return { username, error: 'required' }
  if (length < WEB3_USERNAME_MIN_LENGTH) return { username, error: 'tooShort' }
  if (length > WEB3_USERNAME_MAX_LENGTH) return { username, error: 'tooLong' }
  if (containsControlCharacter(username)) return { username, error: 'invalid' }

  return { username, error: null }
}

function containsControlCharacter(value: string): boolean {
  return Array.from(value).some((character) => {
    const codePoint = character.codePointAt(0) ?? 0
    return codePoint <= 31 || (codePoint >= 127 && codePoint <= 159)
  })
}
