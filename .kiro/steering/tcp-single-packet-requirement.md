# TCP Single Packet Requirement - Client BP_MQX_ETH

## Découverte Critique

Le client BP_MQX_ETH embarqué a un parser HTTP **extrêmement simple** qui s'attend à recevoir **toute la réponse HTTP en un seul paquet TCP**. Si la réponse est fragmentée en plusieurs paquets, le client ne peut pas la parser correctement et reste bloqué.

## Symptômes du Problème

Quand le serveur Go envoyait les réponses HTTP de manière standard (headers puis body séparément), le client BP_MQX_ETH:
- ✅ Appelait `/api/serverinfos` avec succès
- ❌ N'appelait JAMAIS `/api/mystatus` ni `/api/myactions`
- ❌ Restait bloqué après avoir reçu la première réponse

## Analyse avec tcpdump

Avec le **serveur Go (avant correction)**:
```
Paquet 1: HTTP/1.1 200 OK\r\n
Paquet 2: Connection: close\r\n
Paquet 3: Content-Length: 97\r\n
Paquet 4: Content-Type: application/json ;charset=UTF-8\r\n
Paquet 5: \r\n
Paquet 6: {JSON body}
```
→ Le client ne pouvait pas parser cette réponse fragmentée

Avec le **server.sample (qui fonctionne)**:
```
Paquet 1: HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Length: 96\r\nContent-Type: application/json ;charset=UTF-8\r\n\r\n{JSON body}
```
→ Tout en un seul paquet, le client parse correctement

## Solution Implémentée

Dans `internal/server/legacy_http_server.go`, la fonction `flush()` du `legacyResponseWriter` doit:

1. **Bufferiser toute la réponse** (headers + body) dans un `bytes.Buffer`
2. **Envoyer tout en un seul appel** `conn.Write(response.Bytes())`

```go
func (w *legacyResponseWriter) flush() error {
    // Build the entire response in a buffer
    var response bytes.Buffer
    
    // Write status line
    fmt.Fprintf(&response, "HTTP/1.1 %d %s\r\n", w.statusCode, statusText)
    
    // Write all headers
    fmt.Fprintf(&response, "Connection: close\r\n")
    fmt.Fprintf(&response, "Content-Length: %d\r\n", w.bodyBuffer.Len())
    for key, values := range w.header {
        for _, value := range values {
            fmt.Fprintf(&response, "%s: %s\r\n", key, value)
        }
    }
    
    // End of headers
    fmt.Fprintf(&response, "\r\n")
    
    // Append body
    response.Write(w.bodyBuffer.Bytes())
    
    // CRITICAL: Send everything in a SINGLE write() call
    _, err := w.conn.Write(response.Bytes())
    return err
}
```

## Pourquoi C'est Critique

Le client BP_MQX_ETH est un système embarqué ancien avec:
- **Mémoire limitée** - Pas de buffer pour réassembler les paquets TCP
- **Parser HTTP simple** - Lit une seule fois depuis le socket avec `recv()`
- **Pas de gestion de fragmentation** - S'attend à tout recevoir d'un coup

C'est une contrainte **non-documentée** du protocole legacy qui a été découverte par analyse réseau avec `tcpdump`.

## Validation

Après correction, le client BP_MQX_ETH:
- ✅ Appelle `/api/serverinfos` toutes les ~20 secondes
- ✅ Appelle `/api/mystatus` toutes les ~2 secondes
- ✅ Appelle `/api/myactions` toutes les ~2 secondes
- ✅ Récupère et exécute les actions (allumer/éteindre lumières)
- ✅ Acknowledge les actions via `/api/done/{guid}`

## Headers Requis

En plus du single-packet, ces headers sont **obligatoires**:

1. **Connection: close** - Le client attend que la connexion se ferme
2. **Content-Length: N** - Le client a besoin de la taille exacte du body
3. **Content-Type: application/json ;charset=UTF-8** - Avec espace avant `;charset`

## Outils de Diagnostic

Pour diagnostiquer ce type de problème:

```bash
# Capturer les paquets TCP du client
sudo tcpdump -i any -s 0 -A 'tcp port 80 and host 192.168.0.151' -c 10

# Comparer avec un serveur qui fonctionne (server.sample)
# Observer le nombre de paquets TCP par réponse HTTP
```

## Leçons Apprises

1. **Ne jamais supposer** que les clients legacy suivent les standards HTTP
2. **Toujours tester avec le matériel réel** - Les simulateurs ne révèlent pas ces problèmes
3. **Utiliser tcpdump/tshark** pour comparer le trafic réseau avec un serveur de référence
4. **Bufferiser et envoyer en une fois** pour les clients embarqués anciens

## Impact sur le Code

Cette contrainte affecte **uniquement** `internal/server/legacy_http_server.go`. Le reste du code (handlers, services, store) fonctionne normalement car ils utilisent l'interface standard `http.ResponseWriter`.

Le `legacyResponseWriter` agit comme un **adaptateur** qui bufferise tout et envoie en un seul paquet, rendant le serveur compatible avec le client legacy tout en gardant le code métier propre.
