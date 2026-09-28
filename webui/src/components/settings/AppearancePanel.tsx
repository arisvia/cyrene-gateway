import { type Component, For, Show, createEffect, createSignal } from 'solid-js'
import { useI18n } from '@/i18n'
import { useThemeStore } from '@/stores/theme'
import { useBackgroundStore } from '@/stores/background'
import { useToast } from '@/lib/toast'
import { fetchRemoteImageDataUrl } from '@/lib/backgroundStore'
import { Badge, Button, Card, FileUpload, Input, Slider, SegmentedControl, confirm, IconCheck, IconDownload, IconMoon, IconPalette, IconSparkles, IconSun, IconUpload } from '@/components/ui'

const AppearancePanel: Component = () => {
  const { t } = useI18n()
  const themeStore = useThemeStore()
  const bgStore = useBackgroundStore()
  const toast = useToast()
  const [bgUrlInput, setBgUrlInput] = createSignal(bgStore.config().remoteUrl || '')
  const [loadingBgUrl, setLoadingBgUrl] = createSignal(false)
  createEffect(() => {
    const rUrl = bgStore.config().remoteUrl
    if (rUrl && !bgUrlInput()) {
      setBgUrlInput(rUrl)
    }
  })

  return (
        <div class="space-y-3.5">

  {/* 主题色与视觉风格卡片 */}
  <Card class="p-5 space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
      <div class="flex flex-wrap items-center gap-2 min-w-0">
        <IconPalette size={16} class="text-accent shrink-0" />
        <div>
          <h3 class="text-sm font-semibold">{t('settings.appearance.themeCardTitle')}</h3>
          <p class="text-xs text-faint mt-0.5">{t('settings.appearance.themeCardSubtitle')}</p>
        </div>
      </div>
      <Badge tone="blue">
        {t(themeStore.colorOptions.find(o => o.id === themeStore.color())?.nameKey ?? 'settings.appearance.themeColorIndigo')}
      </Badge>
    </div>

    <div class="space-y-4">
      {/* 明暗模式 */}
      <div class="p-4 rounded-xl bg-surface-inset space-y-2.5">
        <div class="text-xs font-semibold text-foreground flex items-center gap-1.5">
          <IconSun size={14} class="text-accent" />
          <span>{t('settings.appearance.themeModeTitle')}</span>
          <span class="text-faint font-normal">· {t('settings.appearance.themeModeSubtitle')}</span>
        </div>
        <div class="w-full sm:w-64 pt-0.5">
          <SegmentedControl
            value={themeStore.mode()}
            onChange={v => themeStore.setMode(v as 'dark' | 'light')}
            options={[
              { value: 'dark', label: t('settings.appearance.themeModeDark'), icon: <IconMoon size={14} /> },
              { value: 'light', label: t('settings.appearance.themeModeLight'), icon: <IconSun size={14} /> },
            ]}
            size="sm"
          />
        </div>
      </div>

      {/* 品牌主题色选择网格 */}
      <div class="p-4 rounded-xl bg-surface-inset space-y-2.5">
        <div class="text-xs font-semibold text-foreground flex items-center justify-between gap-1.5">
          <div class="flex items-center gap-1.5">
            <IconSparkles size={14} class="text-accent" />
            <span>{t('settings.appearance.themeColorTitle')}</span>
            <span class="text-faint font-normal">· {t('settings.appearance.themeColorSubtitle')}</span>
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3 pt-0.5">
          <For each={themeStore.colorOptions}>
            {opt => {
              const isSelected = () => themeStore.color() === opt.id
              return (
                <button
                  type="button"
                  class={`group relative flex flex-col p-3 rounded-xl border text-left transition-all duration-200 cursor-pointer overflow-hidden ${
                    isSelected()
                      ? 'bg-accent/10 border-accent shadow-xs ring-1 ring-accent/30'
                      : 'bg-surface-inset hover:bg-glass border-control-border hover:border-subtle/80'
                  }`}
                  onClick={() => themeStore.setColor(opt.id)}
                >
                  <div class="flex items-center justify-between w-full mb-2">
                    <div class="flex items-center gap-2">
                      <span
                        class="w-5 h-5 rounded-full shrink-0 shadow-xs border border-white/20 flex items-center justify-center transition-transform group-hover:scale-105"
                        style={{
                          background: `linear-gradient(135deg, ${opt.primary} 0%, ${opt.secondary} 100%)`,
                        }}
                      />
                      <span class="text-xs font-semibold text-foreground">{t(opt.nameKey)}</span>
                    </div>
                    <Show when={isSelected()}>
                      <span class="w-4 h-4 rounded-full bg-accent text-on-accent flex items-center justify-center shrink-0 animate-scale-in">
                        <IconCheck size={11} stroke-width={3} />
                      </span>
                    </Show>
                  </div>
                  <p class="text-[11px] text-faint leading-relaxed line-clamp-2">
                    {t(opt.descKey)}
                  </p>
                </button>
              )
            }}
          </For>
        </div>
      </div>
    </div>
  </Card>

  {/* 界面与壁纸卡片 */}
  <Card class="p-5 space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
      <div class="flex flex-wrap items-center gap-2 min-w-0">
        <IconPalette size={16} class="text-accent shrink-0" />
        <div>
          <h3 class="text-sm font-semibold">{t('settings.appearance.cardTitle')}</h3>
          <p class="text-xs text-faint mt-0.5">{t('settings.appearance.cardSubtitle')}</p>
        </div>
      </div>
      <Show when={bgStore.hasCustomBg()}>
        <Button
          size="sm"
          variant="danger"
          onClick={async () => {
            const ok = await confirm({
              title: t('settings.appearance.clearBgConfirmTitle'),
              message: t('settings.appearance.clearBgConfirmMessage'),
              variant: 'danger',
            })
            if (!ok) return
            await bgStore.resetWallpaper()
            setBgUrlInput('')
            toast.success(t('toast.resetWallpaperSuccess'))
          }}
        >
          {t('settings.clearWallpaper')}
        </Button>
      </Show>
    </div>

    <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
      {/* 远程图片链接 */}
      <div class="min-w-0 p-4 rounded-xl bg-surface-inset space-y-2.5">
        <label class="text-xs font-semibold text-foreground flex items-center gap-1.5">
          <IconDownload size={14} class="text-accent" />
          <span>{t('settings.remoteImageUrl')}</span>
        </label>
        <div class="grid grid-cols-1 sm:grid-cols-[minmax(0,1fr)_auto] items-center gap-2 pt-0.5">
          <Input
            value={bgUrlInput()}
            placeholder="https://..."
            onInput={setBgUrlInput}
            class="min-w-0"
          />
          <Button
            variant="secondary"
            disabled={!bgUrlInput().trim()}
            loading={loadingBgUrl()}
            class="shrink-0"
            onClick={async () => {
              const url = bgUrlInput().trim()
              if (!url) return
              setLoadingBgUrl(true)
              try {
                // 预先校验图片是否能够成功渲染与加载，杜绝失效 URL 或拦截导致的静默白屏
                const img = new Image()
                img.referrerPolicy = 'no-referrer'
                const { promise: imgPromise, resolve: imgResolve, reject: imgReject } = Promise.withResolvers<boolean>()
                img.onload = () => imgResolve(true)
                img.onerror = () => imgReject(new Error('image load failed'))
                img.src = url

                try {
                  await imgPromise
                } catch {
                  toast.error(t('toast.remoteImageLoadFailed'))
                  return
                }

                let dataUrl = ''
                try {
                  const base64 = await fetchRemoteImageDataUrl(url)
                  if (base64) dataUrl = base64
                } catch {
                  // 跨域或安全拦截直接拉取时，降级使用 direct url 保存进 IndexedDB
                }

                if (!dataUrl) {
                  dataUrl = url
                }

                await bgStore.setWallpaper(dataUrl, { sourceType: 'remote', remoteUrl: url })
                toast.success(t('toast.saveRemoteWallpaperSuccess'))
              } catch {
                toast.error(t('toast.saveRemoteWallpaperFailed'))
              } finally {
                setLoadingBgUrl(false)
              }
            }}
          >
            {t('settings.apply')}
          </Button>
        </div>
      </div>

      {/* 本地图片上传 */}
      <div class="p-4 rounded-xl bg-surface-inset space-y-2.5">
        <label class="text-xs font-semibold text-foreground flex items-center gap-1.5">
          <IconUpload size={14} class="text-accent" />
          <span>{t('settings.localImageUpload')}</span>
        </label>
        <div class="pt-0.5">
          <FileUpload
            compact
            accept="image/*"
            placeholder={t('settings.appearance.selectLocalImage')}
            onChange={async file => {
              if (!file) return
              const reader = new FileReader()
              reader.onload = async () => {
                const dataUrl = reader.result as string
                await bgStore.setWallpaper(dataUrl, { sourceType: 'upload' })
                toast.success(t('toast.saveUploadWallpaperSuccess', { name: file.name }))
              }
              reader.readAsDataURL(file)
            }}
          />
        </div>
      </div>
    </div>

    {/* 壁纸与毛玻璃微调控制面板（仿 zashboard 外观微调系统） */}
    <Show when={bgStore.hasCustomBg()}>
      <div class="p-4 rounded-xl bg-surface-inset space-y-3.5">
        <div class="text-xs font-semibold text-foreground flex items-center gap-1.5">
          <IconSparkles size={14} class="text-accent" />
          <span>{t('settings.appearance.appearanceSliders')}</span>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <Slider
            label={t('settings.appearance.blur')}
            min={0}
            max={30}
            step={1}
            unit="px"
            value={bgStore.config().blur || 0}
            onChange={v => bgStore.updateConfig({ blur: v })}
          />
          <Slider
            label={t('settings.appearance.opacity')}
            min={0.1}
            max={1}
            step={0.05}
            valueDisplay={`${Math.round((bgStore.config().opacity ?? 1) * 100)}%`}
            value={bgStore.config().opacity ?? 1}
            onChange={v => bgStore.updateConfig({ opacity: v })}
          />
          <Slider
            label={t('settings.appearance.surfaceAlpha')}
            min={0.2}
            max={0.95}
            step={0.02}
            valueDisplay={`${Math.round((bgStore.config().surfaceAlpha ?? 0.62) * 100)}%`}
            value={bgStore.config().surfaceAlpha ?? 0.62}
            onChange={v => bgStore.updateConfig({ surfaceAlpha: v })}
          />
          <Slider
            label={t('settings.appearance.glassBlur')}
            min={0}
            max={40}
            step={2}
            unit="px"
            value={bgStore.config().glassBlur ?? 20}
            onChange={v => bgStore.updateConfig({ glassBlur: v })}
          />
        </div>
      </div>
    </Show>
  </Card>
        </div>
  )
}

export default AppearancePanel
