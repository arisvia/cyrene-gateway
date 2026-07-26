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

	// RunwayML Video
	mergeProvider("runwayml", "RunwayML", KindVideo,
		[]ModelEntry{
			{ID: "gen4-turbo", Name: "Gen4 Turbo", Kind: KindVideo},
			{ID: "gen3a-turbo", Name: "Gen3A Turbo", Kind: KindVideo},
		},
		ProviderConfig{
			Provider:   "runwayml",
			Kind:       KindVideo,
			BaseURL:    "https://api.dev.runwayml.com/v1",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "runway",
		},
	)

	// Fal.ai Video
	mergeProvider("fal-ai", "Fal.ai", KindVideo, nil, ProviderConfig{
		Provider:   "fal-ai",
		Kind:       KindVideo,
		BaseURL:    "https://queue.fal.run",
		AuthType:   "apikey",
		AuthHeader: "bearer",
		Format:     "fal",
	})

	// HuggingFace Video
	mergeProvider("huggingface", "HuggingFace", KindVideo, nil, ProviderConfig{
		Provider:   "huggingface",
		Kind:       KindVideo,
		BaseURL:    "https://api-inference.huggingface.co/models",
		AuthType:   "apikey",
		AuthHeader: "bearer",
		Format:     "huggingface",
	})
}
