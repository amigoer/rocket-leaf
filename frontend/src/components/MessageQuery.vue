<script setup lang="ts">
import { computed, h, ref } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NDatePicker,
  NDescriptions,
  NDescriptionsItem,
  NDrawer,
  NDrawerContent,
  NForm,
  NFormItem,
  NGrid,
  NGi,
  NInput,
  NPopconfirm,
  NSelect,
  NSpace,
  NTag,
  useMessage
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { CopyOutline, EyeOutline, RefreshOutline, SearchOutline } from '@vicons/ionicons5'

type MessageStatus = 'normal' | 'retry' | 'dlq'

interface MessageItem {
  id: number
  cluster: string
  topic: string
  messageId: string
  tags: string
  keys: string
  producerGroup: string
  queueId: number
  queueOffset: number
  storeHost: string
  bornHost: string
  storeTime: string
  storeTimestamp: number
  status: MessageStatus
  retryTimes: number
  body: string
  properties: Record<string, string>
}

const message = useMessage()

const clusterOptions = [
  { label: '生产集群', value: '生产集群' },
  { label: '测试集群', value: '测试集群' },
  { label: '开发集群', value: '开发集群' }
]

const statusOptions = [
  { label: '正常', value: 'normal' },
  { label: '重试中', value: 'retry' },
  { label: '死信', value: 'dlq' }
]

const messageList = ref<MessageItem[]>([
  {
    id: 1,
    cluster: '生产集群',
    topic: 'order_event',
    messageId: '0A0A0A01E5A30000000000000001',
    tags: 'order.created',
    keys: 'ORDER_20260209_10001',
    producerGroup: 'order_producer_group',
    queueId: 2,
    queueOffset: 128930,
    storeHost: '10.10.1.21:10911',
    bornHost: '10.20.3.15:52931',
    storeTime: '2026-02-09 16:32:20',
    storeTimestamp: new Date('2026-02-09T16:32:20').getTime(),
    status: 'normal',
    retryTimes: 0,
    body: '{"orderId":10001,"amount":199.00,"status":"CREATED"}',
    properties: {
      UNIQ_KEY: 'ORDER_20260209_10001',
      CLUSTER: 'PROD',
      TRACE_ON: 'true'
    }
  },
  {
    id: 2,
    cluster: '生产集群',
    topic: 'payment_result',
    messageId: '0A0A0A01E5A30000000000000002',
    tags: 'payment.success',
    keys: 'PAY_20260209_55661',
    producerGroup: 'payment_producer_group',
    queueId: 0,
    queueOffset: 73621,
    storeHost: '10.10.1.22:10911',
    bornHost: '10.20.2.11:48620',
    storeTime: '2026-02-09 16:30:08',
    storeTimestamp: new Date('2026-02-09T16:30:08').getTime(),
    status: 'retry',
    retryTimes: 2,
    body: '{"paymentId":55661,"orderId":10001,"result":"SUCCESS"}',
    properties: {
      RETRY_TOPIC: '%RETRY%payment_group',
      MAX_RECONSUME_TIMES: '16',
      REGION: 'cn-hz'
    }
  },
  {
    id: 3,
    cluster: '测试集群',
    topic: 'notify_event',
    messageId: '0A0A0A01E5A30000000000000003',
    tags: 'sms',
    keys: 'NOTIFY_20260209_90021',
    producerGroup: 'notify_producer_group',
    queueId: 1,
    queueOffset: 1923,
    storeHost: '10.11.0.13:10911',
    bornHost: '10.31.1.8:39812',
    storeTime: '2026-02-09 16:14:09',
    storeTimestamp: new Date('2026-02-09T16:14:09').getTime(),
    status: 'normal',
    retryTimes: 0,
    body: '{"channel":"sms","phone":"138****8899","template":"login_code"}',
    properties: {
      SHARDING_KEY: 'sms_01',
      DELAY_LEVEL: '0'
    }
  },
  {
    id: 4,
    cluster: '测试集群',
    topic: 'notify_event',
    messageId: '0A0A0A01E5A30000000000000004',
    tags: 'email',
    keys: 'NOTIFY_20260209_90022',
    producerGroup: 'notify_producer_group',
    queueId: 3,
    queueOffset: 1944,
    storeHost: '10.11.0.13:10911',
    bornHost: '10.31.1.9:40892',
    storeTime: '2026-02-09 16:08:47',
    storeTimestamp: new Date('2026-02-09T16:08:47').getTime(),
    status: 'dlq',
    retryTimes: 16,
    body: '{"channel":"email","to":"dev@example.com","template":"welcome"}',
    properties: {
      RETRY_TOPIC: '%RETRY%notify_group',
      DEAD_LETTER_QUEUE: '%DLQ%notify_group',
      BORN_TIMESTAMP: '1739088510000'
    }
  },
  {
    id: 5,
    cluster: '开发集群',
    topic: 'dev_test_topic',
    messageId: '0A0A0A01E5A30000000000000005',
    tags: 'debug',
    keys: 'DEV_20260209_01',
    producerGroup: 'dev_producer_group',
    queueId: 0,
    queueOffset: 188,
    storeHost: '127.0.0.1:10911',
    bornHost: '127.0.0.1:56123',
    storeTime: '2026-02-09 15:58:10',
    storeTimestamp: new Date('2026-02-09T15:58:10').getTime(),
    status: 'normal',
    retryTimes: 0,
    body: '{"scene":"debug","payload":"hello rocket leaf"}',
    properties: {
      TRACE_ON: 'false',
      REGION: 'local'
    }
  }
])

