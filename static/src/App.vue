<template>
  <MDBNavbar expand="lg" bg="dark" classNavbar="navbar-dark" container>
    <MDBNavbarBrand href="/">qdevrunner</MDBNavbarBrand>
    <MDBNavbarToggler target="#navbarNav" @click="collapse = !collapse" />
    <MDBCollapse v-model="collapse" id="navbarNav">
      <MDBNavbarNav>
        <MDBNavbarItem>
          <router-link class="nav-link" to="/">Services</router-link>
        </MDBNavbarItem>
        <MDBNavbarItem>
          <router-link class="nav-link" to="/resources">Resources</router-link>
        </MDBNavbarItem>
        <MDBNavbarItem>
          <router-link class="nav-link" to="/traces">Traces</router-link>
        </MDBNavbarItem>
        <MDBNavbarItem>
          <router-link class="nav-link" to="/metrics">Metrics</router-link>
        </MDBNavbarItem>
        <MDBNavbarItem>
          <router-link class="nav-link" to="/logs">Logs</router-link>
        </MDBNavbarItem>
        <MDBNavbarItem>
          <router-link class="nav-link" to="/mcp-setup">MCP Setup</router-link>
        </MDBNavbarItem>
      </MDBNavbarNav>
      <span class="navbar-text">
        <MDBBadge :color="connected ? 'success' : 'danger'" pill>
          {{ connected ? 'Connected' : 'Disconnected' }}
        </MDBBadge>
      </span>
    </MDBCollapse>
  </MDBNavbar>
  <div class="container-fluid mt-3">
    <router-view />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import {
  MDBNavbar, MDBNavbarBrand, MDBNavbarToggler, MDBCollapse,
  MDBNavbarNav, MDBNavbarItem, MDBBadge
} from 'mdb-vue-ui-kit'
import { useWebSocket } from './composables/useWebSocket'

const collapse = ref(false)
const { connected } = useWebSocket()
</script>
