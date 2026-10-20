<script setup lang="ts">
import { computed, ref } from 'vue'
import { BOARD_SIZE, type Color } from '../protocol'
import type { Stone } from '../store'

const props = defineProps<{
  stones: Stone[]
  /** Color of the ghost stone shown on hover; null when the player cannot move. */
  hoverColor: Color | null
  canPlay: (x: number, y: number) => boolean
}>()
const emit = defineEmits<{ play: [x: number, y: number] }>()

const CELL = 40
const MARGIN = 30
const SIZE = MARGIN * 2 + CELL * (BOARD_SIZE - 1)
const STARS = [[3, 3], [3, 11], [11, 3], [11, 11], [7, 7]]
const idx = [...Array(BOARD_SIZE).keys()]
const pos = (i: number) => MARGIN + i * CELL

const hover = ref<{ x: number; y: number } | null>(null)
const last = computed(() => props.stones[props.stones.length - 1])
const showGhost = computed(
  () => hover.value && props.hoverColor && props.canPlay(hover.value.x, hover.value.y),
)

// Column letters A–O and row numbers 15–1, top to bottom.
const colLabel = (i: number) => String.fromCharCode(65 + i)
const rowLabel = (i: number) => String(BOARD_SIZE - i)
</script>

<template>
  <svg
    class="board"
    :viewBox="`0 0 ${SIZE} ${SIZE}`"
    role="grid"
    aria-label="15×15 棋盤"
    @mouseleave="hover = null"
  >
    <defs>
      <radialGradient id="stone-black" cx="35%" cy="30%" r="70%">
        <stop offset="0%" stop-color="#666" />
        <stop offset="100%" stop-color="#111" />
      </radialGradient>
      <radialGradient id="stone-white" cx="35%" cy="30%" r="75%">
        <stop offset="0%" stop-color="#fff" />
        <stop offset="100%" stop-color="#cfcac0" />
      </radialGradient>
    </defs>
    <rect class="wood" x="0" y="0" :width="SIZE" :height="SIZE" rx="8" />
    <g class="grid">
      <line v-for="i in idx" :key="'h' + i" :x1="pos(0)" :x2="pos(14)" :y1="pos(i)" :y2="pos(i)" />
      <line v-for="i in idx" :key="'v' + i" :y1="pos(0)" :y2="pos(14)" :x1="pos(i)" :x2="pos(i)" />
      <circle v-for="[sx, sy] in STARS" :key="`s${sx}-${sy}`" :cx="pos(sx)" :cy="pos(sy)" r="4" class="star" />
    </g>
    <g class="labels">
      <text v-for="i in idx" :key="'c' + i" :x="pos(i)" :y="MARGIN / 2 + 3">{{ colLabel(i) }}</text>
      <text v-for="i in idx" :key="'r' + i" :x="MARGIN / 2 - 2" :y="pos(i) + 4">{{ rowLabel(i) }}</text>
    </g>
    <circle
      v-for="s in stones"
      :key="`${s.x}-${s.y}`"
      :cx="pos(s.x)"
      :cy="pos(s.y)"
      r="17"
      :class="['stone', s.color]"
      :fill="`url(#stone-${s.color})`"
    />
    <circle v-if="last" :cx="pos(last.x)" :cy="pos(last.y)" r="5" class="last" />
    <circle
      v-if="showGhost && hover"
      :cx="pos(hover.x)"
      :cy="pos(hover.y)"
      r="17"
      :class="['ghost', hoverColor]"
      :fill="`url(#stone-${hoverColor})`"
    />
    <g class="hits">
      <template v-for="x in idx" :key="'x' + x">
        <rect
          v-for="y in idx"
          :key="`${x}-${y}`"
          :data-x="x"
          :data-y="y"
          :x="pos(x) - CELL / 2"
          :y="pos(y) - CELL / 2"
          :width="CELL"
          :height="CELL"
          :class="{ playable: hoverColor && canPlay(x, y) }"
          @mouseenter="hover = { x, y }"
          @click="emit('play', x, y)"
        />
      </template>
    </g>
  </svg>
</template>

<style scoped>
.board {
  width: 100%;
  max-width: 620px;
  display: block;
  margin: 0 auto;
  user-select: none;
  touch-action: manipulation;
}
.wood {
  fill: var(--board);
}
.grid line {
  stroke: var(--grid);
  stroke-width: 1.2;
}
.star {
  fill: var(--grid);
}
.labels text {
  fill: var(--grid);
  font-size: 12px;
  text-anchor: middle;
  opacity: 0.7;
}
.stone.black {
  stroke: #000;
  stroke-width: 0.5;
}
.stone.white {
  stroke: #9a948a;
  stroke-width: 0.8;
}
.last {
  fill: #e0412f;
}
.ghost {
  opacity: 0.45;
  pointer-events: none;
}
.hits rect {
  fill: transparent;
}
.hits rect.playable {
  cursor: pointer;
}
</style>
