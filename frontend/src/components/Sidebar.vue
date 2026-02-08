<script setup lang="ts">
// Sidebar.vue
// 使用 Naive UI 和 Ionicons5 图标构建的左侧边栏
import { ref, h, Component, computed } from 'vue'
import { NMenu, NIcon, NLayoutSider, NTooltip, NPopover, NDivider } from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import {
    SpeedometerOutline,
    LinkOutline,
    FolderOutline,
    PeopleOutline,
    ChatbubblesOutline,
    ServerOutline,
    SettingsOutline,
    ChevronDownOutline,
    CheckmarkOutline,
    AddOutline,
    LogoGithub,
    OpenOutline
} from '@vicons/ionicons5'

// 渲染图标的辅助函数
const renderIcon = (icon: Component) => {
    return () => h(NIcon, null, { default: () => h(icon) })
}

// Tooltip 统一样式（移除默认黑底，使用自定义面板）
const tooltipContentStyle = {
    padding: '0',
    backgroundColor: 'transparent',
    border: 'none',
    boxShadow: 'none'
} as const

// RocketMQ 图标（来自 Iconify 的 simple-icons:apacherocketmq）
const RocketMQIcon = {
    render() {
        return h('span', { class: 'rocketmq-icon' })
    }
}

// 侧边栏是否折叠
const collapsed = ref(false)

// 当前选中的菜单项
const activeKey = ref<string>('dashboard')

// 连接弹窗显示状态
const showConnectionPopover = ref(false)

// RocketMQ 实例列表（模拟数据）
interface Instance {
    label: string
    value: string
    ip: string
    status: 'online' | 'offline'  // 连接状态
}

const instances = ref<Instance[]>([
    { label: '生产环境', value: 'prod', ip: '192.168.1.100:9876', status: 'online' },
    { label: '测试环境', value: 'test', ip: '192.168.1.101:9876', status: 'online' },
    { label: '开发环境', value: 'dev', ip: 'localhost:9876', status: 'offline' }
])

// 当前选中的实例
const selectedInstance = ref<string>('prod')

// 获取当前选中实例的信息
const currentInstance = computed(() => {
    return instances.value.find(i => i.value === selectedInstance.value) || instances.value[0]
})

// 选择实例
const selectInstance = (value: string) => {
    selectedInstance.value = value
    showConnectionPopover.value = false
}

// 添加新连接
const addNewConnection = () => {
    showConnectionPopover.value = false
    // TODO: 打开添加连接对话框
    console.log('Add new connection')
}

// 菜单选项 - 只保留一级导航
const menuOptions: MenuOption[] = [
    {
        label: '仪表盘',
        key: 'dashboard',
        icon: renderIcon(SpeedometerOutline)
    },
    {
        label: '连接管理',
        key: 'connections',
        icon: renderIcon(LinkOutline)
    },
    {
        label: 'Topic 管理',
        key: 'topics',
        icon: renderIcon(FolderOutline)
    },
    {
        label: '消费者组',
        key: 'consumer-groups',
        icon: renderIcon(PeopleOutline)
    },
    {
        label: '消息查询',
        key: 'messages',
        icon: renderIcon(ChatbubblesOutline)
    },
    {
        label: '集群状态',
        key: 'cluster',
        icon: renderIcon(ServerOutline)
    }
]

// 打开 GitHub 仓库
const openGithub = () => {
    window.open('https://github.com/codermast/rocket-leaf', '_blank')
}

// 定义事件
const emit = defineEmits<{
    (e: 'update:currentPage', key: string): void
}>()

// 处理菜单选择
const handleSelect = (key: string) => {
    activeKey.value = key
    emit('update:currentPage', key)
    console.log('Selected:', key)
}
</script>

