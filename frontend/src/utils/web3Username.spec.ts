import { validateWeb3Username } from './web3Username'

describe('validateWeb3Username', () => {
  it('normalizes a valid username', () => {
    expect(validateWeb3Username('  Alice  ')).toEqual({
      username: 'Alice',
      error: null,
    })
  })

  it.each([
    ['', 'required'],
    ['A', 'tooShort'],
    ['用'.repeat(101), 'tooLong'],
    ['Alice\nAdmin', 'invalid'],
  ] as const)('rejects invalid username %j', (username, error) => {
    expect(validateWeb3Username(username)).toEqual({
      username: username.trim(),
      error,
    })
  })
})
