import { describe, it, expect } from 'vitest'
import { htmlToText } from '../components/RichEditor'

describe('htmlToText', () => {
  it('strips tags', () => {
    expect(htmlToText('<p>Hello <b>world</b></p>')).toBe('Hello world')
  })

  it('handles empty', () => {
    expect(htmlToText('')).toBe('')
    expect(htmlToText(null)).toBe('')
  })

  it('does not execute scripts', () => {
    // DOMParser produces an inert document; script content is not run and
    // textContent excludes it from rendering.
    const out = htmlToText('<div>safe</div><script>window.__x=1</script>')
    expect(window.__x).toBeUndefined()
    expect(out).toContain('safe')
  })
})
