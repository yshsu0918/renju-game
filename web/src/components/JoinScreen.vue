<script setup lang="ts">
import { ref } from 'vue'
import type { Store } from '../store'

const props = defineProps<{ store: Store }>()
const code = ref('')

function submit() {
  props.store.join(code.value)
}
</script>

<template>
  <section class="panel">
    <h2>加入房間</h2>
    <form @submit.prevent="submit">
      <input
        v-model="code"
        class="code-input"
        maxlength="6"
        autocomplete="off"
        autocapitalize="characters"
        spellcheck="false"
        placeholder="加入碼"
        aria-label="加入碼"
        @input="code = code.toUpperCase()"
      />
      <div class="buttons row">
        <button type="button" @click="store.home()">返回</button>
        <button type="submit" class="primary" :disabled="store.state.busy || !store.state.connected">加入</button>
      </div>
    </form>
    <p v-if="store.state.error" class="error" role="alert">{{ store.state.error }}</p>
  </section>
</template>
