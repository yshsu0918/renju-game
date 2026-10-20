<script setup lang="ts">
import type { Store } from '../store'

defineProps<{ store: Store }>()
</script>

<template>
  <section class="panel">
    <h1>五子棋</h1>
    <div class="buttons">
      <button class="primary" :disabled="!store.state.connected" @click="store.chooseMode('ai')">AI 對戰</button>
      <button class="primary" :disabled="!store.state.connected" @click="store.chooseMode('pvp')">人人對戰</button>
      <button :disabled="!store.state.connected" @click="store.openJoin()">加入房間</button>
    </div>
    <p v-if="!store.state.connected" class="muted">正在連線到伺服器…</p>
    <p v-if="store.state.error" class="error" role="alert">{{ store.state.error }}</p>
  </section>
</template>
