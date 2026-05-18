<template>
  <div class="d-flex justify-content-between align-items-center mb-3">
    <h4 class="mb-0">Logs</h4>
    <div class="d-flex align-items-center gap-2">
      <select class="form-select form-select-sm" style="width: auto;" v-model="filterService">
        <option value="">All services</option>
        <option v-for="s in serviceNames" :key="s" :value="s">{{ s }}</option>
      </select>
      <MDBBtnGroup size="sm">
        <MDBBtn :color="paused ? 'success' : 'warning'" @click="paused = !paused">
          {{ paused ? 'Resume' : 'Pause' }}
        </MDBBtn>
        <MDBBtn color="secondary" @click="entries = []">Clear</MDBBtn>
      </MDBBtnGroup>
    </div>
  </div>

  <MDBCard>
    <MDBCardBody class="p-0">
      <div class="log-container" ref="logContainer"
           style="height: calc(100vh - 200px); overflow-y: auto; font-family: monospace; font-size: 0.8rem; padding: 12px;">
        <div v-for="(entry, i) in filteredEntries" :key="i" class="log-line">
          <span class="text-muted">{{ formatTime(entry.timestamp) }}</span>
          <MDBBadge :color="serviceColor(entry.service)" class="ms-1">{{ entry.service }}</MDBBadge>
          <MDBBadge :color="streamColor(entry.stream)" class="ms-1">{{ entry.stream }}</MDBBadge>
          <span class="ms-1">{{ entry.text }}</span>
        </div>
        <p v-if="filteredEntries.length === 0" class="text-muted text-center mt-3">
          <MDBSpinner size="sm" class="me-2" /> Waiting for logs...
        </p>
      </div>
    </MDBCardBody>
  </MDBCard>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted } from 'vue'
import { MDBBtn, MDBBtnGroup, MDBBadge, MDBCard, MDBCardBody, MDBSpinner } from 'mdb-vue-ui-kit'
import { useWebSocket } from '../composables/useWebSocket'
import type { LogEntry } from '../types'

const entries = ref<LogEntry[]>([])
const paused = ref(false)
const filterService = ref('')
const logContainer = ref<HTMLElement | null>(null)
const MAX_ENTRIES = 2000

const { onLog, requestBacklog } = useWebSocket()

const serviceNames = computed(() => {
  const names = new Set(entries.value.map(e => e.service))
  return Array.from(names).sort()
})

const filteredEntries = computed(() => {
  if (!filterService.value) return entries.value
  return entries.value.filter(e => e.service === filterService.value)
})

onLog((entry) => {
  if (!paused.value) {
    entries.value.push(entry)
    if (entries.value.length > MAX_ENTRIES) {
      entries.value.splice(0, entries.value.length - MAX_ENTRIES)
    }
    nextTick(() => {
      if (logContainer.value) {
        logContainer.value.scrollTop = logContainer.value.scrollHeight
      }
    })
  }
})

onMounted(requestBacklog)

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString()
}

function serviceColor(_service: string): string {
  return 'info'
}

function streamColor(stream: string): string {
  switch (stream) {
    case 'stderr': return 'danger'
    case 'build': return 'warning'
    default: return 'secondary'
  }
}
</script>
