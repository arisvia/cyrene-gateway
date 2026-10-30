package media

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Response-side normalization: provider-native media bodies are converted to
// the OpenAI-compatible contract the gateway advertises downstream
// (/v1/images/generations, /v1/audio/speech, /v1/audio/transcriptions).
// Formats whose upstream already speaks OpenAI ("openai", "openrouter",
// "elevenlabs" raw audio) pass through untouched.

// NeedsImageNormalization reports whether the image format returns a body
// that is not OpenAI-shaped and must be converted before proxying.
func NeedsImageNormalization(format string) bool {
	return format == "gemini" || format == "stability"
}

// NeedsAudioNormalization reports whether the TTS format returns JSON
// (generateContent envelope) instead of raw audio bytes.
func NeedsAudioNormalization(format string) bool {
	return format == "gemini-tts"
}

// NeedsSTTNormalization reports whether the STT format returns a
// provider-native transcript shape instead of OpenAI's {"text": ...}.
func NeedsSTTNormalization(format string) bool {
	return format == "deepgram" || format == "gemini-stt"
}

type geminiPart struct {
	Text       string `json:"text"`
	InlineData *struct {
		MimeType  string `json:"mimeType"`
		MimeSnake string `json:"mime_type"`
		Data      string `json:"data"`
	} `json:"inlineData"`
	InlineDataSnake *struct {
		MimeType  string `json:"mimeType"`
		MimeSnake string `json:"mime_type"`
		Data      string `json:"data"`
	} `json:"inline_data"`
}

func (p geminiPart) inline() (mime, data string, ok bool) {
	in := p.InlineData
	if in == nil {
		in = p.InlineDataSnake
	}
	if in == nil || in.Data == "" {
		return "", "", false
	}
	mime = in.MimeType
	if mime == "" {
		mime = in.MimeSnake
	}
	return mime, in.Data, true
}

type geminiGenerateContent struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func decodeGemini(raw []byte) (geminiGenerateContent, error) {
	var gc geminiGenerateContent
	if err := json.Unmarshal(raw, &gc); err != nil {
		return gc, fmt.Errorf("parse gemini response: %w", err)
	}
	return gc, nil
}

// NormalizeImage converts gemini/stability image generation bodies into the
// OpenAI {created, data:[{b64_json}]} shape. Multiple inline images become
// multiple data entries.
func NormalizeImage(format string, raw []byte) ([]byte, error) {
	switch format {
	case "gemini":
		gc, err := decodeGemini(raw)
		if err != nil {
			return nil, err
		}
		var images []string
		for _, cand := range gc.Candidates {
			for _, part := range cand.Content.Parts {
				mime, data, ok := part.inline()
				if !ok || !strings.HasPrefix(mime, "image/") {
					continue
				}
				images = append(images, data)
			}
		}
		if len(images) == 0 {
			return nil, fmt.Errorf("gemini image response contained no image parts")
		}
		return openAIImageBody(images)
	case "stability":
		var st struct {
			Image string `json:"image"`
		}
		if err := json.Unmarshal(raw, &st); err != nil {
			return nil, fmt.Errorf("parse stability response: %w", err)
		}
		if st.Image == "" {
			return nil, fmt.Errorf("stability response contained no image")
		}
		return openAIImageBody([]string{st.Image})
	}
	return raw, nil
}

func openAIImageBody(b64s []string) ([]byte, error) {
	data := make([]map[string]any, 0, len(b64s))
	for _, b := range b64s {
		data = append(data, map[string]any{"b64_json": b})
	}
	return json.Marshal(map[string]any{
		"created": time.Now().Unix(),
		"data":    data,
	})
}

// NormalizeAudio converts a gemini-tts generateContent envelope into raw
// audio bytes plus the upstream mime type for the Content-Type header.
func NormalizeAudio(format string, raw []byte) ([]byte, string, error) {
	if format != "gemini-tts" {
		return raw, "", nil
	}
	gc, err := decodeGemini(raw)
	if err != nil {
		return nil, "", err
	}
	for _, cand := range gc.Candidates {
		for _, part := range cand.Content.Parts {
			mime, data, ok := part.inline()
			if !ok || !strings.HasPrefix(mime, "audio/") {
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return nil, "", fmt.Errorf("decode gemini audio: %w", err)
			}
			return decoded, mime, nil
		}
	}
	return nil, "", fmt.Errorf("gemini tts response contained no audio parts")
}

// NormalizeSTT converts deepgram/gemini-stt transcript bodies into the
// OpenAI {"text": ...} shape.
func NormalizeSTT(format string, raw []byte) ([]byte, error) {
	switch format {
	case "deepgram":
		var dg struct {
			Results struct {
				Channels []struct {
					Alternatives []struct {
						Transcript string `json:"transcript"`
					} `json:"alternatives"`
				} `json:"channels"`
			} `json:"results"`
		}
		if err := json.Unmarshal(raw, &dg); err != nil {
			return nil, fmt.Errorf("parse deepgram response: %w", err)
		}
		var text string
		for _, ch := range dg.Results.Channels {
			for _, alt := range ch.Alternatives {
				text += alt.Transcript
			}
		}
		return json.Marshal(map[string]string{"text": strings.TrimSpace(text)})
	case "gemini-stt":
		gc, err := decodeGemini(raw)
		if err != nil {
			return nil, err
		}
		var sb strings.Builder
		for _, cand := range gc.Candidates {
			for _, part := range cand.Content.Parts {
				sb.WriteString(part.Text)
			}
		}
		return json.Marshal(map[string]string{"text": strings.TrimSpace(sb.String())})
	}
	return raw, nil
}
