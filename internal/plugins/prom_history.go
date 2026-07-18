package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	plugin "github.com/essensys-hub/essensys-plugin-framework/go"
)

// promHistory interroge l'API HTTP de Prometheus (query_range) pour fournir
// les courbes long terme (semaine/mois/année) que Redis ne conserve pas
// (historisation Redis limitée à 48 h, cf. redisStore.Put).
type promHistory struct {
	base   string // ex. http://127.0.0.1:9090
	client *http.Client
}

// newPromHistory crée un client Prometheus. base vide → valeur par défaut LAN.
func newPromHistory(base string) *promHistory {
	if base == "" {
		base = "http://127.0.0.1:9090"
	}
	return &promHistory{base: strings.TrimRight(base, "/"), client: &http.Client{Timeout: 8 * time.Second}}
}

// QueryRange renvoie les points d'une métrique de plugin sur [start, end],
// sous-échantillonnés à ~200 points. Ne panique jamais : toute erreur réseau,
// HTTP ou de parsing renvoie nil (la couche HTTP normalise nil → []).
func (p *promHistory) QueryRange(pluginID, metric string, start, end time.Time, step time.Duration) []plugin.Point {
	q := fmt.Sprintf(`max without (machine) (essensys_plugin_metric{plugin=%q,metric=%q})`, pluginID, metric)
	u, err := url.Parse(p.base + "/api/v1/query_range")
	if err != nil {
		return nil
	}
	qs := u.Query()
	qs.Set("query", q)
	qs.Set("start", strconv.FormatInt(start.Unix(), 10))
	qs.Set("end", strconv.FormatInt(end.Unix(), 10))
	qs.Set("step", strconv.FormatInt(int64(step.Seconds()), 10))
	u.RawQuery = qs.Encode()

	resp, err := p.client.Get(u.String())
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	pts, err := parsePromMatrix(body)
	if err != nil {
		return nil
	}
	return downsample(pts, 200)
}

// promQueryRangeResponse modélise la réponse JSON de /api/v1/query_range
// (on ne garde que resultType=matrix, seul cas produit par nos requêtes).
type promQueryRangeResponse struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType,omitempty"`
	Error     string `json:"error,omitempty"`
	Data      struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Values [][2]json.RawMessage `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// parsePromMatrix parse une réponse query_range et aplatit toutes les séries
// (une par label "machine" résiduel côté Prometheus, déjà agrégées par notre
// requête "max without (machine)") en une unique liste de points triée.
func parsePromMatrix(raw []byte) ([]plugin.Point, error) {
	var resp promQueryRangeResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "success" {
		if resp.Error != "" {
			return nil, fmt.Errorf("prometheus: %s: %s", resp.ErrorType, resp.Error)
		}
		return nil, fmt.Errorf("prometheus: status=%s", resp.Status)
	}
	var pts []plugin.Point
	for _, series := range resp.Data.Result {
		for _, v := range series.Values {
			var ts float64
			if err := json.Unmarshal(v[0], &ts); err != nil {
				continue
			}
			var valStr string
			if err := json.Unmarshal(v[1], &valStr); err != nil {
				continue
			}
			val, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				continue
			}
			pts = append(pts, plugin.Point{TS: time.Unix(int64(ts), 0), Value: val})
		}
	}
	return pts, nil
}

// downsample réduit pts à environ n points en conservant l'ordre et le
// dernier point (même logique que redisStore.History, extraite pour
// réutilisation par le client Prometheus).
func downsample(pts []plugin.Point, n int) []plugin.Point {
	if len(pts) <= n {
		return pts
	}
	step := len(pts) / n
	ds := make([]plugin.Point, 0, n+1)
	for i := 0; i < len(pts); i += step {
		ds = append(ds, pts[i])
	}
	if ds[len(ds)-1] != pts[len(pts)-1] {
		ds = append(ds, pts[len(pts)-1])
	}
	return ds
}
