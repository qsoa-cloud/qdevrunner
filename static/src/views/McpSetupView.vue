<template>
  <h4 class="mb-3">MCP Setup</h4>

  <p class="text-muted mb-4">
    qdevrunner exposes an
    <a href="https://modelcontextprotocol.io" target="_blank">MCP</a>
    (Model Context Protocol) server that gives AI assistants access to your services,
    traces, logs, metrics, and configuration. Two transports are supported: stdio (recommended
    for Claude Code) and Streamable HTTP.
  </p>

  <MDBCard class="mb-4">
    <MDBCardBody>
      <MDBCardTitle>MCP Server URL</MDBCardTitle>
      <div class="d-flex align-items-center gap-2">
        <code class="fs-5">{{ mcpUrl }}</code>
        <MDBBtn outline="primary" size="sm" @click="copy(mcpUrl)">
          {{ copied === mcpUrl ? 'Copied!' : 'Copy' }}
        </MDBBtn>
      </div>
    </MDBCardBody>
  </MDBCard>

  <MDBTabs v-model="activeTab">
    <MDBTabNav tabsClasses="mb-3">
      <MDBTabItem tabId="general">General JSON</MDBTabItem>
      <MDBTabItem tabId="claude-code">Claude Code</MDBTabItem>
      <MDBTabItem tabId="claude-desktop">Claude Desktop</MDBTabItem>
      <MDBTabItem tabId="cursor">Cursor</MDBTabItem>
      <MDBTabItem tabId="codex">Codex CLI</MDBTabItem>
    </MDBTabNav>
    <MDBTabContent>
      <!-- General JSON -->
      <MDBTabPane tabId="general">
        <h5>Add</h5>
        <p>Use this configuration for any MCP-compatible tool:</p>
        <div class="position-relative">
          <pre class="bg-dark text-light p-3 rounded"><code>{{ generalJson }}</code></pre>
          <MDBBtn outline="light" size="sm" class="position-absolute top-0 end-0 m-2" @click="copy(generalJson)">
            {{ copied === generalJson ? 'Copied!' : 'Copy' }}
          </MDBBtn>
        </div>
        <p class="text-muted small mt-2">
          The server uses Streamable HTTP transport. It provides tools for service management,
          observability (traces, logs, metrics), and configuration, plus resources with a usage guide
          and live config/status.
        </p>

        <h5 class="mt-4">Remove</h5>
        <p>Remove the <code>qdevrunner</code> entry from your client's MCP server configuration and restart the client.</p>
      </MDBTabPane>

      <!-- Claude Code -->
      <MDBTabPane tabId="claude-code">
        <h5>Stdio (recommended)</h5>
        <p>Claude Code starts qdevrunner automatically. One-time global setup:</p>
        <div class="position-relative">
          <pre class="bg-dark text-light p-3 rounded"><code>{{ claudeCodeStdioCmd }}</code></pre>
          <MDBBtn outline="light" size="sm" class="position-absolute top-0 end-0 m-2" @click="copy(claudeCodeStdioCmd)">
            {{ copied === claudeCodeStdioCmd ? 'Copied!' : 'Copy' }}
          </MDBBtn>
        </div>
        <p class="text-muted small mt-2">The Web UI is also available at <code>http://127.0.0.1:8090</code> while the MCP server is running.</p>

        <h5 class="mt-4">HTTP (alternative)</h5>
        <p>If qdevrunner is already running, connect via HTTP:</p>
        <div class="position-relative">
          <pre class="bg-dark text-light p-3 rounded"><code>{{ claudeCodeAddCmd }}</code></pre>
          <MDBBtn outline="light" size="sm" class="position-absolute top-0 end-0 m-2" @click="copy(claudeCodeAddCmd)">
            {{ copied === claudeCodeAddCmd ? 'Copied!' : 'Copy' }}
          </MDBBtn>
        </div>

        <h5 class="mt-4">Remove</h5>
        <div class="position-relative">
          <pre class="bg-dark text-light p-3 rounded"><code>{{ claudeCodeRemoveCmd }}</code></pre>
          <MDBBtn outline="light" size="sm" class="position-absolute top-0 end-0 m-2" @click="copy(claudeCodeRemoveCmd)">
            {{ copied === claudeCodeRemoveCmd ? 'Copied!' : 'Copy' }}
          </MDBBtn>
        </div>
      </MDBTabPane>

      <!-- Claude Desktop -->
      <MDBTabPane tabId="claude-desktop">
        <h5>Add</h5>
        <p>Add to your Claude Desktop config file:</p>
        <ul class="text-muted small mb-3">
          <li><strong>macOS:</strong> <code>~/Library/Application Support/Claude/claude_desktop_config.json</code></li>
          <li><strong>Windows:</strong> <code>%APPDATA%\Claude\claude_desktop_config.json</code></li>
        </ul>
        <div class="position-relative">
          <pre class="bg-dark text-light p-3 rounded"><code>{{ claudeDesktopJson }}</code></pre>
          <MDBBtn outline="light" size="sm" class="position-absolute top-0 end-0 m-2" @click="copy(claudeDesktopJson)">
            {{ copied === claudeDesktopJson ? 'Copied!' : 'Copy' }}
          </MDBBtn>
        </div>

        <h5 class="mt-4">Remove</h5>
        <p>Remove the <code>qdevrunner</code> key from <code>mcpServers</code> in the config file above, then restart Claude Desktop.</p>
      </MDBTabPane>

      <!-- Cursor -->
      <MDBTabPane tabId="cursor">
        <h5>Add</h5>
        <p>Add to <code>.cursor/mcp.json</code> in your project directory (or <code>~/.cursor/mcp.json</code> globally):</p>
        <div class="position-relative">
          <pre class="bg-dark text-light p-3 rounded"><code>{{ cursorJson }}</code></pre>
          <MDBBtn outline="light" size="sm" class="position-absolute top-0 end-0 m-2" @click="copy(cursorJson)">
            {{ copied === cursorJson ? 'Copied!' : 'Copy' }}
          </MDBBtn>
        </div>

        <h5 class="mt-4">Remove</h5>
        <p>Remove the <code>qdevrunner</code> key from <code>mcpServers</code> in <code>.cursor/mcp.json</code> and restart Cursor.</p>
      </MDBTabPane>

      <!-- Codex CLI -->
      <MDBTabPane tabId="codex">
        <h5>Add</h5>
        <p>Run this command:</p>
        <div class="position-relative">
          <pre class="bg-dark text-light p-3 rounded"><code>{{ codexAddCmd }}</code></pre>
          <MDBBtn outline="light" size="sm" class="position-absolute top-0 end-0 m-2" @click="copy(codexAddCmd)">
            {{ copied === codexAddCmd ? 'Copied!' : 'Copy' }}
          </MDBBtn>
        </div>

        <p class="mt-3">Or add to <code>~/.codex/config.yaml</code> manually:</p>
        <div class="position-relative">
          <pre class="bg-dark text-light p-3 rounded"><code>{{ codexYaml }}</code></pre>
          <MDBBtn outline="light" size="sm" class="position-absolute top-0 end-0 m-2" @click="copy(codexYaml)">
            {{ copied === codexYaml ? 'Copied!' : 'Copy' }}
          </MDBBtn>
        </div>

        <h5 class="mt-4">Remove</h5>
        <p>Remove the <code>qdevrunner</code> entry from the <code>mcp_servers</code> list in <code>~/.codex/config.yaml</code>.</p>
      </MDBTabPane>
    </MDBTabContent>
  </MDBTabs>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  MDBCard, MDBCardBody, MDBCardTitle, MDBBtn,
  MDBTabs, MDBTabNav, MDBTabItem, MDBTabPane, MDBTabContent,
} from 'mdb-vue-ui-kit'

