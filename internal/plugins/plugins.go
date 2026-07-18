// Package plugins câble le framework de plugins Essensys dans le backend LAN.
//
// Il fournit les implémentations concrètes des ports du SDK (Bus over MQTT,
// Store over Redis, MetricSink over Prometheus, AuthFunc over l'IAM LAN) et
// enregistre les adaptateurs compilés (aucun chargement dynamique).
package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/internal/models"

	plugin "github.com/essensys-hub/essensys-plugin-framework/go"
	sungrow "github.com/essensys-hub/essensys-plugin-sungrow/adapter"
)

// Deps regroupe les ports fournis par l'application hôte.
type Deps struct {
	Store         plugin.Store      // last-value (Redis en prod)
	Sink          plugin.MetricSink // séries (Prometheus en prod)
	Bus           plugin.Bus        // ingestion (MQTT en prod) ; nil si MQTT désactivé
	PrometheusURL string            // API HTTP Prometheus pour l'historique week/month/year
}

// New construit le registre, enregistre les plugins compilés, configure leurs
// manifests, s'abonne au bus (si présent) et renvoie le handler HTTP à monter
// sous /api/plugins/ (derrière l'auth LAN existante).
func New(d Deps) (*plugin.Registry, http.Handler, error) {
	store := WrapHistory(d.Store, d.PrometheusURL)
	reg := plugin.New(store, d.Sink)

	// Enregistrement compilé des adaptateurs.
	reg.Register(sungrow.New())

	// Configuration par manifest.
	for _, m := range manifests() {
		if err := reg.Configure(m); err != nil {
			return nil, nil, err
		}
	}

	if d.Bus != nil {
		if err := reg.Subscribe(d.Bus); err != nil {
			return nil, nil, err
		}
	}

	mux := http.NewServeMux()
	reg.Mount(mux, LanAuth)
	return reg, mux, nil
}

// LanAuth traduit l'identité LAN (posée par middleware.LanRequireSession) en
// identité framework. lan_guest est volontairement exclu (invité ≠ membre).
func LanAuth(r *http.Request) (plugin.Identity, bool) {
	u, ok := middleware.GetLanUser(r)
	if !ok {
		return plugin.Identity{}, false
	}
	return mapRole(u.Role)
}

// mapRole traduit un rôle LAN en rôles framework (pur, testable).
func mapRole(role string) (plugin.Identity, bool) {
	switch role {
	case models.LanRoleAdmin:
		return plugin.Identity{Roles: []plugin.Role{plugin.RoleLANAdmin}}, true
	case models.LanRoleUser:
		return plugin.Identity{Roles: []plugin.Role{plugin.RoleLANUser}}, true
	default:
		return plugin.Identity{}, false
	}
}

// manifests renvoie les manifests des plugins compilés. En Phase 2 ils seront
// lus depuis un répertoire de config déployé ; ici ils accompagnent l'adaptateur.
func manifests() []plugin.Manifest {
	sg := plugin.Manifest{
		ID: sungrow.ID, Name: "Solaire", Version: sungrow.Version,
		Description: "Production photovoltaïque Sungrow (onduleur SH-RS + batterie SBR) : tableau de bord, schéma de flux, autoconsommation.",
		ManifestVersion: 1, FrameworkVersion: "^1.0",
		Capabilities: []string{"metrics", "device-poll", "ui-tile", "ui-page"},
		Perimeters:   []plugin.Perimeter{plugin.PerimeterLANCM5, plugin.PerimeterHubCloudsync},
		Visibility: []plugin.Role{
			plugin.RoleUser, plugin.RoleAdminLocal, plugin.RoleAdminGlobal,
			plugin.RoleLANUser, plugin.RoleLANAdmin,
		},
		WriteScope: "read-only",
	}
	sg.Surfaces.Backend = &struct {
		Adapter string `json:"adapter"`
	}{Adapter: sungrow.ID}
	return []plugin.Manifest{sg}
}

// ---- Bus over MQTT ----

// SubscribeFunc est la forme d'abonnement d'un client MQTT (internal/mqtt.Client
// expose exactement Subscribe(topic, func(topic, payload))).
type SubscribeFunc func(topic string, handler func(topic string, payload []byte)) error

type mqttBus struct{ sub SubscribeFunc }

// NewMQTTBus adapte l'abonnement d'un client MQTT en plugin.Bus.
func NewMQTTBus(sub SubscribeFunc) plugin.Bus { return mqttBus{sub: sub} }

func (b mqttBus) Subscribe(filter string, h func(plugin.BusMessage)) error {
	return b.sub(filter, func(topic string, payload []byte) {
		id, machine := parseTopic(topic)
		h(plugin.BusMessage{Topic: topic, PluginID: id, MachineID: machine, Payload: payload})
	})
}

// parseTopic extrait (pluginID, machineID) de essensys/plugins/<id>/<machine>/<metric>.
func parseTopic(topic string) (id, machine string) {
	parts := strings.Split(topic, "/")
	if len(parts) >= 4 && parts[0] == "essensys" && parts[1] == "plugins" {
		return parts[2], parts[3]
	}
	return "", ""
}

// ---- Store over Redis ----

type redisStore struct {
	c        *redis.Client
	staleTTL time.Duration
	mu       *sync.Mutex
}

