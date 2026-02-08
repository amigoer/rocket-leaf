<script setup lang="ts">
import { ref, watchEffect } from 'vue'
import { NLayout, NLayoutContent, NConfigProvider, NGlobalStyle, darkTheme } from 'naive-ui'
import TitleBar from './components/TitleBar.vue'
import Sidebar from './components/Sidebar.vue'
import ContentTopBar from './components/ContentTopBar.vue'

// 当前选中的页面
const currentPage = ref('dashboard')

// 主题切换
const isDark = ref(false)

let themeTransitionTimer: number | undefined

const toggleTheme = () => {
  if (typeof document !== 'undefined') {
    document.documentElement.classList.add('theme-transition')
    if (themeTransitionTimer) {
      window.clearTimeout(themeTransitionTimer)
    }
    themeTransitionTimer = window.setTimeout(() => {
      document.documentElement.classList.remove('theme-transition')
    }, 260)
  }
  isDark.value = !isDark.value
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
    <div class="app-container">
      <TitleBar />
      <n-layout has-sider class="main-layout">
        <Sidebar @update:currentPage="currentPage = $event" />
        <n-layout class="content-wrapper">
          <!-- 内容区域顶部导航 -->
          <ContentTopBar :currentPage="currentPage" :isDark="isDark" @toggle-theme="toggleTheme" />
          <!-- 主内容区域 -->
          <n-layout-content class="content">
            <div class="placeholder">
              <h1>Welcome to Rocket Leaf</h1>
              <p>A lightweight RocketMQ client.</p>
              <p>请从左侧菜单选择功能</p>
            </div>
          </n-layout-content>
        </n-layout>
      </n-layout>
    </div>
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
</style>
