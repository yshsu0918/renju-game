<script setup lang="ts">
import { ref } from 'vue'
import { colorName } from '../protocol'
import type { Store } from '../store'

const props = defineProps<{ store: Store }>()
const copied = ref(false)

async function copy() {
  try {
    await navigator.clipboard.writeText(props.store.state.code)
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch {
    // Clipboard unavailable (e.g. plain http on a LAN address); the code is still on screen.
  }
}
</script>

<template>
  <section class="panel">
    <h2>等待對手加入</h2>
    <p class="muted">把加入碼告訴對手，對手在主畫面點「加入房間」輸入即可開始。</p>
    <button class="code" title="點擊複製" @click="copy">{{ store.state.code }}</button>
    <p class="muted small">{{ copied ? '已複製' : '點擊加入碼可複製' }}</p>
    <p>你執{{ colorName(store.state.color ?? 'black') }}</p>
    <div class="buttons row">
      <button @click="store.cancelRoom()">取消</button>
    </div>
  </section>
</template>
