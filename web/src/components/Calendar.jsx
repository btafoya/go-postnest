import React, { useState, useEffect } from 'react'
import { ChevronLeft, ChevronRight, Plus, X, Clock, MapPin, AlignLeft } from 'lucide-react'
import { getCalendarEvents, createCalendarEvent, updateCalendarEvent, deleteCalendarEvent } from '../api'
import { startOfMonth, endOfMonth, startOfWeek, endOfWeek, addDays, format, isSameMonth, isSameDay, addMonths, subMonths } from 'date-fns'

export default function CalendarView() {
  const [currentDate, setCurrentDate] = useState(new Date())
  const [events, setEvents] = useState([])
  const [loading, setLoading] = useState(true)
  const [selectedDate, setSelectedDate] = useState(null)
  const [showForm, setShowForm] = useState(false)
  const [editingEvent, setEditingEvent] = useState(null)
  const [form, setForm] = useState({ title: '', start: '', end: '', description: '', location: '' })

  const fetchEvents = async () => {
    setLoading(true)
    try {
      const start = format(startOfMonth(currentDate), 'yyyy-MM-dd')
      const end = format(endOfMonth(currentDate), 'yyyy-MM-dd')
      const data = await getCalendarEvents(start, end)
      setEvents(data)
    } catch (err) {
      console.error('Failed to load events:', err)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchEvents()
  }, [currentDate])

  const monthStart = startOfMonth(currentDate)
  const monthEnd = endOfMonth(monthStart)
  const calendarStart = startOfWeek(monthStart)
  const calendarEnd = endOfWeek(monthEnd)

  const days = []
  let day = calendarStart
  while (day <= calendarEnd) {
    days.push(day)
    day = addDays(day, 1)
  }

  const getEventsForDay = (date) => {
    return events.filter((e) => {
      const eventDate = new Date(e.start_time || e.start)
      return isSameDay(eventDate, date)
    })
  }

  const handleSave = async () => {
    try {
      const payload = {
        title: form.title,
        description: form.description,
        location: form.location,
        start_time: new Date(form.start).toISOString(),
        end_time: new Date(form.end).toISOString(),
      }
      if (editingEvent) {
        await updateCalendarEvent(editingEvent.id, payload)
      } else {
        await createCalendarEvent(payload)
      }
      setShowForm(false)
      setEditingEvent(null)
      setForm({ title: '', start: '', end: '', description: '', location: '' })
      fetchEvents()
    } catch (err) {
      alert('Failed to save event')
    }
  }

  const handleDelete = async (id) => {
    if (!confirm('Delete this event?')) return
    try {
      await deleteCalendarEvent(id)
      fetchEvents()
    } catch (err) {
      alert('Failed to delete event')
    }
  }

  const openNewEvent = (date) => {
    const start = new Date(date)
    start.setHours(9, 0, 0, 0)
    const end = new Date(date)
    end.setHours(10, 0, 0, 0)
    setForm({
      title: '',
      start: format(start, "yyyy-MM-dd'T'HH:mm"),
      end: format(end, "yyyy-MM-dd'T'HH:mm"),
      description: '',
      location: '',
    })
    setEditingEvent(null)
    setShowForm(true)
  }

  const weekDays = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-surface-200 bg-white">
        <div className="flex items-center gap-4">
          <h1 className="text-lg font-semibold text-surface-900">{format(currentDate, 'MMMM yyyy')}</h1>
          <div className="flex gap-1">
            <button onClick={() => setCurrentDate(subMonths(currentDate, 1))} className="p-1.5 hover:bg-surface-100 rounded">
              <ChevronLeft className="h-4 w-4" />
            </button>
            <button onClick={() => setCurrentDate(addMonths(currentDate, 1))} className="p-1.5 hover:bg-surface-100 rounded">
              <ChevronRight className="h-4 w-4" />
            </button>
            <button onClick={() => setCurrentDate(new Date())} className="btn-secondary text-xs px-2 py-1">Today</button>
          </div>
        </div>
        <button onClick={() => openNewEvent(new Date())} className="btn-primary gap-2">
          <Plus className="h-4 w-4" />
          New Event
        </button>
      </div>

      {/* Calendar grid */}
      <div className="flex-1 overflow-y-auto p-4">
        <div className="grid grid-cols-7 gap-px bg-surface-200 border border-surface-200 rounded-lg overflow-hidden">
          {weekDays.map((d) => (
            <div key={d} className="bg-surface-50 px-2 py-1 text-xs font-medium text-surface-600 text-center">
              {d}
            </div>
          ))}
          {days.map((d) => {
            const dayEvents = getEventsForDay(d)
            const isCurrentMonth = isSameMonth(d, currentDate)
            const isToday = isSameDay(d, new Date())
            return (
              <div
                key={d.toISOString()}
                onClick={() => openNewEvent(d)}
                className={`bg-white min-h-[100px] p-1 cursor-pointer hover:bg-surface-50 transition-colors ${
                  !isCurrentMonth ? 'bg-surface-50/50' : ''
                }`}
              >
                <div className={`text-xs font-medium mb-1 w-6 h-6 flex items-center justify-center rounded-full ${
                  isToday ? 'bg-primary-600 text-white' : 'text-surface-700'
                }`}>
                  {format(d, 'd')}
                </div>
                <div className="space-y-0.5">
                  {dayEvents.slice(0, 3).map((e) => (
                    <div
                      key={e.id}
                      onClick={(ev) => { ev.stopPropagation(); setEditingEvent(e); setForm({
                        title: e.title || '',
                        start: format(new Date(e.start_time || e.start), "yyyy-MM-dd'T'HH:mm"),
                        end: format(new Date(e.end_time || e.end), "yyyy-MM-dd'T'HH:mm"),
                        description: e.description || '',
                        location: e.location || '',
                      }); setShowForm(true); }}
                      className="text-[10px] px-1.5 py-0.5 rounded bg-primary-100 text-primary-700 truncate cursor-pointer hover:bg-primary-200"
                    >
                      {e.title}
                    </div>
                  ))}
                  {dayEvents.length > 3 && (
                    <div className="text-[10px] text-surface-400 px-1.5">+{dayEvents.length - 3} more</div>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      </div>

      {/* Event form modal */}
      {showForm && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg shadow-xl w-full max-w-md p-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-semibold">{editingEvent ? 'Edit Event' : 'New Event'}</h2>
              <button onClick={() => setShowForm(false)} className="p-1 hover:bg-surface-100 rounded">
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="space-y-3">
              <input placeholder="Title" value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} className="input-field" />
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="text-xs text-surface-500 mb-1 block">Start</label>
                  <input type="datetime-local" value={form.start} onChange={(e) => setForm({ ...form, start: e.target.value })} className="input-field" />
                </div>
                <div>
                  <label className="text-xs text-surface-500 mb-1 block">End</label>
                  <input type="datetime-local" value={form.end} onChange={(e) => setForm({ ...form, end: e.target.value })} className="input-field" />
                </div>
              </div>
              <input placeholder="Location" value={form.location} onChange={(e) => setForm({ ...form, location: e.target.value })} className="input-field" />
              <textarea placeholder="Description" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className="input-field min-h-[80px]" />
              <div className="flex justify-end gap-2">
                {editingEvent && (
                  <button onClick={() => handleDelete(editingEvent.id)} className="btn-secondary text-red-600 border-red-200 hover:bg-red-50 mr-auto">Delete</button>
                )}
                <button onClick={() => setShowForm(false)} className="btn-secondary">Cancel</button>
                <button onClick={handleSave} className="btn-primary">Save</button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
