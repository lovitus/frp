<template>
  <div class="gateway-page">
    <div class="page-header">
      <div class="title-section">
        <h1 class="page-title">Gateway Tunnels</h1>
        <p class="page-subtitle">
          Create runtime TCP or UDP listeners on frps and route them through an
          opted-in client.
        </p>
      </div>
      <div class="actions-section">
        <ActionButton variant="outline" size="small" @click="fetchData(true)">
          Refresh
        </ActionButton>
        <ActionButton
          variant="outline"
          size="small"
          :disabled="eligibleClients.length === 0"
          @click="openCreateDialog"
        >
          Create Tunnel
        </ActionButton>
      </div>
    </div>

    <div v-if="eligibleClients.length === 0" class="warning-banner">
      No eligible gateway clients. Gateway tunnels currently require a stable
      `clientID` plus `allowGatewayTunnels` or `mixAllowGateway` on frpc.
    </div>

    <div class="stats-grid">
      <div class="stat-card">
        <span class="stat-label">Gateway Clients</span>
        <span class="stat-value">
          {{ gatewayOnlineCount }}/{{ gatewayRegisteredCount }}
          <span class="stat-inline-meta">online / registered</span>
        </span>
        <ActionButton
          variant="outline"
          size="small"
          class="snapshot-toggle"
          @click="gatewayListExpanded = !gatewayListExpanded"
        >
          {{ gatewayListExpanded ? 'Hide Gateway List' : 'Show Gateway List' }}
        </ActionButton>
      </div>
      <div class="stat-card">
        <span class="stat-label">Total</span>
        <span class="stat-value">{{ tunnels.length }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">Online</span>
        <span class="stat-value">{{ onlineCount }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">Need Attention</span>
        <span class="stat-value">{{ attentionCount }}</span>
      </div>
    </div>

    <div v-if="gatewayListExpanded" class="gateway-snapshot">
      <div class="snapshot-title">Gateway Nodes Snapshot (loaded once on page entry)</div>
      <div v-if="gatewaySnapshotClients.length === 0" class="snapshot-empty">
        No gateway clients found in this snapshot.
      </div>
      <div v-else class="snapshot-list">
        <div
          v-for="client in gatewaySnapshotClients"
          :key="client.key"
          class="snapshot-item"
        >
          <div class="snapshot-head">
            <span class="snapshot-name">{{ client.displayName }}</span>
            <el-tag size="small" :type="client.online ? 'success' : 'info'">
              {{ client.online ? 'online' : 'offline' }}
            </el-tag>
          </div>
          <div v-if="buildClientSubLabel(client)" class="snapshot-meta">
            {{ buildClientSubLabel(client) }}
          </div>
          <div v-if="buildClientMetaLine(client)" class="snapshot-meta">
            {{ buildClientMetaLine(client) }}
          </div>
          <div class="snapshot-meta">key {{ client.key }}</div>
        </div>
      </div>
    </div>

    <div class="filter-row">
      <el-input
        v-model="searchText"
        placeholder="Search gateway tunnels..."
        clearable
        class="search-input"
      />
      <el-select v-model="statusFilter" class="status-select">
        <el-option label="All Statuses" value="all" />
        <el-option
          v-for="item in statusOptions"
          :key="item.value"
          :label="item.label"
          :value="item.value"
        />
      </el-select>
    </div>

    <div v-loading="loading" class="table-wrapper">
      <div v-if="filteredTunnels.length > 0" class="tunnels-list">
        <article
          v-for="row in filteredTunnels"
          :key="row.id"
          class="tunnel-card"
        >
          <div class="tunnel-card-header">
            <div class="tunnel-headline">
              <div class="tunnel-title-row">
                <div class="tunnel-name">{{ row.name }}</div>
                <div class="tunnel-tags">
                  <el-tag size="small">{{ row.protocol.toUpperCase() }}</el-tag>
                  <el-tag size="small" :type="getStatusMeta(row.status).type">
                    {{ getStatusMeta(row.status).label }}
                  </el-tag>
                </div>
              </div>
              <div v-if="row.remark" class="tunnel-remark">{{ row.remark }}</div>
            </div>

            <div class="row-actions">
              <ActionButton
                variant="outline"
                size="small"
                @click="openEditDialog(row)"
              >
                Edit
              </ActionButton>
              <ActionButton
                variant="outline"
                size="small"
                danger
                @click="confirmDelete(row)"
              >
                Delete
              </ActionButton>
            </div>
          </div>

          <div class="tunnel-card-grid">
            <section class="detail-panel">
              <div class="detail-label">Listen</div>
              <code class="detail-code">{{ row.bindAddr }}:{{ row.listenPort }}</code>
              <div class="detail-meta">
                Public entrypoint on frps
              </div>
            </section>

            <section class="detail-panel">
              <div class="detail-label">Gateway</div>
              <div class="gateway-client-head">
                <span class="gateway-client-name">
                  {{ getClientLabel(row.clientKey) }}
                </span>
                <el-tag
                  size="small"
                  :type="getClientOnline(row.clientKey) ? 'success' : 'info'"
                >
                  {{ getClientOnline(row.clientKey) ? 'online' : 'offline' }}
                </el-tag>
              </div>
              <div v-if="getClientSubLabel(row.clientKey)" class="detail-meta">
                {{ getClientSubLabel(row.clientKey) }}
              </div>
              <div v-if="getClientMetaLine(row.clientKey)" class="detail-meta">
                {{ getClientMetaLine(row.clientKey) }}
              </div>
              <div class="detail-meta">key {{ row.clientKey }}</div>
            </section>

            <section class="detail-panel">
              <div class="detail-label">Target</div>
              <code class="detail-code">{{ row.targetHost }}:{{ row.targetPort }}</code>
              <div class="detail-meta">
                Local endpoint on the gateway client
              </div>
            </section>

            <section class="detail-panel detail-panel-status">
              <div class="detail-label">Status</div>
              <div v-if="row.remoteAddr" class="status-detail">
                remote {{ row.remoteAddr }}
              </div>
              <div v-if="row.message" class="status-message">
                {{ row.message }}
              </div>
              <div v-else class="detail-meta">No recent status message</div>
            </section>
          </div>

          <div class="tunnel-card-footer">
            <span class="footer-label">Updated</span>
            <span class="footer-value">{{ formatUpdatedAt(row.updatedAt) }}</span>
          </div>
        </article>
      </div>

      <div v-else-if="!loading" class="empty-state">
        <el-empty description="No gateway tunnels found" />
      </div>
    </div>

    <BaseDialog
      v-model="dialogVisible"
      :title="editingTunnel ? 'Edit Gateway Tunnel' : 'Create Gateway Tunnel'"
      width="720px"
      :close-on-click-modal="false"
      :append-to-body="true"
      :is-mobile="isMobile"
    >
      <el-form
        ref="formRef"
        :model="formState"
        :rules="formRules"
        label-position="top"
        class="gateway-form"
      >
        <div class="form-grid">
          <el-form-item label="Name" prop="name">
            <el-input v-model="formState.name" maxlength="64" />
          </el-form-item>

          <el-form-item label="Protocol" prop="protocol">
            <el-select v-model="formState.protocol">
              <el-option label="TCP" value="tcp" />
              <el-option label="UDP" value="udp" />
            </el-select>
          </el-form-item>

          <el-form-item label="Bind Address" prop="bindAddr">
            <el-input v-model="formState.bindAddr" placeholder="0.0.0.0" />
          </el-form-item>

          <el-form-item label="Listen Port" prop="listenPort">
            <el-input-number
              v-model="formState.listenPort"
              :min="1"
              :max="65535"
              controls-position="right"
              class="full-width"
            />
          </el-form-item>

          <el-form-item label="Gateway Client" prop="clientKey" class="wide">
            <el-select
              v-model="formState.clientKey"
              filterable
              placeholder="Select a gateway client"
              class="gateway-client-select"
            >
              <el-option
                v-for="client in eligibleClients"
                :key="client.key"
                :label="formatClientOption(client)"
                :value="client.key"
              >
                <div class="client-option">
                  <div class="gateway-client-head">
                    <span class="gateway-client-name">
                      {{ client.displayName }}
                    </span>
                    <el-tag size="small" :type="client.online ? 'success' : 'info'">
                      {{ client.online ? 'online' : 'offline' }}
                    </el-tag>
                  </div>
                  <div v-if="buildClientSubLabel(client)" class="client-option-subtitle">
                    {{ buildClientSubLabel(client) }}
                  </div>
                  <div v-if="buildClientMetaLine(client)" class="client-option-subtitle">
                    {{ buildClientMetaLine(client) }}
                  </div>
                  <div class="client-option-subtitle">key {{ client.key }}</div>
                </div>
              </el-option>
            </el-select>
          </el-form-item>

          <el-form-item label="Remark" prop="remark" class="wide">
            <el-input v-model="formState.remark" maxlength="256" />
          </el-form-item>

          <el-form-item label="Target Host" prop="targetHost">
            <el-input v-model="formState.targetHost" placeholder="127.0.0.1" />
          </el-form-item>

          <el-form-item label="Target Port" prop="targetPort">
            <el-input-number
              v-model="formState.targetPort"
              :min="1"
              :max="65535"
              controls-position="right"
              class="full-width"
            />
          </el-form-item>
        </div>
      </el-form>

      <template #footer>
        <div class="dialog-footer">
          <ActionButton variant="outline" @click="dialogVisible = false">
            Cancel
          </ActionButton>
          <ActionButton :loading="saving" @click="submitForm">
            {{ editingTunnel ? 'Save Changes' : 'Create Tunnel' }}
          </ActionButton>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      v-model="deleteDialogVisible"
      title="Delete Gateway Tunnel"
      :message="
        pendingDelete
          ? `Delete gateway tunnel '${pendingDelete.name}'?`
          : 'Delete gateway tunnel?'
      "
      confirm-text="Delete"
      danger
      :loading="deleting"
      :is-mobile="isMobile"
      @confirm="deleteTunnel"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import ActionButton from '@shared/components/ActionButton.vue'
import BaseDialog from '@shared/components/BaseDialog.vue'
import ConfirmDialog from '@shared/components/ConfirmDialog.vue'
import { useResponsive } from '../composables/useResponsive'
import { getClients } from '../api/client'
import {
  createGatewayTunnel,
  deleteGatewayTunnel,
  getGatewayTunnels,
  updateGatewayTunnel,
} from '../api/gateway'
import type { GatewayProtocol, GatewayTunnelData } from '../types/gateway'
import type { ClientInfoData } from '../types/client'
import { Client } from '../utils/client'
import { formatDistanceToNow } from '../utils/format'

const { isMobile } = useResponsive()

const clients = ref<Client[]>([])
const tunnels = ref<GatewayTunnelData[]>([])
const gatewayRegisteredCount = ref(0)
const gatewayOnlineCount = ref(0)
const gatewaySnapshotClients = ref<Client[]>([])
const gatewayListExpanded = ref(false)
const loading = ref(false)
const saving = ref(false)
const deleting = ref(false)
const searchText = ref('')
const statusFilter = ref('all')
const dialogVisible = ref(false)
const deleteDialogVisible = ref(false)
const editingTunnel = ref<GatewayTunnelData | null>(null)
const pendingDelete = ref<GatewayTunnelData | null>(null)
const formRef = ref<FormInstance>()

const formState = reactive({
  name: '',
  remark: '',
  protocol: 'tcp' as GatewayProtocol,
  bindAddr: '0.0.0.0',
  listenPort: 0,
  clientKey: '',
  targetHost: '127.0.0.1',
  targetPort: 0,
})

const formRules: FormRules<typeof formState> = {
  name: [{ required: true, message: 'Name is required', trigger: 'blur' }],
  protocol: [
    { required: true, message: 'Protocol is required', trigger: 'change' },
  ],
  bindAddr: [
    { required: true, message: 'Bind address is required', trigger: 'blur' },
  ],
  listenPort: [
    { required: true, message: 'Listen port is required', trigger: 'change' },
  ],
  clientKey: [
    { required: true, message: 'Gateway client is required', trigger: 'change' },
  ],
  targetHost: [
    { required: true, message: 'Target host is required', trigger: 'blur' },
  ],
  targetPort: [
    { required: true, message: 'Target port is required', trigger: 'change' },
  ],
}

const statusOptions = [
  { value: 'online', label: 'Online' },
  { value: 'pending', label: 'Pending' },
  { value: 'client-offline', label: 'Client Offline' },
  { value: 'disabled', label: 'Disabled' },
  { value: 'register-failed', label: 'Register Failed' },
  { value: 'target-invalid', label: 'Target Invalid' },
  { value: 'target-unreachable', label: 'Target Unreachable' },
  { value: 'invalid-config', label: 'Invalid Config' },
  { value: 'apply-failed', label: 'Apply Failed' },
]

const clientMap = computed<Record<string, Client>>(() =>
  Object.fromEntries(clients.value.map((client) => [client.key, client])),
)

const eligibleClients = computed(() =>
  [...clients.value]
    .filter((client) => client.allowGatewayTunnels && client.hasStableClientID)
    .sort((a, b) => a.displayName.localeCompare(b.displayName)),
)

const buildClientSubLabel = (client: Client) => {
  return [client.hostname, client.ip].filter(Boolean).join(' • ')
}

const buildClientMetaLine = (client: Client) => {
  const parts = []
  if (client.version) {
    parts.push(`v${client.version}`)
  }
  if (client.selectedProtocol) {
    parts.push(client.selectedProtocol)
  }
  return parts.join(' • ')
}

const filteredTunnels = computed(() => {
  const query = searchText.value.trim().toLowerCase()

  return [...tunnels.value]
    .filter((tunnel) => {
      if (statusFilter.value !== 'all' && tunnel.status !== statusFilter.value) {
        return false
      }
      if (!query) {
        return true
      }
      const clientLabel = [
        getClientLabel(tunnel.clientKey),
        getClientSubLabel(tunnel.clientKey),
        getClientMetaLine(tunnel.clientKey),
        tunnel.clientKey,
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
      return [
        tunnel.name,
        tunnel.remark || '',
        tunnel.protocol,
        tunnel.bindAddr,
        String(tunnel.listenPort),
        tunnel.targetHost,
        String(tunnel.targetPort),
        tunnel.status,
        tunnel.message || '',
        clientLabel,
      ].some((value) => value.toLowerCase().includes(query))
    })
    .sort((a, b) => a.name.localeCompare(b.name))
})

const onlineCount = computed(
  () => tunnels.value.filter((tunnel) => tunnel.status === 'online').length,
)

const attentionCount = computed(
  () =>
    tunnels.value.filter(
      (tunnel) => tunnel.status !== 'online' && tunnel.status !== 'pending',
    ).length,
)

const getClientLabel = (clientKey: string) => {
  const client = clientMap.value[clientKey]
  return client ? client.displayName : clientKey
}

const getClientSubLabel = (clientKey: string) => {
  const client = clientMap.value[clientKey]
  return client ? buildClientSubLabel(client) : ''
}

const getClientMetaLine = (clientKey: string) => {
  const client = clientMap.value[clientKey]
  return client ? buildClientMetaLine(client) : ''
}

const getClientOnline = (clientKey: string) => {
  return clientMap.value[clientKey]?.online ?? false
}

const formatClientOption = (client: Client) => {
  return [
    client.displayName,
    buildClientSubLabel(client),
    buildClientMetaLine(client),
    client.key,
    client.online ? 'online' : 'offline',
  ]
    .filter(Boolean)
    .join(' ')
}

const getStatusMeta = (status: string) => {
  switch (status) {
    case 'online':
      return { label: 'Online', type: 'success' as const }
    case 'pending':
      return { label: 'Pending', type: 'warning' as const }
    case 'client-offline':
      return { label: 'Client Offline', type: 'info' as const }
    case 'disabled':
      return { label: 'Disabled', type: 'info' as const }
    case 'register-failed':
      return { label: 'Register Failed', type: 'danger' as const }
    case 'target-invalid':
      return { label: 'Target Invalid', type: 'danger' as const }
    case 'target-unreachable':
      return { label: 'Target Unreachable', type: 'danger' as const }
    case 'invalid-config':
      return { label: 'Invalid Config', type: 'danger' as const }
    case 'apply-failed':
      return { label: 'Apply Failed', type: 'danger' as const }
    default:
      return { label: status || 'Unknown', type: 'info' as const }
  }
}

const formatUpdatedAt = (value: string) => {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return formatDistanceToNow(date)
}

const resetForm = () => {
  formState.name = ''
  formState.remark = ''
  formState.protocol = 'tcp'
  formState.bindAddr = '0.0.0.0'
  formState.listenPort = 0
  formState.clientKey = eligibleClients.value[0]?.key || ''
  formState.targetHost = '127.0.0.1'
  formState.targetPort = 0
}

const populateForm = (tunnel: GatewayTunnelData) => {
  formState.name = tunnel.name
  formState.remark = tunnel.remark || ''
  formState.protocol = tunnel.protocol
  formState.bindAddr = tunnel.bindAddr
  formState.listenPort = tunnel.listenPort
  formState.clientKey = tunnel.clientKey
  formState.targetHost = tunnel.targetHost
  formState.targetPort = tunnel.targetPort
}

const refreshGatewayClientSnapshot = (list: Client[]) => {
  const gateways = list
    .filter((client) => client.allowGatewayTunnels && client.hasStableClientID)
    .sort((a, b) => a.displayName.localeCompare(b.displayName))
  gatewayRegisteredCount.value = gateways.length
  gatewayOnlineCount.value = gateways.filter((client) => client.online).length
  gatewaySnapshotClients.value = gateways
}

const fetchClients = async (refreshSnapshot = false) => {
  const payload = await getClients()
  clients.value = payload.map((item: ClientInfoData) => new Client(item))
  if (refreshSnapshot) {
    refreshGatewayClientSnapshot(clients.value)
  }
}

const fetchTunnels = async () => {
  tunnels.value = await getGatewayTunnels(true)
}

const fetchData = async (refreshSnapshot = false) => {
  loading.value = true
  try {
    if (refreshSnapshot) {
      await Promise.all([fetchClients(true), fetchTunnels()])
    } else {
      await fetchTunnels()
    }
  } catch (error: any) {
    ElMessage({
      type: 'error',
      showClose: true,
      message: 'Failed to fetch gateway tunnels: ' + error.message,
    })
  } finally {
    loading.value = false
  }
}

const loadPageSnapshot = async () => {
  loading.value = true
  try {
    await Promise.all([fetchClients(true), fetchTunnels()])
  } catch (error: any) {
    ElMessage({
      type: 'error',
      showClose: true,
      message: 'Failed to fetch gateway tunnels: ' + error.message,
    })
  } finally {
    loading.value = false
  }
}

const openCreateDialog = () => {
  editingTunnel.value = null
  resetForm()
  dialogVisible.value = true
}

const openEditDialog = (tunnel: GatewayTunnelData) => {
  editingTunnel.value = tunnel
  populateForm(tunnel)
  dialogVisible.value = true
}

const submitForm = async () => {
  if (!formRef.value) return

  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  try {
    const payload = {
      name: formState.name.trim(),
      remark: formState.remark.trim(),
      protocol: formState.protocol,
      bindAddr: formState.bindAddr.trim(),
      listenPort: formState.listenPort,
      clientKey: formState.clientKey,
      targetHost: formState.targetHost.trim(),
      targetPort: formState.targetPort,
    }

    if (editingTunnel.value) {
      await updateGatewayTunnel(editingTunnel.value.id, payload)
      ElMessage({ type: 'success', message: 'Gateway tunnel updated' })
    } else {
      await createGatewayTunnel(payload)
      ElMessage({ type: 'success', message: 'Gateway tunnel created' })
    }

    dialogVisible.value = false
    await fetchData()
  } catch (error: any) {
    ElMessage({
      type: 'error',
      showClose: true,
      message: 'Failed to save gateway tunnel: ' + error.message,
    })
  } finally {
    saving.value = false
  }
}

const confirmDelete = (tunnel: GatewayTunnelData) => {
  pendingDelete.value = tunnel
  deleteDialogVisible.value = true
}

const deleteTunnel = async () => {
  if (!pendingDelete.value) return

  deleting.value = true
  try {
    await deleteGatewayTunnel(pendingDelete.value.id)
    ElMessage({ type: 'success', message: 'Gateway tunnel deleted' })
    deleteDialogVisible.value = false
    pendingDelete.value = null
    await fetchData()
  } catch (error: any) {
    ElMessage({
      type: 'error',
      showClose: true,
      message: 'Failed to delete gateway tunnel: ' + error.message,
    })
  } finally {
    deleting.value = false
  }
}

onMounted(() => {
  loadPageSnapshot()
})
</script>

<style scoped lang="scss">
.gateway-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.page-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.title-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.page-title {
  margin: 0;
  font-size: 28px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.page-subtitle {
  margin: 0;
  max-width: 720px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
}

.actions-section {
  display: flex;
  gap: 10px;
}

.warning-banner {
  border: 1px solid rgba(245, 158, 11, 0.28);
  background: rgba(245, 158, 11, 0.1);
  color: var(--el-text-color-primary);
  padding: 14px 16px;
  border-radius: 14px;
  line-height: 1.5;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.stat-card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: 16px;
  padding: 18px 20px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.stat-label {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.stat-value {
  font-size: 28px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.stat-inline-meta {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  font-weight: 500;
  margin-left: 8px;
}

.snapshot-toggle {
  margin-top: 2px;
  align-self: flex-start;
}

.gateway-snapshot {
  border: 1px solid var(--el-border-color-light);
  border-radius: 16px;
  background: var(--el-bg-color);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.snapshot-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.snapshot-empty {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.snapshot-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.snapshot-item {
  border: 1px solid var(--el-border-color-extra-light);
  border-radius: 12px;
  background: var(--el-fill-color-light);
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.snapshot-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.snapshot-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.snapshot-meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  word-break: break-word;
}

.filter-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.search-input {
  flex: 1;
  min-width: 240px;
}

.status-select {
  width: 220px;
}

.table-wrapper {
  min-height: 220px;
}

.tunnels-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.tunnel-card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: 18px;
  overflow: hidden;
}

.tunnel-card-header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 20px 0;
  flex-wrap: wrap;
}

.tunnel-headline {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

.tunnel-title-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.tunnel-tags {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.tunnel-card-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
  padding: 18px 20px;
}

.detail-panel {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
  padding: 14px 16px;
  border-radius: 14px;
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-extra-light);
}

.detail-panel-status {
  justify-content: center;
}

.detail-label {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--el-text-color-secondary);
}

.detail-code {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  font-size: 13px;
}

.gateway-client-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.gateway-client-name {
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.tunnel-name {
  font-weight: 600;
  font-size: 16px;
  color: var(--el-text-color-primary);
}

.tunnel-remark,
.detail-meta,
.status-detail,
.status-message {
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.4;
}

.status-message {
  word-break: break-word;
}

.tunnel-card-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 0 20px 18px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.footer-label {
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.footer-value {
  color: var(--el-text-color-primary);
  font-weight: 500;
}

.row-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.gateway-form {
  padding-top: 4px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 16px;
}

.wide {
  grid-column: 1 / -1;
}

.full-width {
  width: 100%;
}

.client-option {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 4px 0;
}

.client-option-subtitle {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.4;
  word-break: break-word;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.empty-state {
  padding: 40px 0;
}

code {
  font-family:
    ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, Liberation Mono,
    monospace;
  font-size: 12px;
}

@media (max-width: 900px) {
  .stats-grid {
    grid-template-columns: 1fr;
  }

  .tunnel-card-grid,
  .snapshot-list,
  .form-grid {
    grid-template-columns: 1fr;
  }

  .actions-section,
  .filter-row,
  .row-actions {
    width: 100%;
  }

  .actions-section > * {
    flex: 1;
  }

  .tunnel-card-header {
    padding-top: 16px;
  }

  .tunnel-card-footer {
    justify-content: flex-start;
  }
}
</style>
