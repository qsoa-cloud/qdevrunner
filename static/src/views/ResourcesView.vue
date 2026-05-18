<template>
  <h4 class="mb-3">Resources</h4>

  <MDBSpinner v-if="!config" />

  <template v-else>
    <MDBTabs v-model="activeTab">
      <MDBTabNav tabsClasses="mb-3">
        <MDBTabItem v-for="t in tabs" :key="t.id" :tabId="t.id">
          {{ t.label }}
          <MDBBadge v-if="t.count > 0" color="secondary" class="ms-1">{{ t.count }}</MDBBadge>
        </MDBTabItem>
      </MDBTabNav>
      <MDBTabContent>
        <!-- Env Variables -->
        <MDBTabPane tabId="envs">
          <div class="d-flex justify-content-end mb-3">
            <MDBBtn color="primary" size="sm" @click="showAddEnv = true">Add Variable</MDBBtn>
          </div>
          <MDBTable v-if="Object.keys(config.env_vars).length > 0" striped hover small>
            <thead><tr><th>Name</th><th>Value</th><th>Injected As</th><th width="100"></th></tr></thead>
            <tbody>
              <tr v-for="(value, name) in config.env_vars" :key="name">
                <td><strong>{{ name }}</strong></td>
                <td><code>{{ value }}</code></td>
                <td><code>QSOA_{{ name }}</code></td>
                <td><MDBBtn outline="danger" size="sm" @click="removeResource('envs', {name})">Remove</MDBBtn></td>
              </tr>
            </tbody>
          </MDBTable>
          <p v-else class="text-muted text-center py-4">No environment variables configured.</p>
        </MDBTabPane>

        <!-- Databases -->
        <MDBTabPane tabId="databases">
          <div class="d-flex justify-content-end mb-3">
            <MDBBtn color="primary" size="sm" @click="showAddDb = true">Add Database</MDBBtn>
          </div>
          <MDBTable v-if="Object.keys(config.databases).length > 0" striped hover small>
            <thead><tr><th>Name</th><th>Type</th><th>DSN</th><th width="100"></th></tr></thead>
            <tbody>
              <tr v-for="(db, name) in config.databases" :key="name">
                <td><strong>{{ name }}</strong></td>
                <td><MDBBadge color="info">{{ db.type }}</MDBBadge></td>
                <td><code>{{ db.dsn }}</code></td>
                <td><MDBBtn outline="danger" size="sm" @click="removeResource('databases', {name})">Remove</MDBBtn></td>
              </tr>
            </tbody>
          </MDBTable>
          <p v-else class="text-muted text-center py-4">No databases configured.</p>
        </MDBTabPane>

        <!-- DFS Buckets -->
        <MDBTabPane tabId="buckets">
          <div class="d-flex justify-content-end mb-3">
            <MDBBtn color="primary" size="sm" @click="showAddBucket = true">Add Bucket</MDBBtn>
          </div>
          <MDBTable v-if="Object.keys(config.buckets).length > 0" striped hover small>
            <thead><tr><th>Bucket</th><th>Local Path</th><th width="100"></th></tr></thead>
            <tbody>
              <tr v-for="(path, name) in config.buckets" :key="name">
                <td><strong>{{ name }}</strong></td>
                <td><code>{{ path }}</code></td>
                <td><MDBBtn outline="danger" size="sm" @click="removeResource('buckets', {name})">Remove</MDBBtn></td>
              </tr>
            </tbody>
          </MDBTable>
          <p v-else class="text-muted text-center py-4">No DFS buckets configured.</p>
        </MDBTabPane>

        <!-- Mailboxes -->
        <MDBTabPane tabId="mailboxes">
          <div class="d-flex justify-content-end mb-3">
            <MDBBtn color="primary" size="sm" @click="showAddMailbox = true">Add Mailbox</MDBBtn>
          </div>
          <MDBTable v-if="config.mailboxes.length > 0" striped hover small>
            <thead><tr><th>Address</th><th width="100"></th></tr></thead>
            <tbody>
              <tr v-for="addr in config.mailboxes" :key="addr">
                <td><strong>{{ addr }}</strong></td>
                <td><MDBBtn outline="danger" size="sm" @click="removeResource('mailboxes', {address: addr})">Remove</MDBBtn></td>
              </tr>
            </tbody>
          </MDBTable>
          <p v-else class="text-muted text-center py-4">No mailboxes configured.</p>
        </MDBTabPane>
      </MDBTabContent>
    </MDBTabs>
  </template>

  <!-- Add Database Modal -->
  <MDBModal v-model="showAddDb" tabindex="-1">
    <MDBModalHeader>
      <MDBModalTitle>Add Database</MDBModalTitle>
    </MDBModalHeader>
    <MDBModalBody>
      <MDBInput v-model="form.name" label="Name" class="mb-4" />
      <MDBInput v-model="form.dsn" label="DSN" class="mb-2" />
      <div class="form-text">e.g. root:pass@tcp(127.0.0.1:3306)/mydb</div>
    </MDBModalBody>
    <MDBModalFooter>
      <MDBBtn color="secondary" @click="showAddDb = false">Cancel</MDBBtn>
      <MDBBtn color="primary" @click="addResource('databases', {name: form.name, dsn: form.dsn}); showAddDb = false">Add</MDBBtn>
    </MDBModalFooter>
  </MDBModal>

  <!-- Add Bucket Modal -->
  <MDBModal v-model="showAddBucket" tabindex="-1">
    <MDBModalHeader>
      <MDBModalTitle>Add DFS Bucket</MDBModalTitle>
    </MDBModalHeader>
    <MDBModalBody>
      <MDBInput v-model="form.name" label="Name" class="mb-4" />
      <MDBInput v-model="form.path" label="Local Path" class="mb-2" />
      <div class="form-text">e.g. /tmp/dfs-uploads</div>
    </MDBModalBody>
    <MDBModalFooter>
      <MDBBtn color="secondary" @click="showAddBucket = false">Cancel</MDBBtn>
      <MDBBtn color="primary" @click="addResource('buckets', {name: form.name, path: form.path}); showAddBucket = false">Add</MDBBtn>
    </MDBModalFooter>
  </MDBModal>

  <!-- Add Mailbox Modal -->
  <MDBModal v-model="showAddMailbox" tabindex="-1">
    <MDBModalHeader>
      <MDBModalTitle>Add Mailbox</MDBModalTitle>
    </MDBModalHeader>
    <MDBModalBody>
      <MDBInput v-model="form.address" label="Email Address" class="mb-4" />
      <MDBRow>
        <MDBCol md="6"><MDBInput v-model="form.smtp" label="SMTP Server" class="mb-4" /></MDBCol>
        <MDBCol md="6"><MDBInput v-model="form.smtp_password" label="SMTP Password" type="password" class="mb-4" /></MDBCol>
      </MDBRow>
      <MDBRow>
        <MDBCol md="6"><MDBInput v-model="form.imap" label="IMAP Server" class="mb-4" /></MDBCol>
        <MDBCol md="6"><MDBInput v-model="form.imap_password" label="IMAP Password" type="password" class="mb-4" /></MDBCol>
      </MDBRow>
    </MDBModalBody>
    <MDBModalFooter>
      <MDBBtn color="secondary" @click="showAddMailbox = false">Cancel</MDBBtn>
      <MDBBtn color="primary" @click="addResource('mailboxes', form); showAddMailbox = false">Add</MDBBtn>
    </MDBModalFooter>
  </MDBModal>

  <!-- Add Env Variable Modal -->
  <MDBModal v-model="showAddEnv" tabindex="-1">
    <MDBModalHeader>
      <MDBModalTitle>Add Environment Variable</MDBModalTitle>
    </MDBModalHeader>
    <MDBModalBody>
      <MDBInput v-model="form.name" label="Name" class="mb-1" />
      <div class="form-text mb-4">Injected as QSOA_{{ form.name || 'NAME' }}</div>
      <MDBInput v-model="form.value" label="Value" />
    </MDBModalBody>
    <MDBModalFooter>
      <MDBBtn color="secondary" @click="showAddEnv = false">Cancel</MDBBtn>
      <MDBBtn color="primary" @click="addResource('envs', {name: form.name, value: form.value}); showAddEnv = false">Add</MDBBtn>
    </MDBModalFooter>
  </MDBModal>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import {
  MDBTable, MDBBtn, MDBBadge, MDBSpinner, MDBInput, MDBRow, MDBCol,
  MDBTabs, MDBTabNav, MDBTabItem, MDBTabPane, MDBTabContent,
  MDBModal, MDBModalHeader, MDBModalTitle, MDBModalBody, MDBModalFooter,
} from 'mdb-vue-ui-kit'
import type { ConfigInfo } from '../types'

const config = ref<ConfigInfo | null>(null)
const activeTab = ref('envs')
const showAddDb = ref(false)
const showAddBucket = ref(false)
const showAddMailbox = ref(false)
const showAddEnv = ref(false)
const form = reactive<Record<string, string>>({})

const tabs = computed(() => {
  const c = config.value
  return [
    { id: 'envs', label: 'Env Variables', count: c ? Object.keys(c.env_vars).length : 0 },
    { id: 'databases', label: 'Databases', count: c ? Object.keys(c.databases).length : 0 },
    { id: 'buckets', label: 'DFS Buckets', count: c ? Object.keys(c.buckets).length : 0 },
    { id: 'mailboxes', label: 'Mailboxes', count: c ? c.mailboxes.length : 0 },
  ]
})

async function fetchConfig() {
  const resp = await fetch('/api/config')
  config.value = await resp.json()
}

async function addResource(resource: string, data: Record<string, string>) {
  await fetch(`/api/config/${resource}/add`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  Object.keys(form).forEach(k => delete form[k])
  await fetchConfig()
}

async function removeResource(resource: string, data: Record<string, string>) {
  await fetch(`/api/config/${resource}/remove`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  await fetchConfig()
}

onMounted(fetchConfig)
</script>
