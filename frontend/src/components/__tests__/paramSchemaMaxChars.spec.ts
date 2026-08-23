import { describe, expect, it } from 'vitest'
import { makeSchemaRow, rowToSchema, schemaToRow } from '@/components/common/paramSchemaRow'

describe('paramSchemaRow: max chars', () => {
  it('round-trips an optional string max_chars value', () => {
    const row = makeSchemaRow({ key: 'message', type: 'string', maxChars: 24 })
    expect(rowToSchema(row)).toMatchObject({ value: '', max_chars: 24 })
    expect(schemaToRow('message', { value: '', max_chars: 24 }).maxChars).toBe(24)
  })

  it('omits the limit when it is unset', () => {
    expect(rowToSchema(makeSchemaRow({ key: 'message', type: 'string' }))).not.toHaveProperty('max_chars')
  })
})
