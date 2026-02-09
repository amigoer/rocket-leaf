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
  useMessage
} from 'naive-ui'
import type { DataTableColumns, FormInst, FormRules } from 'naive-ui'

type TopicPerm = 'RW' | 'R' | 'W' | 'DENY'
type TopicMessageType = 'Normal' | 'FIFO' | 'Delay'

interface TopicRouteItem {
  broker: string
  brokerAddr: string
  readQueue: number
  writeQueue: number
  perm: TopicPerm
}

interface TopicItem {
  id: number
  topic: string
  cluster: string
  readQueue: number
  writeQueue: number
  perm: TopicPerm
  messageType: TopicMessageType
  consumerGroups: number
  tpsIn: number
  tpsOut: number
  lastUpdated: string
  description: string
  routes: TopicRouteItem[]
}

interface TopicFormModel {
  topic: string
  cluster: string
  readQueue: number
  writeQueue: number
  perm: TopicPerm
  messageType: TopicMessageType
  description: string
}

const clusterOptions = [
  { label: '生产集群', value: '生产集群' },
  { label: '测试集群', value: '测试集群' },
  { label: '开发集群', value: '开发集群' }
]

const permOptions = [
  { label: '读写 (RW)', value: 'RW' },
  { label: '只读 (R)', value: 'R' },
  { label: '只写 (W)', value: 'W' },
  { label: '禁用 (DENY)', value: 'DENY' }
]

const messageTypeOptions = [
  { label: '普通消息', value: 'Normal' },
  { label: '顺序消息', value: 'FIFO' },
  { label: '延迟消息', value: 'Delay' }
]

const topicList = ref<TopicItem[]>([
  {
    id: 1,
    topic: 'order_event',
    cluster: '生产集群',
    readQueue: 16,
    writeQueue: 16,
    perm: 'RW',
    messageType: 'Normal',
    consumerGroups: 4,
    tpsIn: 528,
    tpsOut: 503,
    lastUpdated: '2026-02-09 15:22:11',
    description: '订单生命周期事件流',
    routes: [
      { broker: 'broker-a', brokerAddr: '192.168.1.11:10911', readQueue: 8, writeQueue: 8, perm: 'RW' },
      { broker: 'broker-b', brokerAddr: '192.168.1.12:10911', readQueue: 8, writeQueue: 8, perm: 'RW' }
    ]
  },
  {
    id: 2,
    topic: 'payment_result',
    cluster: '生产集群',
    readQueue: 12,
    writeQueue: 12,
    perm: 'RW',
    messageType: 'FIFO',
    consumerGroups: 3,
    tpsIn: 214,
    tpsOut: 208,
    lastUpdated: '2026-02-09 15:18:46',
    description: '支付结果通知',
    routes: [
      { broker: 'broker-a', brokerAddr: '192.168.1.11:10911', readQueue: 6, writeQueue: 6, perm: 'RW' },
      { broker: 'broker-c', brokerAddr: '192.168.1.13:10911', readQueue: 6, writeQueue: 6, perm: 'RW' }
    ]
  },
  {
    id: 3,
    topic: 'dev_test_topic',
    cluster: '开发集群',
    readQueue: 4,
    writeQueue: 4,
    perm: 'R',
    messageType: 'Delay',
    consumerGroups: 1,
    tpsIn: 12,
    tpsOut: 8,
    lastUpdated: '2026-02-09 14:57:03',
    description: '开发环境测试 Topic',
    routes: [
      { broker: 'broker-dev', brokerAddr: '127.0.0.1:10911', readQueue: 4, writeQueue: 4, perm: 'R' }
    ]
  }
])

const keyword = ref('')
const selectedCluster = ref<string | null>(null)
const selectedPerm = ref<TopicPerm | null>(null)

const showEditor = ref(false)
const editingId = ref<number | null>(null)
const saving = ref(false)

const showDetail = ref(false)
const currentTopic = ref<TopicItem | null>(null)
const message = useMessage()

const formRef = ref<FormInst | null>(null)

