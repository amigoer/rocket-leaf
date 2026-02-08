<script setup lang="ts">
// ContentTopBar.vue
// 内容区域顶部导航栏，显示当前菜单名称等公共信息
import { NIcon, NBreadcrumb, NBreadcrumbItem, NSpace, NButton, NTooltip } from 'naive-ui'
import {
    RefreshOutline,
    SearchOutline,
    NotificationsOutline,
    MoonOutline,
    SunnyOutline,
    HomeOutline,
    SpeedometerOutline,
    LinkOutline,
    FolderOutline,
    PeopleOutline,
    ChatbubblesOutline,
    ServerOutline,
    SettingsOutline
} from '@vicons/ionicons5'
import type { Component } from 'vue'

// 定义 props
defineProps<{
    currentPage: string
    isDark: boolean
}>()

const emit = defineEmits<{
    (e: 'toggle-theme'): void
}>()

// 菜单名称映射
const menuNames: Record<string, string> = {
    'dashboard': '仪表盘',
    'connections': '连接管理',
    'topics': 'Topic 管理',
    'consumer-groups': '消费者组',
    'messages': '消息查询',
    'cluster': '集群状态',
    'settings': '设置'
}

const menuIcons: Record<string, Component> = {
    'dashboard': SpeedometerOutline,
    'connections': LinkOutline,
    'topics': FolderOutline,
    'consumer-groups': PeopleOutline,
    'messages': ChatbubblesOutline,
    'cluster': ServerOutline,
    'settings': SettingsOutline
}

// 获取当前页面名称
const getPageName = (key: string) => {
    return menuNames[key] || key
}

// 获取当前页面图标
const getPageIcon = (key: string) => {
    return menuIcons[key] || FolderOutline
}
</script>

<template>
    <div class="content-topbar">
        <!-- 左侧：面包屑 / 当前页面名称 -->
        <div class="topbar-left">
            <n-breadcrumb>
                <n-breadcrumb-item>
                    <span class="breadcrumb-item">
                        <n-icon :size="16">
                            <HomeOutline />
                        </n-icon>
                        <span>首页</span>
                    </span>
                </n-breadcrumb-item>
                <n-breadcrumb-item>
                    <span class="breadcrumb-item">
                        <n-icon :size="16">
                            <component :is="getPageIcon(currentPage)" />
                        </n-icon>
                        <span>{{ getPageName(currentPage) }}</span>
                    </span>
                </n-breadcrumb-item>
            </n-breadcrumb>
        </div>

        <!-- 右侧：操作按钮 -->
        <div class="topbar-right">
            <n-space :size="4">
                <n-tooltip trigger="hover">
                    <template #trigger>
                        <n-button quaternary circle size="small">
                            <template #icon>
                                <n-icon :size="18">
                                    <SearchOutline />
                                </n-icon>
                            </template>
                        </n-button>
                    </template>
                    搜索
                </n-tooltip>

                <n-tooltip trigger="hover">
                    <template #trigger>
                        <n-button quaternary circle size="small">
                            <template #icon>
                                <n-icon :size="18">
                                    <RefreshOutline />
                                </n-icon>
                            </template>
                        </n-button>
                    </template>
                    刷新
                </n-tooltip>

                <n-tooltip trigger="hover">
                    <template #trigger>
                        <n-button quaternary circle size="small">
                            <template #icon>
                                <n-icon :size="18">
                                    <NotificationsOutline />
                                </n-icon>
                            </template>
                        </n-button>
                    </template>
                    通知
                </n-tooltip>

                <n-tooltip trigger="hover">
                    <template #trigger>
                        <n-button quaternary circle size="small" @click="emit('toggle-theme')">
                            <template #icon>
                                <n-icon :size="18">
                                    <SunnyOutline v-if="isDark" />
                                    <MoonOutline v-else />
                                </n-icon>
                            </template>
                        </n-button>
                    </template>
                    {{ isDark ? '浅色模式' : '深色模式' }}
                </n-tooltip>
            </n-space>
        </div>
    </div>
</template>

<style scoped>
.content-topbar {
    height: 73px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 20px;
    background: var(--bg-color, #ffffff);
    border-bottom: 1px solid var(--border-color, rgba(0, 0, 0, 0.06));
    flex-shrink: 0;
}

.topbar-left {
    display: flex;
    align-items: center;
}

.topbar-right {
    display: flex;
    align-items: center;
}

.breadcrumb-item {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--text-color, #333);
}
</style>
