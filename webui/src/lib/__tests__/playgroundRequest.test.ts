import { describe, it, expect } from 'vitest'
import {
  buildRequestPayload,
  extractPlaygroundResponseData,
  extractPlaygroundSSEDelta,
  generateCodeSnippet,
  type CodeLang,
  type SamplingParams,
} from '@/lib/playgroundRequest'
import type { ProtocolType } from '@/stores/playground'
import { t } from '@/i18n'

const ORIGIN = 'http://gw.test'
const MESSAGES = [{ role: 'user', content: 'hi' }]

function params(over: Partial<SamplingParams> = {}): SamplingParams {
  return { systemPrompt: '', temperature: 0.7, topP: 0.95, maxTokens: 0, ...over }
}

function snippet(lang: CodeLang, proto: ProtocolType, over: Partial<SamplingParams> = {}, stream = false) {
  return generateCodeSnippet({
    lang, proto, model: 'qd/gemini-2.5-flash', prompt: 'hi',
    origin: ORIGIN, stream, params: params(over),
  })
}

describe('buildRequestPayload 各家协议规范', () => {
  it('端点与请求头按协议区分，gemini 走 v1beta 并带流式动作', () => {
    expect(buildRequestPayload('openai', 'm', MESSAGES, false, params()).url).toBe('/v1/chat/completions')
    expect(buildRequestPayload('responses', 'm', MESSAGES, false, params()).url).toBe('/v1/responses')
    expect(buildRequestPayload('anthropic', 'm', MESSAGES, false, params()).url).toBe('/v1/messages')
    expect(buildRequestPayload('gemini', 'm', MESSAGES, false, params()).url).toBe('/v1beta/models/m:generateContent')
    expect(buildRequestPayload('gemini', 'm', MESSAGES, true, params()).url).toBe('/v1beta/models/m:streamGenerateContent?alt=sse')
    expect(buildRequestPayload('anthropic', 'm', MESSAGES, false, params()).headers['anthropic-version']).toBe('2023-06-01')
  })

  it('模型名进入 gemini 路径时必须编码，前缀斜杠不改变端点段数', () => {
    expect(buildRequestPayload('gemini', 'qd/gemini-2.5-flash', MESSAGES, false, params()).url)
      .toBe('/v1beta/models/qd%2Fgemini-2.5-flash:generateContent')
  })

  it('系统提示按各家字段落位，空串则整个字段缺席', () => {
    const sys = params({ systemPrompt: '  be terse  ' })
    const openai = buildRequestPayload('openai', 'm', MESSAGES, false, sys)
    expect((openai.body.messages as unknown[])[0]).toEqual({ role: 'system', content: 'be terse' })
    expect(buildRequestPayload('anthropic', 'm', MESSAGES, false, sys).body.system).toBe('be terse')
    expect(buildRequestPayload('gemini', 'm', MESSAGES, false, sys).body.systemInstruction)
      .toEqual({ parts: [{ text: 'be terse' }] })
    expect(buildRequestPayload('responses', 'm', MESSAGES, false, sys).body.instructions).toBe('be terse')

    for (const proto of ['openai', 'responses', 'anthropic', 'gemini'] as ProtocolType[]) {
      const bare = buildRequestPayload(proto, 'm', MESSAGES, false, params())
      expect(Object.keys(bare.body)).not.toContain('system')
      expect(Object.keys(bare.body)).not.toContain('instructions')
      expect(Object.keys(bare.body)).not.toContain('systemInstruction')
      // gemini 把模型放在路径里，其余协议放在请求体
      if (proto === 'gemini') expect(bare.body).not.toHaveProperty('model')
      else expect(bare.body.model).toBe('m')
    }
  })

  it('maxTokens=0 时各家按各自规则处理：省略或落到默认值', () => {
    expect(buildRequestPayload('responses', 'm', MESSAGES, false, params()).body).not.toHaveProperty('max_output_tokens')
    expect(buildRequestPayload('openai', 'm', MESSAGES, false, params()).body).not.toHaveProperty('max_tokens')
    expect(buildRequestPayload('anthropic', 'm', MESSAGES, false, params()).body.max_tokens).toBe(4096)
    expect((buildRequestPayload('gemini', 'm', MESSAGES, false, params()).body.generationConfig as Record<string, unknown>).maxOutputTokens).toBe(2048)
    expect(buildRequestPayload('responses', 'm', MESSAGES, false, params({ maxTokens: 512 })).body.max_output_tokens).toBe(512)
  })

  it('assistant 轮次在 gemini 侧映射为 model，其余协议保持 assistant', () => {
    const history = [{ role: 'assistant', content: 'yo' }]
    expect((buildRequestPayload('gemini', 'm', history, false, params()).body.contents as unknown[])[0])
      .toEqual({ role: 'model', parts: [{ text: 'yo' }] })
    expect((buildRequestPayload('openai', 'm', history, false, params()).body.messages as unknown[])[0])
      .toEqual({ role: 'assistant', content: 'yo' })
  })
})

