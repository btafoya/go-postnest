class SSEClient {
  constructor() {
    this.eventSource = null
    this.listeners = new Map()
    this.reconnectTimer = null
    this.reconnectDelay = 1000
  }

  connect() {
    if (this.eventSource) return

    this.eventSource = new EventSource('/events')

    this.eventSource.addEventListener('connected', (e) => {
      console.log('SSE connected:', JSON.parse(e.data))
      this.reconnectDelay = 1000
    })

    this.eventSource.addEventListener('heartbeat', () => {
      // Keep-alive, no action needed
    })

    this.eventSource.onmessage = (e) => {
      try {
        const event = JSON.parse(e.data)
        this.emit(event.type, event.payload)
      } catch (err) {
        console.warn('SSE parse error:', err)
      }
    }

    this.eventSource.onerror = () => {
      console.warn('SSE connection error, reconnecting...')
      this.disconnect()
      this.reconnectDelay = Math.min(this.reconnectDelay * 2, 30000)
      this.reconnectTimer = setTimeout(() => this.connect(), this.reconnectDelay)
    }
  }

  disconnect() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    if (this.eventSource) {
      this.eventSource.close()
      this.eventSource = null
    }
  }

  on(event, callback) {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set())
    }
    this.listeners.get(event).add(callback)
    return () => this.listeners.get(event)?.delete(callback)
  }

  emit(event, payload) {
    this.listeners.get(event)?.forEach((cb) => cb(payload))
  }
}

export const sse = new SSEClient()
export default sse
