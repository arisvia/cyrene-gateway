import { t } from '@/i18n'
import type { ProtocolType } from '@/stores/playground'

/** 协议徽章色调，未知协议（历史轮次）落灰。 */
export function protocolBadgeTone(proto?: ProtocolType): 'green' | 'purple' | 'amber' | 'blue' | 'gray' {
  switch (proto) {
    case 'responses':
      return 'purple'
    case 'anthropic':
      return 'amber'
    case 'gemini':
      return 'blue'
    case 'openai':
      return 'green'
    default:
      return 'gray'
  }
}

/** 协议在 UI 上的规范名称，与各厂商对外叫法一致。 */
export function protocolLabel(proto?: ProtocolType): string {
  switch (proto) {
    case 'responses':
      return 'Responses'
    case 'anthropic':
      return 'Messages'
    case 'gemini':
      return 'Gemini'
    case 'openai':
      return 'Chat'
    default:
      return 'Chat'
  }
}

/** 采样参数由调用方给出：与协议无关，构造请求体时按各家规范落位。 */
export interface SamplingParams {
  systemPrompt: string
  temperature: number
  topP: number
  maxTokens: number
}

export interface HistoryMessage {
  role: string
  content: string
}

export interface BuiltRequest {
  url: string
  headers: Record<string, string>
  body: Record<string, unknown>
}

export type CodeLang = 'curl' | 'python' | 'node'

/**
 * 按选定协议构造各家规范的请求体与请求头。
 *
 * 纯函数：采样参数由调用方给，便于与代码示例生成共用同一份真值。
 */
export function buildRequestPayload(
  proto: ProtocolType,
  targetModel: string,
  historyMessages: HistoryMessage[],
  isStream: boolean,
  params: SamplingParams
): BuiltRequest {
  const cleanSys = params.systemPrompt.trim()
  const temp = params.temperature
  const top_p = params.topP
  const max_tokens = params.maxTokens

  switch (proto) {
    case 'responses': {
      const input = historyMessages.map(m => ({
        role: m.role === 'assistant' ? 'assistant' : 'user',
        content: m.content,
      }))
      const body: Record<string, unknown> = {
        model: targetModel,
        input,
        stream: isStream,
        temperature: temp,
        top_p,
      }
      if (max_tokens > 0) {
        body.max_output_tokens = max_tokens
      }
      if (cleanSys) {
        body.instructions = cleanSys
      }
      return {
        url: '/v1/responses',
        headers: { 'Content-Type': 'application/json' },
        body,
      }
    }
    case 'anthropic': {
      const messages = historyMessages.map(m => ({
        role: m.role === 'assistant' ? 'assistant' : 'user',
        content: m.content,
      }))
      const body: Record<string, unknown> = {
        model: targetModel,
        messages,
        max_tokens: max_tokens > 0 ? max_tokens : 4096,
        temperature: temp,
        top_p,
        stream: isStream,
      }
      if (cleanSys) {
        body.system = cleanSys
      }
      return {
        url: '/v1/messages',
        headers: {
          'Content-Type': 'application/json',
          'anthropic-version': '2023-06-01',
        },
        body,
      }
    }
    case 'gemini': {
      const contents = historyMessages.map(m => ({
        role: m.role === 'assistant' ? 'model' : 'user',
        parts: [{ text: m.content }],
      }))
      const body: Record<string, unknown> = {
        contents,
        generationConfig: {
          temperature: temp,
          topP: top_p,
          maxOutputTokens: max_tokens > 0 ? max_tokens : 2048,
        },
      }
      if (cleanSys) {
        body.systemInstruction = {
          parts: [{ text: cleanSys }],
        }
      }
      const action = isStream ? 'streamGenerateContent?alt=sse' : 'generateContent'
      const encodedModel = encodeURIComponent(targetModel)
      return {
        url: `/v1beta/models/${encodedModel}:${action}`,
        headers: { 'Content-Type': 'application/json' },
        body,
      }
    }
    case 'openai':
    default: {
      const reqMessages = cleanSys
        ? [{ role: 'system', content: cleanSys }, ...historyMessages]
        : historyMessages
      const body: Record<string, unknown> = {
        model: targetModel,
        messages: reqMessages,
        temperature: temp,
        top_p,
        stream: isStream,
      }
      if (max_tokens > 0) {
        body.max_tokens = max_tokens
      }
      return {
        url: '/v1/chat/completions',
        headers: { 'Content-Type': 'application/json' },
        body,
      }
    }
  }
}

export interface CodeSnippetInput {
  lang: CodeLang
  proto: ProtocolType
  model: string
  /** 示例用户消息：代码片段只演示单轮调用。 */
  prompt: string
  origin: string
  stream: boolean
  params: SamplingParams
}

