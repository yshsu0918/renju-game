import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import GoBoard from './GoBoard.vue'

describe('GoBoard', () => {
  it('draws a 15×15 grid of click targets and the stones', () => {
    const w = mount(GoBoard, {
      props: {
        stones: [
          { x: 7, y: 7, color: 'black' },
          { x: 7, y: 8, color: 'white' },
        ],
        hoverColor: 'black',
        canPlay: () => true,
      },
    })
    expect(w.findAll('.hits rect')).toHaveLength(225)
    expect(w.findAll('circle.stone.black')).toHaveLength(1)
    expect(w.findAll('circle.stone.white')).toHaveLength(1)
    expect(w.findAll('circle.last')).toHaveLength(1)
  })

  it('emits the clicked intersection', async () => {
    const w = mount(GoBoard, { props: { stones: [], hoverColor: 'black', canPlay: () => true } })
    await w.find('rect[data-x="3"][data-y="11"]').trigger('click')
    expect(w.emitted('play')).toEqual([[3, 11]])
  })

  it('shows no ghost stone when it is not your turn', async () => {
    const w = mount(GoBoard, { props: { stones: [], hoverColor: null, canPlay: () => false } })
    await w.find('rect[data-x="0"][data-y="0"]').trigger('mouseenter')
    expect(w.find('circle.ghost').exists()).toBe(false)
  })
})