<template>
    <n-layout-sider bordered collapse-mode="width" :collapsed-width="64" :width="220" :collapsed="collapsed"
        show-trigger @collapse="collapsed = true" @expand="collapsed = false" class="sidebar">
        <!-- 实例选择器区域 -->
        <div class="instance-selector-wrapper">
            <n-popover v-model:show="showConnectionPopover" trigger="click" placement="bottom-start" :show-arrow="false"
                raw style="padding: 0;">
                <template #trigger>
                    <div class="instance-card" :class="{ collapsed }">
                        <!-- 折叠状态：只显示图标 -->
                        <template v-if="collapsed">
                            <n-tooltip placement="right" :show-arrow="false" raw :content-style="tooltipContentStyle">
                                <template #trigger>
                                    <div class="instance-card-icon-only">
                                        <n-icon :size="20">
                                            <RocketMQIcon />
                                        </n-icon>
                                        <span class="status-dot collapsed-dot" :class="currentInstance.status"></span>
                                    </div>
                                </template>
                                <div class="tooltip-panel instance-tooltip">
                                    <div class="instance-tooltip-name">
                                        <span class="status-dot inline" :class="currentInstance.status"></span>
                                        <span>{{ currentInstance.label }}</span>
                                    </div>
                                    <div class="instance-tooltip-ip">{{ currentInstance.ip }}</div>
                                </div>
                            </n-tooltip>
                        </template>


                        <!-- 展开状态：显示卡片 -->
                        <template v-else>
                            <div class="instance-card-content">
                                <div class="instance-card-icon">
                                    <n-icon :size="20">
                                        <RocketMQIcon />
                                    </n-icon>
                                </div>
                                <div class="instance-card-info">
                                    <div class="instance-card-name">
                                        {{ currentInstance.label }}
                                        <span class="status-dot inline" :class="currentInstance.status"></span>
                                    </div>
                                </div>
                                <n-icon :size="16" class="instance-card-arrow">
                                    <ChevronDownOutline />
                                </n-icon>
                            </div>
                        </template>
                    </div>
                </template>

                <!-- 下拉菜单内容 -->
                <div class="connection-popover">
                    <div class="popover-header">切换连接</div>

                    <!-- 连接列表 -->
                    <div class="connection-list">
                        <div v-for="instance in instances" :key="instance.value" class="connection-item"
                            :class="{ active: selectedInstance === instance.value }"
                            @click="selectInstance(instance.value)">
                            <div class="connection-icon">
                                <n-icon :size="18">
                                    <RocketMQIcon />
                                </n-icon>
                            </div>
                            <div class="connection-info">
                                <div class="connection-name">{{ instance.label }}</div>
                                <n-tooltip placement="right" :show-arrow="false" raw :content-style="tooltipContentStyle">
                                    <template #trigger>
                                        <div class="connection-ip">{{ instance.ip }}</div>
                                    </template>
                                    <div class="tooltip-panel">{{ instance.ip }}</div>
                                </n-tooltip>
                            </div>
                            <span class="status-dot" :class="instance.status"></span>
                            <n-icon v-if="selectedInstance === instance.value" :size="16" class="check-icon">
                                <CheckmarkOutline />
                            </n-icon>
                        </div>
                    </div>

                    <!-- 分隔线 -->
                    <div class="popover-divider"></div>

                    <!-- 添加新连接 -->
                    <div class="add-connection" @click="addNewConnection">
                        <div class="add-icon">
                            <n-icon :size="16">
                                <AddOutline />
                            </n-icon>
                        </div>
                        <span>添加新连接</span>
                    </div>
                </div>
            </n-popover>
        </div>

        <!-- 菜单 -->
        <n-menu :value="activeKey" :collapsed="collapsed" :collapsed-width="64" :collapsed-icon-size="20"
            :options="menuOptions" @update:value="handleSelect" class="main-menu" />

        <!-- 底部区域 -->
        <div class="sidebar-footer" :class="{ collapsed }">
            <!-- 设置 -->
            <n-tooltip v-if="collapsed" placement="right">
                <template #trigger>
                    <div class="footer-item" @click="handleSelect('settings')"
                        :class="{ active: activeKey === 'settings' }">
                        <div class="footer-icon">
                            <n-icon :size="20">
                                <SettingsOutline />
                            </n-icon>
                        </div>
                    </div>
                </template>
                设置
            </n-tooltip>
            <div v-else class="footer-item" @click="handleSelect('settings')"
                :class="{ active: activeKey === 'settings' }">
                <div class="footer-icon">
                    <n-icon :size="20">
                        <SettingsOutline />
                    </n-icon>
                </div>
                <span class="footer-label">设置</span>
            </div>

            <!-- GitHub -->
            <n-tooltip v-if="collapsed" placement="right">
                <template #trigger>
                    <div class="footer-item github-item" @click="openGithub">
                        <div class="footer-icon">
                            <n-icon :size="20">
                                <LogoGithub />
                            </n-icon>
                        </div>
                    </div>
                </template>
                <div>
                    <div style="font-weight: 500;">Rocket Leaf</div>
                    <div style="font-size: 12px; color: #999;">开源 RocketMQ 客户端</div>
                </div>
            </n-tooltip>
            <div v-else class="footer-item github-item" @click="openGithub">
                <div class="footer-icon">
                    <n-icon :size="20">
                        <LogoGithub />
                    </n-icon>
                </div>
                <div class="github-info">
                    <div class="github-name">
                        Rocket Leaf
                        <n-icon :size="12" style="margin-left: 4px; opacity: 0.6;">
                            <OpenOutline />
                        </n-icon>
                    </div>
                    <div class="github-desc">开源 RocketMQ 客户端</div>
                </div>
            </div>
        </div>

    </n-layout-sider>
