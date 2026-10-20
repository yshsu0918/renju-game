<script setup lang="ts">
import { computed, ref } from 'vue'
import { colorName, reasonText } from '../protocol'
import type { Store } from '../store'
import GoBoard from './GoBoard.vue'

const props = defineProps<{ store: Store }>()
const s = props.store.state

const opponent = computed(() => (s.mode === 'ai' ? 'AI' : '對手'))
const myTurn = computed(() => !s.result && s.turn === s.color)
const status = computed(() => {
  if (s.result) return ''
  return myTurn.value ? '輪到你' : s.mode === 'ai' ? 'AI 思考中…' : '等待對手落子…'
})
const resultTitle = computed(() => {
  const r = s.result
  if (!r) return ''
  if (!r.winner) return '和局'
  return r.winner === s.color ? '你贏了' : `${opponent.value}獲勝`
})

// Resign and abandon read differently for the winner and the loser.
const resultReason = computed(() => {
  const r = s.result
  if (!r) return ''
  const won = r.winner === s.color
  if (r.reason === 'resign') return won ? `${opponent.value}認輸` : '你認輸了'
  if (r.reason === 'abandoned') return won ? '對手斷線未回' : '你斷線超過時間'
  return reasonText[r.reason] ?? r.reason
})

// Resign needs a second click, so a stray tap cannot end the game.
const confirmResign = ref(false)
function resign() {
  if (!confirmResign.value) {
    confirmResign.value = true
    setTimeout(() => (confirmResign.value = false), 3000)
    return
  }
  props.store.resign()
}
</script>

<template>
  <section class="board-screen">
    <header class="status">
      <div class="players">
        <span class="badge" :class="s.color ?? 'black'">你執{{ colorName(s.color ?? 'black') }}</span>
        <span class="vs">vs</span>
        <span class="badge" :class="s.color === 'black' ? 'white' : 'black'">
          {{ opponent }}執{{ colorName(s.color === 'black' ? 'white' : 'black') }}
        </span>
      </div>
      <div class="turn" :class="{ mine: myTurn }" aria-live="polite">{{ status }}</div>
    </header>

    <p v-if="s.opponentAway" class="notice">對手已斷線，等待重新連線…</p>
    <p v-if="!s.connected" class="notice">與伺服器的連線中斷，正在重新連線…</p>
    <p v-else-if="s.error" class="error" role="alert">{{ s.error }}</p>

    <GoBoard
      :stones="s.moves"
      :hover-color="myTurn ? s.color : null"
      :can-play="store.canPlay"
      @play="store.play"
    />

    <footer class="buttons row">
      <span class="muted small">第 {{ s.moves.length }} 手</span>
      <button v-if="!s.result" :class="{ danger: confirmResign }" @click="resign">
        {{ confirmResign ? '再按一次確認認輸' : '認輸' }}
      </button>
    </footer>

    <div v-if="s.result" class="overlay" role="dialog" aria-modal="true" aria-labelledby="result-title">
      <div class="panel result">
        <h2 id="result-title">{{ resultTitle }}</h2>
        <p class="muted">{{ resultReason }}</p>
        <button class="primary" @click="store.home()">返回主畫面</button>
      </div>
    </div>
  </section>
</template>
