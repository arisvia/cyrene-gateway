package media

func registerSTTProviders() {
	// OpenAI Whisper
	mergeProvider("openai", "OpenAI", KindSTT,
		[]ModelEntry{
			{ID: "whisper-1", Name: "Whisper 1", Kind: KindSTT},
			{ID: "gpt-4o-transcribe", Name: "GPT-4o Transcribe", Kind: KindSTT},
			{ID: "gpt-4o-mini-transcribe", Name: "GPT-4o Mini Transcribe", Kind: KindSTT},
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
	// Deepgram
	mergeProvider("deepgram", "Deepgram", KindSTT,
		[]ModelEntry{
			{ID: "nova-3", Name: "Nova 3", Kind: KindSTT},
			{ID: "nova-2", Name: "Nova 2", Kind: KindSTT},
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
			{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Kind: KindSTT},
			{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Kind: KindSTT},
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
