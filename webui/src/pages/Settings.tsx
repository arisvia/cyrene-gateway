import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { A } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { useBackgroundStore } from '@/stores/background'
import { Card, Badge, Button, Input, Select, Toggle, Field, confirm } from '@/components/ui'
import { useToast } from '@/lib/toast'

const cavemanOptions = [
  { value: 'lite', label: '精简 (lite)' },
  { value: 'full', label: '标准极简 (full)' },
  { value: 'ultra', label: '极致极简 (ultra)' },
  { value: 'wenyan-lite', label: '半文言 (wenyan-lite)' },
  { value: 'wenyan', label: '文言文 (wenyan)' },
  { value: 'wenyan-ultra', label: '极限文言 (wenyan-ultra)' },
]

const ponytailOptions = [
  { value: 'lite', label: '精简建议 (lite)' },
  { value: 'full', label: '阶梯原则 (full)' },
  { value: 'ultra', label: '极致极简 (ultra)' },
]

const Settings: Component = () => {
  const store = useGatewayStore()
  const bgStore = useBackgroundStore()
  const toast = useToast()
  const [saving, setSaving] = createSignal(false)
  const [local, setLocal] = createSignal<Record<string, unknown>>({})
  const [pw, setPw] = createSignal('')
  // 背景自定义状态
  const [bgUrlInput, setBgUrlInput] = createSignal('')
  onMount(async () => {
    await store.loadSettings()
    setLocal({ ...store.settings() })
    if (bgStore.bgConfig().type === 'url') {
      setBgUrlInput(bgStore.bgConfig().value)
    }
  })

  const dirty = () => {
    const orig = store.settings()
    return Object.keys(local()).some(k => local()[k] !== orig[k])
  }

  const set = (k: string, v: unknown) => setLocal(l => ({ ...l, [k]: v }))

  async function save() {
    setSaving(true)
    try {
      await store.saveSettings(local())
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '保存设置失败')
    } finally {
      setSaving(false)
    }
  }

  async function changePassword() {
    if (pw().length < 8) {
      toast.warning('新密码长度至少需要 8 位')
      return
    }
    try {
      await store.setPassword(pw())
      setPw('')
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '密码更新失败')
    }
  }

  return (
    <div class="space-y-5 stagger">
      <div class="sticky top-16 z-20 bg-bg/90 backdrop-blur-md pt-1 pb-3 flex items-center justify-between gap-3 border-b border-subtle/50">
        <div>
          <h1 class="text-xl font-semibold">设置</h1>
          <p class="text-sm text-faint mt-0.5">网关运行参数与访问控制</p>
        </div>
        <Button variant="primary" loading={saving()} disabled={!dirty()} onClick={save}>
          {dirty() ? '保存更改' : '已是最新'}
        </Button>
      </div>
      {/* 界面与背景自定义 */}
      <Card class="p-5 space-y-4">
        <div class="flex items-center justify-between">
          <div>
            <h3 class="text-sm font-semibold">界面与壁纸</h3>
            <p class="text-xs text-faint mt-0.5">支持上传本地图片或远程图片链接作为网关背景，数据存储于浏览器 IndexedDB</p>
          </div>
          <Show when={bgStore.bgConfig().type !== 'none'}>
            <Button
              size="sm"
              variant="danger"
              onClick={async () => {
                const ok = await confirm({
                  title: '清除自定义壁纸',
                  message: '确定要清除当前壁纸并恢复默认纯净背景吗？',
                  variant: 'danger',
                })
                if (!ok) return
                await bgStore.resetBackground()
                setBgUrlInput('')
                toast.success('已恢复默认背景')
              }}
            >
              清除壁纸
            </Button>
          </Show>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 pt-1">
          {/* 远程图片链接 */}
          <div class="space-y-2">
            <label class="text-xs font-medium text-muted">远程图片 URL</label>
            <div class="flex gap-2">
              <Input
                value={bgUrlInput()}
                placeholder="https://example.com/wallpaper.jpg"
                onInput={setBgUrlInput}
              />
              <Button
                variant="secondary"
                disabled={!bgUrlInput().trim()}
                onClick={async () => {
                  const url = bgUrlInput().trim()
                  if (!url) return
                  await bgStore.setBackground({
                    type: 'url',
                    value: url,
                    blur: bgStore.bgConfig().blur ?? 0,
                    opacity: bgStore.bgConfig().opacity ?? 1,
                  })
                  toast.success('已应用远程壁纸')
                }}
              >
                应用
              </Button>
            </div>
          </div>

          {/* 本地图片上传 */}
          <div class="space-y-2">
            <label class="text-xs font-medium text-muted">本地图片上传</label>
            <div class="flex items-center gap-2">
              <label class="flex-1 cursor-pointer flex items-center justify-center gap-2 px-3 py-2 rounded-control border border-dashed border-subtle hover:border-accent text-xs text-muted hover:text-text transition-colors bg-card/40">
                <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <rect width="18" height="18" x="3" y="3" rx="2" ry="2" />
                  <circle cx="9" cy="9" r="2" />
                  <path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21" />
                </svg>
                <span>选择本地图片...</span>
                <input
                  type="file"
                  accept="image/*"
                  class="hidden"
                  onChange={e => {
                    const file = e.currentTarget.files?.[0]
                    if (!file) return
                    const reader = new FileReader()
                    reader.onload = async () => {
                      const dataUrl = reader.result as string
                      await bgStore.setBackground({
                        type: 'image',
                        value: dataUrl,
                        blur: bgStore.bgConfig().blur ?? 0,
                        opacity: bgStore.bgConfig().opacity ?? 1,
                      })
                      toast.success(`已加载本地图片 (${file.name})`)
                    }
                    reader.readAsDataURL(file)
                  }}
                />
              </label>
            </div>
          </div>
        </div>

        {/* 壁纸虚化与透明度微调 */}
        <Show when={bgStore.bgConfig().type !== 'none'}>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2 border-t border-subtle/50">
            <div class="space-y-1">
              <div class="flex justify-between text-xs">
                <span class="text-muted">背景虚化 (Blur)</span>
                <span class="font-mono text-faint">{bgStore.bgConfig().blur || 0}px</span>
              </div>
              <input
                type="range"
                min="0"
                max="30"
                step="1"
                class="w-full accent-accent cursor-pointer"
                value={bgStore.bgConfig().blur || 0}
                onInput={async e => {
                  const val = Number(e.currentTarget.value)
                  await bgStore.setBackground({
                    ...bgStore.bgConfig(),
                    blur: val,
                  })
                }}
              />
            </div>

            <div class="space-y-1">
              <div class="flex justify-between text-xs">
                <span class="text-muted">背景不透明度 (Opacity)</span>
                <span class="font-mono text-faint">{Math.round((bgStore.bgConfig().opacity ?? 1) * 100)}%</span>
              </div>
              <input
                type="range"
                min="0.1"
                max="1"
                step="0.05"
                class="w-full accent-accent cursor-pointer"
                value={bgStore.bgConfig().opacity ?? 1}
                onInput={async e => {
                  const val = Number(e.currentTarget.value)
                  await bgStore.setBackground({
                    ...bgStore.bgConfig(),
                    opacity: val,
                  })
                }}
              />
            </div>
          </div>
        </Show>


      </Card>

      {/* 访问控制 */}
      <Card class="p-5 space-y-4">
        <h3 class="text-sm font-semibold">访问控制</h3>

        <div class="flex items-start justify-between gap-4">
          <Field label="要求登录" hint="开启后管理面板需密码登录">
            <span />
          </Field>
          <Toggle checked={!!local().requireLogin} onChange={v => set('requireLogin', v)} />
        </div>

        <div class="flex items-start justify-between gap-4">
          <Field label="要求 API Key" hint="开启后 /v1/* 请求必须携带有效 Key">
            <span />
          </Field>
          <Toggle checked={!!local().requireApiKey} onChange={v => set('requireApiKey', v)} />
        </div>

        <Field label="API Key 速率限制" hint="单个 Key 每分钟请求上限，0 表示不限">
          <Input
            type="number"
            class="!w-32"
            value={String(local().apiKeyRpm ?? 0)}
            onInput={v => set('apiKeyRpm', Number(v) || 0)}
          />
        </Field>
      </Card>

      {/* Token 节省 */}
      <Card class="p-5 space-y-4">
        <h3 class="text-sm font-semibold">Token 节省</h3>
        <div class="flex items-start justify-between gap-4">
          <Field label="RTK 压缩" hint="无损清洗工具输出（ANSI 剥离、JSON 紧凑化、空行折叠）与超长首尾截断">
            <span />
          </Field>
          <Toggle checked={!!local().rtkEnabled} onChange={v => set('rtkEnabled', v)} />
        </div>
        <div class="space-y-3 pt-1 border-t border-subtle/50">
          <div class="flex items-start justify-between gap-4">
            <Field label="Caveman" hint="注入极简表达指令，大幅削减模型输出 Token">
              <span />
            </Field>
            <Toggle
              checked={!!local().cavemanEnabled}
              onChange={v => {
                set('cavemanEnabled', v)
                if (v && !local().cavemanLevel) set('cavemanLevel', 'lite')
              }}
            />
          </div>
          <Show when={local().cavemanEnabled}>
            <div class="flex items-center justify-between gap-4 pl-4 border-l-2 border-subtle">
              <span class="text-xs text-muted">压缩级别</span>
              <Select
                size="sm"
                class="!w-44"
                value={String(local().cavemanLevel || 'lite')}
                options={cavemanOptions}
                onChange={v => set('cavemanLevel', v)}
              />
            </div>
          </Show>
        </div>
        <div class="space-y-3 pt-1 border-t border-subtle/50">
          <div class="flex items-start justify-between gap-4">
            <Field label="Ponytail" hint="注入极简代码指令（Lazy Senior Dev 原则），抑制过度工程">
              <span />
            </Field>
            <Toggle
              checked={!!local().ponytailEnabled}
              onChange={v => {
                set('ponytailEnabled', v)
                if (v && !local().ponytailLevel) set('ponytailLevel', 'lite')
              }}
            />
          </div>
          <Show when={local().ponytailEnabled}>
            <div class="flex items-center justify-between gap-4 pl-4 border-l-2 border-subtle">
              <span class="text-xs text-muted">约束级别</span>
              <Select
                size="sm"
                class="!w-44"
                value={String(local().ponytailLevel || 'lite')}
                options={ponytailOptions}
                onChange={v => set('ponytailLevel', v)}
              />
            </div>
          </Show>
        </div>
      </Card>


      {/* 改密码 */}
      <Card class="p-5 space-y-3">
        <h3 class="text-sm font-semibold">管理密码</h3>
        <Field label="新密码" hint="至少 8 位">
          <Input type="password" value={pw()} onInput={setPw} placeholder="••••••••" class="!w-64" />
        </Field>
        <div class="flex items-center gap-2">
          <Button variant="secondary" disabled={pw().length < 8} onClick={changePassword}>更新密码</Button>
        </div>
      </Card>
      {/* 版本 */}
      <Card class="p-5">
        <div class="flex items-center justify-between text-xs text-faint">
          <span>Cyrene Gateway</span>
          <Badge tone="gray">v{store.version()}</Badge>
        </div>
      </Card>
    </div>
  )
}

export default Settings
