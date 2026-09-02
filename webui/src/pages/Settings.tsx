import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { Card, Badge, Button, Input, Select, Toggle, Field } from '@/components/ui'

const Settings: Component = () => {
  const store = useGatewayStore()
  const [saving, setSaving] = createSignal(false)
  const [local, setLocal] = createSignal<Record<string, unknown>>({})
  const [pw, setPw] = createSignal('')
  const [pwMsg, setPwMsg] = createSignal('')
  const [keyName, setKeyName] = createSignal('')
  const [creatingKey, setCreatingKey] = createSignal(false)
  onMount(async () => {
    await store.loadSettings()
    setLocal({ ...store.settings() })
  })

  const dirty = () => {
    const orig = store.settings()
    return Object.keys(local()).some(k => local()[k] !== orig[k])
  }

  const set = (k: string, v: unknown) => setLocal(l => ({ ...l, [k]: v }))

  async function save() {
    setSaving(true)
    try { await store.saveSettings(local()) } catch (e) { console.error(e) }
    finally { setSaving(false) }
  }

  async function changePassword() {
    if (pw().length < 8) { setPwMsg('密码至少 8 位'); return }
    setPwMsg('')
    try {
      await store.setPassword(pw())
      setPw('')
      setPwMsg('密码已更新')
    } catch (e: unknown) { setPwMsg(e instanceof Error ? e.message : '设置失败') }
  }

  return (
    <div class="space-y-5">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-xl font-semibold">设置</h1>
          <p class="text-sm text-faint mt-0.5">网关运行参数与访问控制</p>
        </div>
        <Button variant="primary" loading={saving()} disabled={!dirty()} onClick={save}>
          {dirty() ? '保存更改' : '已是最新'}
        </Button>
      </div>

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

      {/* 调度 */}
      <Card class="p-5 space-y-4">
        <h3 class="text-sm font-semibold">调度策略</h3>
        <Field label="组合默认策略" hint="新建组合时使用的默认调度方式">
          <Select
            value={String(local().comboStrategy || 'fallback')}
            options={[
              { value: 'fallback', label: '故障回退' },
              { value: 'round-robin', label: '轮询' },
            ]}
            onChange={v => set('comboStrategy', v)}
          />
        </Field>
      </Card>

      {/* Token 节省 */}
      <Card class="p-5 space-y-4">
        <h3 class="text-sm font-semibold">Token 节省</h3>
        <div class="flex items-start justify-between gap-4">
          <Field label="RTK 压缩" hint="对重复上下文做引用压缩">
            <span />
          </Field>
          <Toggle checked={!!local().rtkEnabled} onChange={v => set('rtkEnabled', v)} />
        </div>
        <div class="flex items-start justify-between gap-4">
          <Field label="Caveman" hint="极简表达，最大幅度压缩">
            <span />
          </Field>
          <Toggle checked={!!local().cavemanEnabled} onChange={v => set('cavemanEnabled', v)} />
        </div>
        <div class="flex items-start justify-between gap-4">
          <Field label="Ponytail" hint="保留尾部关键信息的压缩">
            <span />
          </Field>
          <Toggle checked={!!local().ponytailEnabled} onChange={v => set('ponytailEnabled', v)} />
        </div>
      </Card>

      {/* 密钥管理 */}
      <Card class="p-5 space-y-4">
        <div class="flex items-center justify-between">
          <h3 class="text-sm font-semibold">API 密钥</h3>
          <Button size="sm" variant="secondary" onClick={() => store.loadKeys()}>刷新</Button>
        </div>

        <Show when={store.apiKeys().length > 0} fallback={
          <div class="text-xs text-faint">尚未创建密钥</div>
        }>
          <div class="space-y-1.5">
            <For each={store.apiKeys()}>
              {k => (
                <div class="flex items-center gap-2 text-xs">
                  <span class="truncate">{k.name || '(未命名)'}</span>
                  <code class="flex-1 truncate text-faint font-mono">{k.key}</code>
                  <Button size="sm" variant="danger" onClick={() => store.deleteKey(k.id)}>删除</Button>
                </div>
              )}
            </For>
          </div>
        </Show>

        <div class="flex gap-2 pt-1">
          <Input
            value={keyName()}
            placeholder="新密钥名称"
            onInput={setKeyName}
            disabled={creatingKey()}
          />
          <Button
            variant="secondary"
            loading={creatingKey()}
            disabled={!keyName().trim()}
            onClick={async () => {
              const n = keyName().trim()
              if (!n) return
              setCreatingKey(true)
              try { await store.createKey(n); setKeyName('') }
              catch (e) { console.error('[settings] createKey failed:', e) }
              finally { setCreatingKey(false) }
            }}
          >创建</Button>
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
          <Show when={pwMsg()}><span class="text-xs text-faint">{pwMsg()}</span></Show>
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