/** 生成客户端调用示例（全协议自适应），与真实发送共用 buildRequestPayload 的请求体。 */
export function generateCodeSnippet(o: CodeSnippetInput): string {
  const { lang, proto, model, origin } = o
  const historySample: HistoryMessage[] = [{ role: 'user', content: o.prompt }]
  const reqInfo = buildRequestPayload(proto, model, historySample, o.stream, o.params)
  const boolLit = (yes: string, no: string) => (o.stream ? yes : no)

  if (lang === 'curl') {
    const headerLines = Object.entries(reqInfo.headers)
      .map(([k, v]) => `  -H "${k}: ${v}" \\`)
      .join('\n')
    return `curl -X POST "${origin}${reqInfo.url}" \\\n${headerLines}\n  -d '${JSON.stringify(reqInfo.body, null, 2)}'`
  }

  if (lang === 'python') {
    switch (proto) {
      case 'responses':
        return `from openai import OpenAI

client = OpenAI(
    base_url="${origin}/v1",
    api_key="cyrene-local"  # ${t('playground.pyApiKeyComment')}
)

response = client.responses.create(
    model="${model}",
    input=${JSON.stringify(reqInfo.body.input, null, 4)},${reqInfo.body.instructions ? `\n    instructions=${JSON.stringify(reqInfo.body.instructions)},` : ''}
    stream=${boolLit('True', 'False')}
)

${o.stream ? 'for event in response:\n    if hasattr(event, "delta") and event.delta:\n        print(event.delta, end="", flush=True)' : 'print(response.output_text)'}
`
      case 'anthropic':
        return `import anthropic

client = anthropic.Anthropic(
    base_url="${origin}",
    api_key="cyrene-local"  # ${t('playground.pyApiKeyComment')}
)

response = client.messages.create(
    model="${model}",
    messages=${JSON.stringify(reqInfo.body.messages, null, 4)},${reqInfo.body.system ? `\n    system=${JSON.stringify(reqInfo.body.system)},` : ''}
    max_tokens=${reqInfo.body.max_tokens},
    temperature=${o.params.temperature},
    stream=${boolLit('True', 'False')}
)

${o.stream ? 'for event in response:\n    if event.type == "content_block_delta" and hasattr(event.delta, "text"):\n        print(event.delta.text, end="", flush=True)' : 'print("".join(block.text for block in response.content if hasattr(block, "text")))'}
`
      case 'gemini':
        return `from google import genai

client = genai.Client(
    http_options={"api_version": "v1beta", "base_url": "${origin}"},
    api_key="cyrene-local"  # ${t('playground.pyApiKeyComment')}
)

${o.stream ? `response = client.models.generate_content_stream(
    model="${model}",
    contents=${JSON.stringify(reqInfo.body.contents, null, 4)},
)
for chunk in response:
    print(chunk.text, end="", flush=True)` : `response = client.models.generate_content(
    model="${model}",
    contents=${JSON.stringify(reqInfo.body.contents, null, 4)},
)
print(response.text)`}
`
      case 'openai':
      default:
        return `from openai import OpenAI

client = OpenAI(
    base_url="${origin}/v1",
    api_key="cyrene-local"  # ${t('playground.pyApiKeyComment')}
)

response = client.chat.completions.create(
    model="${model}",
    messages=${JSON.stringify(reqInfo.body.messages, null, 4)},
    temperature=${o.params.temperature},
    top_p=${o.params.topP},
    max_tokens=${o.params.maxTokens},
    stream=${boolLit('True', 'False')}
)

${o.stream ? 'for chunk in response:\n    if chunk.choices[0].delta.content:\n        print(chunk.choices[0].delta.content, end="", flush=True)' : 'print(response.choices[0].message.content)'}
`
    }
  }

  // Node.js
  switch (proto) {
    case 'responses':
      return `import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${origin}/v1",
  apiKey: "cyrene-local",
});

async function main() {
  const response = await client.responses.create({
    model: "${model}",
    input: ${JSON.stringify(reqInfo.body.input, null, 4)},${reqInfo.body.instructions ? `\n    instructions: ${JSON.stringify(reqInfo.body.instructions)},` : ''}
    stream: ${boolLit('true', 'false')},
  });

  ${o.stream ? 'for await (const event of response) {\n    if (event.delta) process.stdout.write(event.delta);\n  }' : 'console.log(response.output_text);'}
}

main();
`
    case 'anthropic':
      return `import Anthropic from "@anthropic-ai/sdk";

const client = new Anthropic({
  baseURL: "${origin}",
  apiKey: "cyrene-local",
});

async function main() {
  const response = await client.messages.create({
    model: "${model}",
    messages: ${JSON.stringify(reqInfo.body.messages, null, 4)},${reqInfo.body.system ? `\n    system: ${JSON.stringify(reqInfo.body.system)},` : ''}
    max_tokens: ${reqInfo.body.max_tokens},
    stream: ${boolLit('true', 'false')},
  });

  ${o.stream ? 'for await (const chunk of response) {\n    if (chunk.type === "content_block_delta" && chunk.delta?.text) {\n      process.stdout.write(chunk.delta.text);\n    }\n  }' : 'console.log(response.content.map(c => c.text).join(""));'}
}

main();
`
    case 'gemini':
      return `import { GoogleGenAI } from "@google/genai";

const ai = new GoogleGenAI({
  apiKey: "cyrene-local",
  httpOptions: { baseUrl: "${origin}" },
});

async function main() {
  ${o.stream ? `const response = await ai.models.generateContentStream({
    model: "${model}",
    contents: ${JSON.stringify(reqInfo.body.contents, null, 4)},
  });
  for await (const chunk of response) {
    process.stdout.write(chunk.text || "");
  }` : `const response = await ai.models.generateContent({
    model: "${model}",
    contents: ${JSON.stringify(reqInfo.body.contents, null, 4)},
  });
  console.log(response.text);`}
}

main();
`
    case 'openai':
    default:
      return `import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${origin}/v1",
  apiKey: "cyrene-local",
});

async function main() {
  const response = await client.chat.completions.create({
    model: "${model}",
    messages: ${JSON.stringify(reqInfo.body.messages, null, 4)},
    temperature: ${o.params.temperature},
    top_p: ${o.params.topP},
    max_tokens: ${o.params.maxTokens},
    stream: ${boolLit('true', 'false')},
  });

  ${o.stream ? 'for await (const chunk of response) {\n    process.stdout.write(chunk.choices[0]?.delta?.content || "");\n  }' : 'console.log(response.choices[0]?.message?.content);'}
}

main();
`
  }
}
