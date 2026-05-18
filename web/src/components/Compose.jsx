import React, { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate, useParams, useLocation } from 'react-router-dom'
import { Send, X, Paperclip, Trash2 } from 'lucide-react'
import {
  createDraft, updateDraft, sendDraft, parseRecipients,
  listDraftAttachments, uploadDraftAttachment, deleteDraftAttachment,
} from '../api'
import RichEditor, { htmlToText } from './RichEditor'

export default function Compose() {
  const navigate = useNavigate()
  const { draftId: routeDraftId } = useParams()
  const location = useLocation()
  const replyTo = location.state?.replyTo

  const [draftId, setDraftId] = useState(routeDraftId || null)
  const [to, setTo] = useState(replyTo ? replyTo.from?.email || '' : '')
  const [cc, setCc] = useState('')
  const [bcc, setBcc] = useState('')
  const [subject, setSubject] = useState(replyTo ? `Re: ${replyTo.subject || ''}` : '')
  const [html, setHtml] = useState('')
  const [attachments, setAttachments] = useState([])
  const [sending, setSending] = useState(false)
  const [savedAt, setSavedAt] = useState(null)
  const [error, setError] = useState('')
  const fileRef = useRef(null)

  useEffect(() => {
    if (replyTo) {
      const quoted = replyTo.html_body || replyTo.plain_text || replyTo.snippet || ''
      setHtml(`<p></p><blockquote>On ${new Date(replyTo.date).toLocaleString()}, ${replyTo.from?.name || replyTo.from?.email} wrote:<br/>${quoted}</blockquote>`)
    }
  }, [replyTo])

  useEffect(() => {
    if (draftId) listDraftAttachments(draftId).then(setAttachments).catch(() => {})
  }, [draftId])

  const payload = useCallback(() => ({
    to: parseRecipients(to),
    cc: parseRecipients(cc),
    bcc: parseRecipients(bcc),
    subject,
    html_body: html,
    plain_text: htmlToText(html),
  }), [to, cc, bcc, subject, html])

  const persist = useCallback(async () => {
    if (!to && !subject && !html) return
    try {
      if (draftId) {
        await updateDraft(draftId, payload())
      } else {
        const d = await createDraft(payload())
        if (d?.id) setDraftId(d.id)
      }
      setSavedAt(new Date())
    } catch (e) {
      // autosave failures are non-fatal; surfaced on explicit send
    }
  }, [draftId, payload, to, subject, html])

  // Autosave: debounce 3s after any change.
  useEffect(() => {
    const t = setTimeout(persist, 3000)
    return () => clearTimeout(t)
  }, [to, cc, bcc, subject, html, persist])

  const handleSend = async () => {
    setSending(true)
    setError('')
    try {
      let id = draftId
      if (!id) {
        const d = await createDraft(payload())
        id = d.id
        setDraftId(id)
      } else {
        await updateDraft(id, payload())
      }
      await sendDraft(id)
      navigate('/')
    } catch (err) {
      setError(err.response?.data?.error?.message || err.message || 'Failed to send')
    } finally {
      setSending(false)
    }
  }

  const ensureDraft = async () => {
    if (draftId) return draftId
    const d = await createDraft(payload())
    setDraftId(d.id)
    return d.id
  }

  const handleFiles = async (files) => {
    const id = await ensureDraft()
    for (const f of files) {
      try {
        await uploadDraftAttachment(id, f)
      } catch (e) {
        setError(`Attachment "${f.name}" failed: ${e.response?.data?.error?.message || e.message}`)
      }
    }
    listDraftAttachments(id).then(setAttachments).catch(() => {})
  }

  const onDrop = (e) => {
    e.preventDefault()
    if (e.dataTransfer.files?.length) handleFiles(Array.from(e.dataTransfer.files))
  }

  const removeAttachment = async (attID) => {
    await deleteDraftAttachment(draftId, attID)
    setAttachments((a) => a.filter((x) => x.id !== attID))
  }

  return (
    <div className="flex flex-col h-full bg-white" onDragOver={(e) => e.preventDefault()} onDrop={onDrop}>
      <div className="flex items-center gap-2 px-4 py-2 border-b border-surface-200">
        <button onClick={() => navigate(-1)} className="p-2 hover:bg-surface-100 rounded-md">
          <X className="h-4 w-4" />
        </button>
        <button onClick={handleSend} disabled={sending || parseRecipients(to).length === 0} className="btn-primary gap-2 disabled:opacity-50">
          <Send className="h-4 w-4" />
          {sending ? 'Sending...' : 'Send'}
        </button>
        <button onClick={() => fileRef.current?.click()} className="btn-secondary gap-2">
          <Paperclip className="h-4 w-4" /> Attach
        </button>
        <input ref={fileRef} type="file" multiple className="hidden" onChange={(e) => handleFiles(Array.from(e.target.files))} />
        <div className="flex-1" />
        {savedAt && <span className="text-xs text-surface-400">Saved {savedAt.toLocaleTimeString()}</span>}
      </div>

      {error && (
        <div className="mx-4 mt-2 rounded-md bg-red-50 px-4 py-2 text-sm text-red-700">{error}</div>
      )}

      <div className="flex-1 overflow-y-auto px-6 py-4">
        <div className="max-w-3xl mx-auto flex flex-col h-full">
          <Field label="To" value={to} onChange={setTo} placeholder="recipient@example.com, Name <a@b.com>" />
          <Field label="Cc" value={cc} onChange={setCc} placeholder="" />
          <Field label="Bcc" value={bcc} onChange={setBcc} placeholder="" />
          <Field label="Subject" value={subject} onChange={setSubject} placeholder="Subject" />

          {attachments.length > 0 && (
            <div className="flex flex-wrap gap-2 py-2 border-b border-surface-200">
              {attachments.map((a) => (
                <span key={a.id} className="inline-flex items-center gap-2 rounded bg-surface-100 px-2 py-1 text-xs">
                  <Paperclip className="h-3 w-3" />
                  {a.filename} ({Math.round((a.size || 0) / 1024)}KB)
                  <button onClick={() => removeAttachment(a.id)} className="text-surface-500 hover:text-red-600">
                    <Trash2 className="h-3 w-3" />
                  </button>
                </span>
              ))}
            </div>
          )}

          <div className="flex-1 py-3 flex flex-col">
            <RichEditor value={html} onChange={setHtml} />
          </div>
        </div>
      </div>
    </div>
  )
}

function Field({ label, value, onChange, placeholder }) {
  return (
    <div className="flex items-center gap-3 border-b border-surface-200 py-2">
      <span className="text-sm text-surface-500 w-12">{label}</span>
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="flex-1 border-0 bg-transparent text-sm focus:ring-0 p-0"
      />
    </div>
  )
}
