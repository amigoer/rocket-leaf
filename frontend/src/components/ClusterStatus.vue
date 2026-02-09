<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  NButton,
  NCard,
  NDataTable,
  NDescriptions,
  NDescriptionsItem,
  NDrawer,
  NDrawerContent,
  NGrid,
  NGi,
  NInput,
  NProgress,
  NSelect,
  NSpace,
  NTag,
  useMessage
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'

type NodeStatus = 'online' | 'warning' | 'offline'
type BrokerRole = 'MASTER' | 'SLAVE'

interface NameServerNode {
  id: number
  cluster: string
  address: string
  version: string
  status: NodeStatus
  lastSeen: string
}

interface BrokerNode {
  id: number
  cluster: string
  brokerName: string
  brokerId: number
  role: BrokerRole
  address: string
  haAddress: string
  version: string
  status: NodeStatus
  topics: number
  groups: number
  tpsIn: number
  tpsOut: number
  tpsInHistory: number[]
  tpsOutHistory: number[]
  msgInToday: number
  msgOutToday: number
  commitLogDiskUsage: number
  consumeQueueDiskUsage: number
  lastUpdate: string
  remark: string
}

type BrokerSeed = Omit<BrokerNode, 'tpsInHistory' | 'tpsOutHistory'>

const message = useMessage()

const HISTORY_POINTS = 12
const AUTO_REFRESH_MS = 5000

const pad2 = (value: number) => String(value).padStart(2, '0')

const formatDateTime = (date: Date) => {
  const y = date.getFullYear()
  const m = pad2(date.getMonth() + 1)
  const d = pad2(date.getDate())
  const hh = pad2(date.getHours())
  const mm = pad2(date.getMinutes())
  const ss = pad2(date.getSeconds())
  return `${y}-${m}-${d} ${hh}:${mm}:${ss}`
}

const clamp = (value: number, min: number, max: number) => {
  return Math.max(min, Math.min(max, value))
}

const seedHistory = (base: number) => {
  if (base <= 0) {
    return Array.from({ length: HISTORY_POINTS }, () => 0)
  }
  return Array.from({ length: HISTORY_POINTS }, (_, index) => {
    const wave = Math.sin(index / 2.4) * 0.08
    const jitter = (Math.random() - 0.5) * 0.12
    return Math.max(0, Math.round(base * (1 + wave + jitter)))
  })
}

const createBroker = (seed: BrokerSeed): BrokerNode => ({
  ...seed,
  tpsInHistory: seedHistory(seed.tpsIn),
  tpsOutHistory: seedHistory(seed.tpsOut)
})

const buildSparklinePoints = (series: number[]) => {
  if (!series.length) return ''
  if (series.length === 1) return `0,12 100,12`
  const max = Math.max(...series, 1)
  const min = Math.min(...series)
  const range = Math.max(max - min, 1)

  return series
    .map((value, index) => {
      const x = (index / (series.length - 1)) * 100
      const y = 22 - ((value - min) / range) * 18
      return `${x.toFixed(2)},${y.toFixed(2)}`
    })
    .join(' ')
}

const nameServerList = ref<NameServerNode[]>([
  {
    id: 1,
    cluster: '生产集群',
    address: '10.0.1.10:9876',
    version: '5.1.4',
    status: 'online',
    lastSeen: '2026-02-09 16:52:21'
  },
  {
    id: 2,
    cluster: '生产集群',
    address: '10.0.1.11:9876',
    version: '5.1.4',
    status: 'online',
    lastSeen: '2026-02-09 16:52:18'
  },
  {
    id: 3,
    cluster: '测试集群',
    address: '10.2.0.10:9876',
    version: '5.1.3',
    status: 'warning',
    lastSeen: '2026-02-09 16:51:39'
  },
  {
    id: 4,
    cluster: '开发集群',
    address: '127.0.0.1:9876',
    version: '5.1.2',
    status: 'online',
    lastSeen: '2026-02-09 16:50:10'
  }
])