describe('generateCodeSnippet 客户端示例', () => {
  it('curl 用网关 origin 拼完整地址，并带上协议请求头', () => {
    const curl = snippet('curl', 'anthropic')
    expect(curl).toContain(`curl -X POST "${ORIGIN}/v1/messages" \\`)
    expect(curl).toContain('-H "anthropic-version: 2023-06-01" \\')
    expect(curl).toContain(`-d '${JSON.stringify(buildRequestPayload('anthropic', 'qd/gemini-2.5-flash', MESSAGES, false, params()).body, null, 2)}'`)
  })

  it('python 的 api_key 注释走 i18n，不再吐英文兜底', () => {
    expect(snippet('python', 'openai')).toContain(`# ${t('playground.pyApiKeyComment')}`)
    expect(snippet('python', 'openai')).not.toContain('# ok')
  })

  it('openai 示例带上采样三元组，流式与阻塞分支字面量不同', () => {
    const blocking = snippet('python', 'openai', { temperature: 0.3, topP: 0.9, maxTokens: 1024 })
    expect(blocking).toContain('temperature=0.3')
    expect(blocking).toContain('top_p=0.9')
    expect(blocking).toContain('max_tokens=1024')
    expect(blocking).toContain('stream=False')
    expect(blocking).toContain('print(response.choices[0].message.content)')

    const streaming = snippet('python', 'openai', {}, true)
    expect(streaming).toContain('stream=True')
    expect(streaming).toContain('chunk.choices[0].delta.content')
  })

  it('responses 协议只有存在系统提示时才输出 instructions 行', () => {
    expect(snippet('node', 'responses')).not.toContain('instructions:')
    expect(snippet('node', 'responses', { systemPrompt: 'be terse' })).toContain('instructions: "be terse"')
    expect(snippet('python', 'responses', { systemPrompt: 'be terse' })).toContain('instructions="be terse"')
  })

  it('gemini 流式与非流式使用不同的 SDK 方法', () => {
    expect(snippet('python', 'gemini', {}, true)).toContain('generate_content_stream')
    expect(snippet('python', 'gemini')).toContain('genai.Client')
    expect(snippet('node', 'gemini')).toContain('@google/genai')
    expect(snippet('node', 'gemini')).toContain('ai.models.generateContent({')
  })

  it('anthropic 不再同时发送 temperature 与 top_p，并正确映射思考强度', () => {
    const bare = buildRequestPayload('anthropic', 'm', MESSAGES, false, params())
    expect(bare.body).toHaveProperty('temperature', 0.7)
    expect(bare.body).not.toHaveProperty('top_p')

    const withEffort = buildRequestPayload('anthropic', 'm', MESSAGES, false, params({ reasoningEffort: 'high', maxTokens: 4096 }))
    expect(withEffort.body.thinking).toEqual({ type: 'enabled', budget_tokens: 16384 })
    expect(withEffort.body.max_tokens).toBe(20480)
    expect(withEffort.body).not.toHaveProperty('temperature')

    const oai = buildRequestPayload('openai', 'm', MESSAGES, false, params({ reasoningEffort: 'xhigh' }))
    expect(oai.body.reasoning_effort).toBe('xhigh')

    const resp = buildRequestPayload('responses', 'm', MESSAGES, false, params({ reasoningEffort: 'medium' }))
    expect(resp.body.reasoning).toEqual({ effort: 'medium' })

    const gem = buildRequestPayload('gemini', 'm', MESSAGES, false, params({ reasoningEffort: 'high', maxTokens: 4096 }))
    const gemGC = gem.body.generationConfig as Record<string, unknown>
    expect(gemGC.thinkingConfig).toEqual({ thinkingBudget: 16384, includeThoughts: true })
    expect(gemGC.maxOutputTokens).toBe(20480)
  })

  it('responses 协议可正确提取非流式与流式 reasoning 及 output_text', () => {
    const res = extractPlaygroundResponseData('responses', {
      id: 'resp_1',
      output: [
        { id: 'rs_1', type: 'reasoning', summary: [{ type: 'summary_text', text: 'Plan step' }] },
        { id: 'msg_1', type: 'message', content: [{ type: 'output_text', text: 'Final answer' }] },
      ],
      output_text: 'Final answer',
    })
    expect(res).toEqual({ content: 'Final answer', reasoning: 'Plan step' })

    expect(extractPlaygroundSSEDelta('responses', {
      type: 'response.reasoning_summary_text.delta',
      delta: 'Thinking...',
    })).toEqual({ reasoning: 'Thinking...' })
    expect(extractPlaygroundSSEDelta('responses', {
      type: 'response.output_text.delta',
      delta: 'Hello',
    })).toEqual({ content: 'Hello' })
  })
})
