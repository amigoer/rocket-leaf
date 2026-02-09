<script setup lang="ts">
import { ref, watchEffect } from 'vue'
import { NLayout, NLayoutContent, NConfigProvider, NGlobalStyle, NModal, NCard, NButton, NSpace, NMessageProvider, darkTheme } from 'naive-ui'
import TitleBar from './components/TitleBar.vue'
import Sidebar from './components/Sidebar.vue'
import ContentTopBar from './components/ContentTopBar.vue'
import DashboardContent from './components/DashboardContent.vue'
import ConnectionManagement from './components/ConnectionManagement.vue'
import TopicManagement from './components/TopicManagement.vue'
import ConsumerGroupManagement from './components/ConsumerGroupManagement.vue'

// 当前选中的页面
const currentPage = ref('dashboard')

// 主题切换
const isDark = ref(false)
const showSettings = ref(false)

let themeTransitionTimer: number | undefined
const themeTransitionClass = 'theme-switching'

const toggleTheme = () => {
  if (typeof document !== 'undefined') {
    const root = document.documentElement
    root.classList.add(themeTransitionClass)
    if (themeTransitionTimer) {
      window.clearTimeout(themeTransitionTimer)
    }
    themeTransitionTimer = window.setTimeout(() => {
      root.classList.remove(themeTransitionClass)
    }, 180)
  }
  isDark.value = !isDark.value
}

const handlePageChange = (key: string) => {
  if (key === 'settings') {
    showSettings.value = true
    return
  }
  showSettings.value = false
  currentPage.value = key
}

const openSettings = () => {
  showSettings.value = true
}

watchEffect(() => {
  if (typeof document !== 'undefined') {
    document.documentElement.dataset.theme = isDark.value ? 'dark' : 'light'
  }
})
</script>

<template>
  <n-config-provider :theme="isDark ? darkTheme : null">
    <n-global-style />
    <n-message-provider>
      <div class="app-container">
        <TitleBar />
        <n-layout has-sider class="main-layout">
          <Sidebar @update:currentPage="handlePageChange" @open:settings="openSettings" />
          <n-layout class="content-wrapper">
            <!-- 内容区域顶部导航 -->
            <ContentTopBar :currentPage="currentPage" :isDark="isDark" @toggle-theme="toggleTheme" />
            <!-- 主内容区域 -->
            <n-layout-content class="content">
              <DashboardContent v-if="currentPage === 'dashboard'" />
              <ConnectionManagement v-else-if="currentPage === 'connections'" />
              <TopicManagement v-else-if="currentPage === 'topics'" />
              <ConsumerGroupManagement v-else-if="currentPage === 'consumer-groups'" />
              <div v-else class="placeholder">
                <h1>功能开发中</h1>
                <p>当前页面：{{ currentPage }}</p>
                <p>请继续从左侧菜单切换</p>
              </div>
            </n-layout-content>
          </n-layout>
        </n-layout>
      </div>

      <!-- 设置弹窗 -->
      <n-modal v-model:show="showSettings" :mask-closable="true" :auto-focus="false">
        <n-card title="设置" size="small" class="settings-card" :bordered="false">
          <div class="settings-body">
            <div class="settings-title">基础设置</div>
            <div class="settings-desc">更多配置项后续补充</div>
          </div>
          <template #footer>
            <n-space justify="end">
              <n-button @click="showSettings = false">关闭</n-button>
            </n-space>
          </template>
        </n-card>
      </n-modal>
    </n-message-provider>
  </n-config-provider>
</template>

<style scoped>
.app-container {
  display: flex;
  flex-direction: column;
  height: 100vh;
  width: 100vw;
  background-color: var(--bg-color);
  overflow: hidden;
}

.main-layout {
  flex: 1;
  overflow: hidden;
}

.content-wrapper {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.content {
  flex: 1;
  padding: 20px;
  overflow: auto;
  background: var(--bg-color, #ffffff);
}

.placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #888;
}

.placeholder h1 {
  margin-bottom: 8px;
}

.placeholder p {
  margin: 4px 0;
}

.settings-card {
  width: 420px;
  border-radius: 10px;
}

.settings-body {
  padding: 4px 0 8px;
  color: var(--text-color, #333);
}

.settings-title {
  font-size: 14px;
  font-weight: 600;
}

.settings-desc {
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-muted, #888);
}
</style>
