import axios from 'axios'

const api = axios.create({
  baseURL: '/api/v1',
  headers: {
    'Content-Type': 'application/json',
  },
  withCredentials: true,
})

// Request interceptor: attach CSRF token from cookie on mutating requests.
api.interceptors.request.use((config) => {
  const method = (config.method || 'get').toLowerCase()
  if (['post', 'put', 'patch', 'delete'].includes(method)) {
    const m = document.cookie.match(/(?:^|;\s*)csrf=([^;]+)/)
    if (m) config.headers['X-CSRF-Token'] = decodeURIComponent(m[1])
  }
  return config
})

// Response interceptor for global error handling
api.interceptors.response.use(
  (response) => response,
	(error) => {
		if (error.response?.status === 401 &&
			!error.config?.url?.includes('/auth/')) {
			if (window.location.pathname !== '/login') {
				window.location.assign('/login')
			}
		}
		return Promise.reject(error)
	}
)

export default api

export async function login(email, password) {
  const res = await api.post('/auth/login', { email, password })
  return res.data.user
}

export async function logout() {
  await api.post('/auth/logout')
}

export async function getMe() {
  const res = await api.get('/auth/me')
  return res.data.user
}

export async function getLabels() {
  const res = await api.get('/labels')
  return res.data.labels || []
}

export async function getMessages(labelId, params = {}) {
  const query = new URLSearchParams({ ...params })
  if (labelId) query.set('label_id', labelId)
  const res = await api.get(`/messages?${query}`)
  return res.data
}

export async function getMessage(id) {
  const res = await api.get(`/messages/${id}`)
  return res.data
}

export async function patchMessage(id, updates) {
  const res = await api.patch(`/messages/${id}`, updates)
  return res.data
}

export async function deleteMessage(id) {
  await api.delete(`/messages/${id}`)
}

export async function batchMessages(action, messageIds) {
  const res = await api.post('/messages/batch', { action, message_ids: messageIds })
  return res.data
}

export async function getThread(id) {
  const res = await api.get(`/threads/${id}`)
  return res.data
}

// parseRecipients turns "Name <a@x.com>, b@y.com" into [{name,address}].
export function parseRecipients(input) {
  if (Array.isArray(input)) return input
  if (!input) return []
  return String(input)
    .split(/[,;]/)
    .map((s) => s.trim())
    .filter(Boolean)
    .map((token) => {
      const m = token.match(/^(.*?)\s*<([^>]+)>$/)
      if (m) return { name: m[1].trim(), address: m[2].trim() }
      return { address: token }
    })
}

function toDraftPayload(data) {
  return {
    subject: data.subject || '',
    to: parseRecipients(data.to),
    cc: parseRecipients(data.cc),
    bcc: parseRecipients(data.bcc),
    html_body: data.html_body || '',
    plain_text: data.plain_text || '',
  }
}

export async function createDraft(data) {
  const res = await api.post('/drafts', toDraftPayload(data))
  return res.data
}

export async function updateDraft(id, data) {
  const res = await api.put(`/drafts/${id}`, toDraftPayload(data))
  return res.data
}

export async function sendDraft(id) {
  await api.post(`/drafts/${id}/send`)
}

export async function listDraftAttachments(id) {
  const res = await api.get(`/drafts/${id}/attachments`)
  return res.data.attachments || []
}

export async function uploadDraftAttachment(id, file) {
  const form = new FormData()
  form.append('file', file)
  const res = await api.post(`/drafts/${id}/attachments`, form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return res.data
}

export async function deleteDraftAttachment(id, attID) {
  await api.delete(`/drafts/${id}/attachments/${attID}`)
}

export async function searchMessages(q, params = {}) {
  const query = new URLSearchParams({ q, ...params })
  const res = await api.get(`/search?${query}`)
  return res.data
}

export async function getDomains() {
  const res = await api.get('/admin/api/v1/domains', { baseURL: '' })
  return res.data.domains || []
}

export async function createDomain(data) {
  const res = await api.post('/admin/api/v1/domains', data, { baseURL: '' })
  return res.data
}

export async function updateDomain(id, data) {
  const res = await api.put(`/admin/api/v1/domains/${id}`, data, { baseURL: '' })
  return res.data
}

export async function deleteDomain(id) {
  await api.delete(`/admin/api/v1/domains/${id}`, { baseURL: '' })
}

export async function toggleDomainActive(id, isActive) {
  const res = await api.patch(`/admin/api/v1/domains/${id}/active`, { is_active: isActive }, { baseURL: '' })
  return res.data
}

export async function getHealth() {
  const res = await api.get('/admin/api/v1/health', { baseURL: '' })
  return res.data
}

export async function getAdminUsers(limit = 20, offset = 0) {
  const res = await api.get(`/admin/api/v1/users?limit=${limit}&offset=${offset}`, { baseURL: '' })
  return res.data.users || []
}

export async function createAdminUser(data) {
  const res = await api.post('/admin/api/v1/users', data, { baseURL: '' })
  return res.data
}

export async function updateAdminUser(id, data) {
  const res = await api.put(`/admin/api/v1/users/${id}`, data, { baseURL: '' })
  return res.data
}

export async function deleteAdminUser(id) {
  await api.delete(`/admin/api/v1/users/${id}`, { baseURL: '' })
}

export async function resetAdminUserPassword(id, password) {
  const res = await api.post(`/admin/api/v1/users/${id}/reset-password`, { password }, { baseURL: '' })
  return res.data
}

export async function addUserDomain(userId, domainId, role) {
  const res = await api.post(`/admin/api/v1/users/${userId}/domains`, { domain_id: domainId, role }, { baseURL: '' })
  return res.data.membership
}

export async function updateUserDomainRole(userId, domainId, role) {
  const res = await api.put(`/admin/api/v1/users/${userId}/domains/${domainId}`, { role }, { baseURL: '' })
  return res.data
}

export async function removeUserDomain(userId, domainId) {
  await api.delete(`/admin/api/v1/users/${userId}/domains/${domainId}`, { baseURL: '' })
}

export async function getAdminSettings() {
  const res = await api.get('/admin/api/v1/settings', { baseURL: '' })
  return res.data.settings || {}
}

export async function updateAdminSettings(settings) {
  const res = await api.put('/admin/api/v1/settings', settings, { baseURL: '' })
  return res.data
}

export async function getContacts() {
  const res = await api.get('/contacts')
  return res.data.contacts || []
}

export async function createContact(data) {
  const res = await api.post('/contacts', data)
  return res.data
}

export async function updateContact(id, data) {
  const res = await api.patch(`/contacts/${id}`, data)
  return res.data
}

export async function deleteContact(id) {
  await api.delete(`/contacts/${id}`)
}

export async function getCalendarEvents(start, end) {
  const res = await api.get(`/calendar/events?start=${start}&end=${end}`)
  return res.data.events || []
}

export async function createCalendarEvent(data) {
  const res = await api.post('/calendar/events', data)
  return res.data
}

export async function updateCalendarEvent(id, data) {
  const res = await api.patch(`/calendar/events/${id}`, data)
  return res.data
}

export async function deleteCalendarEvent(id) {
  await api.delete(`/calendar/events/${id}`)
}