const createEmptyForm = (): TopicFormModel => ({
  topic: '',
  cluster: '开发集群',
  readQueue: 4,
  writeQueue: 4,
  perm: 'RW',
  messageType: 'Normal',
  description: ''
})

const formModel = ref<TopicFormModel>(createEmptyForm())

const formRules: FormRules = {
  topic: [{ required: true, message: '请输入 Topic 名称', trigger: ['blur', 'input'] }],
  cluster: [{ required: true, message: '请选择集群', trigger: ['change'] }],
  readQueue: [{ required: true, type: 'number', message: '请输入读队列数', trigger: ['blur', 'change'] }],
  writeQueue: [{ required: true, type: 'number', message: '请输入写队列数', trigger: ['blur', 'change'] }],
  perm: [{ required: true, message: '请选择权限', trigger: ['change'] }],
  messageType: [{ required: true, message: '请选择消息类型', trigger: ['change'] }]
}

const filteredTopics = computed(() => {
  const search = keyword.value.trim().toLowerCase()
  return topicList.value.filter((item) => {
    const matchKeyword = !search
      || item.topic.toLowerCase().includes(search)
      || item.description.toLowerCase().includes(search)

    const matchCluster = !selectedCluster.value || item.cluster === selectedCluster.value
    const matchPerm = !selectedPerm.value || item.perm === selectedPerm.value

    return matchKeyword && matchCluster && matchPerm
  })
})

const summary = computed(() => {
  const total = topicList.value.length
  const rwCount = topicList.value.filter(item => item.perm === 'RW').length
  const totalTpsIn = topicList.value.reduce((sum, item) => sum + item.tpsIn, 0)
  const totalTpsOut = topicList.value.reduce((sum, item) => sum + item.tpsOut, 0)
  return { total, rwCount, totalTpsIn, totalTpsOut }
})

const openCreate = () => {
  editingId.value = null
  formModel.value = createEmptyForm()
  showEditor.value = true
}

const openEdit = (row: TopicItem) => {
  editingId.value = row.id
  formModel.value = {
    topic: row.topic,
    cluster: row.cluster,
    readQueue: row.readQueue,
    writeQueue: row.writeQueue,
    perm: row.perm,
    messageType: row.messageType,
    description: row.description
  }
  showEditor.value = true
}

const openDetail = (row: TopicItem) => {
  currentTopic.value = row
  showDetail.value = true
}

const removeTopic = (id: number) => {
  const removed = topicList.value.find(item => item.id === id)
  topicList.value = topicList.value.filter(item => item.id !== id)
  if (currentTopic.value?.id === id) {
    showDetail.value = false
    currentTopic.value = null
  }
  message.success(`已删除 Topic：${removed?.topic ?? id}`)
}

const saveTopic = async () => {
  if (!formRef.value) return
  await formRef.value.validate()
  saving.value = true

  const payload = formModel.value
  if (editingId.value === null) {
    topicList.value.unshift({
      id: Date.now(),
      topic: payload.topic,
      cluster: payload.cluster,
      readQueue: payload.readQueue,
      writeQueue: payload.writeQueue,
      perm: payload.perm,
      messageType: payload.messageType,
      consumerGroups: 0,
      tpsIn: 0,
      tpsOut: 0,
      lastUpdated: new Date().toLocaleString(),
      description: payload.description,
      routes: [
        {
          broker: `${payload.cluster}-broker-1`,
          brokerAddr: '127.0.0.1:10911',
          readQueue: payload.readQueue,
          writeQueue: payload.writeQueue,
          perm: payload.perm
        }
      ]
    })
  } else {
    topicList.value = topicList.value.map((item) => {
      if (item.id !== editingId.value) return item
      return {
        ...item,
        topic: payload.topic,
        cluster: payload.cluster,
        readQueue: payload.readQueue,
        writeQueue: payload.writeQueue,
        perm: payload.perm,
        messageType: payload.messageType,
        description: payload.description,
        lastUpdated: new Date().toLocaleString()
      }
    })
  }

  saving.value = false
  showEditor.value = false
}

