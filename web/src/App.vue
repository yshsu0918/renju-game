<script setup lang="ts">
import { onBeforeUnmount } from 'vue'
import { createStore, defaultUrl } from './store'
import BoardScreen from './components/BoardScreen.vue'
import HomeScreen from './components/HomeScreen.vue'
import JoinScreen from './components/JoinScreen.vue'
import SetupScreen from './components/SetupScreen.vue'
import WaitingScreen from './components/WaitingScreen.vue'

const store = createStore({ url: defaultUrl() })
onBeforeUnmount(() => store.stop())
</script>

<template>
  <main>
    <HomeScreen v-if="store.state.screen === 'home'" :store="store" />
    <SetupScreen v-else-if="store.state.screen === 'setup'" :key="store.state.setupMode" :store="store" />
    <WaitingScreen v-else-if="store.state.screen === 'waiting'" :store="store" />
    <JoinScreen v-else-if="store.state.screen === 'join'" :store="store" />
    <BoardScreen v-else :store="store" />
  </main>
</template>
