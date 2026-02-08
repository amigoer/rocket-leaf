<script setup lang="ts">
import TitleBar from './components/TitleBar.vue'
</script>

<template>
  <div class="app-container">
    <TitleBar />
    <main class="content">
      <!-- Main content will go here -->
      <div class="placeholder">
        <h1>Welcome to Rocket Leaf</h1>
        <p>A lightweight RocketMQ client.</p>
      </div>
    </main>
  </div>
</template>

<style scoped>
.app-container {
  display: flex;
  flex-direction: column;
  height: 100vh;
  width: 100vw;
  background-color: var(--bg-color);
}

.content {
  flex: 1;
  overflow: auto;
  padding: 20px;
}

.placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #888;
}
</style>
