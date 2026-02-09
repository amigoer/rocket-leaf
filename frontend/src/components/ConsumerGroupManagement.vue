<script setup lang="ts">
import { computed, h, ref } from 'vue'
import {
  NButton,
  NCard,
  NDataTable,
  NDescriptions,
  NDescriptionsItem,
  NDrawer,
  NDrawerContent,
  NForm,
  NFormItem,
  NGrid,
  NGi,
  NInput,
  NInputNumber,
  NPopconfirm,
  NSelect,
  NSpace,
  NTable,
  NTag,
  NModal,
  useMessage
} from 'naive-ui'
import type { DataTableColumns, FormInst, FormRules } from 'naive-ui'

type GroupStatus = 'online' | 'warning' | 'offline'
type ConsumeMode = 'CLUSTERING' | 'BROADCASTING'

interface GroupSubscription {
  topic: string
  expression: string
  consumeTps: number
}

interface GroupClient {
  clientId: string
  ip: string
  version: string
  lastHeartbeat: string
}

interface ConsumerGroupItem {
  id: number
  group: string
  cluster: string
  consumeMode: ConsumeMode
  status: GroupStatus
  onlineClients: number
  topicCount: number
  lag: number
  retryQps: number
  dlq: number
  maxRetry: number
  lastUpdate: string
  remark: string
  subscriptions: GroupSubscription[]
  clients: GroupClient[]
}

interface GroupFormModel {
  group: string
  cluster: string
  consumeMode: ConsumeMode
  maxRetry: number
  topicsCsv: string
  remark: string
}

const message = useMessage()

const clusterOptions = [
  { label: '生产集群', value: '生产集群' },
  { label: '测试集群', value: '测试集群' },
  { label: '开发集群', value: '开发集群' }
]

const statusOptions = [
  { label: '在线', value: 'online' },
  { label: '告警', value: 'warning' },
  { label: '离线', value: 'offline' }
]

const consumeModeOptions = [
  { label: '集群消费 (CLUSTERING)', value: 'CLUSTERING' },
  { label: '广播消费 (BROADCASTING)', value: 'BROADCASTING' }
]

const groupList = ref<ConsumerGroupItem[]>([
  {
    id: 1,
    group: 'order_group',
    cluster: '生产集群',
    consumeMode: 'CLUSTERING',
    status: 'online',
    onlineClients: 3,
    topicCount: 2,
    lag: 128,
    retryQps: 3,
    dlq: 0,
    maxRetry: 16,
    lastUpdate: '2026-02-09 16:18:20',
    remark: '订单核心消费组',
    subscriptions: [
      { topic: 'order_event', expression: '*', consumeTps: 412 },
      { topic: 'payment_result', expression: 'pay_success || pay_failed', consumeTps: 162 }
    ],
    clients: [
      { clientId: 'CID-order-01', ip: '10.0.1.11', version: '5.1.4', lastHeartbeat: '2026-02-09 16:18:19' },
      { clientId: 'CID-order-02', ip: '10.0.1.12', version: '5.1.4', lastHeartbeat: '2026-02-09 16:18:18' },
      { clientId: 'CID-order-03', ip: '10.0.1.13', version: '5.1.4', lastHeartbeat: '2026-02-09 16:18:18' }
    ]
  },
  {
    id: 2,
    group: 'notify_group',
    cluster: '测试集群',
    consumeMode: 'BROADCASTING',
    status: 'warning',
    onlineClients: 1,
    topicCount: 1,
    lag: 684,
    retryQps: 11,
    dlq: 4,
    maxRetry: 8,
    lastUpdate: '2026-02-09 16:15:02',
    remark: '通知系统消费组',
    subscriptions: [
      { topic: 'notify_event', expression: 'email || sms', consumeTps: 86 }
    ],
    clients: [
      { clientId: 'CID-notify-01', ip: '10.2.0.21', version: '5.1.3', lastHeartbeat: '2026-02-09 16:14:59' }
    ]
  },
  {
    id: 3,
    group: 'dev_test_group',
    cluster: '开发集群',
    consumeMode: 'CLUSTERING',
    status: 'offline',
    onlineClients: 0,
    topicCount: 1,
    lag: 0,
    retryQps: 0,
    dlq: 0,
    maxRetry: 5,
    lastUpdate: '2026-02-09 15:42:16',
    remark: '开发验证使用',
    subscriptions: [
      { topic: 'dev_test_topic', expression: '*', consumeTps: 0 }
    ],
    clients: []
  }
])

