import { describe, it, expect } from 'vitest'
import { parseRecipients } from '../api'

describe('parseRecipients', () => {
  it('returns array unchanged', () => {
    const arr = [{ address: 'a@b.com' }]
    expect(parseRecipients(arr)).toBe(arr)
  })

  it('returns empty for falsy', () => {
    expect(parseRecipients('')).toEqual([])
    expect(parseRecipients(null)).toEqual([])
  })

  it('parses comma-separated bare addresses', () => {
    expect(parseRecipients('a@x.com, b@y.com')).toEqual([
      { address: 'a@x.com' },
      { address: 'b@y.com' },
    ])
  })

  it('parses display-name form', () => {
    expect(parseRecipients('Alice <alice@x.com>')).toEqual([
      { name: 'Alice', address: 'alice@x.com' },
    ])
  })

  it('handles semicolons and whitespace', () => {
    expect(parseRecipients(' a@x.com ; b@y.com ')).toEqual([
      { address: 'a@x.com' },
      { address: 'b@y.com' },
    ])
  })
})
