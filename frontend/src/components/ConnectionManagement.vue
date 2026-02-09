<script setup lang="ts">
import { computed, h, ref } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NForm,
  NFormItem,
  NGrid,
  NGi,
  NInput,
  NInputNumber,
  NModal,
  NPopconfirm,
  NSelect,
  NSpace,
  NTag
} from 'naive-ui'
import type { DataTableColumns, FormInst, FormRules } from 'naive-ui'

type ConnectionStatus = 'online' | 'offline'
type ConnectionEnv = '生产' | '测试' | '开发'
type SummaryTone = 'neutral' | 'success' | 'warning'

interface ConnectionItem {
  id: number
  name: string
  env: ConnectionEnv
  nameServer: string
  timeoutSec: number
  status: ConnectionStatus
  lastCheck: string
  isDefault: boolean
  remark: string
}

interface ConnectionFormModel {
  name: string
  env: ConnectionEnv
  nameServer: string
  timeoutSec: number
  remark: string
}

interface SummaryItem {
  key: string
  label: string
  value: number
  hint: string
  tone: SummaryTone
}

const envOptions = [
  { label: '生产', value: '生产' },
  { label: '测试', value: '测试' },
  { label: '开发', value: '开发' }
]

const connectionList = ref<ConnectionItem[]>([
  {
    id: 1,
    name: '生产集群',
    env: '生产',
    nameServer: '192.168.1.100:9876',
    timeoutSec: 5,
    status: 'online',
    lastCheck: '2026-02-09 14:10:22',
    isDefault: true,
    remark: '核心业务集群'
  },
  {
    id: 2,
    name: '测试集群',
    env: '测试',
    nameServer: '192.168.1.101:9876',
    timeoutSec: 6,
    status: 'online',
    lastCheck: '2026-02-09 14:09:48',
    isDefault: false,
    remark: '联调测试环境'
  },
  {
    id: 3,
    name: '开发集群',
    env: '开发',
    nameServer: 'localhost:9876',
    timeoutSec: 8,
    status: 'offline',
    lastCheck: '2026-02-09 13:56:10',
    isDefault: false,
    remark: '本地调试'
  }
])

const searchKeyword = ref('')
const showEditor = ref(false)
const isSubmitting = ref(false)
const editingId = ref<number | null>(null)
const testingIds = ref<number[]>([])
const formRef = ref<FormInst | null>(null)

const createDefaultFormModel = (): ConnectionFormModel => ({
  name: '',
  env: '开发',
  nameServer: '',
  timeoutSec: 5,
  remark: ''
})

const formModel = ref<ConnectionFormModel>(createDefaultFormModel())

const formRules: FormRules = {
  name: [{ required: true, message: '请输入连接名称', trigger: ['blur', 'input'] }],
  env: [{ required: true, message: '请选择环境', trigger: ['change'] }],
  nameServer: [{ required: true, message: '请输入 NameServer 地址', trigger: ['blur', 'input'] }],
  timeoutSec: [{ required: true, type: 'number', message: '请设置超时时间', trigger: ['blur', 'change'] }]
}

const filteredConnections = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase()
  if (!keyword) return connectionList.value
  return connectionList.value.filter(item =>
    item.name.toLowerCase().includes(keyword)
    || item.nameServer.toLowerCase().includes(keyword)
    || item.env.toLowerCase().includes(keyword)
  )
})

const onlineCount = computed(() => connectionList.value.filter(item => item.status === 'online').length)

const summaryItems = computed<SummaryItem[]>(() => {
  const total = connectionList.value.length
  const online = onlineCount.value
  const offline = total - online
  const health = total > 0 ? Math.round((online / total) * 100) : 0

  return [
    { key: 'total', label: '连接总数', value: total, hint: '已配置连接', tone: 'neutral' },
    { key: 'online', label: '在线连接', value: online, hint: `健康度 ${health}%`, tone: 'success' },
    { key: 'offline', label: '离线连接', value: offline, hint: offline > 0 ? '建议排查' : '状态稳定', tone: offline > 0 ? 'warning' : 'success' }
  ]
})