const keyword = ref('')
const selectedCluster = ref<string | null>(null)
const selectedStatus = ref<GroupStatus | null>(null)

const showEditor = ref(false)
const editingId = ref<number | null>(null)
const saving = ref(false)

const showDetail = ref(false)
const currentGroup = ref<ConsumerGroupItem | null>(null)

const formRef = ref<FormInst | null>(null)

const createEmptyForm = (): GroupFormModel => ({
  group: '',
  cluster: '开发集群',
  consumeMode: 'CLUSTERING',
  maxRetry: 16,
  topicsCsv: '',
  remark: ''
})

const formModel = ref<GroupFormModel>(createEmptyForm())

const formRules: FormRules = {
  group: [{ required: true, message: '请输入消费者组名称', trigger: ['blur', 'input'] }],
  cluster: [{ required: true, message: '请选择集群', trigger: ['change'] }],
  consumeMode: [{ required: true, message: '请选择消费模式', trigger: ['change'] }],
  maxRetry: [{ required: true, type: 'number', message: '请输入最大重试次数', trigger: ['blur', 'change'] }],
  topicsCsv: [{ required: true, message: '请输入订阅 Topic（逗号分隔）', trigger: ['blur', 'input'] }]
}

const filteredGroups = computed(() => {
  const search = keyword.value.trim().toLowerCase()
  return groupList.value.filter((item) => {
    const matchKeyword = !search
      || item.group.toLowerCase().includes(search)
      || item.subscriptions.some(sub => sub.topic.toLowerCase().includes(search))
    const matchCluster = !selectedCluster.value || item.cluster === selectedCluster.value
    const matchStatus = !selectedStatus.value || item.status === selectedStatus.value
    return matchKeyword && matchCluster && matchStatus
  })
})

const summary = computed(() => {
  const total = groupList.value.length
  const online = groupList.value.filter(item => item.status === 'online').length
  const warning = groupList.value.filter(item => item.status === 'warning').length
  const totalLag = groupList.value.reduce((sum, item) => sum + item.lag, 0)
  return { total, online, warning, totalLag }
})

const getStatusTagType = (status: GroupStatus) => {
  if (status === 'online') return 'success'
  if (status === 'warning') return 'warning'
  return 'error'
}

const getStatusText = (status: GroupStatus) => {
  if (status === 'online') return '在线'
  if (status === 'warning') return '告警'
  return '离线'
}

const openCreate = () => {
  editingId.value = null
  formModel.value = createEmptyForm()
  showEditor.value = true
}

const openEdit = (row: ConsumerGroupItem) => {
  editingId.value = row.id
  formModel.value = {
    group: row.group,
    cluster: row.cluster,
    consumeMode: row.consumeMode,
    maxRetry: row.maxRetry,
    topicsCsv: row.subscriptions.map(item => item.topic).join(', '),
    remark: row.remark
  }
  showEditor.value = true
}

const openDetail = (row: ConsumerGroupItem) => {
  currentGroup.value = row
  showDetail.value = true
}

const resetOffset = (id: number) => {
  const target = groupList.value.find(item => item.id === id)
  if (!target) return
  target.lag = Math.max(0, Math.floor(target.lag * 0.2))
  target.lastUpdate = new Date().toLocaleString()
  message.success(`已重置 ${target.group} 消费位点`)
}

const deleteGroup = (id: number) => {
  const removed = groupList.value.find(item => item.id === id)
  groupList.value = groupList.value.filter(item => item.id !== id)
  if (currentGroup.value?.id === id) {
    showDetail.value = false
    currentGroup.value = null
  }
  message.success(`已删除消费者组：${removed?.group ?? id}`)
}

const buildSubscriptionsFromCsv = (topicsCsv: string): GroupSubscription[] => {
  return topicsCsv
    .split(/[，,]/)
    .map(item => item.trim())
    .filter(Boolean)
    .map(topic => ({ topic, expression: '*', consumeTps: 0 }))
}

const saveGroup = async () => {
  if (!formRef.value) return
  await formRef.value.validate()
  saving.value = true

  const payload = formModel.value
  const subscriptions = buildSubscriptionsFromCsv(payload.topicsCsv)

  if (editingId.value === null) {
    groupList.value.unshift({
      id: Date.now(),
      group: payload.group,
      cluster: payload.cluster,
      consumeMode: payload.consumeMode,
      status: 'offline',
      onlineClients: 0,
      topicCount: subscriptions.length,
      lag: 0,
      retryQps: 0,
      dlq: 0,
      maxRetry: payload.maxRetry,
      lastUpdate: new Date().toLocaleString(),
      remark: payload.remark,
      subscriptions,
      clients: []
    })
    message.success('消费者组创建成功')
  } else {
    groupList.value = groupList.value.map(item => {
      if (item.id !== editingId.value) return item
      return {
        ...item,
        group: payload.group,
        cluster: payload.cluster,
        consumeMode: payload.consumeMode,
        maxRetry: payload.maxRetry,
        topicCount: subscriptions.length,
        subscriptions,
        remark: payload.remark,
        lastUpdate: new Date().toLocaleString()
      }
    })
    message.success('消费者组更新成功')
  }

  saving.value = false
  showEditor.value = false
}

