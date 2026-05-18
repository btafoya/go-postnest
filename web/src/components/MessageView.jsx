import React, { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import DOMPurify from 'dompurify'
import { ArrowLeft, Reply, ReplyAll, Forward, Trash2, Archive, AlertCircle, Star, Paperclip, Printer, MoreVertical } from 'lucide-react'
import { getMessage, patchMessage, deleteMessage } from '../api'

export default function MessageView() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [message, setMessage] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    getMessage(id)
      .then((data) => {
        setMessage(data)
        // Mark as read if unread
        if (!data.is_read) {
          patchMessage(id, { is_read: true }).catch(console.error)
        }
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [id])

  const handleDelete = async () => {
    await deleteMessage(id)
    navigate(-1)
  }

  const handleToggleStar = async () => {
    await patchMessage(id, { is_flagged: !message.is_flagged })
    setMessage({ ...message, is_flagged: !message.is_flagged })
  }

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
      </div>
    )
  }

  if (!message) {
    return (
      <div className="flex h-full items-center justify-center text-surface-500">
        Message not found
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full bg-white">
      {/* Toolbar */}
      <div className="flex items-center gap-2 px-4 py-2 border-b border-surface-200">
        <button onClick={() => navigate(-1)} className="p-2 hover:bg-surface-100 rounded-md">
          <ArrowLeft className="h-4 w-4" />
        </button>
        <button onClick={handleDelete} className="p-2 hover:bg-surface-100 rounded-md" title="Delete">
          <Trash2 className="h-4 w-4" />
        </button>
        <button onClick={() => navigate('/compose', { state: { replyTo: message } })} className="p-2 hover:bg-surface-100 rounded-md" title="Reply">
          <Reply className="h-4 w-4" />
        </button>
        <button className="p-2 hover:bg-surface-100 rounded-md" title="Forward">
          <Forward className="h-4 w-4" />
        </button>
        <div className="flex-1"></div>
        <button onClick={handleToggleStar} className="p-2 hover:bg-surface-100 rounded-md">
          <Star className={`h-4 w-4 ${message.is_flagged ? 'fill-yellow-400 text-yellow-400' : 'text-surface-300'}`} />
        </button>
      </div>

      {/* Message header */}
      <div className="px-6 py-4 border-b border-surface-200">
        <h1 className="text-xl font-medium text-surface-900 mb-4">{message.subject || '(no subject)'}</h1>

        <div className="flex items-start gap-3">
          <div className="h-10 w-10 rounded-full bg-primary-100 flex items-center justify-center text-primary-700 font-semibold flex-shrink-0">
            {(message.from?.name || message.from?.email || 'U').charAt(0).toUpperCase()}
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-semibold text-surface-900">{message.from?.name || message.from?.email}</p>
                <p className="text-xs text-surface-500">{message.from?.email}</p>
              </div>
              <p className="text-xs text-surface-400">{new Date(message.date).toLocaleString()}</p>
            </div>

            <div className="mt-2 text-xs text-surface-500">
              <span>To: </span>
              {message.to?.map((t) => t.email).join(', ')}
            </div>

            {message.labels?.length > 0 && (
              <div className="flex flex-wrap gap-1 mt-2">
                {message.labels.map((label) => (
                  <span key={label} className="inline-flex items-center px-2 py-0.5 rounded text-xs bg-surface-100 text-surface-600">
                    {label}
                  </span>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Message body */}
      <div className="flex-1 overflow-y-auto px-6 py-4">
        {message.html_body ? (
          <iframe
            title="message-body"
            sandbox=""
            className="w-full h-full min-h-[400px] border-0"
            srcDoc={DOMPurify.sanitize(message.html_body)}
          />
        ) : (
          <div className="prose max-w-none text-surface-800 whitespace-pre-wrap">
            {message.plain_text || message.snippet || 'No content'}
          </div>
        )}
      </div>
    </div>
  )
}