const brokerList = ref<BrokerNode[]>([
  createBroker({
    id: 1,
    cluster: '生产集群',
    brokerName: 'broker-prod-a',
    brokerId: 0,
    role: 'MASTER',
    address: '10.0.1.21:10911',
    haAddress: '10.0.1.21:10912',
    version: '5.1.4',
    status: 'online',
    topics: 63,
    groups: 28,
    tpsIn: 1680,
    tpsOut: 1542,
    msgInToday: 10820431,
    msgOutToday: 10322764,
    commitLogDiskUsage: 62,
    consumeQueueDiskUsage: 48,
    lastUpdate: '2026-02-09 16:52:16',
    remark: '生产主节点，业务高峰时段负载较高'
  }),
  createBroker({
    id: 2,
    cluster: '生产集群',
    brokerName: 'broker-prod-b',
    brokerId: 1,
    role: 'SLAVE',
    address: '10.0.1.22:10911',
    haAddress: '10.0.1.22:10912',
    version: '5.1.4',
    status: 'online',
    topics: 63,
    groups: 28,
    tpsIn: 1360,
    tpsOut: 1291,
    msgInToday: 9623411,
    msgOutToday: 9432754,
    commitLogDiskUsage: 59,
    consumeQueueDiskUsage: 45,
    lastUpdate: '2026-02-09 16:52:09',
    remark: '生产从节点，数据复制稳定'
  }),
  createBroker({
    id: 3,
    cluster: '测试集群',
    brokerName: 'broker-test-a',
    brokerId: 0,
    role: 'MASTER',
    address: '10.2.0.21:10911',
    haAddress: '10.2.0.21:10912',
    version: '5.1.3',
    status: 'warning',
    topics: 21,
    groups: 14,
    tpsIn: 342,
    tpsOut: 319,
    msgInToday: 1204311,
    msgOutToday: 1132764,
    commitLogDiskUsage: 84,
    consumeQueueDiskUsage: 66,
    lastUpdate: '2026-02-09 16:51:43',
    remark: '磁盘水位偏高，建议近期扩容'
  }),
  createBroker({
    id: 4,
    cluster: '开发集群',
    brokerName: 'broker-dev-a',
    brokerId: 0,
    role: 'MASTER',
    address: '127.0.0.1:10911',
    haAddress: '127.0.0.1:10912',
    version: '5.1.2',
    status: 'offline',
    topics: 8,
    groups: 5,
    tpsIn: 0,
    tpsOut: 0,
    msgInToday: 0,
    msgOutToday: 0,
    commitLogDiskUsage: 0,
    consumeQueueDiskUsage: 0,
    lastUpdate: '2026-02-09 15:38:21',
    remark: '本地环境暂未启动'
  })
])

const selectedCluster = ref<string | null>(null)
const selectedStatus = ref<NodeStatus | null>(null)
const keyword = ref('')

const showDetailDrawer = ref(false)
const currentBroker = ref<BrokerNode | null>(null)

const autoRefreshEnabled = ref(true)
const lastRefreshAt = ref(formatDateTime(new Date()))

let refreshTimer: ReturnType<typeof setInterval> | null = null

const clusterOptions = computed(() => {
  const clusters = Array.from(new Set(brokerList.value.map(item => item.cluster)))
  return clusters.map(cluster => ({ label: cluster, value: cluster }))
})

const statusOptions = [
  { label: '在线', value: 'online' },
  { label: '告警', value: 'warning' },
  { label: '离线', value: 'offline' }
]

const filteredBrokers = computed(() => {
  const search = keyword.value.trim().toLowerCase()
  return brokerList.value.filter((item) => {
    const matchCluster = !selectedCluster.value || item.cluster === selectedCluster.value
    const matchStatus = !selectedStatus.value || item.status === selectedStatus.value
    const matchKeyword = !search
      || item.brokerName.toLowerCase().includes(search)
      || item.address.toLowerCase().includes(search)
      || item.version.toLowerCase().includes(search)
    return matchCluster && matchStatus && matchKeyword
  })
})

const summary = computed(() => {
  const totalClusters = new Set(brokerList.value.map(item => item.cluster)).size
  const totalBrokers = brokerList.value.length
  const onlineBrokers = brokerList.value.filter(item => item.status === 'online').length
  const warningBrokers = brokerList.value.filter(item => item.status === 'warning').length
  const offlineBrokers = brokerList.value.filter(item => item.status === 'offline').length
  const avgDiskUsage = totalBrokers === 0
    ? 0
    : Math.round(brokerList.value.reduce((sum, item) => sum + item.commitLogDiskUsage, 0) / totalBrokers)

  return {
    totalClusters,
    totalBrokers,
    onlineBrokers,
    warningBrokers,
    offlineBrokers,
    avgDiskUsage
  }
})

