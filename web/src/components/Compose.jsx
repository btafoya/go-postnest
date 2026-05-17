import React, { useState, useEffect } from 'react'
import { useNavigate, useParams, useLocation } from 'react-router-dom'
import { Send, X, Paperclip } from 'lucide-react'
import { createDraft, updateDraft, sendDraft } from '../api'

export default function Compose() {
  const navigate = useNavigate()
  const { draftId } = useParams()
  const location = useLocation()
  const replyTo = location.state?.replyTo

  const [to, setTo] = useState(replyTo ? replyTo.from?.email : '')
  const [subject, setSubject] = useState(replyTo ? `Re: ${replyTo.subject || ''}` : '')
  const [body, setBody] = useState('')
  const [sending, setSending] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (replyTo) {
      setBody(`\n\nOn ${new Date(replyTo.date).toLocaleString()}, ${replyTo.from?.name || replyTo.from?.email} wrote:\n> ${replyTo.body?.replace(/\n/g, '\n> ') || replyTo.snippet || ''}`)
    }
  }, [replyTo])

  const handleSend = async () => {
    setSending(true)
    try {
      let id = draftId
      if (!id) {
        const draft = await createDraft({ to, subject, body })
        id = draft.id
      } else {
        await updateDraft(id, { to, subject, body })
      }
      await sendDraft(id)
      navigate('/')
    } catch (err) {
      alert('Failed to send: ' + (err.response?.data?.error || err.message))
    } finally {
      setSending(false)
    }
  }

  const handleSaveDraft = async () => {
    setSaving(true)
    try {
      if (draftId) {
        await updateDraft(draftId, { to, subject, body })
      } else {
        await createDraft({ to, subject, body })
      }
    } catch (err) {
      console.error('Save draft failed:', err)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="flex flex-col h-full bg-white">
      {/* Toolbar */}
      <div className="flex items-center gap-2 px-4 py-2 border-b border-surface-200">
        <button onClick={() => navigate(-1)} className="p-2 hover:bg-surface-100 rounded-md">
          <X className="h-4 w-4" />
        </button>
        <button
          onClick={handleSend}
          disabled={sending || !to}
          className="btn-primary gap-2 disabled:opacity-50"
        >
          <Send className="h-4 w-4" />
          {sending ? 'Sending...' : 'Send'}
        </button>
        <button onClick={handleSaveDraft} className="btn-secondary" disabled={saving}>
          {saving ? 'Saving...' : 'Save draft'}
        </button>
      </div>

      {/* Compose form */}
      <div className="flex-1 overflow-y-auto px-6 py-4">
        <div className="max-w-3xl mx-auto space-y-4">
          <div className="flex items-center gap-3 border-b border-surface-200 py-2">
            <span className="text-sm text-surface-500 w-12">To</span>
            <input
              type="text"
              value={to}
              onChange={(e) => setTo(e.target.value)}
              placeholder="recipient@example.com"
              className="flex-1 border-0 bg-transparent text-sm focus:ring-0 p-0"
            />
          </div>

          <div className="flex items-center gap-3 border-b border-surface-200 py-2">
            <span className="text-sm text-surface-500 w-12">Subject</span>
            <input
              type="text"
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              placeholder="Subject"
              className="flex-1 border-0 bg-transparent text-sm focus:ring-0 p-0"
            />
          </div>

          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="Write your message..."
            className="w-full flex-1 min-h-[400px] resize-none border-0 bg-transparent text-sm focus:ring-0 p-0"
            style={{ minHeight: '400px' }}
          />
        </div>
      </div>
    </div>
  )
}
