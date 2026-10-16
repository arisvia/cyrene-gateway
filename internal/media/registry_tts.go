package media

func registerTTSProviders() {
	// OpenAI TTS
	mergeProvider("openai", "OpenAI", KindTTS,
		[]ModelEntry{
			{ID: "tts-1", Name: "OpenAI TTS 1", Kind: KindTTS},
			{ID: "tts-1-hd", Name: "OpenAI TTS 1 HD", Kind: KindTTS},
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

	// ElevenLabs
	mergeProvider("elevenlabs", "ElevenLabs", KindTTS,
		[]ModelEntry{
			{ID: "eleven_multilingual_v2", Name: "Multilingual v2", Kind: KindTTS},
			{ID: "eleven_turbo_v2_5", Name: "Turbo v2.5", Kind: KindTTS},
			{ID: "eleven_flash_v2_5", Name: "Flash v2.5 (75ms)", Kind: KindTTS},
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

	// MiniMax Speech
	mergeProvider("minimax", "MiniMax", KindTTS,
		[]ModelEntry{
			{ID: "speech-01-turbo", Name: "MiniMax Speech-01 Turbo", Kind: KindTTS},
			{ID: "speech-01-hd", Name: "MiniMax Speech-01 HD", Kind: KindTTS},
		},
		ProviderConfig{
			Provider:   "minimax",
			Kind:       KindTTS,
			BaseURL:    "https://api.minimax.chat/v1/t2a_v2",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "minimax-tts",
		},
	)

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
}
