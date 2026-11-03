import { type Component, type JSX, Show } from 'solid-js'
import { IconInfo } from './icons'

// Hint renders a small info affordance next to a heading, with the detail in a
// hover/focus tooltip instead of a permanently visible caption. Cards in a grid
// reserve a fixed row for that caption today, which wastes vertical space and
// forces every card to the height of the tallest hint.
//
// Hover and focus both open it (keyboard parity), and the panel is positioned
// above-centred so it does not stretch the card the way inline text would.
export const Hint: Component<{
  text: string
  class?: string
  children?: JSX.Element
}> = props => (
  <span class={`relative inline-flex items-center group/hint ${props.class ?? ''}`}>
    <button
      type="button"
      aria-label={props.text}
      class="inline-flex items-center justify-center rounded-full text-faint/70 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 transition-colors"
      tabIndex={0}
    >
      <IconInfo size={13} />
    </button>
    <Show when={props.text}>
      <span
        role="tooltip"
        class="pointer-events-none absolute bottom-full left-1/2 -translate-x-1/2 mb-1.5 z-30 w-max max-w-56 rounded-lg border border-subtle bg-card px-2 py-1.5 text-[11px] leading-snug text-foreground opacity-0 shadow-lg transition-opacity duration-150 group-hover/hint:opacity-100 group-focus-within/hint:opacity-100"
      >
        {props.text}
        {props.children}
      </span>
    </Show>
  </span>
)
