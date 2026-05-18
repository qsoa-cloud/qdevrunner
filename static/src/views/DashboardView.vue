<template>
  <div class="d-flex justify-content-between align-items-center mb-3">
    <h4 class="mb-0">Services</h4>
    <MDBBtn color="primary" size="sm" @click="showAddService = true">Add Service</MDBBtn>
  </div>

  <MDBRow>
    <MDBCol md="6" lg="4" class="mb-3 d-flex" v-for="svc in sortedServices" :key="svc.name">
      <MDBCard class="w-100">
        <MDBCardHeader class="d-flex justify-content-between align-items-center">
          <strong>{{ svc.name }}</strong>
          <div class="d-flex align-items-center gap-2">
            <MDBBadge :color="statusColor(svc.status)">{{ svc.status }}</MDBBadge>
            <MDBBtnClose @click="removeService(svc.name)" />
          </div>
        </MDBCardHeader>
        <MDBCardBody>
          <MDBCardText>
            <small class="text-muted">Mode: {{ svc.mode }}</small>
            <template v-if="svc.pid"><br /><small class="text-muted">PID: {{ svc.pid }}</small></template>
            <template v-if="svc.started"><br /><small class="text-muted">Started: {{ formatTime(svc.started) }}</small></template>
            <template v-if="svc.error"><br /><small class="text-danger">{{ svc.error }}</small></template>
          </MDBCardText>
          <div class="mb-2">
            <MDBBadge v-for="t in svc.transports" :key="t" color="info" class="me-1">{{ t }}</MDBBadge>
          </div>
          <div v-if="svc.httpport" class="mb-2">
            <small><a :href="'http://127.0.0.1:' + svc.httpport" target="_blank">http://127.0.0.1:{{ svc.httpport }}</a></small>
          </div>
          <MDBBtnGroup v-if="svc.mode === 'managed'" size="sm">
            <MDBBtn color="success" size="sm" @click="action(svc.name, 'start')"
                    :disabled="svc.status === 'running'">Start</MDBBtn>
            <MDBBtn color="danger" size="sm" @click="action(svc.name, 'stop')"
                    :disabled="svc.status === 'stopped' || svc.status === 'error'">Stop</MDBBtn>
            <MDBBtn color="warning" size="sm" @click="action(svc.name, 'restart')"
                    :disabled="svc.status === 'stopped' || svc.status === 'error'">Restart</MDBBtn>
          </MDBBtnGroup>
          <div v-if="svc.mode === 'manual' && svc.run_command" class="mt-2">
            <small class="text-muted">Run command:</small>
            <pre class="bg-light p-2 rounded mt-1 small" style="white-space: pre-wrap; word-break: break-all;">{{ svc.run_command }}</pre>
          </div>
        </MDBCardBody>
      </MDBCard>
    </MDBCol>
  </MDBRow>

  <p v-if="services.length === 0" class="text-muted text-center py-4">
    No services configured.
  </p>

  <!-- Add Service Modal -->
  <MDBModal v-model="showAddService" tabindex="-1" size="lg">
    <MDBModalHeader>
      <MDBModalTitle>Add Service</MDBModalTitle>
    </MDBModalHeader>
    <MDBModalBody>
      <MDBRow>
        <MDBCol md="6">
          <MDBInput v-model="form.name" label="Name *" class="mb-1" />
          <div class="form-text mb-4">Service discovery name</div>
        </MDBCol>
        <MDBCol md="6">
          <MDBInput v-model="form.workdir" label="Working Directory *" class="mb-1" />
          <div class="form-text mb-4">Path to the service source directory</div>
        </MDBCol>
      </MDBRow>

      <MDBRow>
        <MDBCol md="4">
          <MDBInput v-model.number="form.httpport" label="HTTP Port" type="number" class="mb-1" />
          <div class="form-text mb-4">TCP proxy port (optional)</div>
        </MDBCol>
        <MDBCol md="4" class="d-flex align-items-center">
          <MDBSwitch v-model="form.autostart" label="Autostart" />
        </MDBCol>
        <MDBCol md="4" class="d-flex align-items-center">
          <MDBSwitch v-model="form.watch" label="Watch for changes" />
        </MDBCol>
      </MDBRow>
    </MDBModalBody>
    <MDBModalFooter>
      <MDBBtn color="secondary" @click="showAddService = false">Cancel</MDBBtn>
      <MDBBtn color="primary" @click="addService" :disabled="!form.name || !form.workdir">Add Service</MDBBtn>
    </MDBModalFooter>
  </MDBModal>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import {
  MDBCard, MDBCardBody, MDBCardHeader, MDBCardText, MDBBadge, MDBBtn,
  MDBBtnGroup, MDBBtnClose, MDBRow, MDBCol, MDBInput, MDBSwitch,
  MDBModal, MDBModalHeader, MDBModalTitle, MDBModalBody, MDBModalFooter,
} from 'mdb-vue-ui-kit'
import type { ServiceInfo } from '../types'

const services = ref<ServiceInfo[]>([])
const sortedServices = computed(() =>
  [...services.value].sort((a, b) => a.name.localeCompare(b.name))
)
const showAddService = ref(false)
const form = reactive({
  name: '',
  workdir: '',
  httpport: 0,
  autostart: true,
  watch: true,
})

async function fetchServices() {
  const resp = await fetch('/api/services')
  services.value = await resp.json()
}

async function action(name: string, act: string) {
  await fetch(`/api/services/${name}/${act}`, { method: 'POST' })
  setTimeout(fetchServices, 500)
}

async function addService() {
  await fetch('/api/config/services/add', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name: form.name,
      workdir: form.workdir,
      httpport: form.httpport || 0,
      autostart: form.autostart,
      watch: form.watch,
    }),
  })
  showAddService.value = false
  form.name = ''; form.workdir = ''
  form.httpport = 0; form.autostart = true; form.watch = true
  await fetchServices()
}

async function removeService(name: string) {
  if (!confirm(`Remove service "${name}"?`)) return
  await fetch(`/api/config/services/remove`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
  await fetchServices()
}

function statusColor(status: string): string {
  switch (status) {
    case 'running': return 'success'
    case 'stopped': return 'secondary'
    case 'error': return 'danger'
    case 'starting': return 'warning'
    case 'manual': return 'info'
    default: return 'secondary'
  }
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString()
}

onMounted(fetchServices)
</script>
