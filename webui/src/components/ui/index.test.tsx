import { afterEach, describe, expect, it, vi } from 'vitest'
import { createSignal, onMount } from 'solid-js'
import { cleanup, fireEvent, render, within } from '@solidjs/testing-library'
import { t } from '@/i18n'
import type { BadgeTone } from '@/types/domain'
import {
  Badge, Button, Card, Checkbox, CyreneLogo, FileUpload, IconButton, Input, Modal, PageHeader, ProviderAvatar,
  SegmentedControl, Select, Slider, TabTransition, Textarea, ToastHost, Toggle,
} from './index'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('shared visual primitives', () => {
  it('draws the Logo from the shared source brand palette', () => {
    const view = render(() => <CyreneLogo />)
    const stops = view.container.querySelectorAll('linearGradient:first-child stop')
    expect(Array.from(stops, stop => stop.getAttribute('stop-color'))).toEqual([
      'var(--brand-mint)', 'var(--brand-cyan)', 'var(--brand-indigo)', 'var(--brand-violet)',
    ])
  })

  it.each([['sm', '8'], ['md', '9'], ['lg', '11']] as const)('protects %s single-line fields from vertical flex compression', (size, height) => {
    const view = render(() => <div class="flex flex-col"><Input size={size} class="flex-1" /><Select size={size} options={[]} /></div>)
    for (const field of [view.getByRole('textbox'), view.getByRole('combobox')]) {
      expect(field.classList.contains(`min-h-${height}`)).toBe(true)
      expect(field.classList.contains('min-w-0')).toBe(true)
    }
  })

  it('preserves responsive Select layout classes without activating their unprefixed forms', () => {
    const view = render(() => <Select options={[]} class="sm:flex-1 sm:w-44" />)
    const wrapper = view.getByRole('combobox').parentElement!
    expect(wrapper.classList.contains('ui-select')).toBe(true)
    expect(wrapper.classList.contains('sm:flex-1')).toBe(true)
    expect(wrapper.classList.contains('sm:w-44')).toBe(true)
    expect(wrapper.classList.contains('flex-1')).toBe(false)
    expect(wrapper.classList.contains('w-full')).toBe(false)
  })

  it('uses theme color for monochrome brands and preserves multicolor artwork', () => {
    const view = render(() => <><ProviderAvatar provider="openai" name="OpenAI" /><ProviderAvatar provider="gemini" name="Gemini" /></>)
    const monochrome = view.getByRole('img', { name: 'OpenAI' })
    expect(monochrome.tagName).toBe('SPAN')
    expect(monochrome.classList.contains('bg-current')).toBe(true)
    expect(monochrome.style.maskImage).toContain('data:image/svg+xml')
    expect(view.getByRole('img', { name: 'Gemini' }).tagName).toBe('IMG')
  })

  it('keeps default headers unboxed and only makes explicitly sticky headers glass', () => {
    const [sticky, setSticky] = createSignal<boolean>()
    const view = render(() => <PageHeader title={t('common.loading')} subtitle={t('common.loadFailed')} sticky={sticky()} actions={<Button>{t('common.retry')}</Button>} />)
    const heading = view.getByRole('heading')
    const header = heading.closest('header')!
    expect(header.classList.contains('sticky')).toBe(false)
    expect(header.classList.contains('glass-sticky')).toBe(false)
    expect(header.classList.contains('px-5')).toBe(false)
    expect(header.className).not.toContain('overflow-hidden')
    expect(heading.className).toContain('text-2xl font-semibold tracking-tight')
    expect(view.getByText(t('common.loadFailed')).className).toContain('text-sm text-muted')
    expect(view.getByText(t('common.loadFailed')).className).not.toContain('truncate')
    expect(view.getByRole('button').parentElement?.classList.contains('flex-wrap')).toBe(true)
    setSticky(true)
    expect(header.classList.contains('sticky')).toBe(true)
    expect(header.classList.contains('glass-sticky')).toBe(true)
    setSticky(false)
    expect(header.classList.contains('glass-sticky')).toBe(false)
  })

  it('polishes hover cards without moving their layout', () => {
    const view = render(() => <Card hover />)
    const card = view.container.firstElementChild!
    expect(card.className).toContain('hover:border-glass-border-hover')
    expect(card.className).toContain('hover:shadow-glass-hover')
    expect(card.className).toContain('transition-[background-color,border-color,box-shadow]')
    expect(card.className).not.toContain('translate')
    expect(card.className).not.toContain('transition-all')
  })

  const tones: [BadgeTone, string][] = [
    ['green', 'success'], ['amber', 'warning'], ['red', 'danger'], ['blue', 'info'],
    ['purple', 'accent'], ['violet', 'accent'], ['indigo', 'accent'],
    ['teal', 'accent-2'], ['cyan', 'accent-2'], ['rose', 'danger'], ['pink', 'danger'], ['orange', 'warning'],
  ]
  it.each(tones)('maps the %s badge to the %s semantic token', (tone, token) => {
    const view = render(() => <Badge tone={tone} />)
    const badge = view.container.firstElementChild!
    expect(badge.classList.contains(`text-${token}`)).toBe(true)
    expect(badge.classList.contains(`bg-${token}/12`)).toBe(true)
    expect(badge.classList.contains(`border-${token}/30`)).toBe(true)
    expect(badge.className).not.toContain('dark:')
  })

  it('gives neutral badges an inset surface', () => {
    const view = render(() => <Badge />)
    expect(view.container.firstElementChild?.className).toContain('bg-surface-inset border-subtle')
  })

  it.each(['primary', 'secondary', 'ghost', 'danger'] as const)('shares the %s variant and stable borders across both button types', variant => {
    const view = render(() => <><Button variant={variant} data-testid="button" /><IconButton variant={variant} data-testid="icon" /></>)
    const button = view.getByTestId('button')
    const icon = view.getByTestId('icon')
    const colors = (el: HTMLElement) => el.className.split(' ').filter(value => /(?:bg-|text-(?:on-accent|text|muted|danger|foreground)|border-|shadow-)/.test(value))
    expect(colors(button)).toEqual(colors(icon))
    for (const control of [button, icon]) {
      expect(control.classList.contains('ui-control')).toBe(true)
      expect(control.classList.contains('border')).toBe(true)
      expect(control.classList.contains('h-9')).toBe(true)
    }
    if (variant === 'primary') expect(button.classList.contains('hover:bg-accent-hover')).toBe(true)
    if (variant === 'secondary') expect(button.classList.contains('border-control-border')).toBe(true)
  })

  it('retains loading and disabled button semantics', () => {
    const click = vi.fn()
    const view = render(() => <><Button loading onClick={click} /><IconButton disabled onClick={click} /></>)
    const buttons = view.getAllByRole('button') as HTMLButtonElement[]
    buttons.forEach(button => {
      expect(button.disabled).toBe(true)
      button.click()
    })
    expect(buttons[0].getAttribute('aria-busy')).toBe('true')
    expect(click).not.toHaveBeenCalled()
  })

  it('uses field styling while preserving sizes and disabled state', () => {
    const view = render(() => <><Input ariaLabel={t('common.loading')} size="lg" disabled /><Textarea ariaLabel={t('common.retry')} disabled /><Select options={[]} ariaLabel={t('common.chooseFile')} size="sm" disabled /></>)
    for (const field of [...view.getAllByRole('textbox'), view.getByRole('combobox')]) {
      expect(field.classList.contains('ui-field')).toBe(true)
      expect(field.hasAttribute('disabled')).toBe(true)
    }
    expect(view.getByRole('textbox', { name: t('common.loading') }).classList.contains('h-11')).toBe(true)
    expect(view.getByRole('combobox').classList.contains('h-8')).toBe(true)
    fireEvent.click(view.getByRole('combobox'))
    expect(view.queryByRole('listbox')).toBeNull()
  })

  it('uses semantic toggle and checkbox surfaces', () => {
    const [checked, setChecked] = createSignal(false)
    const view = render(() => <><Toggle ariaLabel={t('common.loading')} checked={checked()} onChange={setChecked} /><Checkbox checked={checked()} label={t('common.loading')} /></>)
    const toggle = view.getByRole('switch')
    expect(toggle.classList.contains('bg-control-border')).toBe(true)
    expect(toggle.firstElementChild?.classList.contains('bg-switch-thumb')).toBe(true)
    const box = view.getByRole('checkbox').nextElementSibling!
    expect(box.classList.contains('bg-control')).toBe(true)
    expect(box.classList.contains('border-control-border')).toBe(true)
    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-checked')).toBe('true')
    expect(toggle.classList.contains('bg-accent')).toBe(true)
    expect(box.classList.contains('bg-accent')).toBe(true)
  })

  it('keeps segmented borders and weight stable without a selected accent ring', () => {
    const [value, setValue] = createSignal('a')
    const view = render(() => <SegmentedControl value={value()} onChange={setValue} options={[{ value: 'a', label: t('common.loading') }, { value: 'b', label: t('common.retry') }]} />)
    expect(view.getByRole('tablist').classList.contains('bg-surface-inset')).toBe(true)
    const tabs = view.getAllByRole('tab')
    for (const tab of tabs) {
      expect(tab.classList.contains('border')).toBe(true)
      expect(tab.classList.contains('font-medium')).toBe(true)
      expect(tab.className).not.toContain('ring-accent')
    }
    fireEvent.click(tabs[1])
    expect(tabs[1].classList.contains('glass-segment')).toBe(true)
    expect(tabs[0].classList.contains('glass-segment')).toBe(false)
  })

  it('caps toast width with margins and keeps native slider tracks transparent', () => {
    const view = render(() => <><ToastHost /><Slider label={t('common.loading')} value={25} /></>)
    const viewport = view.getByRole('status')
    expect(viewport.classList.contains('w-[calc(100%-2rem)]')).toBe(true)
    expect(viewport.classList.contains('max-w-sm')).toBe(true)
    const slider = view.getByRole('slider', { name: t('common.loading') })
    expect(slider.className).toContain('[&::-webkit-slider-runnable-track]:bg-transparent')
    expect(slider.className).toContain('[&::-moz-range-track]:bg-transparent')
    expect(slider.previousElementSibling?.firstElementChild?.getAttribute('style')).toContain('25%')
  })
})

