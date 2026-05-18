export interface ServiceInfo {
  name: string
  status: 'stopped' | 'starting' | 'running' | 'error' | 'manual'
  mode: 'managed' | 'manual'
  pid?: number
  started?: string
  error?: string
  transports: string[]
  httpport?: number
  run_command?: string
}

export interface SpanContext {
  t: number // traceID
  s: number // spanID
  p?: number // parentSpanID
}

export interface LogField {
  k: string
  t: string
  v: string
}

export interface Span {
  o: string // operation
  s: string // startTime
  f: string // finishTime
  c: SpanContext
  t: Record<string, unknown> // tags
  lf: LogField[] // logFields
}

export interface SpanRecord {
  service: string
  received_at: string
  span: Span
}

export interface TraceInfo {
  trace_id: number
  root_operation: string
  services: string[]
  span_count: number
  start_time: string
  duration: number
}

export interface LogEntry {
  service: string
  stream: 'stdout' | 'stderr' | 'build'
  text: string
  timestamp: string
}

export interface MetricValue {
  name: string
  type: 'counter' | 'summary' | 'gauge'
  labels?: Record<string, string>
  value?: number
  sum?: number
  count?: number
}

export interface MetricsSnapshot {
  service: string
  timestamp: string
  metrics: MetricValue[]
}

export interface WSMessage {
  type: 'span' | 'log' | 'metrics' | 'service_event' | 'status'
  payload: unknown
}

export interface DatabaseInfo {
  type: string
  dsn: string
}

export interface ConfigInfo {
  project_name: string
  project_env: string
  databases: Record<string, DatabaseInfo>
  buckets: Record<string, string>
  mailboxes: string[]
  env_vars: Record<string, string>
  services: string[]
}
