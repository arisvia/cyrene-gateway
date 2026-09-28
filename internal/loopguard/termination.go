package loopguard

// TerminationPrompt is the minimal termination contract injected into system messages.
// Based on VansRouter/Moonshot research: reward stopping + anti-repetition.
const TerminationPrompt = `When you have gathered sufficient information to answer the request, STOP calling tools and provide your final answer. Do not call a tool with the same arguments more than once. If a previous attempt returned the same result, change strategy or summarize with available data. Plan briefly (1-3 steps max), then ACT immediately. Do NOT restate your plan — if you have decided what to do, do it now. If you catch yourself repeating the same intention, STOP and give your answer with current knowledge.`