const options = Array.from({ length: 12 }, (_, index) => ({ value: String(index), label: `model ${index}` }))

describe('Select', () => {
  it('names and connects its listbox without placing search inside the options', async () => {
    render(() => <Select options={options} ariaLabel={t('common.loading')} />)
    const view = within(document.body)
    const trigger = view.getByRole('combobox')
    fireEvent.click(trigger)
    await Promise.resolve()
    const listbox = view.getByRole('listbox', { name: t('common.loading') })
    expect(trigger.getAttribute('aria-controls')).toBe(listbox.id)
    expect(listbox.contains(view.getByRole('textbox'))).toBe(false)
    expect(within(listbox).getAllByRole('option')).toHaveLength(options.length)
  })

  it.each([
    { viewport: 200, height: 180, left: -30, top: 40, width: 700, align: 'left' as const },
    { viewport: 320, height: 240, left: 290, top: 180, width: 80, align: 'left' as const },
    { viewport: 320, height: 240, left: -100, top: 40, width: 100, align: 'right' as const },
    { viewport: 700, height: 600, left: 620, top: 550, width: 120, align: 'right' as const },
  ])('clamps $align popovers in a $viewport px viewport', ({ viewport, height, left, top, width, align }) => {
    vi.stubGlobal('innerWidth', viewport)
    vi.stubGlobal('innerHeight', height)
    render(() => <Select options={options} align={align} />)
    const view = within(document.body)
    const trigger = view.getByRole('combobox')
    vi.spyOn(trigger.parentElement!, 'getBoundingClientRect').mockReturnValue(new DOMRect(left, top, width, 36))
    trigger.focus()
    fireEvent.click(trigger)
    const popover = view.getByRole('listbox').parentElement!
    const actualLeft = parseFloat(popover.style.left)
    const actualWidth = parseFloat(popover.style.width)
    expect(actualLeft).toBeGreaterThanOrEqual(8)
    expect(actualLeft + actualWidth).toBeLessThanOrEqual(viewport - 8)
    expect(parseFloat(popover.style.minWidth)).toBeLessThanOrEqual(viewport - 16)
    expect(parseFloat(popover.style.maxHeight)).toBeGreaterThanOrEqual(0)
    const anchor = parseFloat(popover.style.top || popover.style.bottom)
    expect(anchor).toBeGreaterThanOrEqual(8)
    expect(anchor + parseFloat(popover.style.maxHeight)).toBeLessThanOrEqual(height - 8)
    const list = popover.querySelector('.overflow-y-auto')!
    expect(list.classList.contains('min-h-0')).toBe(true)
    expect(list.classList.contains('px-1')).toBe(true)
    vi.stubGlobal('innerWidth', 180)
    fireEvent(window, new Event('resize'))
    expect(parseFloat(popover.style.left) + parseFloat(popover.style.width)).toBeLessThanOrEqual(172)
  })

  it('allows spaces in search and restores trigger focus on Escape and selection', async () => {
    const onChange = vi.fn()
    render(() => <Select options={options} onChange={onChange} />)
    const view = within(document.body)
    const trigger = view.getByRole('combobox')
    trigger.focus()
    fireEvent.click(trigger)
    await Promise.resolve()
    const search = view.getByRole('textbox')
    expect(document.activeElement).toBe(search)
    expect(fireEvent.keyDown(search, { key: ' ' })).toBe(true)
    expect(view.queryByRole('listbox')).not.toBeNull()
    fireEvent.input(search, { target: { value: 'model 1' } })
    expect(view.getAllByRole('option')).toHaveLength(3)
    fireEvent.keyDown(search, { key: 'Escape' })
    expect(view.queryByRole('listbox')).toBeNull()
    expect(document.activeElement).toBe(trigger)
    fireEvent.click(trigger)
    await Promise.resolve()
    fireEvent.click(view.getByRole('option', { name: 'model 10' }))
    expect(onChange).toHaveBeenCalledWith('10')
    expect(view.queryByRole('listbox')).toBeNull()
    expect(document.activeElement).toBe(trigger)
    fireEvent.click(trigger)
    expect(view.getAllByRole('option')).toHaveLength(options.length)
  })

  it('preserves arrow selection and does not reclaim focus after an outside pointer event', () => {
    const [value, setValue] = createSignal('0')
    render(() => <><Select options={options.slice(0, 3)} value={value()} onChange={setValue} /><Button>{t('common.close')}</Button></>)
    const view = within(document.body)
    const trigger = view.getByRole('combobox')
    trigger.focus()
    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    expect(value()).toBe('1')
    fireEvent.keyDown(trigger, { key: 'Enter' })
    expect(view.queryByRole('listbox')).toBeNull()
    expect(document.activeElement).toBe(trigger)
    fireEvent.click(trigger)
    const outside = view.getByRole('button', { name: t('common.close') })
    outside.focus()
    fireEvent.pointerDown(outside)
    expect(view.queryByRole('listbox')).toBeNull()
    expect(document.activeElement).toBe(outside)
  })
})