const openCreate = () => {
  editingId.value = null
  formModel.value = createDefaultFormModel()
  showEditor.value = true
}

const openEdit = (row: ConnectionItem) => {
  editingId.value = row.id
  formModel.value = {
    name: row.name,
    env: row.env,
    nameServer: row.nameServer,
    timeoutSec: row.timeoutSec,
    remark: row.remark
  }
  showEditor.value = true
}

const setAsDefault = (id: number) => {
  connectionList.value = connectionList.value.map(item => ({
    ...item,
    isDefault: item.id === id
  }))
}

const deleteConnection = (id: number) => {
  connectionList.value = connectionList.value.filter(item => item.id !== id)
  if (!connectionList.value.some(item => item.isDefault) && connectionList.value.length > 0) {
    const firstConnection = connectionList.value[0]
    if (firstConnection) {
      firstConnection.isDefault = true
    }
  }
}

const formatNow = () => {
  const now = new Date()
  const pad = (value: number) => value.toString().padStart(2, '0')
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())} ${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`
}

const testConnection = (id: number) => {
  if (testingIds.value.includes(id)) return
  testingIds.value.push(id)

  window.setTimeout(() => {
    const success = Math.random() > 0.25
    connectionList.value = connectionList.value.map(item => {
      if (item.id !== id) return item
      return {
        ...item,
        status: success ? 'online' : 'offline',
        lastCheck: formatNow()
      }
    })
    testingIds.value = testingIds.value.filter(itemId => itemId !== id)
  }, 700)
}

const saveConnection = async () => {
  if (!formRef.value) return
  await formRef.value.validate()
  isSubmitting.value = true

  const payload = formModel.value
  if (editingId.value === null) {
    connectionList.value.unshift({
      id: Date.now(),
      name: payload.name,
      env: payload.env,
      nameServer: payload.nameServer,
      timeoutSec: payload.timeoutSec,
      remark: payload.remark,
      status: 'offline',
      lastCheck: '-',
      isDefault: connectionList.value.length === 0
    })
  } else {
    connectionList.value = connectionList.value.map(item => {
      if (item.id !== editingId.value) return item
      return {
        ...item,
        name: payload.name,
        env: payload.env,
        nameServer: payload.nameServer,
        timeoutSec: payload.timeoutSec,
        remark: payload.remark
      }
    })
  }

  isSubmitting.value = false
  showEditor.value = false
}

const columns: DataTableColumns<ConnectionItem> = [
  {
    title: '连接名称',
    key: 'name',
    render: (row) => h('div', { class: 'name-cell' }, [
      h('span', null, row.name),
      row.isDefault ? h(NTag, { type: 'success', size: 'small', round: true }, { default: () => '默认' }) : null
    ])
  },
  { title: '环境', key: 'env', width: 90 },
  { title: 'NameServer', key: 'nameServer' },
  {
    title: '状态',
    key: 'status',
    width: 90,
    render: (row) => h(
      NTag,
      { type: row.status === 'online' ? 'success' : 'error', size: 'small', round: true },
      { default: () => (row.status === 'online' ? '在线' : '离线') }
    )
  },
  { title: '超时(秒)', key: 'timeoutSec', width: 100 },
  { title: '最近检测', key: 'lastCheck', width: 168 },
  {
    title: '操作',
    key: 'actions',
    width: 280,
    render: (row) => h(NSpace, { size: 6 }, {
      default: () => [
        h(
          NButton,
          { size: 'tiny', quaternary: true, onClick: () => testConnection(row.id), loading: testingIds.value.includes(row.id) },
          { default: () => '测试连接' }
        ),
        h(
          NButton,
          { size: 'tiny', quaternary: true, onClick: () => openEdit(row) },
          { default: () => '编辑' }
        ),
        h(
          NButton,
          { size: 'tiny', quaternary: true, disabled: row.isDefault, onClick: () => setAsDefault(row.id) },
          { default: () => '设为默认' }
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => deleteConnection(row.id) },
          {
            trigger: () => h(
              NButton,
              { size: 'tiny', quaternary: true, type: 'error', disabled: row.isDefault },
              { default: () => '删除' }
            ),
            default: () => '确认删除该连接吗？'
          }
        )
      ]
    })
  }
]
</script>

<template>
  <div class="connection-page">
    <n-alert type="info" :show-icon="false" class="page-hint">
      当前为前端模拟数据页面，后续可直接接入 Wails 后端连接管理接口。
    </n-alert>

    <n-grid responsive="screen" cols="1 s:2 l:3" :x-gap="12" :y-gap="12" class="summary-grid">
      <n-gi v-for="item in summaryItems" :key="item.key">
        <n-card size="small" :bordered="false" class="summary-card">
          <div class="summary-content">
            <div class="summary-label">{{ item.label }}</div>
            <div class="summary-value" :class="`is-${item.tone}`">{{ item.value }}</div>
            <div class="summary-foot">
              <n-tag
                size="small"
                round
                :type="item.tone === 'warning' ? 'warning' : item.tone === 'success' ? 'success' : 'default'"
              >
                {{ item.hint }}
              </n-tag>
            </div>
          </div>
        </n-card>
      </n-gi>
    </n-grid>

    <n-card :bordered="false" class="table-card">
      <div class="toolbar">
        <n-space>
          <n-button type="primary" @click="openCreate">新增连接</n-button>
          <n-input v-model:value="searchKeyword" clearable placeholder="搜索名称 / 环境 / NameServer" style="width: 280px;" />
        </n-space>
      </div>

      <n-data-table
        :columns="columns"
        :data="filteredConnections"
        :pagination="{ pageSize: 8 }"
        :single-line="false"
        size="small"
      />
    </n-card>

    <n-modal v-model:show="showEditor" preset="card" class="editor-modal" :title="editingId ? '编辑连接' : '新增连接'">
      <n-form ref="formRef" :model="formModel" :rules="formRules" label-placement="left" label-width="90">
        <n-form-item label="连接名称" path="name">
          <n-input v-model:value="formModel.name" placeholder="例如：生产集群" />
        </n-form-item>

        <n-form-item label="环境" path="env">
          <n-select v-model:value="formModel.env" :options="envOptions" />
        </n-form-item>

        <n-form-item label="NameServer" path="nameServer">
          <n-input v-model:value="formModel.nameServer" placeholder="例如：127.0.0.1:9876" />
        </n-form-item>

        <n-form-item label="超时(秒)" path="timeoutSec">
          <n-input-number v-model:value="formModel.timeoutSec" :min="1" :max="60" style="width: 160px;" />
        </n-form-item>

        <n-form-item label="备注">
          <n-input v-model:value="formModel.remark" type="textarea" :rows="3" placeholder="可选备注信息" />
        </n-form-item>
      </n-form>

      <template #footer>
        <div class="editor-actions">
          <n-space justify="end">
            <n-button @click="showEditor = false">取消</n-button>
            <n-button type="primary" :loading="isSubmitting" @click="saveConnection">保存</n-button>
          </n-space>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.connection-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.page-hint {
  border-radius: 10px;
}

.summary-card,
.table-card {
  background: var(--surface-2, #fff);
  border-radius: 12px;
}

.summary-grid {
  margin-top: -2px;
}

.summary-content {
  min-height: 102px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}

.summary-label {
  font-size: 13px;
  color: var(--text-muted, #888);
}

.summary-value {
  margin-top: 4px;
  font-size: 34px;
  font-weight: 700;
  line-height: 1;
  color: var(--text-color, #333);
}

.summary-value.is-success {
  color: #18a058;
}

.summary-value.is-warning {
  color: #d97706;
}

.summary-foot {
  margin-top: 10px;
}

.toolbar {
  margin-bottom: 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.editor-modal {
  width: 560px;
}

.editor-actions {
  width: 100%;
}
</style>
