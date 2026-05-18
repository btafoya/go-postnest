import React, { useState, useEffect } from 'react'
import { UserPlus, Search, Trash2, Edit2, X, Mail, Phone, Save } from 'lucide-react'
import { getContacts, createContact, updateContact, deleteContact } from '../api'

export default function Contacts() {
  const [contacts, setContacts] = useState([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [editing, setEditing] = useState(null)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: '', email: '', phone: '', notes: '' })
  const [formError, setFormError] = useState('')

  const fetchContacts = () => {
    setLoading(true)
    getContacts()
      .then((data) => setContacts(data.map((c) => ({ ...c, notes: c.vcard_data || '' }))))
      .catch(console.error)
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    fetchContacts()
  }, [])

  const filtered = contacts.filter((c) =>
    (c.name || '').toLowerCase().includes(search.toLowerCase()) ||
    (c.email || '').toLowerCase().includes(search.toLowerCase())
  )

  const handleSave = async () => {
    setFormError('')
    try {
      if (editing) {
        await updateContact(editing.id, form)
      } else {
        await createContact(form)
      }
      setShowForm(false)
      setEditing(null)
      setForm({ name: '', email: '', phone: '', notes: '' })
      fetchContacts()
    } catch (err) {
      setFormError(err.response?.data?.error?.message || 'Failed to save contact')
    }
  }

  const handleEdit = (contact) => {
    setEditing(contact)
    setForm({
      name: contact.name || '',
      email: contact.email || '',
      phone: contact.phone || '',
      notes: contact.notes || '',
    })
    setShowForm(true)
  }

  const handleDelete = async (id) => {
    if (!confirm('Delete this contact?')) return
    try {
      await deleteContact(id)
      fetchContacts()
    } catch (err) {
      console.error('Delete failed:', err)
    }
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 py-3 border-b border-surface-200 bg-white">
        <h1 className="text-lg font-semibold text-surface-900">Contacts</h1>
        <button
          onClick={() => { setShowForm(true); setEditing(null); setForm({ name: '', email: '', phone: '', notes: '' }) }}
          className="btn-primary gap-2"
        >
          <UserPlus className="h-4 w-4" />
          New Contact
        </button>
      </div>

      <div className="px-4 py-2 border-b border-surface-200 bg-white">
        <div className="relative max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-surface-400" />
          <input
            type="text"
            placeholder="Search contacts"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="input-field pl-10"
          />
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        {loading ? (
          <div className="flex h-32 items-center justify-center">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-64 text-surface-500">
            <p className="text-lg font-medium">No contacts</p>
            <p className="text-sm">Add your first contact to get started</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {filtered.map((contact) => (
              <div key={contact.id} className="card p-4 hover:shadow-md transition-shadow">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className="h-10 w-10 rounded-full bg-primary-100 flex items-center justify-center text-primary-700 font-semibold">
                      {(contact.name || contact.email || 'U').charAt(0).toUpperCase()}
                    </div>
                    <div>
                      <p className="font-medium text-surface-900">{contact.name || 'Unnamed'}</p>
                      <p className="text-sm text-surface-500">{contact.email}</p>
                    </div>
                  </div>
                  <div className="flex gap-1">
                    <button onClick={() => handleEdit(contact)} className="p-1.5 hover:bg-surface-100 rounded">
                      <Edit2 className="h-4 w-4 text-surface-400" />
                    </button>
                    <button onClick={() => handleDelete(contact.id)} className="p-1.5 hover:bg-surface-100 rounded">
                      <Trash2 className="h-4 w-4 text-surface-400" />
                    </button>
                  </div>
                </div>
                {contact.phone && <p className="text-sm text-surface-500 mt-2 flex items-center gap-1"><Phone className="h-3 w-3" /> {contact.phone}</p>}
                {contact.notes && <p className="text-sm text-surface-500 mt-1">{contact.notes}</p>}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Contact form modal */}
      {showForm && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg shadow-xl w-full max-w-md p-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-semibold">{editing ? 'Edit Contact' : 'New Contact'}</h2>
              <button onClick={() => setShowForm(false)} className="p-1 hover:bg-surface-100 rounded">
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="space-y-3">
              {formError && <div className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">{formError}</div>}
              <input placeholder="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="input-field" />
              <input placeholder="Email" type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} className="input-field" />
              <input placeholder="Phone" value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} className="input-field" />
              <textarea placeholder="Notes" value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} className="input-field min-h-[80px]" />
              <div className="flex justify-end gap-2">
                <button onClick={() => setShowForm(false)} className="btn-secondary">Cancel</button>
                <button onClick={handleSave} className="btn-primary gap-2">
                  <Save className="h-4 w-4" /> Save
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
