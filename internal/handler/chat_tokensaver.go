package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/rtk"
)

func isTokenSaverExcluded(settings *db.Settings, providerID, model string) bool {
	if settings == nil || len(settings.TokenSaverExclude) == 0 {
		return false
	}
	candidates := []string{providerID}
	if model != "" {
		candidates = append(candidates, model)
		if idx := strings.Index(model, "/"); idx > 0 {
			candidates = append(candidates, model[:idx])
		}
	}
	for _, cand := range candidates {
		candLower := strings.ToLower(strings.TrimSpace(cand))
		if candLower == "" {
			continue
		}
		for _, ex := range settings.TokenSaverExclude {
			if strings.ToLower(strings.TrimSpace(ex)) == candLower {
				return true
			}
		}
	}
	return false
}

// applyTokenSaver applies RTK compression, caveman, and ponytail based on settings.
// It accepts an optional *http.Request to honor per-request header overrides (e.g. X-Cyrene-Tool-Slimmer).
func (s *Server) applyTokenSaver(bodyMap map[string]any, format string, providerID string, req ...*http.Request) {
	settings, err := s.DB.GetSettings()
	if err != nil {
		return
	}

	model, _ := bodyMap["model"].(string)
	if isTokenSaverExcluded(settings, providerID, model) {
		return
	}

	rtkEnabled := settings.RTKEnabled
	if len(req) > 0 && req[0] != nil {
		if val := req[0].Header.Get("X-Cyrene-Tool-Slimmer"); val != "" {
			rtkEnabled = (val == "1" || strings.EqualFold(val, "true"))
		}
	}

	// RTK compression of tool results
	if rtkEnabled {
		if saved := rtk.CompressMessages(bodyMap, true); saved > 0 {
			slog.Debug("RTK compressed tool results", slog.Int("bytes_saved", saved))
		}
	}

	// Caveman: inject terse-style system prompt (defaults to lite if level is unset)
	if settings.CavemanEnabled {
		level := settings.CavemanLevel
		if level == "" {
			level = rtk.CavemanLite
		}
		rtk.InjectCaveman(bodyMap, format, level)
	}

	// Ponytail: inject lazy-senior-dev system prompt (defaults to lite if level is unset)
	if settings.PonytailEnabled {
		level := settings.PonytailLevel
		if level == "" {
			level = rtk.PonytailLite
		}
		rtk.InjectPonytail(bodyMap, format, level)
	}
}

// loopGuardEnabled resolves the settings switch once so every upstream path
// composes the same guard prompts, or none at all.
func (s *Server) loopGuardEnabled() bool {
	settings, err := s.DB.GetSettings()
	return err == nil && settings.LoopGuardEnabled
}

// autoPromptCacheEnabled resolves the settings switch for prompt cache injection.
func (s *Server) autoPromptCacheEnabled() bool {
	settings, err := s.DB.GetSettings()
	return err == nil && settings != nil && settings.AutoPromptCache
}

// applyLoopGuard composes the loop guard prompts into a prepared upstream body.
// Format selects the shape being rewritten, exactly as the token saver does.
func (s *Server) applyLoopGuard(bodyMap map[string]any, rawBody []byte, format string, hasTools bool) {
	if !s.loopGuardEnabled() {
		return
	}
	provider.ApplyLoopGuard(bodyMap, rawBody, format, hasTools)
}
