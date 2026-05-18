import React, { useState, useEffect } from 'react'
import { Shield, Users, Globe, Activity, AlertTriangle } from 'lucide-react'
import { getDomains, getHealth } from '../api'

export default function Admin() {
  const [domains, setDomains] = useState([])
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState('overview')
  const [health, setHealth] = useState(null)

  useEffect(() => {
    setLoading(true)
    getDomains()
      .then((data) => setDomains(data))
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    let active = true
    const poll = () => {
      getHealth()
        .then((h) => { if (active) setHealth(h) })
        .catch(() => { if (active) setHealth(null) })
    }
    poll()
    const t = setInterval(poll, 15000)
    return () => { active = false; clearInterval(t) }
  }, [])

  const tabs = [
    { id: 'overview', label: 'Overview', icon: Activity },
    { id: 'domains', label: 'Domains', icon: Globe },
    { id: 'users', label: 'Users', icon: Users },
    { id: 'security', label: 'Security', icon: Shield },
  ]

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-4 py-3 border-b border-surface-200 bg-white">
        <Shield className="h-5 w-5 text-primary-600" />
        <h1 className="text-lg font-semibold text-surface-900">Admin Dashboard</h1>
      </div>

      <div className="flex border-b border-surface-200 bg-white">
        {tabs.map((tab) => {
          const Icon = tab.icon
          return (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
                activeTab === tab.id
                  ? 'border-primary-600 text-primary-600'
                  : 'border-transparent text-surface-500 hover:text-surface-700'
              }`}
            >
              <Icon className="h-4 w-4" />
              {tab.label}
            </button>
          )
        })}
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        {activeTab === 'overview' && (
          <div className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="card p-4">
                <p className="text-sm text-surface-500">Total Domains</p>
                <p className="text-2xl font-semibold text-surface-900">{health?.total_domains ?? '—'}</p>
              </div>
              <div className="card p-4">
                <p className="text-sm text-surface-500">Active Users</p>
                <p className="text-2xl font-semibold text-surface-900">{health?.active_users ?? '—'}</p>
              </div>
              <div className="card p-4">
                <p className="text-sm text-surface-500">Messages Today</p>
                <p className="text-2xl font-semibold text-surface-900">{health?.messages_today ?? '—'}</p>
              </div>
            </div>

            <div className="card p-4">
              <h3 className="text-sm font-medium text-surface-900 mb-3">System Status</h3>
              {!health ? (
                <p className="text-sm text-surface-500">Loading status…</p>
              ) : (
                <div className="space-y-2">
                  {[
                    ['Database', health.database],
                    ['Redis', health.redis],
                    ['IMAP Server', health.imap],
                    ['SMTP Server', health.smtp],
                  ].map(([label, c]) => {
                    const up = c?.status === 'up'
                    return (
                      <div key={label} className="flex items-center gap-2">
                        <div className={`h-2 w-2 rounded-full ${up ? 'bg-green-500' : 'bg-red-500'}`}></div>
                        <span className="text-sm text-surface-700">
                          {label}: {up ? 'Running' : 'Down'}
                          {up && c.latency_ms != null ? ` (${c.latency_ms}ms)` : ''}
                          {!up && c?.error ? ` — ${c.error}` : ''}
                        </span>
                      </div>
                    )
                  })}
                  <div className="flex items-center gap-2">
                    <div className="h-2 w-2 rounded-full bg-surface-400"></div>
                    <span className="text-sm text-surface-700">
                      Worker queue: {health.worker_queue?.depth ?? 0} pending, {health.worker_queue?.dead ?? 0} dead
                    </span>
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {activeTab === 'domains' && (
          <div className="card">
            <div className="px-4 py-3 border-b border-surface-200">
              <h3 className="text-sm font-medium text-surface-900">Managed Domains</h3>
            </div>
            {loading ? (
              <div className="flex h-32 items-center justify-center">
                <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
              </div>
            ) : domains.length === 0 ? (
              <div className="p-8 text-center text-surface-500">No domains configured</div>
            ) : (
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-surface-200">
                    <th className="px-4 py-2 text-left font-medium text-surface-600">Domain</th>
                    <th className="px-4 py-2 text-left font-medium text-surface-600">Status</th>
                    <th className="px-4 py-2 text-left font-medium text-surface-600">Users</th>
                    <th className="px-4 py-2 text-left font-medium text-surface-600">Created</th>
                  </tr>
                </thead>
                <tbody>
                  {domains.map((domain) => (
                    <tr key={domain.id} className="border-b border-surface-100 hover:bg-surface-50">
                      <td className="px-4 py-2 font-medium text-surface-900">{domain.name}</td>
                      <td className="px-4 py-2">
                        <span className="inline-flex items-center px-2 py-0.5 rounded text-xs bg-green-100 text-green-700">Active</span>
                      </td>
                      <td className="px-4 py-2 text-surface-600">{domain.user_count ?? '—'}</td>
                      <td className="px-4 py-2 text-surface-500">{domain.created_at ? new Date(domain.created_at).toLocaleDateString() : '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        )}

        {activeTab === 'users' && (
          <div className="card p-8 text-center text-surface-500">
            <Users className="h-8 w-8 mx-auto mb-2 text-surface-400" />
            <p>User management coming soon</p>
          </div>
        )}

        {activeTab === 'security' && (
          <div className="card p-8 text-center text-surface-500">
            <Shield className="h-8 w-8 mx-auto mb-2 text-surface-400" />
            <p>Security settings coming soon</p>
          </div>
        )}
      </div>
    </div>
  )
}
