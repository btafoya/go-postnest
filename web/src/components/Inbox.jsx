import React, { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { Star, Trash2, Archive, AlertCircle, ChevronLeft, ChevronRight, RefreshCw, MoreVertical, CheckSquare, Square } from 'lucide-react'
import { getMessages, getLabels, patchMessage, batchMessages, deleteMessage } from '../api'
import sse from '../sse'

export default function Inbox() {
  const { labelId } = useParams()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const searchQuery = searchParams.get('q')

  const [messages, setMessages] = useState([])
  const [labels, setLabels] = useState([])
  const [loading, setLoading] = useState(true)
  const [selectedIds, setSelectedIds] = useState(new Set())
  const [offset, setOffset] = useState(0)
  const [total, setTotal] = useState(0)
  const [notice, setNotice] = useState('')
  const limit = 50

  const currentLabel = labels.find((l) => l.id === labelId) || { name: 'Inbox', id: null }

  const fetchMessages = useCallback(async () => {
    setLoading(true)
    try {
      const params = { limit, offset }
      if (searchQuery) params.q = searchQuery
      const data = await getMessages(labelId || null, params)
      setMessages(data.messages || [])
      setTotal(data.total || 0)
    } catch (err) {
      console.error('Failed to load messages:', err)
    } finally {
      setLoading(false)
    }
  }, [labelId, offset, searchQuery])

  useEffect(() => {
    getLabels().then((data) => setLabels(data)).catch(console.error)
  }, [])

  useEffect(() => {
    fetchMessages()
    setSelectedIds(new Set())
  }, [fetchMessages])

  useEffect(() => {
    const unsub = sse.on('message:new', () => {
      fetchMessages()
    })
    return unsub
  }, [fetchMessages])

  const toggleSelect = (id) => {
    const next = new Set(selectedIds)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setSelectedIds(next)
  }

  const toggleSelectAll = () => {
    if (selectedIds.size === messages.length) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(messages.map((m) => m.id)))
    }
  }

  const handleBatch = async (action) => {
    if (selectedIds.size === 0) return
    try {
      const res = await batchMessages(action, Array.from(selectedIds))
      if (res?.failed?.length > 0) {
        setNotice(`${res.failed.length} of ${selectedIds.size} message(s) failed to ${action}`)
      } else {
        setNotice('')
      }
      fetchMessages()
      setSelectedIds(new Set())
    } catch (err) {
      setNotice(`Batch ${action} failed: ${err.response?.data?.error?.message || err.message}`)
    }
  }

  const handleDelete = async (id) => {
    try {
      await deleteMessage(id)
      fetchMessages()
    } catch (err) {
      console.error('Delete failed:', err)
    }
  }

  const handleToggleStar = async (msg) => {
    try {
      await patchMessage(msg.id, { is_flagged: !msg.is_flagged })
      fetchMessages()
    } catch (err) {
      console.error('Star toggle failed:', err)
    }
  }

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="flex items-center gap-2 px-4 py-2 border-b border-surface-200 bg-white">
        <button onClick={toggleSelectAll} className="p-2 hover:bg-surface-100 rounded-md">
          {selectedIds.size === messages.length && messages.length > 0 ? <CheckSquare className="h-4 w-4" /> : <Square className="h-4 w-4" />}
        </button>

        {selectedIds.size > 0 && (
          <>
            <button onClick={() => handleBatch('archive')} className="p-2 hover:bg-surface-100 rounded-md" title="Archive">
              <Archive className="h-4 w-4" />
            </button>
            <button onClick={() => handleBatch('spam')} className="p-2 hover:bg-surface-100 rounded-md" title="Report spam">
              <AlertCircle className="h-4 w-4" />
            </button>
            <button onClick={() => handleBatch('delete')} className="p-2 hover:bg-surface-100 rounded-md" title="Delete">
              <Trash2 className="h-4 w-4" />
            </button>
          </>
        )}

        <div className="flex-1"></div>

        <button onClick={fetchMessages} className="p-2 hover:bg-surface-100 rounded-md">
          <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>

      {notice && (
        <div className="flex items-center justify-between bg-amber-50 px-4 py-2 text-sm text-amber-800">
          <span>{notice}</span>
          <button onClick={() => setNotice('')} className="text-amber-600 hover:text-amber-900">×</button>
        </div>
      )}

      {/* Message list */}
      <div className="flex-1 overflow-y-auto">
        {loading && messages.length === 0 ? (
          <div className="flex h-32 items-center justify-center">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
          </div>
        ) : messages.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-64 text-surface-500">
            <p className="text-lg font-medium">No messages</p>
            <p className="text-sm">{searchQuery ? 'Try a different search' : 'Your inbox is empty'}</p>
          </div>
        ) : (
          <>
            {messages.map((msg) => (
              <div
                key={msg.id}
                onClick={() => navigate(`/message/${msg.id}`)}
                className={`flex items-center gap-3 px-4 py-2 border-b border-surface-100 hover:shadow-sm cursor-pointer transition-all ${
                  msg.is_read ? 'bg-white' : 'bg-blue-50'
                }`}
              >
                <button
                  onClick={(e) => { e.stopPropagation(); toggleSelect(msg.id) }}
                  className="flex-shrink-0"
                >
                  {selectedIds.has(msg.id) ? <CheckSquare className="h-4 w-4 text-primary-600" /> : <Square className="h-4 w-4 text-surface-400" />}
                </button>

                <button
                  onClick={(e) => { e.stopPropagation(); handleToggleStar(msg) }}
                  className="flex-shrink-0"
                >
                  <Star className={`h-4 w-4 ${msg.is_flagged ? 'fill-yellow-400 text-yellow-400' : 'text-surface-300'}`} />
                </button>

                <div className="flex-1 min-w-0 grid grid-cols-[200px_1fr_80px] gap-4 items-center">
                  <span className={`text-sm truncate ${msg.is_read ? 'text-surface-600' : 'text-surface-900 font-semibold'}`}>
                    {msg.from?.name || msg.from?.email || 'Unknown'}
                  </span>
                  <span className="text-sm truncate">
                    <span className={msg.is_read ? 'text-surface-700' : 'text-surface-900 font-medium'}>{msg.subject || '(no subject)'}</span>
                    <span className="text-surface-400 ml-2">— {msg.snippet || ''}</span>
                  </span>
                  <span className="text-xs text-surface-400 text-right">{formatDate(msg.date)}</span>
                </div>
              </div>
            ))}

            {/* Pagination */}
            <div className="flex items-center justify-between px-4 py-3 border-t border-surface-200 bg-white">
              <span className="text-sm text-surface-500">{offset + 1}–{Math.min(offset + limit, total)} of {total}</span>
              <div className="flex gap-2">
                <button
                  onClick={() => setOffset(Math.max(0, offset - limit))}
                  disabled={offset === 0}
                  className="p-2 hover:bg-surface-100 rounded-md disabled:opacity-30"
                >
                  <ChevronLeft className="h-4 w-4" />
                </button>
                <button
                  onClick={() => setOffset(offset + limit)}
                  disabled={offset + limit >= total}
                  className="p-2 hover:bg-surface-100 rounded-md disabled:opacity-30"
                >
                  <ChevronRight className="h-4 w-4" />
                </button>
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

function formatDate(dateStr) {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  const now = new Date()
  const isToday = d.toDateString() === now.toDateString()
  if (isToday) {
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }
  return d.toLocaleDateString([], { month: 'short', day: 'numeric' })
}
