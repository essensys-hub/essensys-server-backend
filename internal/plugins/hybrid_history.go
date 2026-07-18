package plugins

import (
	"time"

	plugin "github.com/essensys-hub/essensys-plugin-framework/go"
)

// hybridStore agrège l'historique court terme (Redis, 48 h — cf.
// redisStore.Put) et long terme (Prometheus, via query_range) derrière le
// port plugin.RangeHistoryProvider. Il embarque le redisStore concret pour
// hériter de toutes les méthodes de plugin.Store (Current, Put, SetEnabled,
// PersistedEnabled, Purge, History) sans avoir à les redéclarer.
type hybridStore struct {
	redisStore
	prom *promHistory
}

// WrapHistory enrichit un Store Redis (issu de NewRedisStore) d'un client
// Prometheus pour les plages week/month/year. Si store n'est pas un
// redisStore (ex. plugin.MemStore en test), il est renvoyé inchangé : pas de
// RangeHistoryProvider, la route /history retombe sur le fallback HTTP
// (History + rangeToSince) déjà géré par le framework.
func WrapHistory(store plugin.Store, prometheusURL string) plugin.Store {
	rs, ok := store.(redisStore)
	if !ok {
		return store
	}
	return hybridStore{redisStore: rs, prom: newPromHistory(prometheusURL)}
}

// HistoryRange implémente plugin.RangeHistoryProvider : "day" reste sur Redis
// (fenêtre 48 h déjà historisée), "week"/"month"/"year" vont chercher
// Prometheus (Prom down ou erreur de parsing → nil, jamais de panique).
func (h hybridStore) HistoryRange(pluginID, metric, rangeKey string) []plugin.Point {
	now := time.Now()
	switch rangeKey {
	case "day":
		return h.redisStore.History(pluginID, metric, now.Add(-24*time.Hour))
	case "week":
		return h.prom.QueryRange(pluginID, metric, now.Add(-7*24*time.Hour), now, 15*time.Minute)
	case "month":
		return h.prom.QueryRange(pluginID, metric, now.Add(-30*24*time.Hour), now, 2*time.Hour)
	case "year":
		return h.prom.QueryRange(pluginID, metric, now.Add(-365*24*time.Hour), now, 24*time.Hour)
	default:
		return nil
	}
}