const selectedCluster = ref<string | null>(null)
const selectedTopic = ref<string | null>(null)
const selectedStatus = ref<MessageStatus | null>(null)
const keyword = ref('')
const messageId = ref('')
const messageKey = ref('')
const timeRange = ref<[number, number] | null>(null)

const showDetailDrawer = ref(false)
const currentMessage = ref<MessageItem | null>(null)

const topicOptions = computed(() => {
  const list = selectedCluster.value
    ? messageList.value.filter(item => item.cluster === selectedCluster.value)
    : messageList.value

  const uniqueTopics = Array.from(new Set(list.map(item => item.topic)))
  return uniqueTopics.map(topic => ({ label: topic, value: topic }))
})

const summary = computed(() => {
  const total = filteredMessages.value.length
  const retry = filteredMessages.value.filter(item => item.status === 'retry').length
  const dlq = filteredMessages.value.filter(item => item.status === 'dlq').length
  return {
    total,
    retry,
    dlq
  }
})

const filteredMessages = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  const messageIdKeyword = messageId.value.trim().toLowerCase()
  const messageKeyKeyword = messageKey.value.trim().toLowerCase()

  return messageList.value.filter((item) => {
    const matchCluster = !selectedCluster.value || item.cluster === selectedCluster.value
    const matchTopic = !selectedTopic.value || item.topic === selectedTopic.value
    const matchStatus = !selectedStatus.value || item.status === selectedStatus.value

    const matchKeyword = !kw
      || item.topic.toLowerCase().includes(kw)
      || item.tags.toLowerCase().includes(kw)
      || item.keys.toLowerCase().includes(kw)
      || item.messageId.toLowerCase().includes(kw)

    const matchMessageId = !messageIdKeyword || item.messageId.toLowerCase().includes(messageIdKeyword)
    const matchMessageKey = !messageKeyKeyword || item.keys.toLowerCase().includes(messageKeyKeyword)

    const matchTime = !timeRange.value
      || (item.storeTimestamp >= timeRange.value[0] && item.storeTimestamp <= timeRange.value[1])

    return matchCluster && matchTopic && matchStatus && matchKeyword && matchMessageId && matchMessageKey && matchTime
  })
})

const getStatusTagType = (status: MessageStatus) => {
  if (status === 'normal') return 'success'
  if (status === 'retry') return 'warning'
  return 'error'
}

const getStatusText = (status: MessageStatus) => {
  if (status === 'normal') return '正常'
  if (status === 'retry') return '重试中'
  return '死信'
}

const handleReset = () => {
  selectedCluster.value = null
  selectedTopic.value = null
  selectedStatus.value = null
  keyword.value = ''
  messageId.value = ''
  messageKey.value = ''
  timeRange.value = null
  message.success('筛选条件已重置')
}

const handleSearch = () => {
  message.success(`查询完成，共 ${filteredMessages.value.length} 条消息`)
}

const handlePreview = (row: MessageItem) => {
  currentMessage.value = row
  showDetailDrawer.value = true
}

const handleResend = (row: MessageItem) => {
  message.success(`已提交重投：${row.messageId}`)
}

const handleCopyMessageId = async (value: string) => {
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
      message.success('Message ID 已复制')
      return
    }
    message.info(`请手动复制：${value}`)
  } catch {
    message.error('复制失败，请手动复制')
  }
}

