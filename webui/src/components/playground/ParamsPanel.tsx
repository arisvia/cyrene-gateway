import { type Component, For, Show } from 'solid-js'
import { useI18n } from '@/i18n'
import { usePlaygroundStore, type ProtocolOption, type ProtocolType } from '@/stores/playground'
import { Badge, Button, Card, IconButton, IconClose, IconSliders, IconSparkles, IconZap, Input, Select, Slider, Textarea, Toggle } from '@/components/ui'

interface ParamsPanelProps {
  open: () => boolean
  onClose: () => void
  protocolOptions: () => ProtocolOption[]
  activeMeta: () => ProtocolOption
  presets: () => { label: string; text: string }[]
}

/** 右侧推理参数 Inspector：大屏并列内滚动，小屏侧滑抽屉。采样值直接读写演练场 store。 */
const ParamsPanel: Component<ParamsPanelProps> = props => {
  const { t } = useI18n()
  const {
    protocol, setProtocol, systemPrompt, setSystemPrompt, temperature, setTemperature,
    topP, setTopP, maxTokens, setMaxTokens, stream, setStream,
  } = usePlaygroundStore()

  return (
    <Show when={props.open()}>
      <div
        class="lg:hidden fixed inset-0 z-40 bg-overlay backdrop-blur-sm animate-fade-in"
        onClick={() => props.onClose()}
      />
      <div class="fixed inset-y-0 right-0 z-50 w-80 max-w-[85vw] lg:static lg:w-auto lg:z-auto lg:col-span-4 xl:col-span-3 flex flex-col h-dvh lg:h-full min-h-0 min-w-0 animate-slide-up lg:animate-none">
        <Card class="flex flex-col h-full min-h-0 min-w-0 p-4 rounded-none lg:rounded-card border-l lg:border border-subtle/50">
          <div class="flex items-center justify-between border-b border-subtle/50 pb-2 shrink-0">
            <span class="text-xs font-semibold text-text flex items-center gap-1.5">
              <IconSliders size={14} />
              {t('playground.paramsConfig')}
            </span>
            <div class="flex items-center gap-2">
              <span class="text-xs text-faint">{t('playground.autoSaved')}</span>
              <IconButton
                size="sm"
                class="lg:hidden"
                aria-label={t('common.close')}
                onClick={() => props.onClose()}
              >
                <IconClose size="xs" />
              </IconButton>
            </div>
          </div>
          {/* 参数项独立滚动容器 */}
          <div class="flex-1 min-h-0 overflow-y-auto space-y-4 px-1 pt-2">
          {/* 1. API 协议端点选择卡片 */}
          <div class="p-3 rounded-xl bg-surface-inset border border-subtle/50 space-y-2.5">
            <div class="flex items-center justify-between text-xs">
              <span class="font-medium text-text flex items-center gap-1.5">
                <IconZap size={14} class="text-accent" />
                <span>{t('playground.protocol')}</span>
              </span>
              <Badge tone={props.activeMeta().badgeTone} class="text-[10px] font-mono px-1.5 py-0 uppercase">
                POST
              </Badge>
            </div>
            <Select
              class="w-full text-xs font-mono"
              size="sm"
              value={protocol()}
              options={props.protocolOptions()}
              onChange={v => setProtocol(v as ProtocolType)}
            />
          </div>

          {/* 2. 系统提示词卡片 */}
          <div class="p-3 rounded-xl bg-surface-inset border border-subtle/50 space-y-2.5">
            <div class="flex items-center justify-between text-xs">
              <span class="font-medium text-text flex items-center gap-1.5">
                <IconSparkles size={14} class="text-accent" />
                <span>{t('playground.systemPrompt')}</span>
              </span>
              <Show when={systemPrompt()}>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-5! min-h-0! px-1.5! py-0! text-[11px]! text-muted hover:text-danger cursor-pointer"
                  onClick={() => setSystemPrompt('')}
                >
                  {t('playground.systemPromptReset')}
                </Button>
              </Show>
            </div>
            <Textarea
              class="p-2 text-xs min-h-[80px] resize-y"
              ariaLabel={t('playground.systemPrompt')}
              placeholder={t('playground.systemPromptPlaceholder')}
              value={systemPrompt()}
              onInput={setSystemPrompt}
            />
            <div class="flex flex-wrap items-center gap-1.5 pt-0.5">
              <For each={props.presets()}>
                {preset => {
                  const isActive = () => systemPrompt() === preset.text
                  return (
                    <Button
                      variant={isActive() ? 'primary' : 'secondary'}
                      size="sm"
                      class="h-6! min-h-0! px-2.5! py-0! text-[11px]! rounded-full font-normal transition-all"
                      onClick={() => setSystemPrompt(preset.text)}
                      title={preset.text}
                    >
                      <span class={`w-1.5 h-1.5 rounded-full shrink-0 mr-1.5 ${isActive() ? 'bg-on-accent' : 'bg-muted/70'}`} />
                      <span>{preset.label}</span>
                    </Button>
                  )
                }}
              </For>
            </div>
          </div>

          {/* 3. 采样与推理参数卡片 */}
          <div class="p-3 rounded-xl bg-surface-inset border border-subtle/50 space-y-3.5">
            <div class="flex items-center justify-between text-xs">
              <span class="font-medium text-text flex items-center gap-1.5">
                <IconSliders size={14} class="text-accent" />
                <span>{t('playground.samplingParams')}</span>
              </span>
            </div>

            {/* Temperature (温度) */}
            <div class="space-y-1">
              <Slider
                label={t('playground.paramTemperature')}
                min={0}
                max={2}
                step={0.05}
                value={temperature()}
                valueDisplay={temperature().toFixed(2)}
                onChange={setTemperature}
              />
              <div class="flex justify-between text-[11px] text-faint">
                <span>{t('playground.tempPrecise')}</span>
                <span>{t('playground.tempCreative')}</span>
              </div>
            </div>

            {/* Top P */}
            <div class="space-y-1">
              <Slider
                label={t('playground.paramTopP')}
                min={0}
                max={1}
                step={0.05}
                value={topP()}
                valueDisplay={topP().toFixed(2)}
                onChange={setTopP}
              />
            </div>

            {/* 最大输出 Tokens */}
            <div class="space-y-1.5">
              <label class="block text-xs font-medium text-muted leading-none">{t('playground.paramMaxTokens')}</label>
              <Input
                type="number"
                min={64}
                max={32768}
                step={256}
                size="sm"
                value={maxTokens()}
                onInput={(v: string) => setMaxTokens(parseInt(v) || 2048)}
                class="font-mono"
              />
            </div>

            {/* 流式传输开关 */}
            <div class="flex items-center justify-between gap-3 pt-2.5 border-t border-subtle/40">
              <div class="min-w-0">
                <div class="text-xs font-medium text-text">{t('playground.streamLabel')}</div>
                <div class="text-[11px] text-faint">{t('playground.streamDesc')}</div>
              </div>
              <Toggle
                ariaLabel={t('playground.streamLabel')}
                checked={stream()}
                onChange={setStream}
              />
            </div>
          </div>
          </div>
        </Card>
      </div>
    </Show>
  )
}

export default ParamsPanel
