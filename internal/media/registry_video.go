package media

func registerVideoProviders() {
	// xAI Grok Video
	mergeProvider("xai", "xAI", KindVideo,
		[]ModelEntry{
			{ID: "grok-2-video", Name: "Grok 2 Video", Kind: KindVideo},
		},
		ProviderConfig{
			Provider:   "xai",
			Kind:       KindVideo,
			BaseURL:    chatOriginURL("xai", "/v1/videos"),
			AuthType:   "api-key",
			AuthHeader: "bearer",
			Format:     "xai-video",
		},
	)
}
