<template>
  <div class="gateway-page">
    <div class="page-header">
      <div class="title-section">
        <div class="title-row">
          <h1 class="page-title">Gateway Tunnels</h1>
          <span class="mem-chip" @click="openExportDialog">⚠ Memory-only · Export before restart</span>
        </div>
        <p class="page-subtitle">
          Create runtime TCP or UDP listeners on frps and route them through an
          opted-in client.
        </p>
      </div>
      <div class="actions-section">
        <ActionButton variant="outline" size="small" @click="fetchData(true)">
          Refresh
        </ActionButton>
        <PopoverMenu :width="180" placement="bottom-end">
          <template #trigger>
            <ActionButton variant="outline" size="small">···</ActionButton>
          </template>
          <PopoverMenuItem @click="openExportDialog">Export YAML</PopoverMenuItem>
          <PopoverMenuItem @click="openImportDialog">Import YAML</PopoverMenuItem>
        </PopoverMenu>
        <ActionButton
          size="small"
          :disabled="eligibleClients.length === 0"
          @click="openCreateDialog"
        >
          + Create Tunnel
        </ActionButton>
      </div>
    </div>

    <div v-if="eligibleClients.length === 0 && !isMockMode" class="warning-banner">
      No eligible gateway clients. Gateway tunnels require a stable
      <code>clientID</code> plus <code>allowGatewayTunnels</code> or <code>mixAllowGateway</code> on frpc.
    </div>

    <div class="stats-grid">
      <div
        class="stat-card stat-card-clickable"
        @click="gatewayListExpanded = !gatewayListExpanded"
      >
        <span class="stat-label">Gateway Nodes</span>
        <span class="stat-value">
          {{ gatewayOnlineCount }}/{{ gatewayRegisteredCount }}
          <span class="stat-inline-meta">online / total</span>
        </span>
        <span class="stat-hint">{{ gatewayListExpanded ? '▲ Hide nodes' : '▼ View nodes' }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">Total</span>
        <span class="stat-value">{{ displayTunnels.length }}</span>
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
            <div class="snapshot-head-main">
              <span class="snapshot-name">{{ client.displayName }}</span>
              <div v-if="buildClientSubLabel(client)" class="snapshot-subline">
                {{ buildClientSubLabel(client) }}
              </div>
            </div>
            <div class="snapshot-head-actions">
              <el-tag size="small" :type="client.online ? 'success' : 'info'">
                {{ client.online ? 'online' : 'offline' }}
              </el-tag>
              <ActionButton
                variant="outline"
                size="small"
                @click="openSystemInfoDialog(client)"
              >
                More
              </ActionButton>
            </div>
          </div>
          <div
            v-if="[buildClientMetaLine(client), client.platformLabel, client.poolCount ? `pool ${client.poolCount}` : '']
              .filter(Boolean)
              .join(' • ')"
            class="snapshot-meta"
          >
            {{
              [buildClientMetaLine(client), client.platformLabel, client.poolCount ? `pool ${client.poolCount}` : '']
                .filter(Boolean)
                .join(' • ')
            }}
          </div>
          <div class="snapshot-time-line">
            <span v-if="client.loginTimestamp" class="snapshot-time-pill">
              <span class="snapshot-time-label">Login</span>
              <span class="snapshot-time-value">{{ formatSnapshotAgo(client.loginTimestamp) }}</span>
            </span>
            <span class="snapshot-time-pill">
              <span class="snapshot-time-label">First</span>
              <span class="snapshot-time-value">{{ formatSnapshotAgo(client.firstConnectedAt) }}</span>
            </span>
            <span class="snapshot-time-pill">
              <span class="snapshot-time-label">Last</span>
              <span class="snapshot-time-value">{{ formatSnapshotAgo(client.lastConnectedAt) }}</span>
            </span>
            <span v-if="client.disconnectedAt" class="snapshot-time-pill">
              <span class="snapshot-time-label">Offline</span>
              <span class="snapshot-time-value">{{ formatSnapshotAgo(client.disconnectedAt) }}</span>
            </span>
          </div>
          <div class="snapshot-identity-line">
            <span>Key {{ client.key }}</span>
            <span v-if="client.runID">Run {{ client.shortRunId }}</span>
            <span v-if="client.metasArray.length > 0">Meta {{ client.metasArray.length }}</span>
          </div>
          <div v-if="client.metasArray.length > 0" class="snapshot-metas">
            <span
              v-for="item in client.metasArray"
              :key="`${client.key}-${item.key}`"
              class="snapshot-chip"
            >
              {{ item.key }}={{ item.value }}
            </span>
          </div>
        </div>
      </div>
    </div>

    <BaseDialog
      v-model="systemInfoDialogVisible"
      title="Gateway System Info"
      width="920px"
      :append-to-body="true"
      :is-mobile="isMobile"
    >
      <div v-loading="systemInfoLoading" class="system-info-dialog">
        <template v-if="systemInfo">
          <section class="system-info-section">
            <div class="system-info-section-title">Summary</div>
            <div class="system-info-grid">
              <div class="system-info-item"><span>Client</span><strong>{{ systemInfo.displayName }}</strong></div>
              <div class="system-info-item"><span>Hostname</span><strong>{{ systemInfo.hostname || '-' }}</strong></div>
              <div class="system-info-item"><span>OS / Arch</span><strong>{{ [systemInfo.os, systemInfo.arch].filter(Boolean).join(' / ') || '-' }}</strong></div>
              <div class="system-info-item"><span>Kernel</span><strong>{{ [systemInfo.platform, systemInfo.platformVersion, systemInfo.kernelVersion].filter(Boolean).join(' / ') || '-' }}</strong></div>
              <div class="system-info-item"><span>frpc</span><strong>{{ [systemInfo.frpcVersion ? `v${systemInfo.frpcVersion}` : '', systemInfo.selectedProtocol].filter(Boolean).join(' • ') || '-' }}</strong></div>
              <div class="system-info-item"><span>User</span><strong>{{ systemInfo.currentUser || '-' }}</strong></div>
              <div class="system-info-item"><span>ClientID</span><strong>{{ systemInfo.clientID || '-' }}</strong></div>
              <div class="system-info-item"><span>RunID</span><strong>{{ systemInfo.runID || '-' }}</strong></div>
              <div class="system-info-item"><span>Source IP</span><strong>{{ systemInfo.observedSourceIP || '-' }}</strong></div>
              <div class="system-info-item"><span>Default Route</span><strong>{{ systemInfo.defaultRouteIP || '-' }}</strong></div>
              <div class="system-info-item"><span>Timezone</span><strong>{{ systemInfo.timezone || '-' }}</strong></div>
              <div class="system-info-item"><span>Uptime</span><strong>{{ formatDurationSeconds(systemInfo.uptimeSeconds) }}</strong></div>
              <div class="system-info-item"><span>Collected</span><strong>{{ formatUnixSnapshot(systemInfo.collectedAt) }}</strong></div>
              <div class="system-info-item"><span>frpc Start</span><strong>{{ formatUnixSnapshot(systemInfo.frpcStartTime) }}</strong></div>
            </div>
          </section>

          <section class="system-info-section">
            <div class="system-info-section-title">Resources</div>
            <div class="system-info-grid">
              <div class="system-info-item"><span>CPU</span><strong>{{ systemInfo.cpuCount || 0 }} cores</strong></div>
              <div class="system-info-item"><span>Load</span><strong>{{ formatLoad(systemInfo.load1, systemInfo.load5, systemInfo.load15) }}</strong></div>
              <div class="system-info-item"><span>Memory</span><strong>{{ formatBytes(systemInfo.memoryUsed) }} / {{ formatBytes(systemInfo.memoryTotal) }}</strong></div>
              <div class="system-info-item"><span>Memory Free</span><strong>{{ formatBytes(systemInfo.memoryAvailable) }}</strong></div>
              <div class="system-info-item"><span>Swap</span><strong>{{ formatBytes(systemInfo.swapUsed) }} / {{ formatBytes(systemInfo.swapTotal) }}</strong></div>
              <div class="system-info-item"><span>Disk</span><strong>{{ formatBytes(systemInfo.diskUsed) }} / {{ formatBytes(systemInfo.diskTotal) }}</strong></div>
              <div class="system-info-item"><span>Disk Path</span><strong>{{ systemInfo.diskPath || '-' }}</strong></div>
              <div class="system-info-item"><span>PID / Goroutines</span><strong>{{ systemInfo.frpcPid || '-' }} / {{ systemInfo.goroutines || 0 }}</strong></div>
            </div>
          </section>

          <section class="system-info-section">
            <div class="system-info-section-title">Gateway</div>
            <div class="system-info-grid">
              <div class="system-info-item"><span>Enabled</span><strong>{{ systemInfo.gateway.enabled ? 'yes' : 'no' }}</strong></div>
              <div class="system-info-item"><span>Tunnels</span><strong>{{ systemInfo.gateway.tunnel_count }}</strong></div>
              <div class="system-info-item"><span>Online</span><strong>{{ systemInfo.gateway.online_count }}</strong></div>
              <div class="system-info-item"><span>Pending</span><strong>{{ systemInfo.gateway.pending_count }}</strong></div>
              <div class="system-info-item"><span>Disabled</span><strong>{{ systemInfo.gateway.disabled_count }}</strong></div>
              <div class="system-info-item"><span>Last Apply Error</span><strong>{{ systemInfo.gateway.last_apply_err || '-' }}</strong></div>
            </div>
          </section>

          <section class="system-info-section">
            <div class="system-info-section-title">Network Interfaces</div>
            <div v-if="systemInfo.interfaces && systemInfo.interfaces.length > 0" class="system-info-interface-list">
              <article
                v-for="item in systemInfo.interfaces"
                :key="item.name"
                class="system-info-interface"
              >
                <div class="system-info-interface-name">{{ item.name }}</div>
                <div class="system-info-interface-meta">{{ (item.flags || []).join(', ') || '-' }}</div>
                <div class="system-info-interface-addresses">
                  {{ (item.addresses || []).join(' • ') || '-' }}
                </div>
              </article>
            </div>
            <div v-else class="system-info-empty">No interfaces reported.</div>
          </section>

          <section class="system-info-section">
            <div class="system-info-section-title">Top Memory Processes</div>
            <div v-if="systemInfo.topMemoryProcs && systemInfo.topMemoryProcs.length > 0" class="system-info-process-list">
              <div class="system-info-process-head">
                <span>PID</span>
                <span>Name</span>
                <span>RSS</span>
                <span>Percent</span>
              </div>
              <div
                v-for="proc in systemInfo.topMemoryProcs"
                :key="`${proc.pid}-${proc.name}`"
                class="system-info-process-row"
              >
                <span>{{ proc.pid }}</span>
                <span>{{ proc.name || '-' }}</span>
                <span>{{ formatBytes(proc.memory_rss) }}</span>
                <span>{{ formatPercent(proc.memory_percent) }}</span>
              </div>
            </div>
            <div v-else class="system-info-empty">No process memory data available.</div>
          </section>

          <section v-if="systemInfo.metas && Object.keys(systemInfo.metas).length > 0" class="system-info-section">
            <div class="system-info-section-title">Metas</div>
            <div class="snapshot-metas">
              <span
                v-for="(value, key) in systemInfo.metas"
                :key="`meta-${key}`"
                class="snapshot-chip"
              >
                {{ key }}={{ value }}
              </span>
            </div>
          </section>
        </template>
        <div v-else-if="!systemInfoLoading" class="system-info-empty">
          No system info loaded.
        </div>
      </div>

      <template #footer>
        <div class="dialog-footer">
          <ActionButton variant="outline" @click="systemInfoDialogVisible = false">
            Close
          </ActionButton>
        </div>
      </template>
    </BaseDialog>

    <div class="filter-bar">
      <div class="status-tabs">
        <button
          v-for="tab in statusTabs"
          :key="tab.value"
          class="status-tab"
          :class="{ active: statusFilter === tab.value }"
          @click="statusFilter = tab.value"
        >
          <span class="tab-label">{{ tab.label }}</span>
          <span class="tab-count">{{ tab.count }}</span>
        </button>
      </div>
      <el-input
        v-model="searchText"
        placeholder="Search tunnels..."
        clearable
        class="search-input"
      />
    </div>

    <div v-if="isMockMode" class="demo-banner">
      <span class="demo-badge">示例数据</span>
      暂无实际 gateway tunnel，以下为样式预览。接入支持 gateway 的 frpc 客户端并创建 tunnel 后自动替换。
    </div>

    <div v-loading="loading" class="table-wrapper">
      <div v-if="filteredTunnels.length > 0" class="tunnels-list">
        <article
          v-for="row in filteredTunnels"
          :key="row.id"
          class="tunnel-card"
          :class="`status-${row.status}`"
        >
          <div class="tunnel-card-header">
            <div class="tunnel-headline">
              <div class="tunnel-title-row">
                <div class="tunnel-name">{{ row.name }}</div>
                <div class="tunnel-tags">
                  <el-tag size="small" :type="getStatusMeta(row.status).type">
                    {{ getStatusMeta(row.status).label }}
                  </el-tag>
                  <el-tag size="small">{{ row.protocol.toUpperCase() }}</el-tag>
                  <el-tag size="small" effect="plain">
                    {{ formatTargetTypeLabel(row.targetType) }}
                  </el-tag>
                </div>
              </div>
              <div v-if="row.remark" class="tunnel-remark">{{ row.remark }}</div>
            </div>

            <div class="tunnel-header-right">
              <div class="row-actions">
                <ActionButton
                  variant="outline"
                  size="small"
                  :disabled="isMockMode"
                  @click="openEditDialog(row)"
                >
                  Edit
                </ActionButton>
                <ActionButton
                  variant="outline"
                  size="small"
                  danger
                  :disabled="isMockMode"
                  @click="confirmDelete(row)"
                >
                  Delete
                </ActionButton>
              </div>
            </div>
          </div>

          <div class="tunnel-card-body">
            <div class="route-flow">
              <section class="route-node route-node-listen">
                <div class="detail-label">Listen</div>
                <code class="detail-code">{{ row.bindAddr }}:{{ row.listenPort }}</code>
                <div class="detail-meta">Public entrypoint on frps</div>
              </section>

              <div class="route-arrow">→</div>

              <section class="route-node route-node-gateway">
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

              <div class="route-arrow">→</div>

              <section class="route-node route-node-target">
                <div class="detail-label">Target</div>
                <code class="detail-code">{{ formatTunnelTarget(row) }}</code>
                <div class="detail-meta">{{ formatTunnelTargetMeta(row) }}</div>
              </section>
            </div>

            <section class="status-panel">
              <div class="status-panel-head">
                <div class="detail-label">Runtime</div>
                <span class="tunnel-updated">{{ formatUpdatedAt(row.updatedAt) }}</span>
              </div>
              <div class="status-detail">{{ formatTunnelValidity(row) }}</div>
              <div v-if="row.remoteAddr" class="status-detail">remote {{ row.remoteAddr }}</div>
              <div v-if="row.message" class="status-message">{{ row.message }}</div>
              <div v-else class="detail-meta">No recent status message</div>
            </section>
          </div>
        </article>
      </div>

      <div v-else-if="!loading" class="empty-state">
        <el-empty description="No gateway tunnels found" />
      </div>
    </div>

    <BaseDialog
      v-model="exportDialogVisible"
      title="Export Gateway Tunnels"
      width="760px"
      :append-to-body="true"
      :is-mobile="isMobile"
    >
      <div class="yaml-dialog">
        <p class="yaml-help">
          Exported as YAML text only. No server-side file paths are involved.
        </p>
        <el-input
          v-model="exportYAML"
          type="textarea"
          :rows="14"
          readonly
          class="yaml-textarea"
        />
      </div>

      <template #footer>
        <div class="dialog-footer">
          <ActionButton variant="outline" @click="copyExportYAML">
            Copy
          </ActionButton>
          <ActionButton variant="outline" @click="downloadExportYAML">
            Download .yaml
          </ActionButton>
          <ActionButton variant="outline" @click="exportDialogVisible = false">
            Close
          </ActionButton>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog
      v-model="importDialogVisible"
      title="Import Gateway Tunnels"
      width="760px"
      :append-to-body="true"
      :is-mobile="isMobile"
    >
      <div class="yaml-dialog">
        <p class="yaml-help">
          Paste YAML or load a local file. Import uses content only and performs
          upsert by `clientKey + name`.
        </p>
        <div class="yaml-file-row">
          <input
            ref="importFileInputRef"
            type="file"
            accept=".yml,.yaml,.txt,text/yaml,text/plain"
            class="yaml-file-input"
            @change="handleImportFileChange"
          />
          <ActionButton variant="outline" @click="openImportFilePicker">
            Choose File
          </ActionButton>
        </div>
        <el-input
          v-model="importYAML"
          type="textarea"
          :rows="14"
          class="yaml-textarea"
          placeholder="version: 1&#10;tunnels:&#10;  - name: ssh-main&#10;    protocol: tcp&#10;    bindAddr: 0.0.0.0&#10;    listenPort: 6000&#10;    clientKey: edge-kr-01&#10;    targetHost: 127.0.0.1&#10;    targetPort: 22"
        />
      </div>

      <template #footer>
        <div class="dialog-footer">
          <ActionButton variant="outline" @click="importDialogVisible = false">
            Cancel
          </ActionButton>
          <ActionButton :loading="importing" @click="submitImportYAML">
            Import
          </ActionButton>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog
      v-model="dialogVisible"
      :title="editingTunnel ? 'Edit Gateway Tunnel' : 'Create Gateway Tunnel'"
      width="780px"
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
        <section class="form-section form-section-entry">
          <div class="form-section-head">
            <span class="form-section-kicker">1</span>
            <div class="form-section-title">
              Entry Point
              <span class="form-section-desc">Define the frps listener users will connect to.</span>
            </div>
          </div>
          <div class="form-row-2 compact-row">
            <div class="compact-field">
              <span class="compact-label required">Name</span>
              <el-form-item prop="name" class="compact-form-item">
                <el-input v-model="formState.name" maxlength="64" placeholder="e.g. ssh-edge-01" />
              </el-form-item>
            </div>
            <div class="compact-field">
              <span class="compact-label required">Protocol</span>
              <el-form-item prop="protocol" class="compact-form-item">
                <div class="seg-control">
                  <button
                    type="button"
                    class="seg-btn"
                    :class="{ active: formState.protocol === 'tcp' }"
                    @click="formState.protocol = 'tcp'"
                  >
                    TCP
                  </button>
                  <button
                    type="button"
                    class="seg-btn"
                    :class="{ active: formState.protocol === 'udp' }"
                    :disabled="formState.targetType === 'socks5_proxy'"
                    @click="formState.protocol = 'udp'"
                  >
                    UDP
                  </button>
                </div>
              </el-form-item>
            </div>
            <div class="compact-field">
              <span class="compact-label required">Bind Address</span>
              <el-form-item prop="bindAddr" class="compact-form-item">
                <el-input v-model="formState.bindAddr" placeholder="0.0.0.0" />
              </el-form-item>
            </div>
            <div class="compact-field">
              <span class="compact-label required">Listen Port</span>
              <el-form-item prop="listenPort" class="compact-form-item">
                <el-input-number
                  v-model="formState.listenPort"
                  :min="1"
                  :max="65535"
                  controls-position="right"
                  class="full-width"
                />
              </el-form-item>
            </div>
          </div>
        </section>

        <section class="form-section form-section-gateway">
          <div class="form-section-head">
            <span class="form-section-kicker">2</span>
            <div class="form-section-title">
              Gateway Client
              <span class="form-section-desc">Pick the frpc node that will host the local target service.</span>
            </div>
          </div>
          <div class="compact-field compact-field-full">
            <span class="compact-label required">Client</span>
            <el-form-item prop="clientKey" class="compact-form-item">
              <div class="client-select-row">
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
                        <span class="gateway-client-name">{{ client.displayName }}</span>
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
                <el-tag
                  v-if="selectedClient"
                  size="small"
                  :type="selectedClient.online ? 'success' : 'info'"
                  class="client-state-tag"
                >
                  {{ selectedClient.online ? 'online' : 'offline' }}
                </el-tag>
              </div>
            </el-form-item>
          </div>
        </section>

        <section class="form-section form-section-target">
          <div class="form-section-head">
            <span class="form-section-kicker">3</span>
            <div class="form-section-title">
              Target Mode
              <span class="form-section-desc">Choose whether frpc forwards directly or starts an embedded proxy.</span>
            </div>
          </div>
          <div class="compact-field compact-field-full">
            <span class="compact-label">Target</span>
            <el-form-item prop="targetType" class="compact-form-item">
              <div class="target-pill-row">
                <button
                  type="button"
                  class="target-pill"
                  :class="{ active: formState.targetType === 'direct' }"
                  @click="formState.targetType = 'direct'"
                >
                  Direct
                </button>
                <button
                  type="button"
                  class="target-pill"
                  :class="{ active: formState.targetType === 'ss_proxy' }"
                  @click="formState.targetType = 'ss_proxy'"
                >
                  Shadowsocks
                </button>
                <button
                  type="button"
                  class="target-pill"
                  :class="{ active: formState.targetType === 'sing_ss_proxy' }"
                  @click="formState.targetType = 'sing_ss_proxy'"
                >
                  Sing SS
                </button>
                <button
                  type="button"
                  class="target-pill"
                  :class="{ active: formState.targetType === 'socks5_proxy' }"
                  @click="formState.targetType = 'socks5_proxy'"
                >
                  SOCKS5
                </button>
              </div>
            </el-form-item>
          </div>

          <div class="target-mode-note">
            <strong>{{ formatTargetTypeLabel(formState.targetType) }}</strong>
            <span>{{ formatTargetTypeDescription(formState.targetType) }}</span>
          </div>

          <div v-if="isDirectTarget" class="form-row-2">
            <div class="compact-field">
              <span class="compact-label required">Target Host</span>
              <el-form-item prop="targetHost" class="compact-form-item">
                <el-input v-model="formState.targetHost" placeholder="127.0.0.1" />
              </el-form-item>
            </div>
            <div class="compact-field">
              <span class="compact-label required">Target Port</span>
              <el-form-item prop="targetPort" class="compact-form-item">
                <el-input-number
                  v-model="formState.targetPort"
                  :min="1"
                  :max="65535"
                  controls-position="right"
                  class="full-width"
                />
              </el-form-item>
            </div>
          </div>

          <template v-if="isSSTarget">
            <div v-if="isSingSSTarget" class="form-callout">
              Mihomo maps cipher to ssMethod and password to ssPassword. 2022 ciphers require a base64 PSK, not a normal password.
            </div>
            <div class="form-row-2">
              <div class="compact-field">
                <span class="compact-label required">Cipher</span>
                <el-form-item prop="ssMethod" class="compact-form-item">
                  <el-select v-model="formState.ssMethod">
                    <template v-if="isSingSSTarget">
                      <el-option
                        v-for="method in SING_SS_METHODS"
                        :key="method"
                        :label="method"
                        :value="method"
                      />
                    </template>
                    <template v-else>
                      <el-option label="chacha20-ietf-poly1305" value="chacha20-ietf-poly1305" />
                      <el-option label="aes-256-gcm" value="aes-256-gcm" />
                      <el-option label="aes-128-gcm" value="aes-128-gcm" />
                    </template>
                  </el-select>
                </el-form-item>
              </div>
              <div class="compact-field">
                <span class="compact-label required">Password</span>
                <el-form-item prop="ssPassword" class="compact-form-item">
                  <el-input
                    v-model="formState.ssPassword"
                    show-password
                    :placeholder="canReuseExistingSSSecret ? 'Leave blank to keep existing' : ''"
                  />
                </el-form-item>
              </div>
            </div>
            <div v-if="isSingSSTarget && formState.protocol === 'tcp'" class="form-row-2">
              <div class="compact-field">
                <span class="compact-label">UDP over TCP</span>
                <el-form-item class="compact-form-item">
                  <div class="inline-switch">
                    <el-switch v-model="formState.uotEnabled" />
                    <span class="inline-switch-label">
                      {{ formState.uotEnabled ? 'Enabled' : 'Disabled' }}
                    </span>
                  </div>
                </el-form-item>
              </div>
              <div v-if="formState.uotEnabled" class="compact-field">
                <span class="compact-label">UOT Version</span>
                <el-form-item class="compact-form-item">
                  <el-segmented
                    v-model="formState.uotVersion"
                    :options="[
                      { label: 'v2', value: 2 },
                      { label: 'v1', value: 1 },
                    ]"
                  />
                </el-form-item>
              </div>
            </div>
          </template>

          <template v-if="isSocks5Target">
            <div class="form-callout form-callout-warn">
              SOCKS5 is easier to fingerprint than Shadowsocks. Prefer SS unless you specifically need it.
            </div>
            <div class="form-row-2">
              <div class="compact-field compact-field-full">
                <span class="compact-label">Authentication</span>
                <el-form-item class="compact-form-item">
                  <div class="inline-switch">
                    <el-switch v-model="formState.socks5Auth" />
                    <span class="inline-switch-label">
                      {{ formState.socks5Auth ? 'Require credentials' : 'No authentication' }}
                    </span>
                  </div>
                </el-form-item>
              </div>
            </div>
            <div v-if="formState.socks5Auth" class="form-row-2">
              <div class="compact-field">
                <span class="compact-label required">Username</span>
                <el-form-item prop="socks5User" class="compact-form-item">
                  <el-input
                    v-model="formState.socks5User"
                    :placeholder="editingTunnel?.targetType === 'socks5_proxy' ? 'Leave blank to keep existing' : ''"
                  />
                </el-form-item>
              </div>
              <div class="compact-field">
                <span class="compact-label required">Password</span>
                <el-form-item prop="socks5Pass" class="compact-form-item">
                  <el-input
                    v-model="formState.socks5Pass"
                    show-password
                    :placeholder="editingTunnel?.targetType === 'socks5_proxy' ? 'Leave blank to keep existing' : ''"
                  />
                </el-form-item>
              </div>
            </div>
          </template>
        </section>

        <section class="form-section form-section-lifecycle">
          <div class="form-section-head">
            <span class="form-section-kicker">4</span>
            <div class="form-section-title">
              Lifecycle
              <span class="form-section-desc">Add operator context and optional expiration.</span>
            </div>
          </div>
          <div class="form-row-2">
            <div class="compact-field">
              <span class="compact-label">Remark</span>
              <el-form-item prop="remark" class="compact-form-item">
                <el-input v-model="formState.remark" maxlength="256" placeholder="Optional note" />
              </el-form-item>
            </div>
            <div class="compact-field">
              <span class="compact-label">Validity</span>
              <el-form-item prop="validityValue" class="compact-form-item">
                <div class="validity-row">
                  <el-input
                    v-model.number="formState.validityValue"
                    type="number"
                    min="1"
                    max="3650"
                    step="1"
                    placeholder="1"
                    :disabled="formState.validityUnit === 'permanent'"
                    @keydown="blockNonIntegerNumberInput"
                  />
                  <el-select v-model="formState.validityUnit" class="validity-unit-select">
                    <el-option label="Permanent" value="permanent" />
                    <el-option label="Hours" value="h" />
                    <el-option label="Days" value="d" />
                  </el-select>
                </div>
              </el-form-item>
            </div>
          </div>
        </section>
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
import { computed, onMounted, reactive, ref, watch } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import ActionButton from '@shared/components/ActionButton.vue'
import BaseDialog from '@shared/components/BaseDialog.vue'
import ConfirmDialog from '@shared/components/ConfirmDialog.vue'
import PopoverMenu from '@shared/components/PopoverMenu.vue'
import PopoverMenuItem from '@shared/components/PopoverMenuItem.vue'
import { useResponsive } from '../composables/useResponsive'
import { getClientGatewaySystemInfo, getClients } from '../api/client'
import {
  createGatewayTunnel,
  deleteGatewayTunnel,
  exportGatewayTunnels,
  getGatewayTunnels,
  importGatewayTunnels,
  updateGatewayTunnel,
} from '../api/gateway'
import type {
  GatewayProtocol,
  GatewayTargetType,
  GatewayTunnelData,
  GatewayTunnelPayload,
  GatewayValidityUnit,
} from '../types/gateway'
import type { ClientInfoData } from '../types/client'
import type { GatewaySystemInfoData } from '../types/client-system'
import { Client } from '../utils/client'
import { formatDistanceToNow, formatFileSize } from '../utils/format'

