package main

// Script de test manuel pour la Phase 2
// Usage: go run test/phase2_manual_test.go

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/internal/data/database"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
	"github.com/jmoiron/sqlx"
)

func main() {
	// Configuration de la base de données
	config := database.Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnvInt("DB_PORT", 5432),
		User:     getEnv("DB_USER", "postgres"),
		Password: getEnv("DB_PASSWORD", "postgres"),
		DBName:   getEnv("DB_NAME", "essensys_test"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}

	fmt.Println("==========================================")
	fmt.Println("Test Manuel Phase 2 : Repositories")
	fmt.Println("==========================================")
	fmt.Println("")

	// Connexion à la base de données
	fmt.Println("1. Connexion à PostgreSQL...")
	db, err := database.Connect(config)
	if err != nil {
		log.Fatalf("Erreur de connexion: %v", err)
	}
	defer database.Close()
	fmt.Println("   ✓ Connexion réussie")
	fmt.Println("")

	// Test 1: UserRepository
	fmt.Println("2. Test UserRepository...")
	testUserRepository(db)
	fmt.Println("   ✓ UserRepository OK")
	fmt.Println("")

	// Test 2: MachineRepository
	fmt.Println("3. Test MachineRepository...")
	machine := testMachineRepository(db)
	fmt.Println("   ✓ MachineRepository OK")
	fmt.Println("")

	// Test 3: DatabaseStore (interface Store)
	fmt.Println("4. Test DatabaseStore (interface Store)...")
	testDatabaseStore(db, machine)
	fmt.Println("   ✓ DatabaseStore OK")
	fmt.Println("")

	// Test 4: ActionRepository
	fmt.Println("5. Test ActionRepository...")
	testActionRepository(db, machine.ID)
	fmt.Println("   ✓ ActionRepository OK")
	fmt.Println("")

	// Test 5: StateRepository
	fmt.Println("6. Test StateRepository...")
	testStateRepository(db, machine.ID)
	fmt.Println("   ✓ StateRepository OK")
	fmt.Println("")

	fmt.Println("==========================================")
	fmt.Println("Tous les tests manuels sont passés !")
	fmt.Println("==========================================")
}

func testUserRepository(db *sqlx.DB) {
	repo := database.NewUserRepository(db)

	// Créer un utilisateur de test
	user := &models.User{
		Mail:         "test-manual@example.com",
		Password:     "hashed-password-sha1",
		Nom:          "Test",
		Prenom:       "Manual",
		Question:     "What is your favorite color?",
		Reponse:      "hashed-answer-sha1",
		IsValid:      false,
		SendInfos:    false,
		Obsolete:     false,
		DateCreation: time.Now(),
		Guid:         "test-guid-manual-123",
	}

	err := repo.Create(user)
	if err != nil {
		log.Fatalf("Erreur création utilisateur: %v", err)
	}
	fmt.Printf("   - Utilisateur créé (ID: %d)\n", user.ID)

	// Récupérer par email
	retrieved, err := repo.GetByEmail("test-manual@example.com")
	if err != nil || retrieved == nil {
		log.Fatalf("Erreur récupération utilisateur: %v", err)
	}
	fmt.Printf("   - Utilisateur récupéré par email: %s\n", retrieved.Mail)

	// Vérifier email existe
	exists, err := repo.CheckEmailExists("test-manual@example.com")
	if err != nil || !exists {
		log.Fatalf("Erreur vérification email: %v", err)
	}
	fmt.Printf("   - Email vérifié: existe = %v\n", exists)
}

func testMachineRepository(db *sqlx.DB) *models.Machine {
	repo := database.NewMachineRepository(db)

	// Créer une machine de test
	machine := &models.Machine{
		NoSerie:        "TEST-MANUAL-001",
		Version:        "V1",
		Pkey:           "test-key-32-chars-long-123456",
		HashedPkey:     "hashed-key-test",
		AutoriseAlarme: true,
		IsActive:       true,
		DateCreation:   time.Now(),
		DateModification: time.Now(),
	}

	err := repo.Create(machine)
	if err != nil {
		log.Fatalf("Erreur création machine: %v", err)
	}
	fmt.Printf("   - Machine créée (ID: %d, NoSerie: %s)\n", machine.ID, machine.NoSerie)

	// Récupérer par no_serie
	retrieved, err := repo.GetByNoSerie("TEST-MANUAL-001")
	if err != nil || retrieved == nil {
		log.Fatalf("Erreur récupération machine: %v", err)
	}
	fmt.Printf("   - Machine récupérée par no_serie: %s\n", retrieved.NoSerie)

	return machine
}

