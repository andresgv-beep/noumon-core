package main

import (
	"net/http"
	"time"
)

// handleCacheMetrics expone solo contadores agregados para poder comprobar el
// camino caliente sin registrar tokens, usuarios ni rutas. Se monta en adminMux.
func (s *Server) handleCacheMetrics(media *mediaDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "solo GET"})
			return
		}

		s.sessCacheMu.RLock()
		sessionEntries := len(s.sessCache)
		s.sessCacheMu.RUnlock()

		s.accessCacheMu.RLock()
		accessEntries := len(s.accessCache)
		accessAge := cacheAgeMillis(s.accessCachedAt)
		s.accessCacheMu.RUnlock()

		media.mu.Lock()
		catalogEntries := 0
		if media.catalog != nil {
			catalogEntries = len(media.catalog.items)
		}
		catalogAge := cacheAgeMillis(media.builtAt)
		generation := media.generation
		building := media.building
		media.mu.Unlock()

		writeJSON(w, http.StatusOK, map[string]any{
			"session": map[string]any{
				"entries": sessionEntries,
				"hits":    s.sessHits.Load(),
				"misses":  s.sessMisses.Load(),
			},
			"access": map[string]any{
				"entries": accessEntries,
				"ageMs":   accessAge,
				"hits":    s.accessHits.Load(),
				"misses":  s.accessMisses.Load(),
				"builds":  s.accessBuilds.Load(),
				"waits":   s.accessWaits.Load(),
			},
			"catalog": map[string]any{
				"entries":      catalogEntries,
				"ageMs":        catalogAge,
				"generation":   generation,
				"building":     building,
				"hits":         media.catalogHits.Load(),
				"misses":       media.catalogMisses.Load(),
				"builds":       media.catalogBuilds.Load(),
				"waits":        media.catalogWaits.Load(),
				"buildMsTotal": media.catalogBuildNanos.Load() / uint64(time.Millisecond),
			},
		})
	}
}

func cacheAgeMillis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return time.Since(t).Milliseconds()
}