const { isMobile } = useResponsive()

const SING_SS_METHODS = [
  'aes-128-gcm',
  'aes-192-gcm',
  'aes-256-gcm',
  'chacha20-ietf-poly1305',
  'xchacha20-ietf-poly1305',
  '2022-blake3-aes-128-gcm',
  '2022-blake3-aes-256-gcm',
  '2022-blake3-chacha20-poly1305',
]
const LEGACY_SS_METHODS = [
  'chacha20-ietf-poly1305',
  'aes-256-gcm',
  'aes-128-gcm',
]

const MOCK_TUNNELS: GatewayTunnelData[] = [
  {
    id: '__mock__1',
    name: 'ssh-edge-01',
    remark: 'SSH access via edge gateway',
    protocol: 'tcp',
    bindAddr: '0.0.0.0',
    listenPort: 6022,
    clientKey: 'edge-node-01',
    targetType: 'direct',
    targetHost: '127.0.0.1',
    targetPort: 22,
    status: 'online',
    remoteAddr: '0.0.0.0:6022',
    updatedAt: new Date().toISOString(),
  },
  {
    id: '__mock__2',
    name: 'ss-proxy-hk',
    remark: 'Shadowsocks outbound relay',
    protocol: 'tcp',
    bindAddr: '0.0.0.0',
    listenPort: 8388,
    clientKey: 'hk-relay-01',
    targetType: 'ss_proxy',
    ssMethod: 'chacha20-ietf-poly1305',
    targetHost: '127.0.0.1',
    targetPort: 0,
    status: 'pending',
    updatedAt: new Date().toISOString(),
  },
  {
    id: '__mock__3',
    name: 'rdp-office',
    remark: 'Windows Remote Desktop (office PC)',
    protocol: 'tcp',
    bindAddr: '0.0.0.0',
    listenPort: 13389,
    clientKey: 'office-pc',
    targetType: 'direct',
    targetHost: '192.168.1.100',
    targetPort: 3389,
    status: 'client-offline',
    message: 'Gateway client is not connected',
    updatedAt: new Date(Date.now() - 3_600_000).toISOString(),
  },
  {
    id: '__mock__4',
    name: 'socks5-us',
    protocol: 'tcp',
    bindAddr: '0.0.0.0',
    listenPort: 1080,
    clientKey: 'us-vps-01',
    targetType: 'socks5_proxy',
    socks5Auth: false,
    targetHost: '127.0.0.1',
    targetPort: 0,
    status: 'apply-failed',
    message: 'listen tcp 0.0.0.0:1080: bind: address already in use',
    updatedAt: new Date(Date.now() - 7_200_000).toISOString(),
  },
]

