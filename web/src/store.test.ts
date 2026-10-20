import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createStore, type SocketLike } from './store'
import type { Msg } from './protocol'

class FakeSocket implements SocketLike {
  readyState = 0
  sent: Msg[] = []
  onopen: ((ev: Event) => void) | null = null
  onclose: ((ev: CloseEvent) => void) | null = null
  onmessage: ((ev: MessageEvent) => void) | null = null
  send(data: string) {
    this.sent.push(JSON.parse(data))
  }
  close() {
    this.readyState = 3
    this.onclose?.({} as CloseEvent)
  }
  open() {
    this.readyState = 1
    this.onopen?.({} as Event)
  }
  receive(m: Msg) {
    this.onmessage?.({ data: JSON.stringify(m) } as MessageEvent)
  }
  last() {
    return this.sent[this.sent.length - 1]
  }
}

function memoryStorage() {
  const m = new Map<string, string>()
  return {
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, v),
    removeItem: (k: string) => void m.delete(k),
  }
}

function setup(storage = memoryStorage()) {
  const sockets: FakeSocket[] = []
  const store = createStore({
    url: 'ws://test/ws',
    storage,
    reconnectDelayMs: 10,
    socket: () => {
      const s = new FakeSocket()
      sockets.push(s)
      return s
    },
  })
  sockets[0].open()
  return { store, sockets, storage, sock: () => sockets[sockets.length - 1] }
}

const start = (color: 'black' | 'white', mode: 'ai' | 'pvp' = 'ai'): Msg => ({
  type: 'game_start', mode, color, rule: 'freestyle', size: 15, turn: 'black', token: 'tok', code: mode === 'pvp' ? 'ABC234' : '',
})

beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

describe('store', () => {
  it('AI 對戰: picks a side and goes straight to the board', () => {
    const { store, sock } = setup()
    store.chooseMode('ai')
    expect(store.state.screen).toBe('setup')
    store.start('white')
    expect(sock().last()).toEqual({ type: 'start_ai', color: 'white', rule: 'freestyle' })
    sock().receive(start('white'))
    expect(store.state.screen).toBe('board')
    expect(store.state.color).toBe('white')
  })

  it('人人對戰: creating a room shows the join code', () => {
    const { store, sock, storage } = setup()
    store.chooseMode('pvp')
    store.start('black')
    expect(sock().last().type).toBe('create_room')
    sock().receive({ type: 'room_created', code: 'K7QX2M', color: 'black', rule: 'freestyle', token: 't1' })
    expect(store.state.screen).toBe('waiting')
    expect(store.state.code).toBe('K7QX2M')
    expect(storage.getItem('renju.token')).toBe('t1')
    sock().receive(start('black', 'pvp'))
    expect(store.state.screen).toBe('board')
  })

  it('加入房間: sends the code upper-cased and shows server errors', () => {
    const { store, sock } = setup()
    store.openJoin()
    store.join(' k7qx2m ')
    expect(sock().last()).toEqual({ type: 'join_room', code: 'K7QX2M' })
    sock().receive({ type: 'error', error: 'room_not_found' })
    expect(store.state.screen).toBe('join')
    expect(store.state.error).toBe('找不到這個加入碼的房間')
  })

  it('only sends a move on your turn, on an empty point, with the ply', () => {
    const { store, sock } = setup()
    sock().receive(start('black'))
    store.play(7, 7)
    expect(sock().last()).toEqual({ type: 'move', move: { x: 7, y: 7 }, ply: 0 })
    sock().receive({ type: 'move', move: { x: 7, y: 7 }, ply: 0, color: 'black', turn: 'white' })
    const sent = sock().sent.length
    store.play(8, 8) // AI's turn
    expect(sock().sent.length).toBe(sent)
    sock().receive({ type: 'move', move: { x: 8, y: 8 }, ply: 1, color: 'white', turn: 'black' })
    store.play(8, 8) // occupied
    expect(sock().sent.length).toBe(sent)
    store.play(6, 6)
    expect(sock().last()).toEqual({ type: 'move', move: { x: 6, y: 6 }, ply: 2 })
    expect(store.state.moves.map((m) => m.color)).toEqual(['black', 'white'])
  })

  it('game over shows the result, clears the token, and home resets', () => {
    const { store, sock, storage } = setup()
    sock().receive(start('black'))
    sock().receive({ type: 'game_over', winner: 'black', reason: 'five' })
    expect(store.state.result).toEqual({ winner: 'black', reason: 'five' })
    expect(storage.getItem('renju.token')).toBeNull()
    store.play(0, 0)
    expect(sock().sent).toEqual([])
    store.home()
    expect(store.state.screen).toBe('home')
    expect(store.state.moves).toEqual([])
  })

  it('reconnects after a drop and restores the game from the server', () => {
    const { store, sockets, sock } = setup()
    sock().receive(start('white', 'pvp'))
    sock().receive({ type: 'move', move: { x: 7, y: 7 }, ply: 0, color: 'black', turn: 'white' })
    sockets[0].close()
    expect(store.state.connected).toBe(false)
    vi.advanceTimersByTime(20)
    expect(sockets.length).toBe(2)
    sock().open()
    expect(sock().last()).toEqual({ type: 'reconnect', token: 'tok' })
    sock().receive({
      type: 'state', mode: 'pvp', code: 'ABC234', color: 'white', rule: 'freestyle', size: 15,
      turn: 'black', status: 'playing', moves: [{ x: 7, y: 7 }, { x: 7, y: 8 }],
    })
    expect(store.state.screen).toBe('board')
    expect(store.state.moves).toEqual([
      { x: 7, y: 7, color: 'black' },
      { x: 7, y: 8, color: 'white' },
    ])
    // A move broadcast already covered by the state is ignored.
    sock().receive({ type: 'move', move: { x: 7, y: 8 }, ply: 1, color: 'white', turn: 'black' })
    expect(store.state.moves.length).toBe(2)
  })

  it('a reload resumes the game with the saved token', () => {
    const storage = memoryStorage()
    storage.setItem('renju.token', 'saved')
    const { sock } = setup(storage)
    expect(sock().last()).toEqual({ type: 'reconnect', token: 'saved' })
  })

  it('a game that ended while away returns home with a message', () => {
    const { store, sockets, sock, storage } = setup()
    sock().receive(start('black', 'pvp'))
    sockets[0].close()
    vi.advanceTimersByTime(20)
    sock().open()
    sock().receive({ type: 'error', error: 'session_not_found' })
    expect(store.state.screen).toBe('home')
    expect(store.state.error).toBe('對局已在離線期間結束')
    expect(storage.getItem('renju.token')).toBeNull()
  })

  it('tracks the opponent disconnecting and coming back', () => {
    const { store, sock } = setup()
    sock().receive(start('black', 'pvp'))
    sock().receive({ type: 'opponent_disconnected' })
    expect(store.state.opponentAway).toBe(true)
    sock().receive({ type: 'opponent_reconnected' })
    expect(store.state.opponentAway).toBe(false)
  })

  it('cancelling a waiting room leaves it', () => {
    const { store, sock } = setup()
    store.chooseMode('pvp')
    store.start('black')
    sock().receive({ type: 'room_created', code: 'K7QX2M', color: 'black', token: 't1' })
    store.cancelRoom()
    expect(sock().last()).toEqual({ type: 'leave' })
    expect(store.state.screen).toBe('home')
  })
})
