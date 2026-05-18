import { ref, onUnmounted } from 'vue'
import type { WSMessage, SpanRecord, LogEntry, MetricsSnapshot } from '../types'

type Handler = (msg: WSMessage) => void

const handlers = new Set<Handler>()
let ws: WebSocket | null = null
const connected = ref(false)

function getWsUrl(): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${location.host}/ws`
}

function connect() {
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
    return
  }

  ws = new WebSocket(getWsUrl())

  ws.onopen = () => {
    connected.value = true
    console.log('WebSocket connected')
  }

  ws.onmessage = (event) => {
    try {
      const msg: WSMessage = JSON.parse(event.data)
      handlers.forEach((h) => h(msg))
    } catch (e) {
      console.error('WS parse error:', e)
    }
  }

  ws.onclose = () => {
    connected.value = false
    ws = null
    setTimeout(connect, 2000)
  }

  ws.onerror = () => {
    ws?.close()
  }
}

// Start the connection once.
connect()

export function useWebSocket() {
  function subscribe(handler: Handler) {
    handlers.add(handler)
    onUnmounted(() => handlers.delete(handler))
  }

  function onSpan(handler: (record: SpanRecord) => void) {
    subscribe((msg) => {
      if (msg.type === 'span') handler(msg.payload as SpanRecord)
    })
  }

  function onLog(handler: (entry: LogEntry) => void) {
    subscribe((msg) => {
      if (msg.type === 'log') handler(msg.payload as LogEntry)
    })
  }

  function onMetrics(handler: (snapshot: MetricsSnapshot) => void) {
    subscribe((msg) => {
      if (msg.type === 'metrics') handler(msg.payload as MetricsSnapshot)
    })
  }

  function onServiceEvent(handler: (event: unknown) => void) {
    subscribe((msg) => {
      if (msg.type === 'service_event') handler(msg.payload)
    })
  }

  function requestBacklog() {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'backlog' }))
    }
  }

  return { connected, subscribe, onSpan, onLog, onMetrics, onServiceEvent, requestBacklog }
}