const clients = ref<Client[]>([])
const tunnels = ref<GatewayTunnelData[]>([])
const gatewayRegisteredCount = ref(0)
const gatewayOnlineCount = ref(0)
const gatewaySnapshotClients = ref<Client[]>([])
const gatewayListExpanded = ref(false)
const loading = ref(false)
const hasFetched = ref(false)
const saving = ref(false)
const deleting = ref(false)
const searchText = ref('')
const statusFilter = ref('all')
const dialogVisible = ref(false)
const deleteDialogVisible = ref(false)
const editingTunnel = ref<GatewayTunnelData | null>(null)
const pendingDelete = ref<GatewayTunnelData | null>(null)
const exportDialogVisible = ref(false)
const importDialogVisible = ref(false)
const exportYAML = ref('')
const importYAML = ref('')
const importing = ref(false)
const importFileInputRef = ref<HTMLInputElement>()
const formRef = ref<FormInstance>()
const systemInfoDialogVisible = ref(false)
const systemInfoLoading = ref(false)
const systemInfo = ref<GatewaySystemInfoData | null>(null)

const formState = reactive({
  name: '',
  remark: '',
  protocol: 'tcp' as GatewayProtocol,
  bindAddr: '0.0.0.0',
  listenPort: 0,
  clientKey: '',
  targetType: 'direct' as GatewayTargetType,
  targetHost: '127.0.0.1',
  targetPort: 0,
  ssMethod: 'chacha20-ietf-poly1305',
  ssPassword: '',
  uotEnabled: false,
  uotVersion: 2,
  socks5Auth: false,
  socks5User: '',
  socks5Pass: '',
  validityUnit: 'permanent' as GatewayValidityUnit,
  validityValue: 1,
})

