import { type Component } from 'solid-js'
import { Card, Empty } from '@/components/ui'

const CliToolDetail: Component = () => (
  <div class="space-y-4">
    <h1 class="text-lg font-semibold">CLI 工具详情</h1>
    <Card class="p-6"><Empty message="页面迁移中 — 见 docs/FRONTEND_BLUEPRINT.md" /></Card>
  </div>
)

export default CliToolDetail
