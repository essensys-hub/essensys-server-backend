//go:build ignore

// Sonde de faisabilité de la sonnette UniFi (tâche 1.1 de sonnette-unifi-2026-11-007).
//
// LECTURE SEULE : uniquement des GET et l'ouverture du flux d'événements. Rien n'est modifié sur
// la console. L'adresse et la clé viennent de l'environnement et ne sont jamais écrites dans un
// fichier ni affichées ; les identifiants de caméra sont masqués dans la sortie.
//
//	UNIFI_BASE_URL=https://<console> UNIFI_API_KEY=<clé> go run scripts/unifi-doorbell-probe.go
//
// Variables facultatives :
//
//	PROBE_LISTEN_SECONDS  durée d'écoute du flux d'événements (défaut 0 : poignée de main seulement).
//	                      Pendant l'écoute, appuyer sur la sonnette : la sonde affiche le type des
//	                      messages reçus et signale un événement « ring ».
//	PROBE_CAMERA_ID       caméra à sonder (défaut : la première dont le nom ou le modèle évoque
//	                      une sonnette, sinon la première).
//
// Questions de la tâche 1.1 auxquelles la sonde répond :
//  1. L'événement de sonnerie est-il exposé par l'API d'intégration (flux WebSocket) ?
//  2. Sinon, le champ lastRing de la caméra permet-il le repli par interrogation ?
//  3. Quelles caméras (nombre, états) et quel temps de réponse d'un instantané ?
//
// Hors de portée de la sonde (à relever à la main et à consigner dans l'issue) : la cadence de
// lecture de l'armoire (délai entre deux ordres de lumière) et la présence de ffmpeg dans
// l'image de la CM5 (docker exec <conteneur> ffmpeg -version).
package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func mask(id string) string {
	if id == "" {
		return "(vide)"
	}
	h := sha256.Sum256([]byte(id))
	return "cam-" + hex.EncodeToString(h[:])[:6]
}