const blockNonIntegerNumberInput = (event: KeyboardEvent) => {
  if (['e', 'E', '+', '-', '.', ','].includes(event.key)) {
    event.preventDefault()
  }
}

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
    {
      validator: (_rule, value, callback) => {
        if (formState.targetType !== 'direct') {
          callback()
          return
        }
        if (!String(value || '').trim()) {
          callback(new Error('Target host is required'))
          return
        }
        callback()
      },
      trigger: 'blur',
    },
  ],
  targetPort: [
    {
      validator: (_rule, value, callback) => {
        if (formState.targetType !== 'direct') {
          callback()
          return
        }
        if (!value || value < 1 || value > 65535) {
          callback(new Error('Target port must be between 1 and 65535'))
          return
        }
        callback()
      },
      trigger: 'change',
    },
  ],
  ssMethod: [
    {
      validator: (_rule, value, callback) => {
        if (!isSSTarget.value) {
          callback()
          return
        }
        if (!String(value || '').trim()) {
          callback(new Error('SS method is required'))
          return
        }
        callback()
      },
      trigger: 'change',
    },
  ],
  ssPassword: [
    {
      validator: (_rule, value, callback) => {
        if (!isSSTarget.value) {
          callback()
          return
        }
        if (!String(value || '').trim() && canReuseExistingSSSecret.value) {
          callback()
          return
        }
        if (!String(value || '').trim()) {
          callback(new Error('SS password is required'))
          return
        }
        callback()
      },
      trigger: 'blur',
    },
  ],
  socks5User: [
    {
      validator: (_rule, value, callback) => {
        if (formState.targetType !== 'socks5_proxy' || !formState.socks5Auth) {
          callback()
          return
        }
        if (!String(value || '').trim() && canReuseExistingSocks5Secret.value) {
          callback()
          return
        }
        if (!String(value || '').trim()) {
          callback(new Error('SOCKS5 username is required'))
          return
        }
        callback()
      },
      trigger: 'blur',
    },
  ],
  socks5Pass: [
    {
      validator: (_rule, value, callback) => {
        if (formState.targetType !== 'socks5_proxy' || !formState.socks5Auth) {
          callback()
          return
        }
        if (!String(value || '').trim() && canReuseExistingSocks5Secret.value) {
          callback()
          return
        }
        if (!String(value || '').trim()) {
          callback(new Error('SOCKS5 password is required'))
          return
        }
        callback()
      },
      trigger: 'blur',
    },
  ],
  validityValue: [
    {
      validator: (_rule, value, callback) => {
        if (formState.validityUnit === 'permanent') {
          callback()
          return
        }
        if (!Number.isInteger(value) || value < 1 || value > 3650) {
          callback(new Error('Validity value must be an integer between 1 and 3650'))
          return
        }
        callback()
      },
      trigger: 'change',
    },
  ],
}


