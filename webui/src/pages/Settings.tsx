import { type Component } from 'solid-js'
import { Card, Empty } from '@/components/ui'

const Settings: Component = () => (
  <div class="space-y-4">
    <h1 class="text-lg font-semibold">设置（含 apiKeyRpm 限流）</h1>
    <Card class="p-6"><Empty message="页面迁移中 — 见 docs/FRONTEND_BLUEPRINT.md" /></Card>
  </div>
)

export default Settings