const tableScrollX = 1460

const columns: DataTableColumns<ConsumerGroupItem> = [
  {
    title: '消费者组',
    key: 'group',
    minWidth: 170,
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'group-name' }, row.group)
  },
  { title: '集群', key: 'cluster', width: 120 },
  {
    title: '消费模式',
    key: 'consumeMode',
    width: 120,
    render: (row) => h(NTag, { size: 'small', round: true }, { default: () => row.consumeMode })
  },
  {
    title: '状态',
    key: 'status',
    width: 90,
    render: (row) => h(NTag, { size: 'small', round: true, type: getStatusTagType(row.status) }, { default: () => getStatusText(row.status) })
  },
  { title: '在线客户端', key: 'onlineClients', width: 95 },
  { title: '订阅 Topic', key: 'topicCount', width: 90 },
  { title: '堆积量', key: 'lag', width: 100 },
  { title: '重试QPS', key: 'retryQps', width: 90 },
  { title: 'DLQ', key: 'dlq', width: 70 },
  { title: '最近更新', key: 'lastUpdate', width: 170 },
  {
    title: '操作',
    key: 'actions',
    width: 260,
    render: (row) => h(NSpace, { size: 6 }, {
      default: () => [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openDetail(row) }, { default: () => '详情' }),
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
        h(
          NPopconfirm,
          { onPositiveClick: () => resetOffset(row.id) },
          {
            trigger: () => h(NButton, { size: 'tiny', quaternary: true }, { default: () => '重置位点' }),
            default: () => '确认重置该消费者组位点吗？'
          }
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => deleteGroup(row.id) },
          {
            trigger: () => h(NButton, { size: 'tiny', quaternary: true, type: 'error' }, { default: () => '删除' }),
            default: () => '确认删除该消费者组吗？'
          }
        )
      ]
    })
  }
]
</script>