const getPermTagType = (perm: TopicPerm) => {
  if (perm === 'RW') return 'success'
  if (perm === 'DENY') return 'error'
  return 'warning'
}

const getMessageTypeTagType = (type: TopicMessageType) => {
  if (type === 'FIFO') return 'info'
  if (type === 'Delay') return 'warning'
  return 'default'
}

const tableScrollX = 1240

const columns: DataTableColumns<TopicItem> = [
  {
    title: 'Topic',
    key: 'topic',
    minWidth: 180,
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'topic-name' }, row.topic)
  },
  { title: '集群', key: 'cluster', width: 120 },
  {
    title: '读写队列',
    key: 'queues',
    width: 130,
    render: (row) => `${row.readQueue}/${row.writeQueue}`
  },
  {
    title: '权限',
    key: 'perm',
    width: 90,
    render: (row) => h(NTag, { size: 'small', round: true, type: getPermTagType(row.perm) }, { default: () => row.perm })
  },
  {
    title: '消息类型',
    key: 'messageType',
    width: 100,
    render: (row) => h(NTag, { size: 'small', round: true, type: getMessageTypeTagType(row.messageType) }, { default: () => row.messageType })
  },
  { title: '消费者组', key: 'consumerGroups', width: 90 },
  {
    title: 'TPS(入/出)',
    key: 'tps',
    width: 120,
    render: (row) => `${row.tpsIn}/${row.tpsOut}`
  },
  { title: '最近更新', key: 'lastUpdated', width: 170 },
  {
    title: '操作',
    key: 'actions',
    width: 220,
    render: (row) => h(NSpace, { size: 6 }, {
      default: () => [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openDetail(row) }, { default: () => '详情' }),
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
        h(
          NPopconfirm,
          { onPositiveClick: () => removeTopic(row.id) },
          {
            trigger: () => h(NButton, { size: 'tiny', quaternary: true, type: 'error' }, { default: () => '删除' }),
            default: () => '确认删除该 Topic 吗？'
          }
        )
      ]
    })
  }
]
</script>

