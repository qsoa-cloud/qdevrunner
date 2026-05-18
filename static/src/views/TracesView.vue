<template>
  <div class="d-flex justify-content-between align-items-center mb-3">
    <h4 class="mb-0">Traces</h4>
    <MDBBtnGroup size="sm">
      <MDBBtn :color="paused ? 'success' : 'warning'" @click="paused = !paused">
        {{ paused ? 'Resume' : 'Pause' }}
      </MDBBtn>
      <MDBBtn color="secondary" @click="clearTraces">Clear</MDBBtn>
    </MDBBtnGroup>
  </div>

  <!-- Trace detail view -->
  <div v-if="selectedTrace !== null">
    <MDBBtn color="link" size="sm" @click="selectedTrace = null" class="mb-2 ps-0">&larr; Back to list</MDBBtn>
    <TraceTreeSpan v-if="traceTree" :span="traceTree" />
  </div>

  <!-- Trace list view -->
  <div v-else>
    <MDBTable striped hover small responsive>
      <thead>
        <tr>
          <th>Time</th>
          <th>Trace ID</th>
          <th>Service</th>
          <th>Operation</th>
          <th>Spans</th>
          <th>Duration</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="trace in displayTraces" :key="trace.traceId"
            @click="selectedTrace = trace.traceId" style="cursor: pointer;">
          <td class="text-muted small text-nowrap">{{ formatTimestamp(trace.startTime) }}</td>
          <td><code>{{ trace.traceId }}</code></td>
          <td><MDBBadge color="primary">{{ trace.service }}</MDBBadge></td>
          <td>{{ trace.operation }}</td>
          <td><MDBBadge color="secondary">{{ trace.spanCount }}</MDBBadge></td>
          <td><MDBBadge pill color="warning">{{ trace.duration }}</MDBBadge></td>
        </tr>
      </tbody>
    </MDBTable>
    <p v-if="traces.size === 0" class="text-muted text-center py-4">
      <MDBSpinner size="sm" class="me-2" /> Waiting for traces...
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { MDBTable, MDBBadge, MDBBtn, MDBBtnGroup, MDBSpinner } from 'mdb-vue-ui-kit'
import { useWebSocket } from '../composables/useWebSocket'
import TraceTreeSpan from '../components/TraceTreeSpan.vue'
import type { SpanNode } from '../components/TraceTreeSpan.vue'
import type { SpanRecord } from '../types'

interface TraceEntry {
  traceId: number
  service: string
  operation: string
  startTime: string
  duration: string
  spanCount: number
  spans: SpanRecord[]
}

const traces = ref<Map<number, TraceEntry>>(new Map())
const paused = ref(false)
const selectedTrace = ref<number | null>(null)

const { onSpan, requestBacklog } = useWebSocket()

onSpan((record) => {
  if (paused.value) return

  const traceId = record.span.c.t
  const existing = traces.value.get(traceId)

  if (existing) {
    existing.spans.push(record)
    existing.spanCount = existing.spans.length
    // Update duration from root span timing.
    updateTraceInfo(existing)
  } else {
    const entry: TraceEntry = {
      traceId,
      service: record.service,
      operation: record.span.o,
      startTime: record.span.s,
      duration: formatDuration(record.span.s, record.span.f),
      spanCount: 1,
      spans: [record],
    }
    traces.value.set(traceId, entry)
  }
  // Trigger reactivity.
  traces.value = new Map(traces.value)
})

onMounted(requestBacklog)

const displayTraces = computed(() => {
  const arr = Array.from(traces.value.values())
  arr.sort((a, b) => new Date(b.startTime).getTime() - new Date(a.startTime).getTime())
  return arr.slice(0, 100)
})

const traceTree = computed((): SpanNode | null => {
  if (selectedTrace.value === null) return null
  const entry = traces.value.get(selectedTrace.value)
  if (!entry) return null
  return buildTree(entry.spans)
})

function updateTraceInfo(entry: TraceEntry) {
  // Find root span (no parent) or earliest span.
  let root = entry.spans[0]
  for (const s of entry.spans) {
    if (!s.span.c.p) {
      root = s
      break
    }
  }
  entry.service = root.service
  entry.operation = root.span.o
  entry.startTime = root.span.s
  entry.duration = formatDuration(root.span.s, root.span.f)
}

function buildTree(spans: SpanRecord[]): SpanNode {
  const byParent = new Map<number, SpanRecord[]>()
  let rootRecord = spans[0]

  for (const s of spans) {
    if (!s.span.c.p) rootRecord = s
    const parentId = s.span.c.p || 0
    const children = byParent.get(parentId) || []
    children.push(s)
    byParent.set(parentId, children)
  }

  function toNode(record: SpanRecord): SpanNode {
    const node: SpanNode = {
      spanId: record.span.c.s,
      service: record.service,
      operation: record.span.o,
      start: record.span.s,
      finish: record.span.f,
      tags: record.span.t,
      logs: record.span.lf,
      isExternal: false,
      percent: 0,
    }

    const children = byParent.get(record.span.c.s)
    if (children) {
      const sorted = children.sort((a, b) =>
        new Date(a.span.s).getTime() - new Date(b.span.s).getTime()
      )
      node.children = extendChildren(node, sorted.map(toNode))
    }
    return node
  }

  const root = toNode(rootRecord)
  root.isExternal = true
  return root
}

function extendChildren(parent: SpanNode, children: SpanNode[]): SpanNode[] {
  const result: SpanNode[] = []
  const parentMs = new Date(parent.finish).getTime() - new Date(parent.start).getTime()
  if (parentMs <= 0) return children

  for (let i = 0; i < children.length; i++) {
    const span = children[i]
    span.isExternal = span.service !== parent.service
    span.percent = (new Date(span.finish).getTime() - new Date(span.start).getTime()) * 100 / parentMs

    const prevEnd = i === 0
      ? new Date(parent.start).getTime()
      : new Date(children[i - 1].finish).getTime()
    const curStart = new Date(span.start).getTime()

    if (curStart - prevEnd > 1) {
      result.push(makeGap(new Date(prevEnd).toISOString(), new Date(curStart).toISOString(), parentMs))
    }

    result.push(span)

    if (i === children.length - 1) {
      const spanEnd = new Date(span.finish).getTime()
      const parentEnd = new Date(parent.finish).getTime()
      if (parentEnd - spanEnd > 1) {
        result.push(makeGap(new Date(spanEnd).toISOString(), new Date(parentEnd).toISOString(), parentMs))
      }
    }
  }

  return result
}

function makeGap(start: string, finish: string, parentMs: number): SpanNode {
  return {
    spanId: 0,
    service: '',
    operation: 'Something else',
    start,
    finish,
    isExternal: false,
    percent: (new Date(finish).getTime() - new Date(start).getTime()) * 100 / parentMs,
  }
}

function clearTraces() {
  traces.value = new Map()
  selectedTrace.value = null
}

function formatTimestamp(iso: string): string {
  return new Date(iso).toLocaleTimeString()
}

function formatDuration(start: string, end: string): string {
  const ms = new Date(end).getTime() - new Date(start).getTime()
  if (ms < 1) return '<1ms'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}
</script>