<template>
  <div class="consumer-page">
    <n-grid responsive="screen" class="summary-grid" cols="1 s:4 l:4" :x-gap="12" :y-gap="12">
      <n-gi>
        <n-card size="small" hoverable class="summary-card">
          <div class="summary-label">消费者组总数</div>
          <div class="summary-value">{{ summary.total }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card size="small" hoverable class="summary-card">
          <div class="summary-label">在线组</div>
          <div class="summary-value success">{{ summary.online }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card size="small" hoverable class="summary-card">
          <div class="summary-label">告警组</div>
          <div class="summary-value warning">{{ summary.warning }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card size="small" hoverable class="summary-card">
          <div class="summary-label">总堆积量</div>
          <div class="summary-value">{{ summary.totalLag }}</div>
        </n-card>
      </n-gi>
    </n-grid>

    <n-card :bordered="false" class="table-card">
      <div class="toolbar">
        <n-space wrap>
          <n-button type="primary" @click="openCreate">创建消费者组</n-button>
          <n-input v-model:value="keyword" clearable placeholder="搜索组名 / Topic" style="width: 220px;" />
          <n-select v-model:value="selectedCluster" clearable :options="clusterOptions" placeholder="集群筛选"
            style="width: 140px;" />
          <n-select v-model:value="selectedStatus" clearable :options="statusOptions" placeholder="状态筛选"
            style="width: 120px;" />
        </n-space>
      </div>

      <n-data-table :columns="columns" :data="filteredGroups" :pagination="{ pageSize: 10 }" :single-line="true"
        :scroll-x="tableScrollX" size="small" />
    </n-card>

    <n-drawer v-model:show="showDetail" :width="680" placement="right" :trap-focus="false">
      <n-drawer-content title="消费者组详情" closable>
        <template v-if="currentGroup">
          <n-descriptions bordered :column="2" size="small" label-placement="left">
            <n-descriptions-item label="组名">{{ currentGroup.group }}</n-descriptions-item>
            <n-descriptions-item label="集群">{{ currentGroup.cluster }}</n-descriptions-item>
            <n-descriptions-item label="消费模式">{{ currentGroup.consumeMode }}</n-descriptions-item>
            <n-descriptions-item label="状态">
              <n-tag size="small" round :type="getStatusTagType(currentGroup.status)">{{
                getStatusText(currentGroup.status) }}</n-tag>
            </n-descriptions-item>
            <n-descriptions-item label="在线客户端">{{ currentGroup.onlineClients }}</n-descriptions-item>
            <n-descriptions-item label="最大重试">{{ currentGroup.maxRetry }}</n-descriptions-item>
            <n-descriptions-item label="堆积量">{{ currentGroup.lag }}</n-descriptions-item>
            <n-descriptions-item label="最近更新">{{ currentGroup.lastUpdate }}</n-descriptions-item>
            <n-descriptions-item label="备注" :span="2">{{ currentGroup.remark || '-' }}</n-descriptions-item>
          </n-descriptions>

          <div class="detail-block">
            <div class="detail-title">订阅关系</div>
            <n-table size="small" :single-line="false">
              <thead>
                <tr>
                  <th>Topic</th>
                  <th>过滤表达式</th>
                  <th>消费 TPS</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="sub in currentGroup.subscriptions" :key="sub.topic">
                  <td>{{ sub.topic }}</td>
                  <td>{{ sub.expression }}</td>
                  <td>{{ sub.consumeTps }}</td>
                </tr>
              </tbody>
            </n-table>
          </div>

          <div class="detail-block">
            <div class="detail-title">客户端列表</div>
            <n-table size="small" :single-line="false">
              <thead>
                <tr>
                  <th>ClientId</th>
                  <th>IP</th>
                  <th>版本</th>
                  <th>心跳时间</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="currentGroup.clients.length === 0">
                  <td colspan="4" class="empty-row">暂无在线客户端</td>
                </tr>
                <tr v-for="client in currentGroup.clients" :key="client.clientId">
                  <td>{{ client.clientId }}</td>
                  <td>{{ client.ip }}</td>
                  <td>{{ client.version }}</td>
                  <td>{{ client.lastHeartbeat }}</td>
                </tr>
              </tbody>
            </n-table>
          </div>
        </template>
      </n-drawer-content>
    </n-drawer>

    <n-modal v-model:show="showEditor" preset="card" :style="{ width: '640px', maxWidth: 'calc(100vw - 32px)' }"
      :title="editingId ? '编辑消费者组' : '创建消费者组'">
      <n-form ref="formRef" :model="formModel" :rules="formRules" label-placement="left" label-width="110">
        <n-form-item label="组名" path="group">
          <n-input v-model:value="formModel.group" placeholder="例如：order_group" :disabled="editingId !== null" />
        </n-form-item>

        <n-form-item label="集群" path="cluster">
          <n-select v-model:value="formModel.cluster" :options="clusterOptions" />
        </n-form-item>

        <n-grid cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="消费模式" path="consumeMode">
              <n-select v-model:value="formModel.consumeMode" :options="consumeModeOptions" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="最大重试" path="maxRetry">
              <n-input-number v-model:value="formModel.maxRetry" :min="0" :max="64" style="width: 100%;" />
            </n-form-item>
          </n-gi>
        </n-grid>

        <n-form-item label="订阅 Topic" path="topicsCsv">
          <n-input v-model:value="formModel.topicsCsv" placeholder="多个 Topic 用逗号分隔，如 order_event, payment_result" />
        </n-form-item>

        <n-form-item label="备注">
          <n-input v-model:value="formModel.remark" type="textarea" :rows="3" placeholder="可选备注" />
        </n-form-item>
      </n-form>

      <template #footer>
        <n-space justify="end">
          <n-button @click="showEditor = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="saveGroup">保存</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.consumer-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.summary-grid {
  margin-top: 2px;
}

.summary-card,
.table-card {
  background: var(--surface-2, #fff);
  border-radius: 5px;
}

.summary-label {
  font-size: 13px;
  color: var(--text-muted, #888);
}

.summary-value {
  margin-top: 8px;
  font-size: 30px;
  font-weight: 700;
  line-height: 1;
  color: var(--text-color, #333);
}

.summary-value.success {
  color: #18a058;
}

.summary-value.warning {
  color: #d97706;
}

.toolbar {
  margin-bottom: 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.group-name {
  font-weight: 600;
  color: var(--text-color, #333);
}

.detail-block {
  margin-top: 16px;
}

.detail-title {
  margin-bottom: 10px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-color, #333);
}

.empty-row {
  text-align: center;
  color: var(--text-muted, #888);
}
</style>