const clusterRuntimeItems = computed(() => {
  const grouped = new Map<string, BrokerNode[]>()
  brokerList.value.forEach((item) => {
    const list = grouped.get(item.cluster) ?? []
    list.push(item)
    grouped.set(item.cluster, list)
  })

  return Array.from(grouped.entries()).map(([cluster, items]) => {
    const onlineCount = items.filter(item => item.status === 'online').length
    const warningCount = items.filter(item => item.status === 'warning').length
    const avgCommitLog = Math.round(items.reduce((sum, item) => sum + item.commitLogDiskUsage, 0) / items.length)
    const totalTpsIn = items.reduce((sum, item) => sum + item.tpsIn, 0)
    const totalTpsOut = items.reduce((sum, item) => sum + item.tpsOut, 0)

    return {
      cluster,
      onlineCount,
      warningCount,
      totalCount: items.length,
      avgCommitLog,
      totalTpsIn,
      totalTpsOut
    }
  })
})

const currentBrokerInPoints = computed(() => {
  return buildSparklinePoints(currentBroker.value?.tpsInHistory ?? [])
})

const currentBrokerOutPoints = computed(() => {
  return buildSparklinePoints(currentBroker.value?.tpsOutHistory ?? [])
})

const getStatusTagType = (status: NodeStatus) => {
  if (status === 'online') return 'success'
  if (status === 'warning') return 'warning'
  return 'error'
}

const getStatusText = (status: NodeStatus) => {
  if (status === 'online') return '在线'
  if (status === 'warning') return '告警'
  return '离线'
}

const pushHistory = (history: number[], value: number) => {
  const next = history.length >= HISTORY_POINTS ? history.slice(1) : [...history]
  next.push(value)
  return next
}

const updateNameServerStatus = () => {
  const brokerByCluster = new Map<string, BrokerNode[]>()
  brokerList.value.forEach((item) => {
    const list = brokerByCluster.get(item.cluster) ?? []
    list.push(item)
    brokerByCluster.set(item.cluster, list)
  })

  const nowText = formatDateTime(new Date())
  nameServerList.value = nameServerList.value.map((node) => {
    const items = brokerByCluster.get(node.cluster) ?? []
    if (!items.length) {
      return { ...node, status: 'offline', lastSeen: nowText }
    }

    const online = items.filter(item => item.status === 'online').length
    const warning = items.filter(item => item.status === 'warning').length
    const nextStatus: NodeStatus = online === 0 ? 'offline' : (warning > 0 ? 'warning' : 'online')

    return {
      ...node,
      status: nextStatus,
      lastSeen: nowText
    }
  })
}

const simulateBrokerTick = (item: BrokerNode) => {
  const nowText = formatDateTime(new Date())

  if (item.status === 'offline') {
    return {
      ...item,
      tpsIn: 0,
      tpsOut: 0,
      tpsInHistory: pushHistory(item.tpsInHistory, 0),
      tpsOutHistory: pushHistory(item.tpsOutHistory, 0),
      lastUpdate: nowText
    }
  }

  const inFactor = 0.9 + Math.random() * 0.22
  const outFactor = 0.9 + Math.random() * 0.22
  const jitterIn = Math.round((Math.random() - 0.5) * Math.max(item.tpsIn * 0.12, 24))
  const jitterOut = Math.round((Math.random() - 0.5) * Math.max(item.tpsOut * 0.12, 20))

  const nextTpsIn = Math.max(1, Math.round(item.tpsIn * inFactor) + jitterIn)
  const nextTpsOut = Math.max(1, Math.round(item.tpsOut * outFactor) + jitterOut)

  const nextCommitLog = clamp(
    item.commitLogDiskUsage + Math.round((Math.random() - 0.5) * 4),
    18,
    96
  )
  const nextConsumeQueue = clamp(
    item.consumeQueueDiskUsage + Math.round((Math.random() - 0.5) * 4),
    12,
    92
  )

  const nextStatus: NodeStatus = nextCommitLog >= 85 ? 'warning' : 'online'

  const deltaSeconds = AUTO_REFRESH_MS / 1000
  const nextMsgInToday = item.msgInToday + Math.round(nextTpsIn * deltaSeconds)
  const nextMsgOutToday = item.msgOutToday + Math.round(nextTpsOut * deltaSeconds)

  return {
    ...item,
    status: nextStatus,
    tpsIn: nextTpsIn,
    tpsOut: nextTpsOut,
    tpsInHistory: pushHistory(item.tpsInHistory, nextTpsIn),
    tpsOutHistory: pushHistory(item.tpsOutHistory, nextTpsOut),
    commitLogDiskUsage: nextCommitLog,
    consumeQueueDiskUsage: nextConsumeQueue,
    msgInToday: nextMsgInToday,
    msgOutToday: nextMsgOutToday,
    lastUpdate: nowText
  }
}