// NewRedisStore crée un Store plugin adossé à Redis (clés essensys:plugins:*).
func NewRedisStore(addr, password string, db int, staleTTL time.Duration) plugin.Store {
	return redisStore{c: redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db}), staleTTL: staleTTL, mu: &sync.Mutex{}}
}

func (s redisStore) key(pluginID string) string { return "essensys:plugins:" + pluginID + ":current" }

func (s redisStore) Put(pluginID, machineID string, samples []plugin.Sample, at time.Time) {
	// Un message MQTT ne porte qu'une série : fusion avec le snapshot courant
	// (upsert par machine|metric), sérialisée pour éviter les pertes croisées.
	s.mu.Lock()
	defer s.mu.Unlock()
	var existing []plugin.Sample
	if v, err := s.c.Get(context.Background(), s.key(pluginID)).Bytes(); err == nil {
		var prev plugin.Reading
		if json.Unmarshal(v, &prev) == nil {
			existing = prev.Samples
		}
	}
	b, err := json.Marshal(plugin.Reading{PluginID: pluginID, Samples: plugin.MergeSamples(existing, samples), UpdatedAt: at})
	if err != nil {
		return
	}
	ctx := context.Background()
	s.c.Set(ctx, s.key(pluginID), b, 0)
	// Historisation 48 h pour les courbes (route /history).
	for _, sm := range samples {
		ts := sm.TS
		if ts.IsZero() {
			ts = at
		}
		hk := s.histKey(pluginID, sm.Metric)
		s.c.ZAdd(ctx, hk, &redis.Z{Score: float64(ts.Unix()), Member: strconv.FormatInt(ts.Unix(), 10) + "|" + strconv.FormatFloat(sm.Value, 'f', -1, 64)})
		s.c.ZRemRangeByScore(ctx, hk, "0", strconv.FormatInt(at.Add(-48*time.Hour).Unix(), 10))
		s.c.Expire(ctx, hk, 72*time.Hour)
	}
}

func (s redisStore) histKey(pluginID, metric string) string {
	return "essensys:plugins:" + pluginID + ":hist:" + metric
}

// SetEnabled / PersistedEnabled implémentent plugin.StateStore : l'état
// activé/désactivé survit aux redémarrages du backend.
func (s redisStore) SetEnabled(pluginID string, enabled bool) {
	v := "1"
	if !enabled {
		v = "0"
	}
	s.c.Set(context.Background(), "essensys:plugins:"+pluginID+":enabled", v, 0)
}

func (s redisStore) PersistedEnabled(pluginID string) (bool, bool) {
	v, err := s.c.Get(context.Background(), "essensys:plugins:"+pluginID+":enabled").Result()
	if err != nil {
		return false, false
	}
	return v == "1", true
}

// Purge implémente plugin.PurgeStore : efface snapshot + historique du plugin
// (désinstallation côté données ; l'état enabled persisté est conservé).
func (s redisStore) Purge(pluginID string) {
	ctx := context.Background()
	keys, err := s.c.Keys(ctx, "essensys:plugins:"+pluginID+":hist:*").Result()
	if err == nil && len(keys) > 0 {
		s.c.Del(ctx, keys...)
	}
	s.c.Del(ctx, s.key(pluginID))
}

// History implémente plugin.HistoryProvider (route /api/plugins/<id>/history).
func (s redisStore) History(pluginID, metric string, since time.Time) []plugin.Point {
	vals, err := s.c.ZRangeByScore(context.Background(), s.histKey(pluginID, metric), &redis.ZRangeBy{
		Min: strconv.FormatInt(since.Unix(), 10), Max: "+inf",
	}).Result()
	if err != nil {
		return nil
	}
	pts := make([]plugin.Point, 0, len(vals))
	for _, v := range vals {
		parts := strings.SplitN(v, "|", 2)
		if len(parts) != 2 {
			continue
		}
		sec, err1 := strconv.ParseInt(parts[0], 10, 64)
		val, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		pts = append(pts, plugin.Point{TS: time.Unix(sec, 0), Value: val})
	}
	// Sous-échantillonnage : ~200 points suffisent pour la courbe du jour.
	return downsample(pts, 200)
}

func (s redisStore) Current(pluginID string) plugin.Reading {
	v, err := s.c.Get(context.Background(), s.key(pluginID)).Bytes()
	if err != nil {
		return plugin.Reading{PluginID: pluginID, Stale: true}
	}
	var r plugin.Reading
	if err := json.Unmarshal(v, &r); err != nil {
		return plugin.Reading{PluginID: pluginID, Stale: true}
	}
	r.Stale = time.Since(r.UpdatedAt) > s.staleTTL
	return r
}

// ---- MetricSink over Prometheus ----

type promSink struct{ g *prometheus.GaugeVec }

// NewPromSink crée un MetricSink Prometheus. Idempotent à l'enregistrement.
func NewPromSink() plugin.MetricSink {
	g := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: "essensys", Subsystem: "plugin", Name: "metric"},
		[]string{"plugin", "machine", "metric"},
	)
	if err := prometheus.Register(g); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			g = are.ExistingCollector.(*prometheus.GaugeVec)
		}
	}
	return promSink{g: g}
}

func (s promSink) Observe(pluginID string, sm plugin.Sample) {
	s.g.WithLabelValues(pluginID, sm.MachineID, sm.Metric).Set(sm.Value)
}
