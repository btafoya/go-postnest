import React, { useState } from 'react'
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import { Mail, Inbox, Send, FileText, Trash2, AlertCircle, Star, Users, Calendar, Shield, Menu, X, LogOut, Search, Plus, ChevronLeft } from 'lucide-react'
import { logout } from '../api'

const navItems = [
  { id: 'inbox', label: 'Inbox', icon: Inbox, path: '/' },
  { id: 'sent', label: 'Sent', icon: Send, path: '/inbox/sent' },
  { id: 'drafts', label: 'Drafts', icon: FileText, path: '/inbox/drafts' },
  { id: 'important', label: 'Important', icon: Star, path: '/inbox/important' },
  { id: 'junk', label: 'Junk', icon: AlertCircle, path: '/inbox/junk' },
  { id: 'trash', label: 'Trash', icon: Trash2, path: '/inbox/trash' },
  { id: 'contacts', label: 'Contacts', icon: Users, path: '/contacts' },
  { id: 'calendar', label: 'Calendar', icon: Calendar, path: '/calendar' },
]

export default function Layout({ user }) {
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const [searchQuery, setSearchQuery] = useState('')
  const navigate = useNavigate()
  const location = useLocation()

  const handleLogout = async () => {
    await logout()
    window.location.reload()
  }

  const handleSearch = (e) => {
    e.preventDefault()
    if (searchQuery.trim()) {
      navigate(`/?q=${encodeURIComponent(searchQuery.trim())}`)
    }
  }

  const isAdmin = user?.is_super_admin || user?.role === 'admin'

  return (
    <div className="flex h-screen bg-surface-50">
      {/* Sidebar */}
      <aside
        className={`${sidebarOpen ? 'w-64' : 'w-0'} flex-shrink-0 transition-all duration-300 overflow-hidden bg-white border-r border-surface-200 flex flex-col`}
      >
        <div className="flex items-center gap-2 px-4 py-3 border-b border-surface-200">
          <Mail className="h-6 w-6 text-primary-600" />
          <span className="text-lg font-semibold text-surface-900">PostNest</span>
        </div>

        <div className="p-3">
          <button
            onClick={() => navigate('/compose')}
            className="btn-primary w-full gap-2"
          >
            <Plus className="h-4 w-4" />
            Compose
          </button>
        </div>

        <nav className="flex-1 overflow-y-auto py-2">
          {navItems.map((item) => {
            const Icon = item.icon
            const isActive = location.pathname === item.path || location.pathname.startsWith(item.path)
            return (
              <button
                key={item.id}
                onClick={() => navigate(item.path)}
                className={`sidebar-item w-full text-left ${isActive ? 'active' : ''}`}
              >
                <Icon className="h-4 w-4" />
                <span>{item.label}</span>
              </button>
            )
          })}
          {isAdmin && (
            <button
              onClick={() => navigate('/admin')}
              className={`sidebar-item w-full text-left ${location.pathname === '/admin' ? 'active' : ''}`}
            >
              <Shield className="h-4 w-4" />
              <span>Admin</span>
            </button>
          )}
        </nav>

        <div className="border-t border-surface-200 p-3">
          <div className="flex items-center gap-2 mb-2">
            <div className="h-8 w-8 rounded-full bg-primary-100 flex items-center justify-center text-primary-700 text-sm font-semibold">
              {(user.display_name || user.email)?.charAt(0).toUpperCase()}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-surface-900 truncate">{user.display_name || user.email}</p>
              <p className="text-xs text-surface-500 truncate">{user.email}</p>
            </div>
          </div>
          <button
            onClick={handleLogout}
            className="flex items-center gap-2 w-full px-3 py-2 text-sm text-surface-600 hover:bg-surface-100 rounded-md transition-colors"
          >
            <LogOut className="h-4 w-4" />
            Sign out
          </button>
        </div>
      </aside>

      {/* Main content */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Header */}
        <header className="flex items-center gap-3 px-4 py-2 bg-white border-b border-surface-200">
          <button
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="p-2 hover:bg-surface-100 rounded-md transition-colors"
          >
            {sidebarOpen ? <ChevronLeft className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
          </button>

          <form onSubmit={handleSearch} className="flex-1 max-w-2xl">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-surface-400" />
              <input
                type="text"
                placeholder="Search mail"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="input-field pl-10 bg-surface-100 border-transparent focus:bg-white"
              />
            </div>
          </form>
        </header>

        {/* Content area */}
        <main className="flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