</template>


<style scoped>
.sidebar {
    height: 100%;
    background: var(--bg-color, #ffffff);
    display: flex;
    flex-direction: column;
}

/* 确保 NLayoutSider 内部滚动容器也使用 flex */
.sidebar :deep(.n-layout-sider-scroll-container) {
    display: flex;
    flex-direction: column;
}

/* 实例选择器区域 */
.instance-selector-wrapper {
    border-bottom: 1px solid var(--border-color, rgba(0, 0, 0, 0.06));
}

/* 实例卡片 */
.instance-card {
    margin: 12px;
    border-radius: 10px;
    background: var(--surface-1, #f8f8f8);
    cursor: pointer;
    transition: all 0.2s ease;
}

.instance-card:hover {
    background: var(--surface-1-hover, #f0f0f0);
}

.instance-card.collapsed {
    display: flex;
    justify-content: center;
    align-items: center;
    padding: 0;
    height: var(--menu-item-height, 42px);
    margin: 6px 0;
    background: transparent;
    position: relative;
}

.instance-card.collapsed:hover {
    background: transparent;
}

.instance-card.collapsed::before {
    content: '';
    position: absolute;
    left: 8px;
    right: 8px;
    top: 0;
    bottom: 0;
    border-radius: var(--menu-item-radius, 3px);
    background: transparent;
    transition: background-color 0.3s var(--menu-item-bezier, cubic-bezier(.4, 0, .2, 1));
    pointer-events: none;
}

.instance-card.collapsed:hover::before {
    background: var(--menu-item-hover-bg, #f3f3f5);
}

.instance-card-icon-only {
    position: relative;
    width: 36px;
    height: 36px;
    min-width: 36px;
    min-height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 8px;
    background: #18a058;
    color: white;
    flex-shrink: 0;
    transition: transform 0.15s ease, box-shadow 0.15s ease, background 0.15s ease;
    z-index: 1;
}

.instance-card.collapsed:hover .instance-card-icon-only {
    transform: none;
    box-shadow: none;
    background: #18a058;
}

.rocketmq-icon {
    display: inline-block;
    width: 1em;
    height: 1em;
    background-color: currentColor;
    mask: url('https://api.iconify.design/simple-icons/apacherocketmq.svg') no-repeat center / contain;
    -webkit-mask: url('https://api.iconify.design/simple-icons/apacherocketmq.svg') no-repeat center / contain;
}

.instance-card-content {
    display: flex;
    align-items: center;
    padding: 12px;
    gap: 10px;
}

.instance-card-icon {
    width: 36px;
    height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 8px;
    background: #18a058;
    color: white;
    flex-shrink: 0;
}

.instance-card-info {
    flex: 1;
    min-width: 0;
    overflow: hidden;
}

.instance-card-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-color, #333);
    display: flex;
    align-items: center;
    gap: 6px;
}

.instance-card-ip {
    font-size: 12px;
    color: var(--text-secondary, #666);
    margin-top: 4px;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
    letter-spacing: 0.2px;
    background: var(--chip-bg, rgba(0, 0, 0, 0.04));
    padding: 2px 6px;
    border-radius: 6px;
    display: block;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.instance-card-arrow {
    color: #999;
    flex-shrink: 0;
}

/* 状态指示点 */
.status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
}

.status-dot.online {
    background: #18a058;
}

.status-dot.offline {
    background: #e88080;
}

.status-dot.inline {
    display: inline-block;
}

.instance-card-icon-only .status-dot {
    position: absolute;
    bottom: -2px;
    right: -2px;
    width: 10px;
    height: 10px;
    border: 2px solid white;
}

/* 主菜单占据剩余空间 */
.main-menu {
    flex: 1;
    overflow-y: auto;
}

/* 底部区域 - 固定在底部 */
.sidebar-footer {
    margin-top: auto;
    border-top: 1px solid var(--border-color, rgba(0, 0, 0, 0.06));
    padding: 8px;
}

.footer-item {
    display: flex;
    align-items: center;
    padding: 10px 12px;
    gap: 10px;
    border-radius: 8px;
    cursor: pointer;
    transition: background 0.15s;
    color: var(--text-secondary, #666);
}

.footer-item:hover {
    background: var(--surface-2-hover, #f5f5f5);
    color: var(--text-color, #333);
}

.footer-item.active {
    background: var(--surface-2-hover, #f5f5f5);
    color: var(--text-color, #333);
}

/* 折叠状态下居中对齐 */
.sidebar-footer.collapsed .footer-item {
    justify-content: center;
    padding: 10px;
}

.footer-icon {
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    border-radius: 6px;
    background: transparent;
    color: inherit;
    transition: background 0.15s, color 0.15s;
}

.footer-label {
    font-size: 14px;
    font-weight: 500;
}

.github-item {
    margin-top: 4px;
}

.github-info {
    flex: 1;
    min-width: 0;
}

.github-name {
    font-size: 14px;
    font-weight: 500;
    color: var(--text-color, #333);
    display: flex;
    align-items: center;
}

.github-desc {
    font-size: 12px;
    color: var(--text-muted, #888);
    margin-top: 1px;
}
</style>


<style>
/* 全局样式 - 下拉弹窗 */
.connection-popover {
    min-width: 260px;
    background: var(--surface-2, #ffffff);
    border-radius: 12px;
    box-shadow: var(--popover-shadow, 0 8px 24px rgba(0, 0, 0, 0.12));
    overflow: hidden;
}

.tooltip-panel {
    background: var(--surface-2, #ffffff);
    color: var(--text-color, #333);
    border: 1px solid var(--border-color, rgba(0, 0, 0, 0.06));
    box-shadow: var(--popover-shadow, 0 8px 24px rgba(0, 0, 0, 0.12));
    border-radius: 8px;
    padding: 8px 10px;
}

.instance-tooltip {
    padding: 4px 2px;
}

.instance-tooltip-name {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-color, #333);
}

.instance-tooltip-ip {
    margin-top: 2px;
    font-size: 12px;
    color: var(--text-muted, #888);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
    letter-spacing: 0.2px;
}

.popover-header {
    padding: 12px 16px;
    font-size: 13px;
    color: var(--text-muted, #888);
    border-bottom: 1px solid var(--border-color, rgba(0, 0, 0, 0.06));
}

.connection-list {
    padding: 8px;
}

.connection-item {
    display: flex;
    align-items: center;
    padding: 10px 12px;
    gap: 10px;
    border-radius: 8px;
    cursor: pointer;
    transition: background 0.15s;
}

.connection-item:hover {
    background: var(--surface-2-hover, #f5f5f5);
}

.connection-item.active {
    background: rgba(24, 160, 88, 0.08);
}

.connection-icon {
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 6px;
    background: var(--surface-3, #f0f0f0);
    color: var(--text-secondary, #666);
    flex-shrink: 0;
}

.connection-info {
    flex: 1;
    min-width: 0;
}

.connection-name {
    font-size: 14px;
    font-weight: 500;
    color: var(--text-color, #333);
}

.connection-ip {
    font-size: 12px;
    color: var(--text-secondary, #666);
    margin-top: 4px;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
    letter-spacing: 0.2px;
    background: var(--chip-bg, rgba(0, 0, 0, 0.04));
    padding: 2px 6px;
    border-radius: 6px;
    display: block;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.check-icon {
    color: #18a058;
    flex-shrink: 0;
    margin-left: 4px;
}

.popover-divider {
    height: 1px;
    background: var(--border-color, rgba(0, 0, 0, 0.06));
    margin: 0 8px;
}

.add-connection {
    display: flex;
    align-items: center;
    padding: 12px 16px;
    gap: 10px;
    cursor: pointer;
    transition: background 0.15s;
    color: #666;
    font-size: 14px;
}

.add-connection:hover {
    background: var(--surface-2-hover, #f5f5f5);
}

.add-icon {
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 6px;
    border: 1px dashed var(--add-icon-border, #ccc);
    color: var(--text-muted, #999);
}
</style>
