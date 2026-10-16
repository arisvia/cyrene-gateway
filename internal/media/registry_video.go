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
			BaseURL:    "https://api.x.ai/v1/videos",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "xai-video",
		},
	)

	// MiniMax Video (Hailuo)
	mergeProvider("minimax", "MiniMax", KindVideo,
		[]ModelEntry{
			{ID: "video-01", Name: "MiniMax Video-01 (Hailuo)", Kind: KindVideo},
		},
		ProviderConfig{
			Provider:   "minimax",
			Kind:       KindVideo,
			BaseURL:    "https://api.minimax.chat/v1/video_generation",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "minimax-video",
		},
	)
}