describe('Modal', () => {
  it('traps both Tab directions, scrolls only the body, and restores focus and overflow', async () => {
    const originalOverflow = document.body.style.overflow
    document.body.style.overflow = 'scroll'
    const [open, setOpen] = createSignal(false)
    render(() => <><Button onClick={() => setOpen(true)}>{t('common.chooseFile')}</Button><Modal open={open()} title={t('common.loading')} onClose={() => setOpen(false)}><Input disabled /><Button>{t('common.retry')}</Button></Modal></>)
    const view = within(document.body)
    const opener = view.getByRole('button', { name: t('common.chooseFile') })
    opener.focus()
    fireEvent.click(opener)
    await Promise.resolve()
    const dialog = view.getByRole('dialog')
    const first = within(dialog).getByRole('button', { name: t('common.close') })
    const last = within(dialog).getByRole('button', { name: t('common.retry') })
    expect(document.activeElement).toBe(first)
    expect(document.body.style.overflow).toBe('hidden')
    expect(dialog.classList.contains('max-h-[calc(100dvh-2rem)]')).toBe(true)
    expect(dialog.lastElementChild?.className).toContain('min-h-0 overflow-y-auto px-5')
    expect(dialog.previousElementSibling?.classList.contains('bg-overlay')).toBe(true)
    fireEvent.keyDown(first, { key: 'Tab', shiftKey: true })
    expect(document.activeElement).toBe(last)
    fireEvent.keyDown(last, { key: 'Tab' })
    expect(document.activeElement).toBe(first)
    opener.focus()
    fireEvent.keyDown(opener, { key: 'Tab' })
    expect(document.activeElement).toBe(first)
    fireEvent.keyDown(first, { key: 'Escape' })
    expect(view.queryByRole('dialog')).toBeNull()
    expect(document.activeElement).toBe(opener)
    expect(document.body.style.overflow).toBe('scroll')
    document.body.style.overflow = originalOverflow
  })

  it.each([false, true])('continues Tab navigation from a nested select trigger (shift=%s)', async shiftKey => {
    render(() => <Modal open title={t('common.loading')} onClose={() => {}}><Select options={options} /><Button>{t('common.save')}</Button></Modal>)
    await Promise.resolve()
    const view = within(document.body)
    const trigger = view.getByRole('combobox')
    trigger.focus()
    fireEvent.click(trigger)
    await Promise.resolve()
    expect(fireEvent.keyDown(view.getByRole('textbox'), { key: 'Tab', shiftKey })).toBe(true)
    expect(view.queryByRole('listbox')).toBeNull()
    expect(document.activeElement).toBe(trigger)
  })

  it('closes a nested select before dismissing the modal on Escape', async () => {
    const onClose = vi.fn()
    render(() => <Modal open title={t('common.loading')} onClose={onClose}><Select options={options} /></Modal>)
    const view = within(document.body)
    await Promise.resolve()
    const trigger = view.getByRole('combobox')
    trigger.focus()
    fireEvent.click(trigger)
    await Promise.resolve()
    fireEvent.keyDown(view.getByRole('textbox'), { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()
    expect(view.queryByRole('listbox')).toBeNull()
    expect(document.activeElement).toBe(trigger)
    fireEvent.keyDown(trigger, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledOnce()
  })
})

describe('FileUpload', () => {
  it('supports keyboard activation and blocks all disabled drop-zone paths', () => {
    const [disabled, setDisabled] = createSignal(false)
    const onChange = vi.fn()
    const view = render(() => <FileUpload disabled={disabled()} onChange={onChange} />)
    const picker = view.container.querySelector<HTMLInputElement>('input[type="file"]')!
    const click = vi.spyOn(picker, 'click').mockImplementation(() => {})
    const zone = view.getByRole('button')
    const file = new File(['test'], 'fixture.txt', { type: 'text/plain' })
    fireEvent.keyDown(zone, { key: 'Enter' })
    fireEvent.keyDown(zone, { key: ' ' })
    expect(click).toHaveBeenCalledTimes(2)
    fireEvent.drop(zone, { dataTransfer: { files: [file] } })
    expect(onChange).toHaveBeenCalledWith(file)
    click.mockClear()
    onChange.mockClear()
    setDisabled(true)
    expect(zone.tabIndex).toBe(-1)
    expect(zone.getAttribute('aria-disabled')).toBe('true')
    expect(picker.disabled).toBe(true)
    fireEvent.click(zone)
    fireEvent.keyDown(zone, { key: 'Enter' })
    fireEvent.keyDown(zone, { key: ' ' })
    fireEvent.dragOver(zone)
    fireEvent.drop(zone, { dataTransfer: { files: [file] } })
    fireEvent.change(picker, { target: { files: [file] } })
    expect(click).not.toHaveBeenCalled()
    expect(onChange).not.toHaveBeenCalled()
    expect(zone.classList.contains('bg-accent/10')).toBe(false)
  })

  it('disables changing and clearing an existing file, then permits clearing when enabled', () => {
    const [disabled, setDisabled] = createSignal(true)
    const onChange = vi.fn()
    const file = new File(['test'], 'fixture.txt')
    const view = render(() => <FileUpload value={file} disabled={disabled()} onChange={onChange} />)
    const change = view.getByRole('button', { name: t('common.change') }) as HTMLButtonElement
    const clear = view.getByRole('button', { name: t('common.clear') }) as HTMLButtonElement
    expect(change.disabled).toBe(true)
    expect(clear.disabled).toBe(true)
    change.click()
    clear.click()
    expect(onChange).not.toHaveBeenCalled()
    setDisabled(false)
    fireEvent.click(clear)
    expect(onChange).toHaveBeenCalledWith(null)
  })

  it('respects disabled state in compact mode', () => {
    const view = render(() => <FileUpload compact disabled />)
    expect((view.getByRole('button') as HTMLButtonElement).disabled).toBe(true)
  })
})

describe('TabTransition', () => {
  it('makes outgoing snapshots inert and preserves live state after animation cleanup', () => {
    vi.useFakeTimers()
    const [tab, setTab] = createSignal('a')
    const mounted = vi.fn()
    const Stateful = () => {
      const [value, setValue] = createSignal('')
      onMount(mounted)
      return <Input ariaLabel={t('common.loading')} value={value()} onInput={setValue} />
    }
    const view = render(() => <TabTransition value={tab()} order={['a', 'b', 'c']} views={{ a: () => <Stateful />, b: () => <Stateful />, c: () => <Stateful /> }} />)
    setTab('b')
    const snapshot = view.container.querySelector('[inert]')!
    expect(snapshot.getAttribute('aria-hidden')).toBe('true')
    expect(view.getAllByRole('textbox')).toHaveLength(1)
    const input = view.getByRole('textbox')
    input.focus()
    fireEvent.input(input, { target: { value: 'draft' } })
    const mounts = mounted.mock.calls.length
    vi.advanceTimersByTime(260)
    expect(view.container.querySelector('[inert]')).toBeNull()
    expect(view.getByRole('textbox')).toBe(input)
    expect((input as HTMLInputElement).value).toBe('draft')
    expect(document.activeElement).toBe(input)
    expect(mounted).toHaveBeenCalledTimes(mounts)
    expect(view.container.firstElementChild?.className).not.toContain('overflow-x-hidden')
    setTab('c')
    vi.advanceTimersByTime(100)
    setTab('a')
    expect(view.container.querySelectorAll('[inert]')).toHaveLength(1)
    expect(view.container.querySelector('[inert]')?.className).toContain('animate-tab-slide-out-right')
    vi.advanceTimersByTime(260)
    expect(view.container.querySelector('[inert]')).toBeNull()
  })

  it('skips cloning and timers when reduced motion is requested', () => {
    vi.useFakeTimers()
    const media = window.matchMedia('(prefers-reduced-motion: reduce)')
    vi.spyOn(window, 'matchMedia').mockReturnValue({ ...media, matches: true })
    const [tab, setTab] = createSignal('a')
    const view = render(() => <TabTransition value={tab()} views={{ a: () => <Input />, b: () => <Textarea /> }} />)
    const clone = vi.spyOn(view.getByRole('textbox').parentElement!, 'cloneNode')
    const timers = vi.getTimerCount()
    setTab('b')
    expect(clone).not.toHaveBeenCalled()
    expect(view.container.querySelector('[inert]')).toBeNull()
    expect(view.container.firstElementChild?.className).not.toContain('overflow-x-hidden')
    expect(view.getByRole('textbox').parentElement?.className).not.toContain('animate-tab')
    expect(vi.getTimerCount()).toBe(timers)
  })
})
