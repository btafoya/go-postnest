import React, { useState, useEffect } from 'react'
import {
  Shield, Users, Globe, Activity, Plus, Edit2, Trash2, Check, X,
  Save, Lock, AlertTriangle, Search, RefreshCw,
} from 'lucide-react'
import {
  getDomains, createDomain, updateDomain, deleteDomain, toggleDomainActive,
  getHealth, getAdminUsers, createAdminUser, updateAdminUser, deleteAdminUser,
  resetAdminUserPassword, getAdminSettings, updateAdminSettings,
  addUserDomain, updateUserDomainRole, removeUserDomain,
  listAliases, createAlias, deleteAlias, setDomainCatchall,
  getTLSStatus, getTLSProviders, getTLSConfig, updateTLSConfig,
  getTLSDomains, addTLSDomain, deleteTLSDomain, renewTLS,
} from '../api'

const ROLES = ['admin', 'user', 'readonly']

export default function Admin() {
  const [activeTab, setActiveTab] = useState('overview')
  const [health, setHealth] = useState(null)
  const [domains, setDomains] = useState([])
  const [users, setUsers] = useState([])
  const [settings, setSettings] = useState({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // Modals
  const [domainModal, setDomainModal] = useState(null)
  const [userModal, setUserModal] = useState(null)
  const [resetModal, setResetModal] = useState(null)

  // Forms
  const [domainForm, setDomainForm] = useState({ name: '', postmark_token: '', postmark_stream: 'outbound', is_active: true })
  const [editingDomain, setEditingDomain] = useState(null)
  const [domainAliases, setDomainAliases] = useState([])
  const [allUsers, setAllUsers] = useState([])
  const [aliasForm, setAliasForm] = useState({ local_part: '', user_ids: [] })
  const [userForm, setUserForm] = useState({ email: '', password: '', display_name: '', is_super_admin: false })
  const [editingUser, setEditingUser] = useState(null)
  const [memberForm, setMemberForm] = useState({ domain_id: '', role: 'user' })
  const [resetPassword, setResetPassword] = useState('')
  const [settingsForm, setSettingsForm] = useState({})

  // TLS / ACME
  const [tlsStatus, setTlsStatus] = useState(null)
  const [tlsProviders, setTlsProviders] = useState([])
  const [tlsConfig, setTlsConfig] = useState(null)
  const [tlsConfigForm, setTlsConfigForm] = useState({ enabled: false, email: '', directory: 'staging', dns_provider: 'cloudflare', credentials: {} })
  const [tlsDomains, setTlsDomains] = useState([])
  const [newTlsDomain, setNewTlsDomain] = useState('')
  const [tlsBusy, setTlsBusy] = useState(false)

  const tabs = [
    { id: 'overview', label: 'Overview', icon: Activity },
    { id: 'domains', label: 'Domains', icon: Globe },
    { id: 'users', label: 'Users', icon: Users },
    { id: 'security', label: 'Security', icon: Shield },
    { id: 'tls', label: 'TLS / Certificates', icon: Lock },
  ]

  const fetchDomains = () => {
    setLoading(true)
    getDomains()
      .then((data) => setDomains(data))
      .catch((err) => setError(err.response?.data?.error?.message || 'Failed to load domains'))
      .finally(() => setLoading(false))
  }

  const fetchUsers = () => {
    setLoading(true)
    getAdminUsers()
      .then((data) => setUsers(data))
      .catch((err) => setError(err.response?.data?.error?.message || 'Failed to load users'))
      .finally(() => setLoading(false))
  }

  const fetchSettings = () => {
    getAdminSettings()
      .then((data) => {
        setSettings(data)
        setSettingsForm(data)
      })
      .catch((err) => setError(err.response?.data?.error?.message || 'Failed to load settings'))
  }

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

  const fetchTLS = () => {
    setLoading(true)
    Promise.all([
      getTLSStatus(),
      getTLSProviders(),
      getTLSConfig(),
      getTLSDomains(),
    ])
      .then(([status, providers, config, domains]) => {
        setTlsStatus(status)
        setTlsProviders(providers)
        setTlsConfig(config)
        setTlsDomains(domains)
        if (config) {
          setTlsConfigForm({
            enabled: config.enabled || false,
            email: config.email || '',
            directory: config.directory || 'staging',
            dns_provider: config.dns_provider || 'cloudflare',
            credentials: {},
          })
        }
      })
      .catch((err) => setError(err.response?.data?.error?.message || 'Failed to load TLS settings'))
      .finally(() => setLoading(false))
  }

  const saveTLSConfig = async () => {
    setTlsBusy(true)
    setError('')
    try {
      await updateTLSConfig(tlsConfigForm)
      setError('TLS configuration saved')
      fetchTLS()
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to save TLS configuration')
    } finally {
      setTlsBusy(false)
    }
  }

  const addTlsDomainHandler = async () => {
    if (!newTlsDomain.trim()) return
    setTlsBusy(true)
    setError('')
    try {
      await addTLSDomain(newTlsDomain.trim())
      setNewTlsDomain('')
      fetchTLS()
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to add domain')
    } finally {
      setTlsBusy(false)
    }
  }

  const deleteTlsDomainHandler = async (id) => {
    if (!window.confirm('Remove this domain? The certificate will be re-issued without it.')) return
    setTlsBusy(true)
    setError('')
    try {
      await deleteTLSDomain(id)
      fetchTLS()
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to remove domain')
    } finally {
      setTlsBusy(false)
    }
  }

  const forceRenewHandler = async () => {
    if (!window.confirm('Force certificate renewal now?')) return
    setTlsBusy(true)
    setError('')
    try {
      await renewTLS()
      setError('Certificate renewed')
      fetchTLS()
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Renewal failed')
    } finally {
      setTlsBusy(false)
    }
  }

  useEffect(() => {
    if (activeTab === 'domains') fetchDomains()
    if (activeTab === 'users') { fetchUsers(); getDomains().then(setDomains).catch(() => {}) }
    if (activeTab === 'security') fetchSettings()
    if (activeTab === 'tls') fetchTLS()
  }, [activeTab])

  // Domain handlers
  const openDomainModal = (domain = null) => {
    if (domain) {
      setDomainForm({
        name: domain.name || '',
        postmark_token: domain.postmark_token || '',
        postmark_stream: domain.postmark_stream || 'outbound',
        is_active: domain.is_active !== false,
      })
      setEditingDomain(domain)
      setDomainModal(domain.id)
      listAliases(domain.id).then(setDomainAliases).catch(() => setDomainAliases([]))
      getAdminUsers(100, 0).then(setAllUsers).catch(() => setAllUsers([]))
    } else {
      setDomainForm({ name: '', postmark_token: '', postmark_stream: 'outbound', is_active: true })
      setEditingDomain(null)
      setDomainAliases([])
      setDomainModal('new')
    }
    setAliasForm({ local_part: '', user_ids: [] })
    setError('')
  }

  const refreshAliases = async (domainId) => {
    try {
      setDomainAliases(await listAliases(domainId))
    } catch {
      setDomainAliases([])
    }
  }

  const addAlias = async () => {
    if (!aliasForm.local_part || aliasForm.user_ids.length === 0) return
    setError('')
    try {
      await createAlias(editingDomain.id, aliasForm.local_part, aliasForm.user_ids)
      setAliasForm({ local_part: '', user_ids: [] })
      await refreshAliases(editingDomain.id)
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to create alias')
    }
  }

  const removeAlias = async (aliasId) => {
    setError('')
    try {
      await deleteAlias(aliasId)
      await refreshAliases(editingDomain.id)
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to delete alias')
    }
  }

  const changeCatchall = async (userId) => {
    setError('')
    try {
      await setDomainCatchall(editingDomain.id, userId || null)
      setEditingDomain({ ...editingDomain, catchall_user_id: userId || null })
      fetchDomains()
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to set catch-all')
    }
  }

  const saveDomain = async () => {
    setError('')
    try {
      if (domainModal === 'new') {
        await createDomain(domainForm)
      } else {
        await updateDomain(domainModal, domainForm)
      }
      setDomainModal(null)
      fetchDomains()
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to save domain')
    }
  }

  const removeDomain = async (id) => {
    if (!confirm('Delete this domain and all its data? This cannot be undone.')) return
    try {
      await deleteDomain(id)
      fetchDomains()
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to delete domain')
    }
  }

  const toggleDomain = async (id, current) => {
    try {
      await toggleDomainActive(id, !current)
      fetchDomains()
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to toggle domain')
    }
  }

  // User handlers
  const openUserModal = (user = null) => {
    if (user) {
      setUserForm({
        email: user.email || '',
        password: '',
        display_name: user.display_name || '',
        is_super_admin: user.is_super_admin || false,
      })
      setEditingUser(user)
      setUserModal(user.id)
    } else {
      setUserForm({ email: '', password: '', display_name: '', is_super_admin: false })
      setEditingUser(null)
      setUserModal('new')
    }
    setMemberForm({ domain_id: '', role: 'user' })
    setError('')
  }

  const refreshEditingUser = async (id) => {
    const list = await getAdminUsers()
    setUsers(list)
    const found = list.find((u) => u.id === id)
    if (found) setEditingUser(found)
  }

  const addMembership = async () => {
    if (!memberForm.domain_id) return
    setError('')
    try {
      await addUserDomain(editingUser.id, memberForm.domain_id, memberForm.role)
      setMemberForm({ domain_id: '', role: 'user' })
      await refreshEditingUser(editingUser.id)
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to add domain membership')
    }
  }

  const changeMembershipRole = async (domainId, role) => {
    setError('')
    try {
      await updateUserDomainRole(editingUser.id, domainId, role)
      await refreshEditingUser(editingUser.id)
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to update role')
    }
  }

  const removeMembership = async (domainId) => {
    setError('')
    try {
      await removeUserDomain(editingUser.id, domainId)
      await refreshEditingUser(editingUser.id)
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to remove domain membership')
    }
  }

  const saveUser = async () => {
    setError('')
    try {
      if (userModal === 'new') {
        await createAdminUser(userForm)
      } else {
        const payload = { ...userForm }
        delete payload.password
        await updateAdminUser(userModal, payload)
      }
      setUserModal(null)
      fetchUsers()
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to save user')
    }
  }

  const removeUser = async (id) => {
    if (!confirm('Delete this user? This cannot be undone.')) return
    try {
      await deleteAdminUser(id)
      fetchUsers()
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to delete user')
    }
  }

  const openResetModal = (user) => {
    setResetModal(user)
    setResetPassword('')
    setError('')
  }

  const doResetPassword = async () => {
    setError('')
    try {
      await resetAdminUserPassword(resetModal.id, resetPassword)
      setResetModal(null)
      setResetPassword('')
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to reset password')
    }
  }

  // Settings handlers
  const saveSettings = async () => {
    setError('')
    try {
      await updateAdminSettings(settingsForm)
      setSettings({ ...settingsForm })
      setError('Settings saved')
      setTimeout(() => setError(''), 2000)
    } catch (err) {
      setError(err.response?.data?.error?.message || 'Failed to save settings')
    }
  }

  const handleSettingChange = (key, value) => {
    setSettingsForm((prev) => ({ ...prev, [key]: value }))
  }

  const renderError = () => {
    if (!error) return null
    return (
      <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700 flex items-center gap-2">
        <AlertTriangle className="h-4 w-4" />
        {error}
      </div>
    )
  }

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
              onClick={() => { setActiveTab(tab.id); setError('') }}
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
        {renderError()}

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
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-medium text-surface-900">Managed Domains</h2>
              <button onClick={() => openDomainModal()} className="btn-primary gap-2 text-xs">
                <Plus className="h-3.5 w-3.5" />
                Add Domain
              </button>
            </div>

            {loading ? (
              <div className="flex h-32 items-center justify-center">
                <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
              </div>
            ) : domains.length === 0 ? (
              <div className="card p-8 text-center text-surface-500">No domains configured</div>
            ) : (
              <div className="card overflow-hidden">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-surface-200 bg-surface-50">
                      <th className="px-4 py-2 text-left font-medium text-surface-600">Domain</th>
                      <th className="px-4 py-2 text-left font-medium text-surface-600">Status</th>
                      <th className="px-4 py-2 text-left font-medium text-surface-600">Users</th>
                      <th className="px-4 py-2 text-left font-medium text-surface-600">Stream</th>
                      <th className="px-4 py-2 text-right font-medium text-surface-600">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {domains.map((domain) => (
                      <tr key={domain.id} className="border-b border-surface-100 hover:bg-surface-50">
                        <td className="px-4 py-2 font-medium text-surface-900">{domain.name}</td>
                        <td className="px-4 py-2">
                          <button
                            onClick={() => toggleDomain(domain.id, domain.is_active)}
                            className={`inline-flex items-center px-2 py-0.5 rounded text-xs cursor-pointer transition-colors ${
                              domain.is_active
                                ? 'bg-green-100 text-green-700'
                                : 'bg-red-100 text-red-700'
                            }`}
                          >
                            {domain.is_active ? 'Active' : 'Inactive'}
                          </button>
                        </td>
                        <td className="px-4 py-2 text-surface-600">{domain.user_count ?? '—'}</td>
                        <td className="px-4 py-2 text-surface-500">{domain.postmark_stream || 'outbound'}</td>
                        <td className="px-4 py-2 text-right">
                          <div className="flex items-center justify-end gap-2">
                            <button
                              onClick={() => openDomainModal(domain)}
                              className="p-1 rounded hover:bg-surface-100 text-surface-500"
                              title="Edit"
                            >
                              <Edit2 className="h-3.5 w-3.5" />
                            </button>
                            <button
                              onClick={() => removeDomain(domain.id)}
                              className="p-1 rounded hover:bg-red-50 text-red-500"
                              title="Delete"
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            {domainModal && (
              <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
                <div className="w-full max-w-lg rounded-lg bg-white shadow-lg max-h-[90vh] overflow-y-auto">
                  <div className="flex items-center justify-between px-4 py-3 border-b border-surface-200">
                    <h3 className="text-sm font-semibold text-surface-900">
                      {domainModal === 'new' ? 'New Domain' : 'Edit Domain'}
                    </h3>
                    <button onClick={() => setDomainModal(null)} className="p-1 rounded hover:bg-surface-100">
                      <X className="h-4 w-4 text-surface-500" />
                    </button>
                  </div>
                  <div className="p-4 space-y-3">
                    <div>
                      <label className="block text-xs font-medium text-surface-600 mb-1">Domain Name</label>
                      <input
                        type="text"
                        value={domainForm.name}
                        onChange={(e) => setDomainForm({ ...domainForm, name: e.target.value })}
                        className="input-field"
                        placeholder="example.com"
                      />
                    </div>
                    <div>
                      <label className="block text-xs font-medium text-surface-600 mb-1">Postmark Token</label>
                      <input
                        type="text"
                        value={domainForm.postmark_token}
                        onChange={(e) => setDomainForm({ ...domainForm, postmark_token: e.target.value })}
                        className="input-field"
                        placeholder="Optional"
                      />
                    </div>
                    <div>
                      <label className="block text-xs font-medium text-surface-600 mb-1">Postmark Stream</label>
                      <input
                        type="text"
                        value={domainForm.postmark_stream}
                        onChange={(e) => setDomainForm({ ...domainForm, postmark_stream: e.target.value })}
                        className="input-field"
                        placeholder="outbound"
                      />
                    </div>
                    <div className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        id="domain-active"
                        checked={domainForm.is_active}
                        onChange={(e) => setDomainForm({ ...domainForm, is_active: e.target.checked })}
                        className="rounded border-surface-300"
                      />
                      <label htmlFor="domain-active" className="text-sm text-surface-700">Active</label>
                    </div>

                    {domainModal !== 'new' && editingDomain && (
                      <>
                        <div className="pt-2 border-t border-surface-200">
                          <label className="block text-xs font-medium text-surface-600 mb-1">Catch-all User</label>
                          <p className="text-xs text-surface-400 mb-1">Unmatched mail at this domain delivers here.</p>
                          <select
                            value={editingDomain.catchall_user_id || ''}
                            onChange={(e) => changeCatchall(e.target.value)}
                            className="input-field !py-1 !text-xs"
                          >
                            <option value="">None (reject unmatched)</option>
                            {allUsers.map((u) => <option key={u.id} value={u.id}>{u.email}</option>)}
                          </select>
                        </div>

                        <div className="pt-2 border-t border-surface-200">
                          <label className="block text-xs font-medium text-surface-600 mb-2">Aliases</label>
                          {domainAliases.length === 0 && (
                            <p className="text-xs text-surface-400 mb-2">No aliases.</p>
                          )}
                          <div className="space-y-1.5">
                            {domainAliases.map((a) => (
                              <div key={a.id} className="flex items-start gap-2">
                                <div className="flex-1 min-w-0">
                                  <span className="text-sm text-surface-700">{a.local_part}@{editingDomain.name}</span>
                                  <span className="block text-xs text-surface-400 truncate">
                                    → {(a.targets || []).map((t) => t.user_email).join(', ') || '(no targets)'}
                                  </span>
                                </div>
                                <button
                                  onClick={() => removeAlias(a.id)}
                                  className="p-1 rounded hover:bg-red-50"
                                  title="Delete alias"
                                >
                                  <Trash2 className="h-3.5 w-3.5 text-red-500" />
                                </button>
                              </div>
                            ))}
                          </div>
                          <div className="mt-2 space-y-1.5">
                            <input
                              type="text"
                              value={aliasForm.local_part}
                              onChange={(e) => setAliasForm({ ...aliasForm, local_part: e.target.value })}
                              className="input-field !py-1 !text-xs"
                              placeholder="alias local part (e.g. sales)"
                            />
                            <select
                              multiple
                              value={aliasForm.user_ids}
                              onChange={(e) => setAliasForm({ ...aliasForm, user_ids: Array.from(e.target.selectedOptions, (o) => o.value) })}
                              className="input-field !text-xs h-24"
                            >
                              {allUsers.map((u) => <option key={u.id} value={u.id}>{u.email}</option>)}
                            </select>
                            <button
                              onClick={addAlias}
                              disabled={!aliasForm.local_part || aliasForm.user_ids.length === 0}
                              className="btn-secondary !py-1 !text-xs disabled:opacity-40 w-full"
                            >
                              Add alias ({aliasForm.user_ids.length} target{aliasForm.user_ids.length === 1 ? '' : 's'})
                            </button>
                          </div>
                        </div>
                      </>
                    )}
                  </div>
                  <div className="flex justify-end gap-2 px-4 py-3 border-t border-surface-200">
                    <button onClick={() => setDomainModal(null)} className="btn-secondary text-xs">Cancel</button>
                    <button onClick={saveDomain} className="btn-primary gap-2 text-xs">
                      <Save className="h-3.5 w-3.5" />
                      Save
                    </button>
                  </div>
                </div>
              </div>
            )}
          </div>
        )}

        {activeTab === 'users' && (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-medium text-surface-900">Users</h2>
              <button onClick={() => openUserModal()} className="btn-primary gap-2 text-xs">
                <Plus className="h-3.5 w-3.5" />
                Add User
              </button>
            </div>

            {loading ? (
              <div className="flex h-32 items-center justify-center">
                <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
              </div>
            ) : users.length === 0 ? (
              <div className="card p-8 text-center text-surface-500">No users found</div>
            ) : (
              <div className="card overflow-hidden">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-surface-200 bg-surface-50">
                      <th className="px-4 py-2 text-left font-medium text-surface-600">Email</th>
                      <th className="px-4 py-2 text-left font-medium text-surface-600">Name</th>
                      <th className="px-4 py-2 text-left font-medium text-surface-600">Role</th>
                      <th className="px-4 py-2 text-left font-medium text-surface-600">Domains</th>
                      <th className="px-4 py-2 text-right font-medium text-surface-600">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {users.map((user) => (
                      <tr key={user.id} className="border-b border-surface-100 hover:bg-surface-50">
                        <td className="px-4 py-2 font-medium text-surface-900">{user.email}</td>
                        <td className="px-4 py-2 text-surface-600">{user.display_name || '—'}</td>
                        <td className="px-4 py-2">
                          <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs ${
                            user.is_super_admin ? 'bg-purple-100 text-purple-700' : 'bg-surface-100 text-surface-600'
                          }`}>
                            {user.is_super_admin ? 'Super Admin' : 'User'}
                          </span>
                        </td>
                        <td className="px-4 py-2 text-surface-500">
                          {user.memberships?.length ?? 0}
                        </td>
                        <td className="px-4 py-2 text-right">
                          <div className="flex items-center justify-end gap-2">
                            <button
                              onClick={() => openUserModal(user)}
                              className="p-1 rounded hover:bg-surface-100 text-surface-500"
                              title="Edit"
                            >
                              <Edit2 className="h-3.5 w-3.5" />
                            </button>
                            <button
                              onClick={() => openResetModal(user)}
                              className="p-1 rounded hover:bg-surface-100 text-surface-500"
                              title="Reset Password"
                            >
                              <Lock className="h-3.5 w-3.5" />
                            </button>
                            <button
                              onClick={() => removeUser(user.id)}
                              className="p-1 rounded hover:bg-red-50 text-red-500"
                              title="Delete"
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            {userModal && (
              <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
                <div className="w-full max-w-md rounded-lg bg-white shadow-lg">
                  <div className="flex items-center justify-between px-4 py-3 border-b border-surface-200">
                    <h3 className="text-sm font-semibold text-surface-900">
                      {userModal === 'new' ? 'New User' : 'Edit User'}
                    </h3>
                    <button onClick={() => setUserModal(null)} className="p-1 rounded hover:bg-surface-100">
                      <X className="h-4 w-4 text-surface-500" />
                    </button>
                  </div>
                  <div className="p-4 space-y-3">
                    <div>
                      <label className="block text-xs font-medium text-surface-600 mb-1">Email</label>
                      <input
                        type="email"
                        value={userForm.email}
                        onChange={(e) => setUserForm({ ...userForm, email: e.target.value })}
                        className="input-field"
                        placeholder="user@example.com"
                      />
                    </div>
                    {userModal === 'new' && (
                      <div>
                        <label className="block text-xs font-medium text-surface-600 mb-1">Password</label>
                        <input
                          type="password"
                          value={userForm.password}
                          onChange={(e) => setUserForm({ ...userForm, password: e.target.value })}
                          className="input-field"
                          placeholder="Initial password"
                        />
                      </div>
                    )}
                    <div>
                      <label className="block text-xs font-medium text-surface-600 mb-1">Display Name</label>
                      <input
                        type="text"
                        value={userForm.display_name}
                        onChange={(e) => setUserForm({ ...userForm, display_name: e.target.value })}
                        className="input-field"
                        placeholder="Optional"
                      />
                    </div>
                    <div className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        id="user-admin"
                        checked={userForm.is_super_admin}
                        onChange={(e) => setUserForm({ ...userForm, is_super_admin: e.target.checked })}
                        className="rounded border-surface-300"
                      />
                      <label htmlFor="user-admin" className="text-sm text-surface-700">Super Admin</label>
                    </div>

                    {userModal !== 'new' && editingUser && (
                      <div className="pt-2 border-t border-surface-200">
                        <label className="block text-xs font-medium text-surface-600 mb-2">Domain Memberships</label>
                        {(editingUser.memberships?.length ?? 0) === 0 && (
                          <p className="text-xs text-surface-400 mb-2">No domains. User has no mailbox access until added.</p>
                        )}
                        <div className="space-y-1.5">
                          {(editingUser.memberships || []).map((m) => (
                            <div key={m.domain_id} className="flex items-center gap-2">
                              <span className="flex-1 text-sm text-surface-700 truncate">{m.domain_name || m.domain_id}</span>
                              <select
                                value={m.role}
                                onChange={(e) => changeMembershipRole(m.domain_id, e.target.value)}
                                className="input-field !py-1 !text-xs w-28"
                              >
                                {ROLES.map((r) => <option key={r} value={r}>{r}</option>)}
                              </select>
                              <button
                                onClick={() => removeMembership(m.domain_id)}
                                className="p-1 rounded hover:bg-red-50"
                                title="Remove domain"
                              >
                                <Trash2 className="h-3.5 w-3.5 text-red-500" />
                              </button>
                            </div>
                          ))}
                        </div>
                        <div className="flex items-center gap-2 mt-2">
                          <select
                            value={memberForm.domain_id}
                            onChange={(e) => setMemberForm({ ...memberForm, domain_id: e.target.value })}
                            className="input-field !py-1 !text-xs flex-1"
                          >
                            <option value="">Add domain…</option>
                            {domains
                              .filter((d) => !(editingUser.memberships || []).some((m) => m.domain_id === d.id))
                              .map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                          </select>
                          <select
                            value={memberForm.role}
                            onChange={(e) => setMemberForm({ ...memberForm, role: e.target.value })}
                            className="input-field !py-1 !text-xs w-28"
                          >
                            {ROLES.map((r) => <option key={r} value={r}>{r}</option>)}
                          </select>
                          <button
                            onClick={addMembership}
                            disabled={!memberForm.domain_id}
                            className="btn-secondary !py-1 !text-xs disabled:opacity-40"
                          >
                            Add
                          </button>
                        </div>
                      </div>
                    )}
                  </div>
                  <div className="flex justify-end gap-2 px-4 py-3 border-t border-surface-200">
                    <button onClick={() => setUserModal(null)} className="btn-secondary text-xs">Cancel</button>
                    <button onClick={saveUser} className="btn-primary gap-2 text-xs">
                      <Save className="h-3.5 w-3.5" />
                      Save
                    </button>
                  </div>
                </div>
              </div>
            )}

            {resetModal && (
              <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
                <div className="w-full max-w-sm rounded-lg bg-white shadow-lg">
                  <div className="flex items-center justify-between px-4 py-3 border-b border-surface-200">
                    <h3 className="text-sm font-semibold text-surface-900">Reset Password</h3>
                    <button onClick={() => setResetModal(null)} className="p-1 rounded hover:bg-surface-100">
                      <X className="h-4 w-4 text-surface-500" />
                    </button>
                  </div>
                  <div className="p-4 space-y-3">
                    <p className="text-sm text-surface-600">
                      Set a new password for <strong>{resetModal.email}</strong>.
                    </p>
                    <input
                      type="password"
                      value={resetPassword}
                      onChange={(e) => setResetPassword(e.target.value)}
                      className="input-field"
                      placeholder="New password"
                    />
                  </div>
                  <div className="flex justify-end gap-2 px-4 py-3 border-t border-surface-200">
                    <button onClick={() => setResetModal(null)} className="btn-secondary text-xs">Cancel</button>
                    <button onClick={doResetPassword} className="btn-primary gap-2 text-xs">
                      <Lock className="h-3.5 w-3.5" />
                      Reset
                    </button>
                  </div>
                </div>
              </div>
            )}
          </div>
        )}

        {activeTab === 'security' && (
          <div className="max-w-2xl space-y-4">
            <h2 className="text-sm font-medium text-surface-900">Security Settings</h2>

            <div className="card p-4 space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium text-surface-900">Allow Insecure Auth</p>
                  <p className="text-xs text-surface-500">
                    Allow plaintext SMTP/IMAP authentication. Development only.
                  </p>
                </div>
                <button
                  onClick={() => handleSettingChange('allow_insecure_auth', settingsForm['allow_insecure_auth'] === 'true' ? 'false' : 'true')}
                  className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                    settingsForm['allow_insecure_auth'] === 'true' ? 'bg-primary-600' : 'bg-surface-300'
                  }`}
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                      settingsForm['allow_insecure_auth'] === 'true' ? 'translate-x-6' : 'translate-x-1'
                    }`}
                  />
                </button>
              </div>

              <div className="border-t border-surface-100 pt-4 flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium text-surface-900">Require Strong Passwords</p>
                  <p className="text-xs text-surface-500">
                    Enforce minimum length and complexity for new passwords.
                  </p>
                </div>
                <button
                  onClick={() => handleSettingChange('require_strong_passwords', settingsForm['require_strong_passwords'] === 'true' ? 'false' : 'true')}
                  className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                    settingsForm['require_strong_passwords'] === 'true' ? 'bg-primary-600' : 'bg-surface-300'
                  }`}
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                      settingsForm['require_strong_passwords'] === 'true' ? 'translate-x-6' : 'translate-x-1'
                    }`}
                  />
                </button>
              </div>

              <div className="border-t border-surface-100 pt-4">
                <label className="block text-sm font-medium text-surface-900 mb-1">Session Timeout (minutes)</label>
                <p className="text-xs text-surface-500 mb-2">Auto-logout after inactivity period.</p>
                <input
                  type="number"
                  min={5}
                  max={1440}
                  value={settingsForm['session_timeout_minutes'] || '60'}
                  onChange={(e) => handleSettingChange('session_timeout_minutes', e.target.value)}
                  className="input-field w-32"
                />
              </div>

              <div className="border-t border-surface-100 pt-4">
                <label className="block text-sm font-medium text-surface-900 mb-1">Rate Limit (requests/minute)</label>
                <p className="text-xs text-surface-500 mb-2">Max requests per IP per minute for login and API.</p>
                <input
                  type="number"
                  min={10}
                  max={10000}
                  value={settingsForm['rate_limit_requests_per_minute'] || '100'}
                  onChange={(e) => handleSettingChange('rate_limit_requests_per_minute', e.target.value)}
                  className="input-field w-32"
                />
              </div>

              <div className="flex justify-end pt-2">
                <button onClick={saveSettings} className="btn-primary gap-2 text-xs">
                  <Save className="h-3.5 w-3.5" />
                  Save Settings
                </button>
              </div>
            </div>
          </div>
        )}

        {activeTab === 'tls' && (
          <div className="max-w-3xl space-y-4">
            <h2 className="text-sm font-medium text-surface-900">TLS / Certificates</h2>

            <div className="card p-4 space-y-3">
              <div className="flex items-center justify-between">
                <p className="text-sm font-medium text-surface-900">Certificate Status</p>
                <button
                  onClick={forceRenewHandler}
                  disabled={tlsBusy || !tlsStatus}
                  className="btn-secondary gap-2 text-xs disabled:opacity-50"
                >
                  <RefreshCw className="h-3.5 w-3.5" />
                  Force Renew
                </button>
              </div>
              {tlsStatus ? (
                <div className="grid grid-cols-2 gap-3 text-xs">
                  <div>
                    <p className="text-surface-500">Issuer</p>
                    <p className="text-surface-900">{tlsStatus.issuer || '—'}</p>
                  </div>
                  <div>
                    <p className="text-surface-500">Directory</p>
                    <p className="text-surface-900">{tlsStatus.directory || '—'}</p>
                  </div>
                  <div>
                    <p className="text-surface-500">Expires</p>
                    <p className="text-surface-900">
                      {tlsStatus.not_after ? new Date(tlsStatus.not_after).toLocaleString() : '—'}
                      {tlsStatus.has_cert && (
                        <span className={`ml-2 ${tlsStatus.days_remaining < 14 ? 'text-red-600' : 'text-green-600'}`}>
                          ({tlsStatus.days_remaining}d)
                        </span>
                      )}
                    </p>
                  </div>
                  <div>
                    <p className="text-surface-500">DNS Provider</p>
                    <p className="text-surface-900">{tlsStatus.dns_provider || '—'}</p>
                  </div>
                  <div className="col-span-2">
                    <p className="text-surface-500">SAN Domains</p>
                    <p className="text-surface-900">{(tlsStatus.domains || []).join(', ') || '—'}</p>
                  </div>
                </div>
              ) : (
                <p className="text-xs text-surface-500 flex items-center gap-2">
                  <AlertTriangle className="h-3.5 w-3.5" />
                  ACME/TLS is not configured. Set POSTNEST_SECRET_KEY and enable ACME.
                </p>
              )}
            </div>

            <div className="card p-4 space-y-4">
              <div className="flex items-center justify-between">
                <p className="text-sm font-medium text-surface-900">ACME Configuration</p>
                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    id="tls-enabled"
                    checked={tlsConfigForm.enabled}
                    onChange={(e) => setTlsConfigForm((p) => ({ ...p, enabled: e.target.checked }))}
                    className="rounded border-surface-300"
                  />
                  <label htmlFor="tls-enabled" className="text-sm text-surface-700">Enable ACME</label>
                </div>
              </div>

              <div>
                <label className="block text-sm font-medium text-surface-900 mb-1">Account Email</label>
                <input
                  type="email"
                  value={tlsConfigForm.email}
                  onChange={(e) => setTlsConfigForm((p) => ({ ...p, email: e.target.value }))}
                  className="input-field w-full"
                  placeholder="admin@example.com"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-surface-900 mb-1">Directory</label>
                <select
                  value={tlsConfigForm.directory}
                  onChange={(e) => setTlsConfigForm((p) => ({ ...p, directory: e.target.value }))}
                  className="input-field w-48"
                >
                  <option value="staging">Staging (Let's Encrypt test)</option>
                  <option value="production">Production</option>
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-surface-900 mb-1">DNS Provider</label>
                <select
                  value={tlsConfigForm.dns_provider}
                  onChange={(e) => setTlsConfigForm((p) => ({ ...p, dns_provider: e.target.value, credentials: {} }))}
                  className="input-field w-48"
                >
                  {tlsProviders.map((p) => (
                    <option key={p.name} value={p.name}>{p.display}</option>
                  ))}
                </select>
              </div>

              <div className="border-t border-surface-100 pt-4 space-y-3">
                <p className="text-xs text-surface-500">
                  Credentials are encrypted at rest. Leave a field blank to keep the stored value.
                </p>
                {(tlsProviders.find((p) => p.name === tlsConfigForm.dns_provider)?.fields || []).map((f) => {
                  const isSet = tlsConfig?.creds_set?.[f.key]
                  const common = {
                    value: tlsConfigForm.credentials[f.key] || '',
                    onChange: (e) => setTlsConfigForm((p) => ({
                      ...p, credentials: { ...p.credentials, [f.key]: e.target.value },
                    })),
                    placeholder: isSet ? '•••••••• (stored)' : '',
                    className: 'input-field w-full',
                  }
                  return (
                    <div key={f.key}>
                      <label className="block text-sm font-medium text-surface-900 mb-1">
                        {f.label}{f.required && <span className="text-red-500"> *</span>}
                      </label>
                      {f.textarea ? (
                        <textarea rows={4} {...common} />
                      ) : (
                        <input type={f.secret ? 'password' : 'text'} {...common} />
                      )}
                    </div>
                  )
                })}
              </div>

              <div className="flex justify-end pt-2">
                <button onClick={saveTLSConfig} disabled={tlsBusy} className="btn-primary gap-2 text-xs disabled:opacity-50">
                  <Save className="h-3.5 w-3.5" />
                  Save Configuration
                </button>
              </div>
            </div>

            <div className="card p-4 space-y-3">
              <p className="text-sm font-medium text-surface-900">SAN Domains</p>
              <p className="text-xs text-surface-500">
                All domains share a single SAN certificate. Adding or removing a domain re-issues the certificate.
              </p>
              <div className="flex gap-2">
                <input
                  type="text"
                  value={newTlsDomain}
                  onChange={(e) => setNewTlsDomain(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && addTlsDomainHandler()}
                  className="input-field flex-1"
                  placeholder="mail.example.com"
                />
                <button onClick={addTlsDomainHandler} disabled={tlsBusy} className="btn-primary gap-2 text-xs disabled:opacity-50">
                  <Plus className="h-3.5 w-3.5" />
                  Add
                </button>
              </div>
              {tlsDomains.length > 0 ? (
                <ul className="divide-y divide-surface-100">
                  {tlsDomains.map((d) => (
                    <li key={d.id} className="flex items-center justify-between py-2 text-sm">
                      <span className="text-surface-900">{d.domain}</span>
                      <button
                        onClick={() => deleteTlsDomainHandler(d.id)}
                        disabled={tlsBusy}
                        className="p-1 rounded hover:bg-red-50 text-red-500 disabled:opacity-50"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-xs text-surface-500">No domains configured.</p>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