const runRefresh = (mode: 'auto' | 'manual') => {
  brokerList.value = brokerList.value.map(item => simulateBrokerTick(item))

  if (currentBroker.value) {
    const latest = brokerList.value.find(item => item.id === currentBroker.value?.id)
    if (latest) {
      currentBroker.value = latest
    }
  }

  updateNameServerStatus()
  lastRefreshAt.value = formatDateTime(new Date())

  if (mode === 'manual') {
    message.success('已刷新全部集群状态')
  }
}

const refreshSingleBroker = (id: number) => {
  brokerList.value = brokerList.value.map(item => {
    if (item.id !== id) return item
    return simulateBrokerTick(item)
  })

  if (currentBroker.value?.id === id) {
    const latest = brokerList.value.find(item => item.id === id)
    if (latest) {
      currentBroker.value = latest
    }
  }

  updateNameServerStatus()
  lastRefreshAt.value = formatDateTime(new Date())
}

const startAutoRefresh = () => {
  if (refreshTimer) return
  refreshTimer = setInterval(() => {
    runRefresh('auto')
  }, AUTO_REFRESH_MS)
}

const stopAutoRefresh = () => {
  if (!refreshTimer) return
  clearInterval(refreshTimer)
  refreshTimer = null
}

const renderTrendCell = (row: BrokerNode) => {
  const inPoints = buildSparklinePoints(row.tpsInHistory)
  const outPoints = buildSparklinePoints(row.tpsOutHistory)

  return h('div', { class: 'trend-cell' }, [
    h('svg', { viewBox: '0 0 100 24', preserveAspectRatio: 'none', class: 'sparkline-svg' }, [
      h('polyline', { points: inPoints, class: 'sparkline-in' }),
      h('polyline', { points: outPoints, class: 'sparkline-out' })
    ]),
    h('div', { class: 'trend-meta' }, `In ${row.tpsIn} / Out ${row.tpsOut}`)
  ])
}

const handleSearch = () => {
  message.success(`查询完成，当前 ${filteredBrokers.value.length} 个 Broker`)
}

const handleReset = () => {
  selectedCluster.value = null
  selectedStatus.value = null
  keyword.value = ''
  message.success('筛选条件已重置')
}

const handleViewDetail = (row: BrokerNode) => {
  currentBroker.value = row
  showDetailDrawer.value = true
}

const handleRefreshRuntime = (row?: BrokerNode) => {
  if (row) {
    refreshSingleBroker(row.id)
    message.success(`已刷新 ${row.brokerName} 运行状态`)
    return
  }

  runRefresh('manual')
}

const handleToggleAutoRefresh = () => {
  autoRefreshEnabled.value = !autoRefreshEnabled.value
  message.success(autoRefreshEnabled.value ? '已开启自动刷新' : '已关闭自动刷新')
}

const tableScrollX = 1380