const clientMap = computed<Record<string, Client>>(() =>
  Object.fromEntries(clients.value.map((client) => [client.key, client])),
)

const selectedClient = computed(() => clientMap.value[formState.clientKey])

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
  if (client.supportsGatewaySingSSProxy) {
    parts.push('sing-ss')
  }
  return parts.join(' • ')
}

const isDirectTarget = computed(() => formState.targetType === 'direct')
const isLegacySSTarget = computed(() => formState.targetType === 'ss_proxy')
const isSingSSTarget = computed(() => formState.targetType === 'sing_ss_proxy')
const isSSTarget = computed(() => isLegacySSTarget.value || isSingSSTarget.value)
const isSocks5Target = computed(() => formState.targetType === 'socks5_proxy')
const canReuseExistingSSSecret = computed(
  () => editingTunnel.value?.targetType === formState.targetType && isSSTarget.value,
)
const canReuseExistingSocks5Secret = computed(
  () =>
    editingTunnel.value?.targetType === 'socks5_proxy' &&
    formState.targetType === 'socks5_proxy' &&
    Boolean(editingTunnel.value?.socks5Auth) &&
    formState.socks5Auth,
)

const isMockMode = computed(
  () =>
    hasFetched.value &&
    !loading.value &&
    tunnels.value.length === 0 &&
    eligibleClients.value.length === 0,
)
const displayTunnels = computed(() => isMockMode.value ? MOCK_TUNNELS : tunnels.value)

const filteredTunnels = computed(() => {
  const query = searchText.value.trim().toLowerCase()

  return [...displayTunnels.value]
    .filter((tunnel) => {
      if (statusFilter.value === 'issues') {
        if (tunnel.status === 'online' || tunnel.status === 'pending') return false
      } else if (statusFilter.value !== 'all' && tunnel.status !== statusFilter.value) {
        return false
      }
      if (!query) {
        return true
      }
      const clientLabel = [
        getClientLabel(tunnel.clientKey),
        getClientSubLabel(tunnel.clientKey),
        getClientMetaLine(tunnel.clientKey),
        formatTunnelTarget(tunnel),
        formatTunnelValidity(tunnel),
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
        tunnel.targetType || 'direct',
        tunnel.targetHost,
        String(tunnel.targetPort),
        tunnel.ssMethod || '',
        tunnel.status,
        tunnel.message || '',
        clientLabel,
      ].some((value) => value.toLowerCase().includes(query))
    })
    .sort((a, b) => a.name.localeCompare(b.name))
})

const onlineCount = computed(
  () => displayTunnels.value.filter((tunnel) => tunnel.status === 'online').length,
)

const attentionCount = computed(
  () =>
    displayTunnels.value.filter(
      (tunnel) => tunnel.status !== 'online' && tunnel.status !== 'pending',
    ).length,
)

const statusTabs = computed(() => [
  { value: 'all', label: 'All', count: displayTunnels.value.length },
  { value: 'online', label: 'Online', count: displayTunnels.value.filter((t) => t.status === 'online').length },
  { value: 'pending', label: 'Pending', count: displayTunnels.value.filter((t) => t.status === 'pending').length },
  { value: 'issues', label: 'Issues', count: attentionCount.value },
])

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
    case 'client-unsupported':
      return { label: 'Unsupported Client', type: 'danger' as const }
    case 'expired':
      return { label: 'Expired', type: 'warning' as const }
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

const formatTargetTypeLabel = (targetType?: GatewayTargetType) => {
  switch (targetType || 'direct') {
    case 'ss_proxy':
      return 'Shadowsocks'
    case 'sing_ss_proxy':
      return 'Sing SS'
    case 'socks5_proxy':
      return 'SOCKS5'
    default:
      return 'Direct'
  }
}

