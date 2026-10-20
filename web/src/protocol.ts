// Message shapes shared with the Go server (internal/server/protocol.go).

export type Color = 'black' | 'white'
export type Mode = 'ai' | 'pvp'

export interface Point {
  x: number
  y: number
}

export interface Msg {
  type: string
  mode?: Mode
  code?: string
  token?: string
  color?: Color
  rule?: string
  size?: number
  turn?: Color
  move?: Point
  moves?: Point[]
  ply?: number
  status?: 'waiting' | 'playing' | 'finished'
  winner?: Color | ''
  reason?: string
  error?: string
  message?: string
}

export const BOARD_SIZE = 15

export const errorText: Record<string, string> = {
  room_not_found: '找不到這個加入碼的房間',
  room_full: '房間已滿',
  occupied: '這個位置已經有棋子',
  out_of_bounds: '超出棋盤範圍',
  not_your_turn: '還沒輪到你',
  game_over: '對局已經結束',
  not_in_game: '目前不在對局中',
  already_in_game: '你已經在房間裡',
  unsupported_rule: '尚未支援這個規則',
  bad_request: '請求格式錯誤',
}

export const reasonText: Record<string, string> = {
  five: '五子連線',
  draw: '棋盤已滿，和局',
  resign: '認輸',
  abandoned: '對手斷線未回',
  engine_error: 'AI 引擎發生錯誤',
}

export function colorName(c: Color): string {
  return c === 'black' ? '黑' : '白'
}