func testDatabaseStore(db *sqlx.DB, machine *models.Machine) {
	store := data.NewDatabaseStore(db)

	clientID := machine.NoSerie

	// Test SetValue / GetValue
	store.SetValue(clientID, 605, "1")
	store.SetValue(clientID, 613, "64")

	value, exists := store.GetValue(clientID, 605)
	if !exists || value != "1" {
		log.Fatalf("Erreur GetValue: expected 1, got %s (exists: %v)", value, exists)
	}
	fmt.Printf("   - SetValue/GetValue OK (index 605 = %s)\n", value)

	// Test GetAllValues
	values := store.GetAllValues(clientID, []int{605, 613})
	if len(values) != 2 {
		log.Fatalf("Erreur GetAllValues: expected 2 values, got %d", len(values))
	}
	fmt.Printf("   - GetAllValues OK (%d valeurs)\n", len(values))

	// Test EnqueueAction
	action := protocol.Action{
		GUID: "test-guid-manual-123",
		Params: []protocol.ExchangeKV{
			{K: 605, V: "1"},
			{K: 613, V: "64"},
		},
	}
	store.EnqueueAction(clientID, action)
	fmt.Printf("   - EnqueueAction OK (GUID: %s)\n", action.GUID)

	// Test DequeueActions
	actions := store.DequeueActions(clientID)
	if len(actions) == 0 {
		log.Fatal("Erreur DequeueActions: aucune action trouvée")
	}
	fmt.Printf("   - DequeueActions OK (%d actions)\n", len(actions))

	// Test AcknowledgeAction
	_, acknowledged := store.AcknowledgeAction(clientID, action.GUID)
	if !acknowledged {
		log.Fatal("Erreur AcknowledgeAction: action non reconnue")
	}
	fmt.Printf("   - AcknowledgeAction OK\n")
}

func testActionRepository(db *sqlx.DB, machineID int) {
	repo := database.NewActionRepository(db)
	dataIndexRepo := database.NewDataIndexRepository(db)

	// Créer les index de données nécessaires
	index605, _ := dataIndexRepo.GetOrCreateByKey("605")
	index613, _ := dataIndexRepo.GetOrCreateByKey("613")

	// Créer une action
	action := &models.Action{
		MachineID:    machineID,
		Guid:         "test-action-manual-123",
		ActionType:   "TEST",
		ActionInfo:   "Test action",
		IsDone:       false,
		DateCreation: time.Now(),
	}

	actionIndexes := []models.ActionIndex{
		{IndexID: index605.ID, Value: "1"},
		{IndexID: index613.ID, Value: "64"},
	}

	err := repo.Create(action, actionIndexes)
	if err != nil {
		log.Fatalf("Erreur création action: %v", err)
	}
	fmt.Printf("   - Action créée (ID: %d, GUID: %s)\n", action.ID, action.Guid)

	// Récupérer les actions en attente
	pending, err := repo.GetPendingByMachineID(machineID)
	if err != nil {
		log.Fatalf("Erreur récupération actions: %v", err)
	}
	fmt.Printf("   - Actions en attente: %d\n", len(pending))

	// Marquer comme terminée
	err = repo.MarkDone(action.Guid)
	if err != nil {
		log.Fatalf("Erreur marquage action: %v", err)
	}
	fmt.Printf("   - Action marquée comme terminée\n")
}

func testStateRepository(db *sqlx.DB, machineID int) {
	repo := database.NewStateRepository(db)
	dataIndexRepo := database.NewDataIndexRepository(db)

	// Créer les index de données nécessaires
	index605, _ := dataIndexRepo.GetOrCreateByKey("605")
	index613, _ := dataIndexRepo.GetOrCreateByKey("613")

	// Créer un état
	state := &models.State{
		MachineID:  machineID,
		Version:    "V1",
		Completed:  true,
		StateDate:  time.Now(),
	}

	stateIndexes := []models.StateIndex{
		{IndexID: index605.ID, Value: "1", UpdatedAt: time.Now()},
		{IndexID: index613.ID, Value: "64", UpdatedAt: time.Now()},
	}

	err := repo.Create(state, stateIndexes)
	if err != nil {
		log.Fatalf("Erreur création état: %v", err)
	}
	fmt.Printf("   - État créé (ID: %d)\n", state.ID)

	// Récupérer le dernier état
	latest, err := repo.GetLatestByMachineID(machineID)
	if err != nil || latest == nil {
		log.Fatalf("Erreur récupération dernier état: %v", err)
	}
	fmt.Printf("   - Dernier état récupéré (ID: %d)\n", latest.ID)

	// Vérifier si rafraîchi
	hasRefreshed, err := repo.HasRefreshed(machineID, time.Now().Add(-1*time.Hour))
	if err != nil {
		log.Fatalf("Erreur vérification refresh: %v", err)
	}
	fmt.Printf("   - Machine rafraîchie: %v\n", hasRefreshed)
}

// Helpers
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