const columns: DataTableColumns<MessageItem> = [
  {
    title: 'Message ID',
    key: 'messageId',
    minWidth: 220,
    render: row => h('span', { class: 'mono-text' }, row.messageId)
  },
  {
    title: 'Topic',
    key: 'topic',
    width: 150
  },
  {
    title: 'Tag',
    key: 'tags',
    width: 130
  },
  {
    title: 'Key',
    key: 'keys',
    minWidth: 170,
    render: row => h('span', { class: 'mono-text' }, row.keys)
  },
  {
    title: '队列',
    key: 'queue',
    width: 120,
    render: row => `Q${row.queueId} / ${row.queueOffset}`
  },
  {
    title: '状态',
    key: 'status',
    width: 96,
    render: row => h(
      NTag,
      {
        size: 'small',
        type: getStatusTagType(row.status),
        round: true
      },
      { default: () => getStatusText(row.status) }
    )
  },
  {
    title: '存储时间',
    key: 'storeTime',
    width: 170
  },
  {
    title: '操作',
    key: 'actions',
    width: 190,
    fixed: 'right',
    render: row => h(
      NSpace,
      { size: 6 },
      {
        default: () => [
          h(
            NButton,
            {
              size: 'tiny',
              quaternary: true,
              onClick: () => handlePreview(row)
            },
            {
              icon: () => h(NIconProxy, { icon: EyeOutline }),
              default: () => '详情'
            }
          ),
          h(
            NButton,
            {
              size: 'tiny',
              quaternary: true,
              onClick: () => handleCopyMessageId(row.messageId)
            },
            {
              icon: () => h(NIconProxy, { icon: CopyOutline }),
              default: () => '复制'
            }
          ),
          h(
            NPopconfirm,
            {
              onPositiveClick: () => handleResend(row)
            },
            {
              trigger: () => h(
                NButton,
                { size: 'tiny', type: 'warning', quaternary: true },
                {
                  icon: () => h(NIconProxy, { icon: RefreshOutline }),
                  default: () => '重投'
                }
              ),
              default: () => '确认重新投递该消息吗？'
            }
          )
        ]
      }
    )
  }
]

const NIconProxy = (props: { icon: any }) => h('span', { class: 'icon-proxy' }, [h(props.icon)])
</script>

