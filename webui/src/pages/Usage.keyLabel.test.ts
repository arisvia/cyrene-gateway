import { describe, expect, it } from 'vitest'
import { maskKey } from '@/lib/format'

// Mirrors the resolution used by the breakdown rows. Kept as a unit test because
// the surrounding component needs a store and a fetch, while the precedence
// rule -- live list beats the name snapshot -- is what actually regressed.
type LiveKey = { id: string; name?: string; key: string }

function resolveKeyLabel(id: string, liveKeys: LiveKey[], snapshot?: Record<string, string>) {
  const live = liveKeys.find(k => k.id === id)
  const deleted = !live && id !== 'default'
  if (live) return { label: live.name || maskKey(live.key), deleted }
  const snap = snapshot?.[id]
  return { label: snap || (id.startsWith('cg-') ? maskKey(id) : id), deleted }
}

describe('key label resolution', () => {
  it('prefers the live name over the stored snapshot', () => {
    const live = [{ id: 'kid-1', name: 'renamed-team', key: 'cg-aaaaaaaaaaaa1111' }]
    // A stale snapshot would show "old-name"; renaming must be immediate.
    expect(resolveKeyLabel('kid-1', live, { 'kid-1': 'old-name' }).label).toBe('renamed-team')
  })

  it('marks a key absent from the live list as deleted', () => {
    expect(resolveKeyLabel('kid-gone', [], { 'kid-gone': 'team-a' })).toEqual({
      label: 'team-a',
      deleted: true,
    })
  })

  it('does not mark the default bucket as deleted', () => {
    expect(resolveKeyLabel('default', []).deleted).toBe(false)
  })

  it('masks an id it has no name for', () => {
    const out = resolveKeyLabel('cg-e1ef3aaabbbbkkq5M', [])
    expect(out.label).toBe(maskKey('cg-e1ef3aaabbbbkkq5M'))
    expect(out.deleted).toBe(true)
  })
})
