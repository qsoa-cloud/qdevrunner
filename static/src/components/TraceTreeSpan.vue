<template>
  <MDBCard :border="borderVariant" shadow="0" class="mb-1">
    <MDBCardHeader style="background-color: rgba(0,0,0,0.03)" :class="span.spanId ? '' : 'pt-1 pb-1 text-muted'">
      <MDBBadge pill color="primary" title="Span Id" v-if="span.spanId">{{ span.spanId }}</MDBBadge>
      <MDBBadge pill color="success" title="Service" v-if="span.service">{{ span.service }}</MDBBadge>
      <MDBBadge pill color="warning" class="float-end" title="Duration">
        {{ formatDuration(span.start, span.finish) }}
      </MDBBadge>
      <MDBBadge color="secondary" class="float-end me-1" title="Started" v-if="span.spanId">
        {{ formatTime(span.start) }}
      </MDBBadge>

      <MDBBadge color="info" class="me-1" v-for="(v, k) in span.tags" :key="String(k)">
        {{ k }}<span v-if="v !== null && v !== '<nil>'">:&nbsp;{{ v }}</span>
      </MDBBadge>

      <span class="ms-2 d-block"
            :class="{'fw-bold': span.spanId, 'fw-light fst-italic': !span.spanId}"
            title="Operation">{{ span.operation }}</span>

      <MDBProgress v-if="span.percent > 0">
        <MDBProgressBar bg="warning" :value="span.percent" />
      </MDBProgress>

      <div v-if="span.logs?.length" class="alert alert-danger mt-1 mb-1 p-2 small"
           style="max-height: 5rem; overflow-y: auto;">
        <span style="display: block" v-for="(log, i) in span.logs" :key="i">
          {{ log.k }}<span v-if="log.v">:&nbsp;{{ log.v }}</span>
        </span>
      </div>
    </MDBCardHeader>

    <MDBCardBody v-if="span.children?.length" class="p-2">
      <TraceTreeSpan v-for="(s, i) in span.children" :key="i" :span="s" />
    </MDBCardBody>
  </MDBCard>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { MDBBadge, MDBCard, MDBCardBody, MDBCardHeader, MDBProgress, MDBProgressBar } from 'mdb-vue-ui-kit'

export interface SpanNode {
  spanId: number
  service: string
  operation: string
  start: string
  finish: string
  tags?: Record<string, unknown>
  logs?: Array<{ k: string; t: string; v: string }>
  children?: SpanNode[]
  isExternal: boolean
  percent: number
}

const props = defineProps<{ span: SpanNode }>()

const borderVariant = computed(() => {
  if (props.span.isExternal) return 'warning'
  return 'secondary'
})

function formatDuration(start: string, end: string): string {
  const ms = new Date(end).getTime() - new Date(start).getTime()
  if (ms < 1) return '<1ms'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString()
}
</script>
