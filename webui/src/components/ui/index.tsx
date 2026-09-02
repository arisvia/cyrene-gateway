import { type Component, type JSX, For, Show } from 'solid-js'
import { useToast } from '@/lib/toast'

export const Card: Component<{ class?: string; onClick?: () => void; children?: JSX.Element }> = props => (
  <div
    class={`rounded-[12px] border border-subtle bg-card backdrop-blur-xl ${props.class ?? ''}`}
    onClick={props.onClick}
  >
    {props.children}
  </div>
)

export const Badge: Component<{ tone?: 'green' | 'amber' | 'red' | 'gray' | 'blue'; children?: JSX.Element }> = props => {
  const tones: Record<string, string> = {
    green: 'text-[color:var(--green)] bg-[color:var(--green)]/10',
    amber: 'text-[color:var(--amber)] bg-[color:var(--amber)]/10',
    red: 'text-[color:var(--red)] bg-[color:var(--red)]/10',
    blue: 'text-[color:var(--blue)] bg-[color:var(--blue)]/10',
    gray: 'text-faint bg-white/5',
  }
  return (
    <span class={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-medium ${tones[props.tone ?? 'gray']}`}>
      {props.children}
    </span>
  )
}

export const Empty: Component<{ message: string; children?: JSX.Element }> = props => (
  <div class="py-16 text-center text-sm text-faint">{props.message}</div>
)

export const Spinner: Component = () => (
  <div class="h-5 w-5 rounded-full border-2 border-subtle border-t-accent animate-spin" />
)

export function ToastHost() {
  const { toasts } = useToast()
  return (
    <div class="fixed top-4 right-4 z-[80] space-y-2">
      <For each={toasts()}>
        {t => (
          <div
            class={`px-4 py-2.5 rounded-xl text-sm shadow-lg border backdrop-blur-xl ${t.kind === 'success'
              ? 'border-[color:var(--green)]/30 bg-[color:var(--green)]/10 text-[color:var(--green)]'
              : t.kind === 'error'
                ? 'border-[color:var(--red)]/30 bg-[color:var(--red)]/10 text-[color:var(--red)]'
                : 'border-subtle bg-bg-elevated text-text'}`}
          >
            {t.message}
          </div>
        )}
      </For>
    </div>
  )
}

export { Show }