const formatTargetTypeDescription = (targetType?: GatewayTargetType) => {
  switch (targetType || 'direct') {
    case 'ss_proxy':
      return 'Existing go-shadowsocks2 embedded service.'
    case 'sing_ss_proxy':
      return 'Mihomo-compatible Shadowsocks with optional UDP-over-TCP.'
    case 'socks5_proxy':
      return 'Embedded SOCKS5 service on frpc.'
    default:
      return 'Forward to host:port on the client side.'
  }
}

const formatTunnelTarget = (tunnel: GatewayTunnelData) => {
  switch (tunnel.targetType || 'direct') {
    case 'ss_proxy':
      return `embedded ss://${tunnel.ssMethod || '-'}`
    case 'sing_ss_proxy':
      return `embedded sing-ss://${tunnel.ssMethod || '-'}${tunnel.uotEnabled ? `/uot-v${tunnel.uotVersion || 2}` : ''}`
    case 'socks5_proxy':
      return tunnel.socks5Auth ? 'embedded socks5://auth' : 'embedded socks5://no-auth'
    default:
      return `${tunnel.targetHost}:${tunnel.targetPort}`
  }
}

const formatTunnelTargetMeta = (tunnel: GatewayTunnelData) => {
  switch (tunnel.targetType || 'direct') {
    case 'ss_proxy':
      return tunnel.protocol === 'udp'
        ? 'Shadowsocks server on gateway client (TCP/UDP capable)'
        : 'Shadowsocks server on gateway client'
    case 'sing_ss_proxy':
      if (tunnel.protocol === 'udp') {
        return 'Sing Shadowsocks UDP service on gateway client'
      }
      return tunnel.uotEnabled
        ? `Sing Shadowsocks TCP service with UOT v${tunnel.uotVersion || 2}`
        : 'Sing Shadowsocks TCP service on gateway client'
    case 'socks5_proxy':
      return 'SOCKS5 server on gateway client'
    default:
      return 'Local endpoint on the gateway client'
  }
}

const formatTunnelValidity = (tunnel: GatewayTunnelData) => {
  const unit = tunnel.validityUnit || 'permanent'
  if (unit === 'permanent') {
    return 'Permanent'
  }
  const value = tunnel.validityValue || 0
  const expiresAt = tunnel.expiresAt ? new Date(tunnel.expiresAt) : null
  if (expiresAt && !Number.isNaN(expiresAt.getTime())) {
    return `${value}${unit} (valid until ${snapshotTimeFormatter.format(expiresAt)})`
  }
  return `${value}${unit}`
}

const formatBytes = (value?: number) => {
  return formatFileSize(value || 0)
}

const formatPercent = (value?: number) => {
  if (typeof value !== 'number' || Number.isNaN(value)) return '-'
  return `${value.toFixed(2)}%`
}

const formatLoad = (load1?: number, load5?: number, load15?: number) => {
  const values = [load1, load5, load15].map((value) =>
    typeof value === 'number' && !Number.isNaN(value) ? value.toFixed(2) : '-',
  )
  return values.join(' / ')
}

const formatDurationSeconds = (value?: number) => {
  if (!value || value <= 0) return '-'
  const days = Math.floor(value / 86400)
  const hours = Math.floor((value % 86400) / 3600)
  const minutes = Math.floor((value % 3600) / 60)
  const seconds = Math.floor(value % 60)
  const parts = []
  if (days > 0) parts.push(`${days}d`)
  if (hours > 0 || parts.length > 0) parts.push(`${hours}h`)
  if (minutes > 0 || parts.length > 0) parts.push(`${minutes}m`)
  parts.push(`${seconds}s`)
  return parts.join(' ')
}

const unixSnapshotFormatter = new Intl.DateTimeFormat(undefined, {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
})

const formatUnixSnapshot = (value?: number) => {
  if (!value || value <= 0) return '-'
  const date = new Date(value * 1000)
  if (Number.isNaN(date.getTime())) return '-'
  return `${unixSnapshotFormatter.format(date)} (${formatDistanceToNow(date)})`
}

const openSystemInfoDialog = async (client: Client) => {
  systemInfoDialogVisible.value = true
  systemInfoLoading.value = true
  systemInfo.value = null
  try {
    systemInfo.value = await getClientGatewaySystemInfo(client.key)
  } catch (error: any) {
    ElMessage({
      type: 'error',
      showClose: true,
      message: 'Failed to fetch gateway system info: ' + error.message,
    })
  } finally {
    systemInfoLoading.value = false
  }
}

const snapshotTimeFormatter = new Intl.DateTimeFormat(undefined, {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
})

const formatSnapshotAgo = (value?: Date) => {
  if (!value || Number.isNaN(value.getTime())) return '-'
  return formatDistanceToNow(value)
}


const resetForm = () => {
  formState.name = ''
  formState.remark = ''
  formState.protocol = 'tcp'
  formState.bindAddr = '0.0.0.0'
  formState.listenPort = 0
  formState.clientKey = eligibleClients.value[0]?.key || ''
  formState.targetType = 'direct'
  formState.targetHost = '127.0.0.1'
  formState.targetPort = 0
  formState.ssMethod = 'chacha20-ietf-poly1305'
  formState.ssPassword = ''
  formState.uotEnabled = false
  formState.uotVersion = 2
  formState.socks5Auth = false
  formState.socks5User = ''
  formState.socks5Pass = ''
  formState.validityUnit = 'permanent'
  formState.validityValue = 1
}

const populateForm = (tunnel: GatewayTunnelData) => {
  formState.name = tunnel.name
  formState.remark = tunnel.remark || ''
  formState.protocol = tunnel.protocol
  formState.bindAddr = tunnel.bindAddr
  formState.listenPort = tunnel.listenPort
  formState.clientKey = tunnel.clientKey
  formState.targetType = tunnel.targetType || 'direct'
  formState.targetHost = tunnel.targetHost
  formState.targetPort = tunnel.targetPort
  formState.ssMethod = tunnel.ssMethod || 'chacha20-ietf-poly1305'
  formState.ssPassword = tunnel.ssPassword || ''
  formState.uotEnabled = Boolean(tunnel.uotEnabled)
  formState.uotVersion = tunnel.uotVersion || 2
  formState.socks5Auth = Boolean(tunnel.socks5Auth)
  formState.socks5User = tunnel.socks5User || ''
  formState.socks5Pass = tunnel.socks5Pass || ''
  formState.validityUnit = tunnel.validityUnit || 'permanent'
  formState.validityValue = tunnel.validityValue || 1
}

const refreshGatewayClientSnapshot = (list: Client[]) => {
  const gateways = list
    .filter((client) => client.allowGatewayTunnels && client.hasStableClientID)
    .sort((a, b) => a.displayName.localeCompare(b.displayName))
  gatewayRegisteredCount.value = gateways.length
  gatewayOnlineCount.value = gateways.filter((client) => client.online).length
  gatewaySnapshotClients.value = gateways
}

watch(
  () => formState.targetType,
  (value) => {
    if (value === 'socks5_proxy' && formState.protocol === 'udp') {
      formState.protocol = 'tcp'
    }
    if (value === 'sing_ss_proxy' && !SING_SS_METHODS.includes(formState.ssMethod)) {
      formState.ssMethod = 'chacha20-ietf-poly1305'
    }
    if (value === 'ss_proxy' && !LEGACY_SS_METHODS.includes(formState.ssMethod)) {
      formState.ssMethod = 'chacha20-ietf-poly1305'
    }
    if (value !== 'sing_ss_proxy') {
      formState.uotEnabled = false
      formState.uotVersion = 2
    }
  },
)

watch(
  () => formState.protocol,
  (value) => {
    if (value === 'udp') {
      formState.uotEnabled = false
      formState.uotVersion = 2
    }
  },
)

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
    hasFetched.value = true
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
    hasFetched.value = true
  }
}