func main() {
	base := strings.TrimRight(os.Getenv("UNIFI_BASE_URL"), "/")
	key := os.Getenv("UNIFI_API_KEY")
	if base == "" || key == "" {
		fmt.Fprintln(os.Stderr, "UNIFI_BASE_URL et UNIFI_API_KEY sont obligatoires (variables d'environnement).")
		os.Exit(2)
	}
	if _, err := url.Parse(base); err != nil {
		fmt.Fprintln(os.Stderr, "UNIFI_BASE_URL illisible.")
		os.Exit(2)
	}
	listen, _ := strconv.Atoi(os.Getenv("PROBE_LISTEN_SECONDS"))

	hc := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	get := func(path string) (int, http.Header, []byte, time.Duration, error) {
		req, _ := http.NewRequest(http.MethodGet, base+path, nil)
		req.Header.Set("X-API-Key", key)
		req.Header.Set("Accept", "application/json")
		t0 := time.Now()
		resp, err := hc.Do(req)
		if err != nil {
			return 0, nil, nil, time.Since(t0), fmt.Errorf("requête impossible") // l'erreur brute contient l'adresse
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		return resp.StatusCode, resp.Header, b, time.Since(t0), nil
	}

	fmt.Println("== 1. Caméras (API d'intégration, GET)")
	code, _, body, dur, err := get("/proxy/protect/integration/v1/cameras")
	if err != nil || code != 200 {
		fmt.Printf("   KO : statut %d %v\n", code, err)
		os.Exit(1)
	}
	var cams []map[string]any
	if json.Unmarshal(body, &cams) != nil {
		fmt.Println("   KO : réponse illisible")
		os.Exit(1)
	}
	fmt.Printf("   %d caméra(s) en %s\n", len(cams), dur.Round(time.Millisecond))
	target := os.Getenv("PROBE_CAMERA_ID")
	for i, c := range cams {
		id, _ := c["id"].(string)
		name, _ := c["name"].(string)
		model, _ := c["modelKey"].(string)
		state, _ := c["state"].(string)
		var keys []string
		for k := range c {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Printf("   [%d] %s état=%s champs=%v\n", i, mask(id), state, keys)
		hint := strings.ToLower(name + " " + model)
		if target == "" && (strings.Contains(hint, "doorbell") || strings.Contains(hint, "sonnette") || strings.Contains(hint, "g4 doorbell")) {
			target = id
		}
	}
	if target == "" && len(cams) > 0 {
		target, _ = cams[0]["id"].(string)
	}
	if target == "" {
		fmt.Println("   aucune caméra : arrêt")
		os.Exit(1)
	}
	fmt.Printf("   caméra sondée : %s\n", mask(target))

	fmt.Println("== 2. Flux d'événements (WebSocket de l'API d'intégration)")
	wsURL := strings.Replace(base, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1) + "/proxy/protect/integration/v1/subscribe/events"
	dialer := websocket.Dialer{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, HandshakeTimeout: 15 * time.Second}
	hdr := http.Header{}
	hdr.Set("X-API-Key", key)
	conn, resp, derr := dialer.Dial(wsURL, hdr)
	ringSeen := false
	switch {
	case derr != nil && resp != nil:
		fmt.Printf("   flux NON exposé : statut %d à l'ouverture -> repli par interrogation (section 3)\n", resp.StatusCode)
	case derr != nil:
		fmt.Println("   ouverture impossible (réseau ou TLS)")
	default:
		fmt.Println("   flux ouvert (poignée de main acceptée)")
		if listen > 0 {
			fmt.Printf("   écoute %d s : appuyer sur la sonnette maintenant...\n", listen)
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(listen)*time.Second)
			go func() { <-ctx.Done(); _ = conn.Close() }()
			counts := map[string]int{}
			for {
				_, raw, rerr := conn.ReadMessage()
				if rerr != nil {
					break
				}
				var m struct {
					Type string `json:"type"`
					Item struct {
						ModelKey string `json:"modelKey"`
						Type     string `json:"type"`
						Device   string `json:"device"`
					} `json:"item"`
				}
				if json.Unmarshal(raw, &m) != nil {
					counts["(illisible)"]++
					continue
				}
				k := m.Type + "/" + m.Item.ModelKey + "/" + m.Item.Type
				counts[k]++
				if m.Item.Type == "ring" {
					ringSeen = true
					fmt.Printf("   >>> ÉVÉNEMENT RING reçu (caméra %s, message %q)\n", mask(m.Item.Device), m.Type)
				}
			}
			cancel()
			var ks []string
			for k := range counts {
				ks = append(ks, k)
			}
			sort.Strings(ks)
			for _, k := range ks {
				fmt.Printf("   messages %-40s x%d\n", k, counts[k])
			}
			fmt.Printf("   événement de sonnerie exposé par le flux : %v\n", ringSeen)
		} else {
			_ = conn.Close()
			fmt.Println("   (écoute non demandée : PROBE_LISTEN_SECONDS=30 pour tester une pression)")
		}
	}

	fmt.Println("== 3. Repli par interrogation (champ lastRing de la caméra)")
	code, _, body, _, err = get("/proxy/protect/api/cameras/" + url.PathEscape(target))
	if err != nil || code != 200 {
		fmt.Printf("   interrogation KO : statut %d\n", code)
	} else {
		var cam map[string]any
		_ = json.Unmarshal(body, &cam)
		if v, ok := cam["lastRing"]; ok {
			fmt.Printf("   lastRing présent (type %T) : l'interrogation toutes les 2 s est possible\n", v)
		} else {
			fmt.Println("   lastRing ABSENT : le repli par interrogation ne fonctionnera pas tel quel")
		}
	}

	fmt.Println("== 4. Instantané (vidéo en images fixes)")
	var h http.Header
	for i := 0; i < 3; i++ {
		code, h, body, dur, err = get("/proxy/protect/integration/v1/cameras/" + url.PathEscape(target) + "/snapshot")
		if err != nil || code != 200 {
			fmt.Printf("   instantané %d : KO statut %d\n", i+1, code)
			continue
		}
		fmt.Printf("   instantané %d : %d octets, %s, %s\n", i+1, len(body), h.Get("Content-Type"), dur.Round(time.Millisecond))
	}
	fmt.Println("== Terminé (aucune modification effectuée sur la console).")
}
