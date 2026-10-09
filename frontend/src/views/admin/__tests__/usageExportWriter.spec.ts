import { describe, expect, it } from 'vitest'

describe('admin usage SheetJS writer compatibility', () => {
  it('writes the paginated array export as an XLSX workbook', async () => {
    const spreadsheet = await import('xlsx')
    const worksheet = spreadsheet.utils.aoa_to_sheet([
      ['User', 'Requested model', 'Input tokens', 'Cost']
    ])
    spreadsheet.utils.sheet_add_aoa(worksheet, [
      ['admin@example.com', 'gpt-5.6-sol', 42, '0.000123']
    ], { origin: -1 })
    spreadsheet.utils.sheet_add_aoa(worksheet, [
      ['用户@example.com', 'gpt-5.5', 0, '0.000000']
    ], { origin: -1 })

    const workbook = spreadsheet.utils.book_new()
    spreadsheet.utils.book_append_sheet(workbook, worksheet, 'Usage')
    const output = spreadsheet.write(workbook, { bookType: 'xlsx', type: 'array' })
    const bytes = new Uint8Array(output)

    expect(workbook.SheetNames).toEqual(['Usage'])
    expect(worksheet['!ref']).toBe('A1:D3')
    expect(worksheet.C2).toMatchObject({ t: 'n', v: 42 })
    expect(worksheet.D2).toMatchObject({ t: 's', v: '0.000123' })
    expect(Array.from(bytes.slice(0, 4))).toEqual([0x50, 0x4b, 0x03, 0x04])
    expect(bytes.byteLength).toBeGreaterThan(100)
  })

  it('keeps user-controlled export strings as literal cells', async () => {
    const spreadsheet = await import('xlsx')
    const values = ['=HYPERLINK("https://example.com")', '<model>&"用户"', 'a'.repeat(10000)]
    const worksheet = spreadsheet.utils.aoa_to_sheet([values])

    for (const [index, value] of values.entries()) {
      const address = spreadsheet.utils.encode_cell({ r: 0, c: index })
      expect(worksheet[address]).toMatchObject({ t: 's', v: value })
      expect(worksheet[address].f).toBeUndefined()
    }

    const workbook = spreadsheet.utils.book_new()
    spreadsheet.utils.book_append_sheet(workbook, worksheet, 'Usage')
    expect(() => spreadsheet.write(workbook, { bookType: 'xlsx', type: 'array' })).not.toThrow()
  })
})