const columns: DataTableColumns<BrokerNode> = [
  {
    title: 'Broker',
    key: 'brokerName',
    minWidth: 160,
    render: row => h('div', { class: 'broker-name-cell' }, [
      h('div', { class: 'broker-name' }, row.brokerName),
      h('div', { class: 'broker-meta' }, `ID: ${row.brokerId} · ${row.version}`)
    ])
  },
  {
    title: '集群',
    key: 'cluster',
    width: 120
  },
  {
    title: '角色',
    key: 'role',
    width: 92,
    render: row => h(
      NTag,
      {
        size: 'small',
        type: row.role === 'MASTER' ? 'success' : 'info',
        round: true
      },
      { default: () => row.role }
    )
  },
  {
    title: '状态',
    key: 'status',
    width: 90,
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
    title: '近 1 分钟 TPS',
    key: 'tpsTrend',
    minWidth: 180,
    render: row => renderTrendCell(row)
  },
  {
    title: '磁盘水位',
    key: 'disk',
    minWidth: 170,
    render: row => h(
      NProgress,
      {
        type: 'line',
        percentage: row.commitLogDiskUsage,
        indicatorPlacement: 'inside',
        status: row.commitLogDiskUsage >= 80 ? 'error' : row.commitLogDiskUsage >= 70 ? 'warning' : 'success',
        height: 16,
        showIndicator: true,
        processing: false,
        railColor: 'rgba(128,128,128,0.16)'
      }
    )
  },
  {
    title: '地址',
    key: 'address',
    minWidth: 170,
    render: row => h('span', { class: 'mono-text' }, row.address)
  },
  {
    title: '更新时间',
    key: 'lastUpdate',
    width: 170
  },
  {
    title: '操作',
    key: 'actions',
    width: 160,
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
              onClick: () => handleViewDetail(row)
            },
            { default: () => '详情' }
          ),
          h(
            NButton,
            {
              size: 'tiny',
              quaternary: true,
              onClick: () => handleRefreshRuntime(row)
            },
            { default: () => '刷新' }
          )
        ]
      }
    )
  }
]

watch(autoRefreshEnabled, (enabled) => {
  if (enabled) {
    startAutoRefresh()
  } else {
    stopAutoRefresh()
  }
})

onMounted(() => {
  if (autoRefreshEnabled.value) {
    startAutoRefresh()
  }
})

onUnmounted(() => {
  stopAutoRefresh()
})
</script>