<template>
  <div class="message-page">
    <n-alert type="info" :show-icon="false" class="page-hint">
      当前为消息查询模拟页，字段与交互对齐 RocketMQ Dashboard 常见使用流程。
    </n-alert>

    <n-grid responsive="screen" cols="1 s:3" :x-gap="12" :y-gap="12" class="summary-grid">
      <n-gi>
        <n-card size="small" :bordered="false" class="summary-card">
          <div class="summary-label">查询结果</div>
          <div class="summary-value">{{ summary.total }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card size="small" :bordered="false" class="summary-card">
          <div class="summary-label">重试中</div>
          <div class="summary-value is-warning">{{ summary.retry }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card size="small" :bordered="false" class="summary-card">
          <div class="summary-label">死信消息</div>
          <div class="summary-value is-error">{{ summary.dlq }}</div>
        </n-card>
      </n-gi>
    </n-grid>

    <n-card :bordered="false" class="panel-card">
      <n-form label-placement="left" :show-feedback="false">
        <n-grid responsive="screen" cols="1 s:2 m:3 l:6" :x-gap="12" :y-gap="10">
          <n-gi>
            <n-form-item label="集群">
              <n-select
                v-model:value="selectedCluster"
                :options="clusterOptions"
                clearable
                placeholder="全部集群"
                @update:value="selectedTopic = null"
              />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="Topic">
              <n-select v-model:value="selectedTopic" :options="topicOptions" clearable placeholder="全部 Topic" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="状态">
              <n-select v-model:value="selectedStatus" :options="statusOptions" clearable placeholder="全部状态" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="关键字">
              <n-input v-model:value="keyword" placeholder="Topic / Tag / Key / Message ID" clearable />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="Message ID">
              <n-input v-model:value="messageId" placeholder="支持模糊匹配" clearable />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="Message Key">
              <n-input v-model:value="messageKey" placeholder="支持模糊匹配" clearable />
            </n-form-item>
          </n-gi>
          <n-gi :span="2">
            <n-form-item label="存储时间">
              <n-date-picker
                v-model:value="timeRange"
                type="datetimerange"
                clearable
                style="width: 100%"
                :actions="['clear', 'confirm']"
              />
            </n-form-item>
          </n-gi>
          <n-gi :span="1">
            <n-form-item label="操作" class="action-form-item">
              <n-space>
                <n-button type="primary" @click="handleSearch">
                  <template #icon>
                    <NIconProxy :icon="SearchOutline" />
                  </template>
                  查询
                </n-button>
                <n-button @click="handleReset">重置</n-button>
              </n-space>
            </n-form-item>
          </n-gi>
        </n-grid>
      </n-form>
    </n-card>

    <n-card :bordered="false" class="panel-card table-card" title="消息列表">
      <n-data-table
        :columns="columns"
        :data="filteredMessages"
        :row-key="(row: MessageItem) => row.id"
        size="small"
        striped
        flex-height
        max-height="520"
      />
    </n-card>

    <n-drawer v-model:show="showDetailDrawer" :width="560" placement="right" resizable>
      <n-drawer-content title="消息详情" closable>
        <template v-if="currentMessage">
          <n-descriptions label-placement="left" :column="1" bordered size="small">
            <n-descriptions-item label="Message ID">
              <span class="mono-text">{{ currentMessage.messageId }}</span>
            </n-descriptions-item>
            <n-descriptions-item label="Topic">{{ currentMessage.topic }}</n-descriptions-item>
            <n-descriptions-item label="Tag">{{ currentMessage.tags }}</n-descriptions-item>
            <n-descriptions-item label="Key">
              <span class="mono-text">{{ currentMessage.keys }}</span>
            </n-descriptions-item>
            <n-descriptions-item label="Producer Group">{{ currentMessage.producerGroup }}</n-descriptions-item>
            <n-descriptions-item label="队列位点">
              Q{{ currentMessage.queueId }} / {{ currentMessage.queueOffset }}
            </n-descriptions-item>
            <n-descriptions-item label="状态">
              <n-tag size="small" :type="getStatusTagType(currentMessage.status)" round>
                {{ getStatusText(currentMessage.status) }}
              </n-tag>
            </n-descriptions-item>
            <n-descriptions-item label="重试次数">{{ currentMessage.retryTimes }}</n-descriptions-item>
            <n-descriptions-item label="存储节点">
              <span class="mono-text">{{ currentMessage.storeHost }}</span>
            </n-descriptions-item>
            <n-descriptions-item label="生产节点">
              <span class="mono-text">{{ currentMessage.bornHost }}</span>
            </n-descriptions-item>
            <n-descriptions-item label="存储时间">{{ currentMessage.storeTime }}</n-descriptions-item>
          </n-descriptions>

          <n-card title="消息属性" size="small" :bordered="false" class="detail-block">
            <div v-for="(value, key) in currentMessage.properties" :key="key" class="property-row">
              <span class="property-key">{{ key }}</span>
              <span class="property-value">{{ value }}</span>
            </div>
          </n-card>

          <n-card title="消息体" size="small" :bordered="false" class="detail-block">
            <pre class="message-body">{{ currentMessage.body }}</pre>
          </n-card>
        </template>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.message-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.page-hint {
  border-radius: 10px;
}

.summary-grid {
  margin-top: 2px;
}

.summary-card,
.panel-card {
  border-radius: 12px;
  background: var(--surface-2, #fff);
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.03);
}

.summary-label {
  font-size: 12px;
  color: var(--text-muted, #888);
}

.summary-value {
  margin-top: 6px;
  font-size: 24px;
  line-height: 1.2;
  font-weight: 700;
  color: #18a058;
}

.summary-value.is-warning {
  color: #f0a020;
}

.summary-value.is-error {
  color: #d03050;
}

.table-card {
  min-height: 360px;
}

.action-form-item :deep(.n-form-item-blank) {
  align-items: center;
}

.mono-text {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
}

.detail-block {
  margin-top: 12px;
  border-radius: 10px;
  background: var(--surface-1, #f8f8f8);
}

.property-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  padding: 6px 0;
  border-bottom: 1px dashed var(--border-color, rgba(0, 0, 0, 0.08));
}

.property-row:last-child {
  border-bottom: none;
}

.property-key {
  font-size: 12px;
  color: var(--text-muted, #888);
}

.property-value {
  font-size: 12px;
  color: var(--text-color, #333);
  word-break: break-all;
  text-align: right;
}

.message-body {
  margin: 0;
  padding: 10px;
  font-size: 12px;
  line-height: 1.5;
  border-radius: 8px;
  color: var(--text-color, #333);
  background: var(--bg-color, #fff);
  border: 1px solid var(--border-color, rgba(0, 0, 0, 0.06));
  white-space: pre-wrap;
  word-break: break-all;
}

.icon-proxy {
  display: inline-flex;
  line-height: 0;
}

.icon-proxy :deep(svg) {
  width: 14px;
  height: 14px;
}
</style>
