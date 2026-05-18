import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'

vi.mock('../api', () => ({
  getMessage: vi.fn(),
  patchMessage: vi.fn().mockResolvedValue({}),
  deleteMessage: vi.fn(),
}))

import { getMessage } from '../api'
import MessageView from '../components/MessageView'

function renderAt(id) {
  return render(
    <MemoryRouter initialEntries={[`/message/${id}`]}>
      <Routes>
        <Route path="/message/:id" element={<MessageView />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('MessageView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders html_body in a sandboxed iframe', async () => {
    getMessage.mockResolvedValue({
      id: '1', subject: 'Hi', is_read: true,
      from: { email: 'a@b.com' }, to: [{ email: 'c@d.com' }],
      date: new Date().toISOString(),
      html_body: '<p>Rich <b>body</b></p>',
    })
    const { container } = renderAt('1')
    await waitFor(() => {
      const iframe = container.querySelector('iframe[title="message-body"]')
      expect(iframe).toBeTruthy()
      expect(iframe.getAttribute('sandbox')).toBe('')
    })
  })

  it('falls back to plain_text when no html_body', async () => {
    getMessage.mockResolvedValue({
      id: '2', subject: 'Plain', is_read: true,
      from: { email: 'a@b.com' }, to: [{ email: 'c@d.com' }],
      date: new Date().toISOString(),
      plain_text: 'just text',
    })
    renderAt('2')
    await waitFor(() => expect(screen.getByText('just text')).toBeInTheDocument())
  })
})
