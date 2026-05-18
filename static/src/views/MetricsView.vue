<template>
  <div class="d-flex justify-content-between align-items-center mb-3">
    <h4 class="mb-0">Metrics</h4>
    <select class="form-select form-select-sm" style="width: auto;" v-model="selectedService">
      <option value="">All services</option>
      <option v-for="s in serviceNames" :key="s" :value="s">{{ s }}</option>
    </select>
  </div>

  <p v-if="latestMetrics.length === 0" class="text-muted text-center py-4">
    <MDBSpinner size="sm" class="me-2" /> Waiting for metrics... (Prometheus scrape interval: 5s)
  </p>

  <MDBRow>
    <MDBCol md="6" lg="4" xl="3" class="mb-3" v-for="metric in latestMetrics" :key="metric.name">
      <MDBCard>
        <MDBCardBody>
          <div class="d-flex justify-content-between align-items-start mb-2">
            <MDBCardTitle class="small mb-0">{{ metric.name }}</MDBCardTitle>
            <MDBBadge :color="metricTypeColor(metric.type)">{{ metric.type }}</MDBBadge>
          </div>
          <div v-if="metric.type === 'counter' || metric.type === 'gauge'" class="fs-4 fw-bold">
            {{ formatValue(metric.value) }}
          </div>
          <div v-if="metric.type === 'summary'">
            <div>Sum: <strong>{{ formatValue(metric.sum) }}</strong></div>
            <div>Count: <strong>{{ metric.count }}</strong></div>
          </div>
          <div v-if="metric.labels && Object.keys(metric.labels).length > 0" class="mt-2">
            <MDBBadge v-for="(v, k) in metric.labels" :key="String(k)" color="light" class="me-1 text-dark small">
              {{ k }}={{ v }}
            </MDBBadge>
          </div>
        </MDBCardBody>
      </MDBCard>
    </MDBCol>
  </MDBRow>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import {
  MDBCard, MDBCardBody, MDBCardTitle, MDBBadge, MDBRow, MDBCol, MDBSpinner,
} from 'mdb-vue-ui-kit'
import { useWebSocket } from '../composables/useWebSocket'
import type { MetricsSnapshot, MetricValue } from '../types'

const snapshots = ref<MetricsSnapshot[]>([])
const selectedService = ref('')
const MAX_SNAPSHOTS = 100

const { onMetrics } = useWebSocket()

onMetrics((snapshot) => {
  snapshots.value.push(snapshot)
  if (snapshots.value.length > MAX_SNAPSHOTS) {
    snapshots.value.splice(0, snapshots.value.length - MAX_SNAPSHOTS)
  }
})

const serviceNames = computed(() => {
  const names = new Set(snapshots.value.map(s => s.service))
  return Array.from(names).sort()
})

const latestMetrics = computed((): MetricValue[] => {
  const filtered = selectedService.value
    ? snapshots.value.filter(s => s.service === selectedService.value)
    : snapshots.value
  if (filtered.length === 0) return []
  const latest = new Map<string, MetricsSnapshot>()
  for (const s of filtered) {
    latest.set(s.service, s)
  }
  const allMetrics: MetricValue[] = []
  for (const s of latest.values()) {
    allMetrics.push(...s.metrics)
  }
  return allMetrics
})

function metricTypeColor(type: string): string {
  switch (type) {
    case 'counter': return 'primary'
    case 'gauge': return 'success'
    case 'summary': return 'warning'
    default: return 'secondary'
  }
}

function formatValue(v?: number): string {
  if (v === undefined) return '-'
  if (v > 1000000) return `${(v / 1000000).toFixed(2)}M`
  if (v > 1000) return `${(v / 1000).toFixed(2)}K`
  return v.toFixed(2)
}
</script>
