package media

func registerSTTProviders() {
	// OpenAI Whisper
	mergeProvider("openai", "OpenAI", KindSTT,
		[]ModelEntry{
			{ID: "whisper-1", Name: "Whisper 1", Kind: KindSTT},
		},
		ProviderConfig{
			Provider:   "openai",
			Kind:       KindSTT,
			BaseURL:    "https://api.openai.com/v1/audio/transcriptions",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "openai",
		},
	)

	// Groq Whisper (Ultra-fast Cloud Inference)
	mergeProvider("groq", "Groq", KindSTT,
		[]ModelEntry{
			{ID: "whisper-large-v3-turbo", Name: "Whisper Large v3 Turbo (Groq)", Kind: KindSTT},
			{ID: "whisper-large-v3", Name: "Whisper Large v3 (Groq)", Kind: KindSTT},
			{ID: "distil-whisper-large-v3-en", Name: "Distil-Whisper English (Groq)", Kind: KindSTT},
		},
		ProviderConfig{
			Provider:   "groq",
			Kind:       KindSTT,
			BaseURL:    "https://api.groq.com/openai/v1/audio/transcriptions",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "openai",
		},
	)

	// Deepgram
	mergeProvider("deepgram", "Deepgram", KindSTT,
		[]ModelEntry{
			{ID: "nova-3", Name: "Nova 3", Kind: KindSTT},
			{ID: "nova-2", Name: "Nova 2", Kind: KindSTT},
			{ID: "nova-2-general", Name: "Nova 2 General", Kind: KindSTT},
		},
		ProviderConfig{
			Provider:   "deepgram",
			Kind:       KindSTT,
			BaseURL:    "https://api.deepgram.com/v1/listen",
			AuthType:   "apikey",
			AuthHeader: "token",
			Format:     "deepgram",
		},
	)

	// Gemini STT
	mergeProvider("gemini", "Gemini", KindSTT,
		[]ModelEntry{
			{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash Audio", Kind: KindSTT},
			{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro Audio", Kind: KindSTT},
		},
		ProviderConfig{
			Provider:   "gemini",
			Kind:       KindSTT,
			BaseURL:    "https://generativelanguage.googleapis.com/v1beta/models",
			AuthType:   "apikey",
			AuthHeader: "key",
			Format:     "gemini-stt",
		},
	)
}
