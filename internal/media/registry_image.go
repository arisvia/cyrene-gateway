package media

func registerImageProviders() {
	openaiCompat := []struct {
		id, name, baseURL string
	}{
		{"openai", "OpenAI", "https://api.openai.com/v1/images/generations"},
		{"openrouter", "OpenRouter", "https://openrouter.ai/api/v1/images/generations"},
		{"minimax", "MiniMax", "https://api.minimax.chat/v1/images/generations"},
		{"recraft", "Recraft", "https://external.api.recraft.ai/v1/images/generations"},
		{"vercel-ai-gateway", "Vercel AI Gateway", "https://ai-gateway.vercel.sh/v1/images/generations"},
		{"xai", "xAI", "https://api.x.ai/v1/images/generations"},
	}

	for _, p := range openaiCompat {
		mergeProvider(p.id, p.name, KindImage, nil, ProviderConfig{
			Provider:   p.id,
			Kind:       KindImage,
			BaseURL:    p.baseURL,
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "openai",
		})
	}

	// Gemini image generation
	mergeProvider("gemini", "Gemini", KindImage,
		[]ModelEntry{
			{ID: "gemini-2.5-flash-image", Name: "Gemini 2.5 Flash Image", Kind: KindImage},
			{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro Image", Kind: KindImage},
		},
		ProviderConfig{
			Provider:   "gemini",
			Kind:       KindImage,
			BaseURL:    "https://generativelanguage.googleapis.com/v1beta/models",
			AuthType:   "apikey",
			AuthHeader: "key",
			Format:     "gemini",
		},
	)

	// Stability AI
	mergeProvider("stability-ai", "Stability AI", KindImage,
		[]ModelEntry{
			{ID: "stable-image-ultra", Name: "Stable Image Ultra", Kind: KindImage},
			{ID: "stable-image-core", Name: "Stable Image Core", Kind: KindImage},
			{ID: "sd3.5-large", Name: "Stable Diffusion 3.5 Large", Kind: KindImage},
			{ID: "sd3.5-large-turbo", Name: "Stable Diffusion 3.5 Large Turbo", Kind: KindImage},
			{ID: "sd3.5-medium", Name: "Stable Diffusion 3.5 Medium", Kind: KindImage},
		},
		ProviderConfig{
			Provider:   "stability-ai",
			Kind:       KindImage,
			BaseURL:    "https://api.stability.ai/v2beta/stable-image/generate",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "stability",
		},
	)

	// Black Forest Labs (FLUX)
	mergeProvider("black-forest-labs", "Black Forest Labs", KindImage,
		[]ModelEntry{
			{ID: "flux-pro-1.1", Name: "FLUX Pro 1.1", Kind: KindImage},
			{ID: "flux-pro-1.1-ultra", Name: "FLUX Pro 1.1 Ultra", Kind: KindImage},
			{ID: "flux-pro", Name: "FLUX Pro", Kind: KindImage},
			{ID: "flux-dev", Name: "FLUX Dev", Kind: KindImage},
			{ID: "flux-kontext-pro", Name: "FLUX Kontext Pro (Edit)", Kind: KindImage},
			{ID: "flux-kontext-max", Name: "FLUX Kontext Max (Edit)", Kind: KindImage},
		},
		ProviderConfig{
			Provider:   "black-forest-labs",
			Kind:       KindImage,
			BaseURL:    "https://api.bfl.ai/v1",
			AuthType:   "apikey",
			AuthHeader: "x-api-key",
			Format:     "bfl",
		},
	)

	// Fal.ai
	mergeProvider("fal-ai", "Fal.ai", KindImage,
		[]ModelEntry{
			{ID: "fal-ai/flux/schnell", Name: "FLUX Schnell", Kind: KindImage},
			{ID: "fal-ai/flux/dev", Name: "FLUX Dev", Kind: KindImage},
			{ID: "fal-ai/flux-pro/v1.1", Name: "FLUX Pro v1.1", Kind: KindImage},
			{ID: "fal-ai/flux-pro/v1.1-ultra", Name: "FLUX Pro v1.1 Ultra", Kind: KindImage},
			{ID: "fal-ai/recraft-v3", Name: "Recraft V3", Kind: KindImage},
			{ID: "fal-ai/ideogram/v2", Name: "Ideogram V2", Kind: KindImage},
			{ID: "fal-ai/stable-diffusion-v35-large", Name: "SD 3.5 Large", Kind: KindImage},
		},
		ProviderConfig{
			Provider:   "fal-ai",
			Kind:       KindImage,
			BaseURL:    "https://queue.fal.run",
			AuthType:   "apikey",
			AuthHeader: "bearer",
			Format:     "fal",
		})

	// HuggingFace Inference
	mergeProvider("huggingface", "HuggingFace", KindImage, nil, ProviderConfig{
		Provider:   "huggingface",
		Kind:       KindImage,
		BaseURL:    "https://api-inference.huggingface.co/models",
		AuthType:   "apikey",
		AuthHeader: "bearer",
		Format:     "huggingface",
	})

	// RunwayML
	mergeProvider("runwayml", "RunwayML", KindImage, nil, ProviderConfig{
		Provider:   "runwayml",
		Kind:       KindImage,
		BaseURL:    "https://api.dev.runwayml.com/v1",
		AuthType:   "apikey",
		AuthHeader: "bearer",
		Format:     "runway",
	})
}
