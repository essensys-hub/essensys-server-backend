package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func main() {
	port := 8080 // Port alternatif pour ne pas interférer avec le serveur principal
	
	log.Printf("===========================================")
	log.Printf("Debug Auth Listener - Essensys IoT Client")
	log.Printf("===========================================")
	log.Printf("Listening on port %d", port)
	log.Printf("Connect your IoT client to: http://localhost:%d", port)
	log.Printf("===========================================")
	
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log toutes les informations de la requête
		log.Printf("\n===========================================")
		log.Printf("REQUEST: %s %s", r.Method, r.URL.Path)
		log.Printf("From: %s", r.RemoteAddr)
		log.Printf("===========================================")
		
		// Afficher tous les headers
		log.Printf("\n--- Headers ---")
		for name, values := range r.Header {
			for _, value := range values {
				log.Printf("%s: %s", name, value)
			}
		}
		
		// Extraire et décoder le token d'authentification
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			log.Printf("\n--- AUTHENTICATION TOKEN ---")
			log.Printf("Authorization Header: %s", authHeader)
			
			if strings.HasPrefix(authHeader, "Basic ") {
				// Extraire le Base64
				encodedCredentials := strings.TrimPrefix(authHeader, "Basic ")
				log.Printf("Base64 Encoded: %s", encodedCredentials)
				
				// Décoder Base64
				decodedBytes, err := base64.StdEncoding.DecodeString(encodedCredentials)
				if err != nil {
					log.Printf("ERROR: Failed to decode Base64: %v", err)
				} else {
					credentials := string(decodedBytes)
					log.Printf("Decoded (username:password): %s", credentials)
					
					// Parser username:password
					parts := strings.SplitN(credentials, ":", 2)
					if len(parts) == 2 {
						username := parts[0]
						password := parts[1]
						
						log.Printf("\n--- TOKEN BREAKDOWN ---")
						log.Printf("Username (first 16 hex): %s", username)
						log.Printf("Password (last 16 hex): %s", password)
						log.Printf("Hashed Pkey (concatenated): %s%s", username, password)
						log.Printf("Hashed Pkey Length: %d characters", len(username+password))
						
						// Vérifier le format
						if len(username) == 16 && len(password) == 16 {
							log.Printf("✅ Format correct: 16 hex chars + 16 hex chars = 32 hex chars (MD5)")
						} else {
							log.Printf("⚠️  Format unexpected: username=%d chars, password=%d chars", len(username), len(password))
						}
						
						// Afficher pour copier-coller dans SQL
						log.Printf("\n--- SQL QUERY TO FIND MACHINE ---")
						log.Printf("SELECT * FROM es_machine WHERE hashed_pkey = '%s%s' AND is_active = true;", username, password)
					} else {
						log.Printf("ERROR: Invalid credentials format (expected username:password)")
					}
				}
			} else {
				log.Printf("⚠️  Authorization header is not Basic Auth")
			}
		} else {
			log.Printf("\n⚠️  NO AUTHORIZATION HEADER - Client is not sending authentication")
		}
		
		// Afficher le body si présent
		if r.ContentLength > 0 {
			log.Printf("\n--- Request Body ---")
			body := make([]byte, r.ContentLength)
			n, _ := r.Body.Read(body)
			if n > 0 {
				log.Printf("%s", string(body[:n]))
			}
		}
		
		log.Printf("\n===========================================\n")
		
		// Répondre avec un 200 OK simple pour ne pas bloquer le client
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	})
	
	// Créer le serveur
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}
	
	// Démarrer le serveur
	log.Fatal(server.ListenAndServe())
}