<template>
  <div class="cluster-page">
    <n-grid responsive="screen" cols="3 l:6" :x-gap="12" :y-gap="12" class="summary-grid">
      <n-gi>
        <n-card size="small" hoverable class="summary-card">
          <div class="summary-label">集群数量</div>
          <div class="summary-value">{{ summary.totalClusters }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card size="small" hoverable class="summary-card">
          <div class="summary-label">Broker 总数</div>
          <div class="summary-value">{{ summary.totalBrokers }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card size="small" hoverable class="summary-card">
          <div class="summary-label">在线 Broker</div>
          <div class="summary-value is-success">{{ summary.onlineBrokers }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card size="small" hoverable class="summary-card">
          <div class="summary-label">告警 Broker</div>
          <div class="summary-value is-warning">{{ summary.warningBrokers }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card size="small" hoverable class="summary-card">
          <div class="summary-label">离线 Broker</div>
          <div class="summary-value is-error">{{ summary.offlineBrokers }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card size="small" hoverable class="summary-card">
          <div class="summary-label">平均磁盘水位</div>
          <div class="summary-value is-info">{{ summary.avgDiskUsage }}%</div>
        </n-card>
      </n-gi>
    </n-grid>

    <n-card :bordered="false" class="panel-card toolbar-card">
      <n-space align="center" wrap :size="10">
        <n-select v-model:value="selectedCluster" clearable :options="clusterOptions" placeholder="筛选集群"
          style="width: 180px" />
        <n-select v-model:value="selectedStatus" clearable :options="statusOptions" placeholder="筛选状态"
          style="width: 140px" />
        <n-input v-model:value="keyword" clearable placeholder="搜索 Broker 名称 / 地址 / 版本" style="width: 280px" />
        <n-button type="primary" @click="handleSearch">查询</n-button>
        <n-button @click="handleReset">重置</n-button>
        <n-button quaternary @click="handleRefreshRuntime()">刷新全部状态</n-button>
        <n-button :type="autoRefreshEnabled ? 'success' : 'default'" quaternary @click="handleToggleAutoRefresh">
          {{ autoRefreshEnabled ? '自动刷新中' : '自动刷新已关闭' }}
        </n-button>
        <n-tag size="small" :type="autoRefreshEnabled ? 'success' : 'default'" round>
          最近刷新：{{ lastRefreshAt }}
        </n-tag>
      </n-space>
    </n-card>

    <n-grid responsive="screen" cols="1 xl:3" :x-gap="12" :y-gap="12" class="main-grid">
      <n-gi :span="2">
        <n-card :bordered="false" class="panel-card table-card" title="Broker 列表">
          <n-data-table :columns="columns" :data="filteredBrokers" :row-key="(row: BrokerNode) => row.id" size="small"
            striped max-height="560" flex-height :single-line="true" :scroll-x="tableScrollX" />
        </n-card>
      </n-gi>

      <n-gi :span="1">
        <n-card :bordered="false" class="panel-card side-card" title="NameServer 节点">
          <div v-for="node in nameServerList" :key="node.id" class="namesrv-item">
            <div class="namesrv-row">
              <span class="namesrv-cluster">{{ node.cluster }}</span>
              <n-tag size="small" :type="getStatusTagType(node.status)" round>
                {{ getStatusText(node.status) }}
              </n-tag>
            </div>
            <div class="namesrv-address mono-text">{{ node.address }}</div>
            <div class="namesrv-meta">版本 {{ node.version }} · {{ node.lastSeen }}</div>
          </div>
        </n-card>

        <n-card :bordered="false" class="panel-card side-card" title="集群运行概览">
          <div v-for="item in clusterRuntimeItems" :key="item.cluster" class="cluster-runtime-item">
            <div class="runtime-head">
              <span class="runtime-title">{{ item.cluster }}</span>
              <span class="runtime-sub">{{ item.onlineCount }}/{{ item.totalCount }} 在线</span>
            </div>
            <div class="runtime-metrics">
              <span>TPS: {{ item.totalTpsIn }} / {{ item.totalTpsOut }}</span>
              <span>告警: {{ item.warningCount }}</span>
            </div>
            <n-progress type="line" :percentage="item.avgCommitLog"
              :status="item.avgCommitLog >= 80 ? 'error' : item.avgCommitLog >= 70 ? 'warning' : 'success'"
              :show-indicator="false" :height="7" rail-color="rgba(128,128,128,0.14)" />
          </div>
        </n-card>
      </n-gi>
    </n-grid>

    <n-drawer v-model:show="showDetailDrawer" :width="560" placement="right" resizable>
      <n-drawer-content title="Broker 详情" closable>
        <template v-if="currentBroker">
          <n-descriptions label-placement="left" :column="1" size="small" bordered>
            <n-descriptions-item label="Broker">{{ currentBroker.brokerName }}</n-descriptions-item>
            <n-descriptions-item label="集群">{{ currentBroker.cluster }}</n-descriptions-item>
            <n-descriptions-item label="角色">
              <n-tag size="small" :type="currentBroker.role === 'MASTER' ? 'success' : 'info'" round>
                {{ currentBroker.role }}
              </n-tag>
            </n-descriptions-item>
            <n-descriptions-item label="状态">
              <n-tag size="small" :type="getStatusTagType(currentBroker.status)" round>
                {{ getStatusText(currentBroker.status) }}
              </n-tag>
            </n-descriptions-item>
            <n-descriptions-item label="主地址">
              <span class="mono-text">{{ currentBroker.address }}</span>
            </n-descriptions-item>
            <n-descriptions-item label="HA 地址">
              <span class="mono-text">{{ currentBroker.haAddress }}</span>
            </n-descriptions-item>
            <n-descriptions-item label="版本">{{ currentBroker.version }}</n-descriptions-item>
            <n-descriptions-item label="更新时间">{{ currentBroker.lastUpdate }}</n-descriptions-item>
          </n-descriptions>

          <n-card title="TPS 趋势（近 1 分钟）" size="small" :bordered="false" class="detail-block">
            <div class="detail-trend-legend">
              <span class="legend-item is-in">In</span>
              <span class="legend-item is-out">Out</span>
              <span class="legend-tip">每 5 秒采样一次</span>
            </div>
            <svg viewBox="0 0 100 28" preserveAspectRatio="none" class="detail-sparkline-svg">
              <polyline :points="currentBrokerInPoints" class="sparkline-in" />
              <polyline :points="currentBrokerOutPoints" class="sparkline-out" />
            </svg>
          </n-card>

          <n-card title="运行指标" size="small" :bordered="false" class="detail-block">
            <div class="detail-metric-row">
              <span>实时 TPS</span>
              <strong>{{ currentBroker.tpsIn }} / {{ currentBroker.tpsOut }}</strong>
            </div>
            <div class="detail-metric-row">
              <span>今日消息量</span>
              <strong>{{ currentBroker.msgInToday }} / {{ currentBroker.msgOutToday }}</strong>
            </div>
            <div class="detail-metric-row">
              <span>Topic / 消费组</span>
              <strong>{{ currentBroker.topics }} / {{ currentBroker.groups }}</strong>
            </div>
            <div class="detail-metric-row disk-row">
              <span>CommitLog 水位</span>
              <span>{{ currentBroker.commitLogDiskUsage }}%</span>
            </div>
            <n-progress type="line" :percentage="currentBroker.commitLogDiskUsage"
              :status="currentBroker.commitLogDiskUsage >= 80 ? 'error' : currentBroker.commitLogDiskUsage >= 70 ? 'warning' : 'success'"
              :show-indicator="false" :height="8" />
            <div class="detail-metric-row disk-row">
              <span>ConsumeQueue 水位</span>
              <span>{{ currentBroker.consumeQueueDiskUsage }}%</span>
            </div>
            <n-progress type="line" :percentage="currentBroker.consumeQueueDiskUsage"
              :status="currentBroker.consumeQueueDiskUsage >= 80 ? 'error' : currentBroker.consumeQueueDiskUsage >= 70 ? 'warning' : 'success'"
              :show-indicator="false" :height="8" />
          </n-card>

          <n-card title="备注" size="small" :bordered="false" class="detail-block">
            <div class="remark-text">{{ currentBroker.remark }}</div>
          </n-card>
        </template>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.cluster-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.summary-grid {
  margin-top: 2px;
}

.summary-card,
.panel-card {
  border-radius: 5px;
  background: var(--surface-2, #fff);
}

.summary-label {
  font-size: 12px;
  color: var(--text-muted, #888);
}

.summary-value {
  margin-top: 6px;
  font-size: 22px;
  line-height: 1.2;
  font-weight: 700;
  color: #2080f0;
}

.summary-value.is-success {
  color: #18a058;
}

.summary-value.is-warning {
  color: #f0a020;
}

.summary-value.is-error {
  color: #d03050;
}

.summary-value.is-info {
  color: #2080f0;
}

.toolbar-card {
  padding-top: 4px;
  padding-bottom: 4px;
}

.main-grid {
  margin-top: 2px;
}

.table-card {
  min-height: 420px;
}

.side-card+.side-card {
  margin-top: 12px;
}

.namesrv-item {
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--surface-1, #f8f8f8);
}

.namesrv-item+.namesrv-item {
  margin-top: 10px;
}

.namesrv-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.namesrv-cluster {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-color, #333);
}

.namesrv-address {
  margin-top: 6px;
}

.namesrv-meta {
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-muted, #888);
}

.cluster-runtime-item {
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--surface-1, #f8f8f8);
}

.cluster-runtime-item+.cluster-runtime-item {
  margin-top: 10px;
}

.runtime-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.runtime-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-color, #333);
}

.runtime-sub,
.runtime-metrics {
  font-size: 12px;
  color: var(--text-muted, #888);
}

.runtime-metrics {
  margin: 6px 0 8px;
  display: flex;
  justify-content: space-between;
}

.broker-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-color, #333);
}

.broker-meta {
  margin-top: 2px;
  font-size: 12px;
  color: var(--text-muted, #888);
}

.mono-text {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
}

.trend-cell {
  min-width: 142px;
}

.sparkline-svg,
.detail-sparkline-svg {
  width: 100%;
  height: 28px;
  display: block;
}

.sparkline-in,
.sparkline-out {
  fill: none;
  stroke-width: 1.9;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.sparkline-in {
  stroke: #18a058;
}

.sparkline-out {
  stroke: #2080f0;
}

.trend-meta {
  margin-top: 2px;
  font-size: 12px;
  color: var(--text-muted, #888);
}

.detail-block {
  margin-top: 12px;
  border-radius: 10px;
  background: var(--surface-1, #f8f8f8);
}

.detail-trend-legend {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.legend-item {
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 999px;
  color: #fff;
}

.legend-item.is-in {
  background: #18a058;
}

.legend-item.is-out {
  background: #2080f0;
}

.legend-tip {
  margin-left: auto;
  font-size: 12px;
  color: var(--text-muted, #888);
}

.detail-metric-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13px;
  color: var(--text-color, #333);
  margin-bottom: 8px;
}

.detail-metric-row.disk-row {
  margin-top: 8px;
  margin-bottom: 6px;
}

.remark-text {
  font-size: 13px;
  line-height: 1.6;
  color: var(--text-secondary, #666);
}
</style>
