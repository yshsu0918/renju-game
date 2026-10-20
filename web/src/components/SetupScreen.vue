<script setup lang="ts">
import { ref } from 'vue'
import type { Color } from '../protocol'
import type { Store } from '../store'

const props = defineProps<{ store: Store }>()
const color = ref<Color>('black')
const title = props.store.state.setupMode === 'ai' ? 'AI 對戰' : '人人對戰'
</script>

<template>
  <section class="panel">
    <h2>{{ title }}</h2>
    <fieldset class="choice">
      <legend>選擇先後手</legend>
      <label><input v-model="color" type="radio" value="black" name="color" /> 先手（黑）</label>
      <label><input v-model="color" type="radio" value="white" name="color" /> 後手（白）</label>
    </fieldset>
    <fieldset class="choice">
      <legend>規則</legend>
      <label class="disabled" title="之後的版本會開放">
        <input type="checkbox" disabled /> 禁手規則（Renju）<span class="muted">即將推出</span>
      </label>
    </fieldset>
    <div class="buttons row">
      <button @click="store.home()">返回</button>
      <button class="primary" :disabled="store.state.busy || !store.state.connected" @click="store.start(color)">
        確認
      </button>
    </div>
    <p v-if="store.state.error" class="error" role="alert">{{ store.state.error }}</p>
  </section>
</template>