const activeTab = ref('general')

const mcpUrl = computed(() => window.location.origin + '/mcp')

const generalJson = computed(() => JSON.stringify({
  type: "streamable-http",
  url: mcpUrl.value,
}, null, 2))

const claudeCodeStdioCmd = 'claude mcp add -s user qdevrunner -- qdevrunner --stdio'

const claudeCodeAddCmd = computed(() =>
  `claude mcp add qdevrunner --transport http ${mcpUrl.value}`
)

const claudeCodeRemoveCmd = 'claude mcp remove qdevrunner'

const claudeDesktopJson = computed(() => JSON.stringify({
  mcpServers: {
    qdevrunner: {
      type: "streamable-http",
      url: mcpUrl.value,
    }
  }
}, null, 2))

const cursorJson = computed(() => JSON.stringify({
  mcpServers: {
    qdevrunner: {
      type: "streamable-http",
      url: mcpUrl.value,
    }
  }
}, null, 2))

const codexAddCmd = computed(() =>
  `codex mcp add --name qdevrunner --type http --url ${mcpUrl.value}`
)

const codexYaml = computed(() =>
  `mcp_servers:\n  - name: qdevrunner\n    type: http\n    url: ${mcpUrl.value}`
)

const copied = ref('')
let timer: ReturnType<typeof setTimeout> | null = null

function copy(text: string) {
  navigator.clipboard.writeText(text)
  copied.value = text
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => { copied.value = '' }, 2000)
}
</script>
