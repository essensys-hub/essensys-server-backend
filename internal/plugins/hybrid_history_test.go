package plugins

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	plugin "github.com/essensys-hub/essensys-plugin-framework/go"
)

// TestWrapHistoryPassthroughForNonRedisStore vérifie qu'un store qui n'est
// pas un redisStore (ex. MemStore en test) est renvoyé inchangé, sans
// RangeHistoryProvider ajouté (le framework retombe sur son fallback HTTP).
func TestWrapHistoryPassthroughForNonRedisStore(t *testing.T) {
	mem := plugin.NewMemStore(90 * time.Second)
	wrapped := WrapHistory(mem, "http://127.0.0.1:9090")
	if _, ok := wrapped.(plugin.RangeHistoryProvider); ok {
		t.Fatal("un store non-Redis ne doit pas devenir RangeHistoryProvider")
	}
	if wrapped != plugin.Store(mem) {
		t.Fatal("le store d'origine doit être renvoyé inchangé")
	}
}

// TestWrapHistoryRedisStoreImplementsRangeHistoryProvider vérifie que le
// wrapping d'un redisStore produit bien un plugin.RangeHistoryProvider tout
// en conservant les autres méthodes de plugin.Store (Current, Put...).
func TestWrapHistoryRedisStoreImplementsRangeHistoryProvider(t *testing.T) {
	rs := NewRedisStore("127.0.0.1:6399", "", 0, 90*time.Second) // port bidon, pas de connexion réelle nécessaire
	wrapped := WrapHistory(rs, "http://127.0.0.1:9999")
	if _, ok := wrapped.(plugin.RangeHistoryProvider); !ok {
		t.Fatal("un redisStore wrappé doit implémenter RangeHistoryProvider")
	}
	if _, ok := wrapped.(plugin.HistoryProvider); !ok {
		t.Fatal("un redisStore wrappé doit toujours implémenter HistoryProvider")
	}
}

// TestHybridStoreHistoryRangeDayUsesRedis vérifie que "day" ne tape jamais
// Prometheus : le client prom est laissé nil pour le prouver (pas de panique).
func TestHybridStoreHistoryRangeDayUsesRedis(t *testing.T) {
	rs := NewRedisStore("127.0.0.1:6399", "", 0, 90*time.Second).(redisStore)
	h := hybridStore{redisStore: rs, prom: newPromHistory("http://127.0.0.1:1")}
	pts := h.HistoryRange("sungrow-solar", "pv_power", "day")
	if pts != nil {
		t.Fatalf("attendu nil (pas d'historique redis en test), got %+v", pts)
	}
}

// TestHybridStoreHistoryRangeWeekQueriesProm vérifie que "week" appelle bien
// Prometheus avec la bonne requête PromQL et le bon pas d'échantillonnage.
func TestHybridStoreHistoryRangeWeekQueriesProm(t *testing.T) {
	var gotQuery, gotStep string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		gotStep = r.URL.Query().Get("step")
		w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"values":[[1700000000,"3"]]}]}}`))
	}))
	defer srv.Close()

	rs := NewRedisStore("127.0.0.1:6399", "", 0, 90*time.Second).(redisStore)
	h := hybridStore{redisStore: rs, prom: newPromHistory(srv.URL)}
	pts := h.HistoryRange("sungrow-solar", "pv_power", "week")
	if len(pts) != 1 || pts[0].Value != 3 {
		t.Fatalf("%+v", pts)
	}
	if want := `max without (machine) (essensys_plugin_metric{plugin="sungrow-solar",metric="pv_power"})`; gotQuery != want {
		t.Fatalf("query PromQL inattendue: %s", gotQuery)
	}
	if gotStep != "900" { // 15 min
		t.Fatalf("step inattendu: %s", gotStep)
	}
}

// TestHybridStoreHistoryRangePromDownReturnsNil vérifie qu'un Prometheus
// injoignable ne fait jamais paniquer et renvoie nil (normalisé en [] côté HTTP).
func TestHybridStoreHistoryRangePromDownReturnsNil(t *testing.T) {
	rs := NewRedisStore("127.0.0.1:6399", "", 0, 90*time.Second).(redisStore)
	h := hybridStore{redisStore: rs, prom: newPromHistory("http://127.0.0.1:1")}
	for _, rangeKey := range []string{"week", "month", "year"} {
		if pts := h.HistoryRange("sungrow-solar", "pv_power", rangeKey); pts != nil {
			t.Fatalf("range %s: attendu nil sur Prom down, got %+v", rangeKey, pts)
		}
	}
}

// TestHybridStoreHistoryRangeInvalidReturnsNil couvre le default du switch.
func TestHybridStoreHistoryRangeInvalidReturnsNil(t *testing.T) {
	rs := NewRedisStore("127.0.0.1:6399", "", 0, 90*time.Second).(redisStore)
	h := hybridStore{redisStore: rs, prom: newPromHistory("")}
	if pts := h.HistoryRange("sungrow-solar", "pv_power", "bogus"); pts != nil {
		t.Fatalf("attendu nil, got %+v", pts)
	}
}
