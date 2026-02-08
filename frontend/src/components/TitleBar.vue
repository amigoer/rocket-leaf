<script setup lang="ts">
// TitleBar.vue
// 跨平台标题栏组件
import { ref, computed, onMounted } from 'vue'

// 定义应用标题
const appTitle = 'Rocket Leaf'

// 平台检测 - 使用同步检测
const platform = navigator.platform.toLowerCase()
const isMac = computed(() => platform.includes('mac'))
const isWindows = computed(() => platform.includes('win'))
const isLinux = computed(() => platform.includes('linux'))

// 窗口控制函数 (Wails v3 runtime)
const minimizeWindow = () => {
    // @ts-ignore
    window.wails?.Window?.Minimise?.()
}

const maximizeWindow = () => {
    // @ts-ignore
    window.wails?.Window?.ToggleMaximise?.()
}

const closeWindow = () => {
    // @ts-ignore
    window.wails?.Window?.Close?.()
}
</script>

<template>
    <div class="titlebar" style="--wails-draggable:drag">
        <!-- macOS 布局: 红绿灯后面跟图标和标题，靠左 -->
        <template v-if="isMac">
            <div class="mac-traffic-light-spacer"></div>
            <div class="title-left">
                <img src="../assets/leaf-icon.svg" class="app-icon" alt="icon" draggable="false" />
                <span class="app-title">{{ appTitle }}</span>
            </div>
        </template>


        <!-- Windows/Linux 布局: 图标和标题左侧，控制按钮右侧 -->
        <template v-else>
            <div class="title-left">
                <img src="../assets/leaf-icon.svg" class="app-icon" alt="icon" draggable="false" />
                <span class="app-title">{{ appTitle }}</span>
            </div>

            <div class="window-controls" style="--wails-draggable:no-drag">
                <button class="control-btn min-btn" @click="minimizeWindow" title="最小化">
                    <svg width="10" height="1" viewBox="0 0 10 1">
                        <rect fill="currentColor" width="10" height="1" />
                    </svg>
                </button>
                <button class="control-btn max-btn" @click="maximizeWindow" title="最大化">
                    <svg width="10" height="10" viewBox="0 0 10 10">
                        <rect fill="none" stroke="currentColor" stroke-width="1" x="0.5" y="0.5" width="9" height="9" />
                    </svg>
                </button>
                <button class="control-btn close-btn" @click="closeWindow" title="关闭">
                    <svg width="10" height="10" viewBox="0 0 10 10">
                        <line stroke="currentColor" stroke-width="1.2" x1="0" y1="0" x2="10" y2="10" />
                        <line stroke="currentColor" stroke-width="1.2" x1="10" y1="0" x2="0" y2="10" />
                    </svg>
                </button>
            </div>
        </template>
    </div>
</template>

<style scoped>
.titlebar {
    height: var(--titlebar-height, 30px);
    width: 100%;
    display: flex;
    align-items: center;
    background: var(--titlebar-bg, rgba(255, 255, 255, 0.8));
    user-select: none;
    cursor: default;
    flex-shrink: 0;
}

/* macOS 布局 */
.mac-traffic-light-spacer {
    width: 78px;
    /* 红绿灯区域宽度 */
    flex-shrink: 0;
}

.title-center {
    flex: 1;
    display: flex;
    justify-content: center;
    align-items: center;
}

/* Windows/Linux 布局 */
.title-left {
    flex: 1;
    display: flex;
    align-items: center;
    padding-left: 12px;
    gap: 8px;
}

.app-icon {
    width: 16px;
    height: 16px;
    pointer-events: none;
    -webkit-user-drag: none;
}

.app-title {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-color, #333333);
    user-select: none;
    -webkit-user-select: none;
    pointer-events: none;
}

/* 窗口控制按钮 */
.window-controls {
    display: flex;
    height: 100%;
}

.control-btn {
    width: 46px;
    height: 100%;
    border: none;
    background: transparent;
    color: #333;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: background 0.15s;
}

.control-btn:hover {
    background: rgba(0, 0, 0, 0.08);
}

.close-btn:hover {
    background: #e81123;
    color: white;
}
</style>