<template>
  <div class="topic-page">
    <n-grid responsive="screen" cols="1 s:4 l:4" class="summary-grid" :x-gap="12" :y-gap="12">
      <n-gi>
        <n-card hoverable size="small" class="summary-card">
          <div class="summary-label">Topic 总数</div>
          <div class="summary-value">{{ summary.total }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card hoverable size="small" class="summary-card">
          <div class="summary-label">可读写 Topic</div>
          <div class="summary-value success">{{ summary.rwCount }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card hoverable size="small" class="summary-card">
          <div class="summary-label">总入流 TPS</div>
          <div class="summary-value">{{ summary.totalTpsIn }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card hoverable size="small" class="summary-card">
          <div class="summary-label">总出流 TPS</div>
          <div class="summary-value">{{ summary.totalTpsOut }}</div>
        </n-card>
      </n-gi>
    </n-grid>

    <n-card :bordered="false" class="table-card">
      <div class="toolbar">
        <n-space wrap>
          <n-button type="primary" @click="openCreate">创建 Topic</n-button>
          <n-input v-model:value="keyword" clearable placeholder="搜索 Topic 名称 / 描述" style="width: 240px;" />
          <n-select v-model:value="selectedCluster" clearable :options="clusterOptions" placeholder="集群筛选"
            style="width: 140px;" />
          <n-select v-model:value="selectedPerm" clearable :options="permOptions" placeholder="权限筛选"
            style="width: 130px;" />
        </n-space>
      </div>

      <n-data-table :columns="columns" :data="filteredTopics" :pagination="{ pageSize: 10 }" :single-line="true"
        :scroll-x="tableScrollX" size="small" />
    </n-card>

    <n-drawer v-model:show="showDetail" :width="640" placement="right" :trap-focus="false">
      <n-drawer-content title="Topic 详情" closable>
        <template v-if="currentTopic">
          <n-descriptions bordered :column="2" size="small" label-placement="left">
            <n-descriptions-item label="Topic">{{ currentTopic.topic }}</n-descriptions-item>
            <n-descriptions-item label="集群">{{ currentTopic.cluster }}</n-descriptions-item>
            <n-descriptions-item label="读写队列">{{ currentTopic.readQueue }}/{{ currentTopic.writeQueue
            }}</n-descriptions-item>
            <n-descriptions-item label="权限">
              <n-tag size="small" round :type="getPermTagType(currentTopic.perm)">{{ currentTopic.perm }}</n-tag>
            </n-descriptions-item>
            <n-descriptions-item label="消息类型">
              <n-tag size="small" round :type="getMessageTypeTagType(currentTopic.messageType)">{{
                currentTopic.messageType }}</n-tag>
            </n-descriptions-item>
            <n-descriptions-item label="消费者组">{{ currentTopic.consumerGroups }}</n-descriptions-item>
            <n-descriptions-item label="TPS(入/出)">{{ currentTopic.tpsIn }}/{{ currentTopic.tpsOut
            }}</n-descriptions-item>
            <n-descriptions-item label="最近更新">{{ currentTopic.lastUpdated }}</n-descriptions-item>
            <n-descriptions-item label="描述" :span="2">{{ currentTopic.description || '-' }}</n-descriptions-item>
          </n-descriptions>

          <div class="route-block">
            <div class="route-title">路由信息</div>
            <n-table size="small" :single-line="false">
              <thead>
                <tr>
                  <th>Broker</th>
                  <th>地址</th>
                  <th>读队列</th>
                  <th>写队列</th>
                  <th>权限</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="route in currentTopic.routes" :key="`${route.broker}-${route.brokerAddr}`">
                  <td>{{ route.broker }}</td>
                  <td>{{ route.brokerAddr }}</td>
                  <td>{{ route.readQueue }}</td>
                  <td>{{ route.writeQueue }}</td>
                  <td>
                    <n-tag size="small" round :type="getPermTagType(route.perm)">{{ route.perm }}</n-tag>
                  </td>
                </tr>
              </tbody>
            </n-table>
          </div>
        </template>
      </n-drawer-content>
    </n-drawer>

    <n-modal v-model:show="showEditor" preset="card" class="editor-modal"
      :style="{ width: '620px', maxWidth: 'calc(100vw - 32px)' }" :title="editingId ? '编辑 Topic' : '创建 Topic'">
      <n-form ref="formRef" :model="formModel" :rules="formRules" label-placement="left" label-width="92">
        <n-form-item label="Topic 名称" path="topic">
          <n-input v-model:value="formModel.topic" placeholder="例如：order_event" :disabled="editingId !== null" />
        </n-form-item>

        <n-form-item label="集群" path="cluster">
          <n-select v-model:value="formModel.cluster" :options="clusterOptions" />
        </n-form-item>

        <n-grid cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="读队列" path="readQueue">
              <n-input-number v-model:value="formModel.readQueue" :min="1" :max="64" style="width: 100%;" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="写队列" path="writeQueue">
              <n-input-number v-model:value="formModel.writeQueue" :min="1" :max="64" style="width: 100%;" />
            </n-form-item>
          </n-gi>
        </n-grid>

        <n-grid cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="权限" path="perm">
              <n-select v-model:value="formModel.perm" :options="permOptions" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="消息类型" path="messageType">
              <n-select v-model:value="formModel.messageType" :options="messageTypeOptions" />
            </n-form-item>
          </n-gi>
        </n-grid>

        <n-form-item label="描述">
          <n-input v-model:value="formModel.description" type="textarea" :rows="3" placeholder="可选描述" />
        </n-form-item>
      </n-form>

      <template #footer>
        <n-space justify="end">
          <n-button @click="showEditor = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="saveTopic">保存</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.topic-page {
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

.toolbar {
  margin-bottom: 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.topic-name {
  font-weight: 600;
  color: var(--text-color, #333);
}

.route-block {
  margin-top: 16px;
}

.route-title {
  margin-bottom: 10px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-color, #333);
}
</style>
