import React, { useState, useEffect } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import Layout from './components/Layout'
import Login from './components/Login'
import Inbox from './components/Inbox'
import MessageView from './components/MessageView'
import Compose from './components/Compose'
import Contacts from './components/Contacts'
import Calendar from './components/Calendar'
import Admin from './components/Admin'
import { getMe } from './api'
import sse from './sse'

function App() {
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    getMe()
      .then((data) => {
        setUser(data)
        sse.connect()
      })
      .catch(() => setUser(null))
      .finally(() => setLoading(false))

    return () => sse.disconnect()
  }, [])

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
      </div>
    )
  }

  if (!user) {
    return <Login onLogin={(u) => { setUser(u); sse.connect() }} />
  }

  return (
    <Routes>
      <Route path="/login" element={<Navigate to="/" replace />} />
      <Route path="/" element={<Layout user={user} />}>
        <Route index element={<Inbox />} />
        <Route path="inbox/:labelId?" element={<Inbox />} />
        <Route path="message/:id" element={<MessageView />} />
        <Route path="compose" element={<Compose />} />
        <Route path="compose/:draftId" element={<Compose />} />
        <Route path="contacts" element={<Contacts />} />
        <Route path="calendar" element={<Calendar />} />
        <Route path="admin" element={<Admin />} />
      </Route>
    </Routes>
  )
}

export default App