const openExportDialog = async () => {
  loading.value = true
  try {
    const resp = await exportGatewayTunnels()
    exportYAML.value = resp.yaml
    exportDialogVisible.value = true
  } catch (error: any) {
    ElMessage({
      type: 'error',
      showClose: true,
      message: 'Failed to export gateway tunnels: ' + error.message,
    })
  } finally {
    loading.value = false
  }
}

const copyExportYAML = async () => {
  if (!exportYAML.value.trim()) {
    ElMessage({ type: 'warning', message: 'Nothing to copy' })
    return
  }
  try {
    await navigator.clipboard.writeText(exportYAML.value)
    ElMessage({ type: 'success', message: 'YAML copied to clipboard' })
  } catch {
    ElMessage({
      type: 'error',
      showClose: true,
      message: 'Clipboard copy failed, please copy manually',
    })
  }
}

const downloadExportYAML = () => {
  if (!exportYAML.value.trim()) {
    ElMessage({ type: 'warning', message: 'Nothing to download' })
    return
  }
  const now = new Date()
  const dateToken = now
    .toISOString()
    .replace(/[-:]/g, '')
    .replace('T', '-')
    .slice(0, 15)
  const filename = `gateway-tunnels-${dateToken}.yaml`
  const blob = new Blob([exportYAML.value], { type: 'text/yaml;charset=utf-8' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = filename
  link.click()
  URL.revokeObjectURL(link.href)
}

const openImportDialog = () => {
  importDialogVisible.value = true
}

const openImportFilePicker = () => {
  importFileInputRef.value?.click()
}

const handleImportFileChange = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) {
    return
  }
  try {
    importYAML.value = await file.text()
    ElMessage({ type: 'success', message: `Loaded file: ${file.name}` })
  } catch {
    ElMessage({
      type: 'error',
      showClose: true,
      message: 'Failed to read selected file',
    })
  } finally {
    input.value = ''
  }
}

const submitImportYAML = async () => {
  const raw = importYAML.value.trim()
  if (!raw) {
    ElMessage({ type: 'warning', message: 'Paste YAML or load a file first' })
    return
  }

  importing.value = true
  try {
    const result = await importGatewayTunnels({ yaml: raw })
    ElMessage({
      type: 'success',
      message: `Import complete: total ${result.total}, created ${result.created}, updated ${result.updated}`,
    })
    importDialogVisible.value = false
    await fetchData()
  } catch (error: any) {
    ElMessage({
      type: 'error',
      showClose: true,
      message: 'Failed to import gateway tunnels: ' + error.message,
    })
  } finally {
    importing.value = false
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
  if (formState.targetType === 'sing_ss_proxy' && selectedClient.value && !selectedClient.value.supportsGatewaySingSSProxy) {
    ElMessage({
      type: 'error',
      showClose: true,
      message: 'Selected gateway client does not support sing_ss_proxy',
    })
    return
  }

  saving.value = true
  try {
    const payload: GatewayTunnelPayload = {
      name: formState.name.trim(),
      remark: formState.remark.trim(),
      protocol: formState.protocol,
      bindAddr: formState.bindAddr.trim(),
      listenPort: formState.listenPort,
      clientKey: formState.clientKey,
      targetType: formState.targetType,
      targetHost: formState.targetHost.trim(),
      targetPort: formState.targetPort,
      ssMethod: formState.ssMethod.trim(),
      ssPassword: formState.ssPassword || undefined,
      uotEnabled: formState.targetType === 'sing_ss_proxy' && formState.protocol === 'tcp'
        ? formState.uotEnabled
        : false,
      uotVersion:
        formState.targetType === 'sing_ss_proxy' &&
        formState.protocol === 'tcp' &&
        formState.uotEnabled
          ? formState.uotVersion
          : 0,
      socks5Auth: formState.socks5Auth,
      socks5User: formState.socks5User.trim() || undefined,
      socks5Pass: formState.socks5Pass || undefined,
      validityUnit: formState.validityUnit,
      validityValue:
        formState.validityUnit === 'permanent' ? 0 : formState.validityValue,
    }

    if (editingTunnel.value) {
      if (
        editingTunnel.value.targetType === 'ss_proxy' &&
        formState.targetType === 'ss_proxy' &&
        !formState.ssPassword.trim()
      ) {
        payload.ssPassword = undefined
      }
      if (
        editingTunnel.value.targetType === 'sing_ss_proxy' &&
        formState.targetType === 'sing_ss_proxy' &&
        !formState.ssPassword.trim()
      ) {
        payload.ssPassword = undefined
      }
      if (
        editingTunnel.value.targetType === 'socks5_proxy' &&
        formState.targetType === 'socks5_proxy' &&
        editingTunnel.value.socks5Auth &&
        formState.socks5Auth
      ) {
        if (!formState.socks5User.trim()) {
          payload.socks5User = undefined
        }
        if (!formState.socks5Pass.trim()) {
          payload.socks5Pass = undefined
        }
      }
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

.title-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.page-subtitle {
  margin: 0;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
}

.actions-section {
  display: flex;
  gap: 10px;
}

.yaml-dialog {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.yaml-help {
  margin: 0;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
}

.yaml-file-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.yaml-file-input {
  display: none;
}

.yaml-textarea :deep(textarea) {
  font-family: var(--el-font-family-monospace, ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace);
  font-size: 12px;
}

.warning-banner {
  border: 1px solid rgba(245, 158, 11, 0.28);
  background: rgba(245, 158, 11, 0.1);
  color: var(--el-text-color-primary);
  padding: 14px 16px;
  border-radius: 14px;
  line-height: 1.5;
}

.mem-chip {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 20px;
  border: 1px solid rgba(245, 158, 11, 0.3);
  background: rgba(245, 158, 11, 0.08);
  color: rgba(200, 130, 10, 0.95);
  font-size: 12px;
  white-space: nowrap;
  cursor: pointer;
  transition: background 0.15s ease;
}

.mem-chip:hover {
  background: rgba(245, 158, 11, 0.14);
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

.stat-card-clickable {
  cursor: pointer;
  user-select: none;
  transition: background 0.15s ease;
}

.stat-card-clickable:hover {
  background: var(--el-fill-color-light);
}

.stat-hint {
  font-size: 12px;
  color: var(--el-color-primary);
  font-weight: 500;
  margin-top: 2px;
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
  gap: 6px;
}

.snapshot-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
}

.snapshot-head-main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.snapshot-head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.snapshot-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  line-height: 1.2;
}

.snapshot-subline {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  word-break: break-word;
}

.snapshot-meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  word-break: break-word;
}

.snapshot-time-line {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 10px;
}

.snapshot-time-pill {
  min-width: 0;
  display: inline-flex;
  align-items: baseline;
  gap: 5px;
}

.snapshot-time-label {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--el-text-color-secondary);
}

.snapshot-time-value {
  font-size: 12px;
  color: var(--el-text-color-primary);
  word-break: break-word;
}

.snapshot-identity-line {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  font-size: 11px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
}

.snapshot-metas {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.snapshot-chip {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  padding: 3px 8px;
  border-radius: 999px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  font-size: 11px;
  color: var(--el-text-color-secondary);
  word-break: break-word;
}

.validity-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 160px;
  gap: 10px;
  width: 100%;
}

.validity-unit-select {
  width: 100%;
}


.system-info-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-height: 70vh;
  overflow-y: auto;
  padding-right: 4px;
}

.system-info-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.system-info-section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.system-info-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 14px;
}

.system-info-item {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-extra-light);
}

.system-info-item span {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--el-text-color-secondary);
}

.system-info-item strong {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  word-break: break-word;
}

.system-info-interface-list,
.system-info-process-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.system-info-interface {
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-extra-light);
}

.system-info-interface-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.system-info-interface-meta,
.system-info-interface-addresses,
.system-info-empty {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  word-break: break-word;
}

.system-info-process-head,
.system-info-process-row {
  display: grid;
  grid-template-columns: 80px minmax(0, 1fr) 140px 100px;
  gap: 10px;
  align-items: center;
}

.system-info-process-head {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--el-text-color-secondary);
  padding: 0 12px;
}

