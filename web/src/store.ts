import { reactive } from 'vue'
import { errorText, type Color, type Mode, type Msg, type Point } from './protocol'

export type Screen = 'home' | 'setup' | 'waiting' | 'join' | 'board'

export interface Stone extends Point {
  color: Color
}

/** The subset of WebSocket the store uses, so tests can pass a fake. */
export interface SocketLike {
  readonly readyState: number
  send(data: string): void
  close(): void
  onopen: ((ev: Event) => void) | null
  onclose: ((ev: CloseEvent) => void) | null
  onmessage: ((ev: MessageEvent) => void) | null
}

export interface StoreOptions {
  url: string
  socket?: (url: string) => SocketLike
  storage?: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> | null
  reconnectDelayMs?: number
}

const TOKEN_KEY = 'renju.token'
const OPEN = 1

function defaultStorage(): StoreOptions['storage'] {
  // sessionStorage is per tab, so two tabs on one machine can play each other.
  try {
    return window.sessionStorage
  } catch {
    return null
  }
}

function initialState() {
  return {
    screen: 'home' as Screen,
    /** Which home button led to the setup screen. */
    setupMode: 'ai' as Mode,
    mode: null as Mode | null,
    color: null as Color | null,
    rule: 'freestyle',
    code: '',
    moves: [] as Stone[],
    turn: 'black' as Color,
    result: null as { winner: Color | ''; reason: string } | null,
    error: '',
    busy: false,
    opponentAway: false,
  }
}

export function createStore(opts: StoreOptions) {
  const makeSocket = opts.socket ?? ((url: string) => new WebSocket(url))
  const storage = opts.storage === undefined ? defaultStorage() : opts.storage
  const delay = opts.reconnectDelayMs ?? 1000

  const state = reactive({ ...initialState(), connected: false })
  let ws: SocketLike | null = null
  let retry: ReturnType<typeof setTimeout> | null = null
  let stopped = false

  const token = {
    get: () => safe(() => storage?.getItem(TOKEN_KEY) ?? null, null),
    set: (t: string) => safe(() => storage?.setItem(TOKEN_KEY, t), undefined),
    clear: () => safe(() => storage?.removeItem(TOKEN_KEY), undefined),
  }

  function connect() {
    if (stopped) return
    const sock = makeSocket(opts.url)
    ws = sock
    sock.onopen = () => {
      state.connected = true
      const t = token.get()
      if (t) send({ type: 'reconnect', token: t })
    }
    sock.onmessage = (ev) => {
      try {
        handle(JSON.parse(String(ev.data)) as Msg)
      } catch {
        // Ignore malformed frames.
      }
    }
    sock.onclose = () => {
      if (ws !== sock) return
      state.connected = false
      state.busy = false
      ws = null
      if (!stopped) retry = setTimeout(connect, delay)
    }
  }

  function send(m: Msg): boolean {
    if (!ws || ws.readyState !== OPEN) {
      state.error = '與伺服器的連線中斷，正在重新連線…'
      return false
    }
    ws.send(JSON.stringify(m))
    return true
  }

  function reset() {
    Object.assign(state, initialState())
  }

  function handle(m: Msg) {
    switch (m.type) {
      case 'room_created':
        token.set(m.token!)
        Object.assign(state, {
          screen: 'waiting', mode: 'pvp', code: m.code!, color: m.color!,
          rule: m.rule ?? 'freestyle', busy: false, error: '',
        })
        break
      case 'game_start':
        token.set(m.token!)
        Object.assign(state, {
          screen: 'board', mode: m.mode!, code: m.code ?? '', color: m.color!,
          rule: m.rule ?? 'freestyle', turn: m.turn ?? 'black', moves: [], result: null,
          busy: false, error: '', opponentAway: false,
        })
        break
      case 'move':
        // After a reconnect the state message may already include this move.
        if (m.ply !== undefined && m.ply < state.moves.length) break
        state.moves.push({ ...m.move!, color: m.color! })
        state.turn = m.turn ?? state.turn
        state.error = ''
        break
      case 'game_over':
        token.clear()
        state.result = { winner: m.winner ?? '', reason: m.reason ?? '' }
        state.opponentAway = false
        break
      case 'state':
        Object.assign(state, {
          screen: m.status === 'waiting' ? 'waiting' : 'board',
          mode: m.mode!, code: m.code ?? '', color: m.color!, rule: m.rule ?? 'freestyle',
          turn: m.turn ?? 'black', result: null, error: '', busy: false, opponentAway: false,
          moves: (m.moves ?? []).map((p, i) => ({ ...p, color: i % 2 === 0 ? 'black' : 'white' })),
        })
        break
      case 'opponent_disconnected':
        state.opponentAway = true
        break
      case 'opponent_reconnected':
        state.opponentAway = false
        break
      case 'error':
        state.busy = false
        if (m.error === 'session_not_found') {
          // The game ended while we were away.
          token.clear()
          if (state.screen === 'board' && !state.result) {
            reset()
            state.error = '對局已在離線期間結束'
          }
          break
        }
        state.error = errorText[m.error ?? ''] ?? m.message ?? '發生錯誤'
        break
    }
  }

  const actions = {
    /** Home: "AI 對戰" or "人人對戰" opens the side picker. */
    chooseMode(mode: Mode) {
      Object.assign(state, { screen: 'setup', setupMode: mode, error: '' })
    },
    /** Home: "加入房間". */
    openJoin() {
      Object.assign(state, { screen: 'join', error: '' })
    },
    /** Setup: confirm the chosen side. */
    start(color: Color) {
      const type = state.setupMode === 'ai' ? 'start_ai' : 'create_room'
      if (send({ type, color, rule: 'freestyle' })) state.busy = true
    },
    join(code: string) {
      const c = code.trim().toUpperCase()
      if (!c) {
        state.error = '請輸入加入碼'
        return
      }
      if (send({ type: 'join_room', code: c })) state.busy = true
    },
    /** Waiting screen: close the room. */
    cancelRoom() {
      send({ type: 'leave' })
      token.clear()
      reset()
    },
    canPlay(x: number, y: number): boolean {
      return (
        state.screen === 'board' && !state.result && state.turn === state.color &&
        !state.moves.some((s) => s.x === x && s.y === y)
      )
    },
    play(x: number, y: number) {
      if (!actions.canPlay(x, y)) return
      send({ type: 'move', move: { x, y }, ply: state.moves.length })
    },
    resign() {
      if (state.screen === 'board' && !state.result) send({ type: 'leave' })
    },
    home() {
      token.clear()
      reset()
    },
    stop() {
      stopped = true
      if (retry) clearTimeout(retry)
      ws?.close()
    },
  }

  connect()
  return { state, ...actions }
}

export type Store = ReturnType<typeof createStore>

function safe<T>(f: () => T, fallback: T): T {
  try {
    return f()
  } catch {
    return fallback
  }
}

export function defaultUrl(): string {
  const configured = import.meta.env.VITE_WS_URL as string | undefined
  if (configured) return configured
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${location.host}/ws`
}
