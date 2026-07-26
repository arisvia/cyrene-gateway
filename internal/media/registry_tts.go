package media

func registerTTSProviders() {
	// OpenAI TTS
	mergeProvider("openai", "OpenAI", KindTTS,
		[]ModelEntry{
			{ID: "gpt-4o-mini-tts", Name: "GPT-4o Mini TTS", Kind: KindTTS},
			{ID: "tts-1", Name: "TTS-1", Kind: KindTTS},
			{ID: "tts-1-hd", Name: "TTS-1 HD", Kind: KindTTS},
		},
		ProviderConfig{
			Provider:   "openai",
			Kind:       KindTTS,
			BaseURL:    "https://api.openai.com/v1/audio/speech",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "openai",
		},
	)

	// OpenRouter TTS
	mergeProvider("openrouter", "OpenRouter", KindTTS, nil, ProviderConfig{
		Provider:   "openrouter",
		Kind:       KindTTS,
		BaseURL:    "https://openrouter.ai/api/v1/audio/speech",
		AuthType:   "apikey",
		AuthHeader: "bearer",
		Format:     "openai",
	})

	// ElevenLabs
	mergeProvider("elevenlabs", "ElevenLabs", KindTTS,
		[]ModelEntry{
			{ID: "eleven_multilingual_v2", Name: "Multilingual v2", Kind: KindTTS},
			{ID: "eleven_turbo_v2_5", Name: "Turbo v2.5", Kind: KindTTS},
			{ID: "eleven_monolingual_v1", Name: "Monolingual v1", Kind: KindTTS},
		},
		ProviderConfig{
			Provider:   "elevenlabs",
			Kind:       KindTTS,
			BaseURL:    "https://api.elevenlabs.io/v1/text-to-speech",
			AuthType:   "apikey",
			AuthHeader: "x-api-key",
			Format:     "elevenlabs",
		},
	)

	// Google Cloud TTS
	mergeProvider("google-tts", "Google Cloud TTS", KindTTS, nil, ProviderConfig{
		Provider:   "google-tts",
		Kind:       KindTTS,
		BaseURL:    "https://texttospeech.googleapis.com/v1/text:synthesize",
		AuthType:   "apikey",
		AuthHeader: "key",
		Format:     "google-tts",
	})

	// Edge TTS (free, no auth)
	mergeProvider("edge-tts", "Edge TTS", KindTTS, nil, ProviderConfig{
		Provider:   "edge-tts",
		Kind:       KindTTS,
		BaseURL:    "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1",
		AuthType:   "none",
		AuthHeader: "",
		Format:     "edge-tts",
	})

	// Gemini TTS
	mergeProvider("gemini", "Gemini", KindTTS,
		[]ModelEntry{
			{ID: "gemini-2.5-flash-preview-tts", Name: "Gemini 2.5 Flash TTS", Kind: KindTTS},
			{ID: "gemini-2.5-pro-preview-tts", Name: "Gemini 2.5 Pro TTS", Kind: KindTTS},
		},
		ProviderConfig{
			Provider:   "gemini",
			Kind:       KindTTS,
			BaseURL:    "https://generativelanguage.googleapis.com/v1beta/models",
			AuthType:   "apikey",
			AuthHeader: "key",
			Format:     "gemini-tts",
		},
	)

	// Cartesia
	mergeProvider("cartesia", "Cartesia", KindTTS, nil, ProviderConfig{
		Provider:   "cartesia",
		Kind:       KindTTS,
		BaseURL:    "https://api.cartesia.ai/tts/bytes",
		AuthType:   "apikey",
		AuthHeader: "x-api-key",
		Format:     "cartesia",
	})

	// PlayHT
	mergeProvider("playht", "PlayHT", KindTTS, nil, ProviderConfig{
		Provider:   "playht",
		Kind:       KindTTS,
		BaseURL:    "https://api.play.ht/api/v2/tts",
		AuthType:   "apikey",
		AuthHeader: "bearer",
		Format:     "playht",
	})

	// Deepgram (also supports TTS via Aura)
	mergeProvider("deepgram", "Deepgram", KindTTS, nil, ProviderConfig{
		Provider:   "deepgram",
		Kind:       KindTTS,
		BaseURL:    "https://api.deepgram.com/v1/speak",
		AuthType:   "apikey",
		AuthHeader: "token",
		Format:     "deepgram",
	})
}