.system-info-process-row {
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-extra-light);
  font-size: 13px;
  color: var(--el-text-color-primary);
}

.filter-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.status-tabs {
  display: flex;
  gap: 4px;
  background: var(--el-fill-color-light);
  padding: 4px;
  border-radius: 10px;
  flex-shrink: 0;
}

.demo-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border-radius: 12px;
  background: rgba(59, 130, 246, 0.07);
  border: 1px solid rgba(59, 130, 246, 0.18);
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.5;
}

.demo-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 6px;
  background: rgba(59, 130, 246, 0.15);
  color: #3b82f6;
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
  flex-shrink: 0;
}

.status-tab {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: none;
  background: transparent;
  border-radius: 7px;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  white-space: nowrap;
}

.status-tab:hover {
  background: var(--el-bg-color);
  color: var(--el-text-color-primary);
}

.status-tab.active {
  background: var(--el-bg-color);
  color: var(--el-text-color-primary);
  font-weight: 500;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
}

.tab-label {
  color: inherit;
}

.tab-count {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 999px;
  background: var(--el-fill-color);
  color: var(--el-text-color-secondary);
  line-height: 1.6;
  font-weight: 500;
}

.status-tab.active .tab-count {
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}

.search-input {
  flex: 1;
  min-width: 200px;
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
  border-left: 3px solid transparent;
}

.tunnel-card.status-online {
  border-left-color: var(--el-color-success);
}

.tunnel-card.status-pending,
.tunnel-card.status-expired {
  border-left-color: var(--el-color-warning);
}

.tunnel-card.status-register-failed,
.tunnel-card.status-target-invalid,
.tunnel-card.status-target-unreachable,
.tunnel-card.status-invalid-config,
.tunnel-card.status-client-unsupported,
.tunnel-card.status-apply-failed {
  border-left-color: var(--el-color-danger);
}

.tunnel-card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 16px 0;
}

.tunnel-header-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.tunnel-updated {
  font-size: 11px;
  color: var(--el-text-color-placeholder);
  white-space: nowrap;
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

.tunnel-card-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 260px;
  gap: 12px;
  padding: 12px 16px 14px;
}

.route-flow {
  display: grid;
  grid-template-columns: minmax(0, 0.9fr) 26px minmax(0, 1.1fr) 26px minmax(0, 1.1fr);
  gap: 8px;
  align-items: stretch;
  min-width: 0;
}

.route-node,
.status-panel {
  min-width: 0;
  padding: 14px 16px;
  border-radius: 14px;
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-extra-light);
}

.route-node,
.status-panel {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.route-node-listen {
  background: linear-gradient(180deg, rgba(64, 158, 255, 0.08), var(--el-fill-color-light));
}

.route-node-gateway {
  background: linear-gradient(180deg, rgba(103, 194, 58, 0.08), var(--el-fill-color-light));
}

.route-node-target {
  background: linear-gradient(180deg, rgba(230, 162, 60, 0.08), var(--el-fill-color-light));
}

.route-arrow {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--el-text-color-placeholder);
  font-size: 16px;
  font-weight: 700;
}

.status-panel {
  background: var(--el-bg-color);
}

.status-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
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
.status-detail {
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.4;
}

.status-message {
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.4;
  word-break: break-word;
}

.row-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.gateway-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-top: 2px;
}

.form-section {
  --section-rgb: 64, 158, 255;
  --section-color: rgb(var(--section-rgb));
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 16px;
  border: 1px solid rgba(var(--section-rgb), 0.18);
  border-radius: 16px;
  background:
    radial-gradient(circle at 0% 0%, rgba(var(--section-rgb), 0.12), transparent 38%),
    linear-gradient(180deg, rgba(var(--section-rgb), 0.055), var(--el-bg-color) 72%);
}

.form-section-entry {
  --section-rgb: 64, 158, 255;
}

.form-section-gateway {
  --section-rgb: 103, 194, 58;
}

.form-section-target {
  --section-rgb: 230, 162, 60;
}

.form-section-lifecycle {
  --section-rgb: 144, 147, 153;
}

.form-section-head {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.form-section-kicker {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 999px;
  background: rgba(var(--section-rgb), 0.14);
  color: var(--section-color);
  font-size: 12px;
  font-weight: 700;
  flex-shrink: 0;
}

.form-section-title {
  font-size: 14px;
  font-weight: 650;
  color: var(--el-text-color-primary);
  line-height: 1.2;
}

.form-section-desc {
  margin-left: 8px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  font-weight: 400;
  line-height: 1.45;
}

.form-row-2 {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 14px;
}

.compact-row {
  grid-template-rows: auto auto;
}

.compact-field {
  display: grid;
  grid-template-columns: 108px minmax(0, 1fr);
  gap: 10px;
  align-items: center;
  min-width: 0;
}

.compact-field-full {
  grid-column: 1 / -1;
}

.compact-label {
  color: var(--el-text-color-regular);
  font-size: 13px;
  line-height: 1.2;
  white-space: nowrap;
}

.compact-label.required::before {
  content: "*";
  margin-right: 4px;
  color: var(--el-color-danger);
}

.compact-form-item {
  margin-bottom: 0;
  min-width: 0;
}

.compact-form-item :deep(.el-form-item__content) {
  min-width: 0;
}

.compact-form-item :deep(.el-form-item__error) {
  position: static;
  margin-top: 2px;
}

.seg-control {
  display: flex;
  background: var(--el-fill-color-light);
  border-radius: 8px;
  padding: 3px;
  gap: 2px;
  width: 100%;
}

.seg-btn {
  flex: 1;
  padding: 5px 8px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease, box-shadow 0.15s ease;
  white-space: nowrap;
}

.seg-btn:hover:not(:disabled) {
  background: var(--el-bg-color);
  color: var(--el-text-color-primary);
}

.seg-btn.active {
  background: var(--el-bg-color);
  color: var(--el-color-primary);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

.seg-btn:disabled {
  opacity: 0.38;
  cursor: not-allowed;
}

.target-pill-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  width: 100%;
}

.target-pill {
  min-width: 108px;
  padding: 7px 14px;
  border-radius: 999px;
  border: 1px solid var(--el-border-color-light);
  background: var(--el-bg-color);
  color: var(--el-text-color-secondary);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease, box-shadow 0.15s ease;
}

.target-pill:hover {
  border-color: var(--el-color-primary-light-5);
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}

.target-pill.active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  box-shadow: 0 0 0 1px var(--el-color-primary-light-7) inset;
}

.target-mode-note {
  display: flex;
  gap: 8px;
  align-items: baseline;
  padding: 8px 12px;
  border-radius: 10px;
  background: rgba(64, 158, 255, 0.06);
  border: 1px solid rgba(64, 158, 255, 0.14);
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.4;
}

.target-mode-note strong {
  color: var(--el-text-color-primary);
  font-size: 13px;
  white-space: nowrap;
}

.form-callout {
  padding: 9px 12px;
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.5;
  background: rgba(64, 158, 255, 0.07);
  border: 1px solid rgba(64, 158, 255, 0.18);
  color: var(--el-text-color-secondary);
}

.form-callout-warn {
  background: rgba(245, 108, 108, 0.07);
  border: 1px solid rgba(245, 108, 108, 0.18);
  color: var(--el-color-danger);
}

.inline-switch {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 32px;
}

.inline-switch-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.full-width {
  width: 100%;
}

.gateway-client-select {
  width: 100%;
}

.client-select-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.client-state-tag {
  flex-shrink: 0;
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

  .tunnel-card-body,
  .snapshot-list,
  .form-row-2 {
    grid-template-columns: 1fr;
  }

  .compact-field {
    grid-template-columns: 1fr;
    gap: 6px;
    align-items: stretch;
  }

  .route-flow {
    grid-template-columns: 1fr;
  }

  .route-arrow {
    min-height: 14px;
    transform: rotate(90deg);
  }

  .actions-section,
  .filter-bar,
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
