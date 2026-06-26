package data

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/data/database"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// setupTestDB creates a test database connection
func setupTestDB(t *testing.T) *sqlx.DB {
	config := database.Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnvInt("DB_PORT", 5432),
		User:     getEnv("DB_USER", "postgres"),
		Password: getEnv("DB_PASSWORD", "postgres"),
		DBName:   getEnv("DB_NAME", "essensys_test"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}

	db, err := database.Connect(config)
	if err != nil {
		t.Skipf("Skipping test: cannot connect to test database: %v", err)
		return nil
	}

	// Run migrations for test database
	if err := runTestMigrations(db); err != nil {
		t.Fatalf("Failed to run test migrations: %v", err)
	}

	return db
}

// teardownTestDB cleans up test data
func teardownTestDB(t *testing.T, db *sqlx.DB) {
	if db == nil {
		return
	}

	// Clean up test data (in reverse order of dependencies)
	tables := []string{
		"es_state_index",
		"es_state",
		"es_action_index",
		"es_action",
		"es_sms_send",
		"es_phone",
		"es_user",
		"es_cle_machine",
		"es_machine",
		"es_data_index",
	}

	for _, table := range tables {
		_, _ = db.Exec("TRUNCATE TABLE " + table + " CASCADE")
	}
}

// runTestMigrations runs the schema migrations for testing
func runTestMigrations(db *sqlx.DB) error {
	migrationSQL := `
	-- Create tables (simplified for testing)
	CREATE TABLE IF NOT EXISTS es_data_index (
		id SERIAL PRIMARY KEY,
		index_key VARCHAR(50) NOT NULL UNIQUE,
		is_active BOOLEAN NOT NULL DEFAULT true,
		date_creation TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		date_modification TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS es_machine (
		id SERIAL PRIMARY KEY,
		no_serie VARCHAR(100) NOT NULL UNIQUE,
		version VARCHAR(50),
		pkey VARCHAR(256) NOT NULL,
		hashed_pkey VARCHAR(256),
		autorise_alarme BOOLEAN NOT NULL DEFAULT false,
		is_active BOOLEAN NOT NULL DEFAULT true,
		date_creation TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		date_modification TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS es_user (
		id SERIAL PRIMARY KEY,
		mail VARCHAR(255) NOT NULL UNIQUE,
		password VARCHAR(256) NOT NULL,
		nom VARCHAR(255) NOT NULL,
		prenom VARCHAR(255) NOT NULL,
		question VARCHAR(255) NOT NULL,
		reponse VARCHAR(256) NOT NULL,
		isvalid BOOLEAN NOT NULL DEFAULT false,
		send_infos BOOLEAN NOT NULL DEFAULT false,
		obsolete BOOLEAN NOT NULL DEFAULT false,
		date_creation TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		date_cloture TIMESTAMP,
		last_access TIMESTAMP,
		guid VARCHAR(100) UNIQUE,
		machine_id INTEGER REFERENCES es_machine(id) ON DELETE SET NULL
	);

	CREATE TABLE IF NOT EXISTS es_action (
		id SERIAL PRIMARY KEY,
		machine_id INTEGER NOT NULL REFERENCES es_machine(id) ON DELETE CASCADE,
		guid VARCHAR(100) NOT NULL UNIQUE,
		action_type VARCHAR(50) NOT NULL,
		action_info TEXT,
		is_done BOOLEAN NOT NULL DEFAULT false,
		date_creation TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS es_action_index (
		id SERIAL PRIMARY KEY,
		action_id INTEGER NOT NULL REFERENCES es_action(id) ON DELETE CASCADE,
		index_id INTEGER NOT NULL REFERENCES es_data_index(id) ON DELETE CASCADE,
		value VARCHAR(255) NOT NULL,
		UNIQUE(action_id, index_id)
	);

	CREATE TABLE IF NOT EXISTS es_state (
		id SERIAL PRIMARY KEY,
		machine_id INTEGER NOT NULL REFERENCES es_machine(id) ON DELETE CASCADE,
		version VARCHAR(50),
		completed BOOLEAN NOT NULL DEFAULT false,
		state_date TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS es_state_index (
		id SERIAL PRIMARY KEY,
		state_id INTEGER NOT NULL REFERENCES es_state(id) ON DELETE CASCADE,
		index_id INTEGER NOT NULL REFERENCES es_data_index(id) ON DELETE CASCADE,
		value VARCHAR(255) NOT NULL,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(state_id, index_id)
	);

	-- Insert test data indexes
	INSERT INTO es_data_index (index_key, is_active) VALUES
		('605', true), ('606', true), ('613', true), ('590', true)
	ON CONFLICT (index_key) DO NOTHING;
	`

	_, err := db.Exec(migrationSQL)
	return err
}

// Helper functions for environment variables
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

// TestDatabaseStore_StoreInterface tests that DatabaseStore implements Store interface
func TestDatabaseStore_StoreInterface(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer teardownTestDB(t, db)

	store := NewDatabaseStore(db)

	// Verify it implements Store interface
	var _ Store = store
}

// TestDatabaseStore_GetSetValue tests exchange table operations
func TestDatabaseStore_GetSetValue(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer teardownTestDB(t, db)

	store := NewDatabaseStore(db)

	// Create a test machine
	machineRepo := database.NewMachineRepository(db)
	machine := &models.Machine{
		NoSerie:        "TEST001",
		Version:        "V1",
		Pkey:           "test-key-32-chars-long-123456",
		AutoriseAlarme: false,
		IsActive:       true,
		DateCreation:   time.Now(),
		DateModification: time.Now(),
	}
	err := machineRepo.Create(machine)
	if err != nil {
		t.Fatalf("Failed to create test machine: %v", err)
	}

	// Test SetValue and GetValue
	clientID := "TEST001"
	index := 605
	value := "1"

	store.SetValue(clientID, index, value)

	retrievedValue, exists := store.GetValue(clientID, index)
	if !exists {
		t.Error("Value should exist after SetValue")
	}
	if retrievedValue != value {
		t.Errorf("Expected value %s, got %s", value, retrievedValue)
	}
}

// TestDatabaseStore_ActionQueue tests action queue operations
func TestDatabaseStore_ActionQueue(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer teardownTestDB(t, db)

	store := NewDatabaseStore(db)

	// Create a test machine
	machineRepo := database.NewMachineRepository(db)
	machine := &models.Machine{
		NoSerie:        "TEST002",
		Version:        "V1",
		Pkey:           "test-key-32-chars-long-123456",
		AutoriseAlarme: false,
		IsActive:       true,
		DateCreation:   time.Now(),
		DateModification: time.Now(),
	}
	err := machineRepo.Create(machine)
	if err != nil {
		t.Fatalf("Failed to create test machine: %v", err)
	}

	clientID := "TEST002"

	// Test EnqueueAction
	action := protocol.Action{
		GUID: "test-guid-123",
		Params: []protocol.ExchangeKV{
			{K: 605, V: "1"},
			{K: 613, V: "64"},
		},
	}

	store.EnqueueAction(clientID, action)

	// Test DequeueActions
	actions := store.DequeueActions(clientID)
	if len(actions) == 0 {
		t.Error("Expected at least one action")
	}

	found := false
	for _, a := range actions {
		if a.GUID == action.GUID {
			found = true
			if len(a.Params) != len(action.Params) {
				t.Errorf("Expected %d params, got %d", len(action.Params), len(a.Params))
			}
			break
		}
	}
	if !found {
		t.Error("Action not found in queue")
	}

	// Test AcknowledgeAction
	_, acknowledged := store.AcknowledgeAction(clientID, action.GUID)
	if !acknowledged {
		t.Error("Action should be acknowledged")
	}

	// Verify action is removed
	actionsAfter := store.DequeueActions(clientID)
	for _, a := range actionsAfter {
		if a.GUID == action.GUID {
			t.Error("Action should be removed after acknowledge")
		}
	}
}

// TestUserRepository tests user repository operations
func TestUserRepository(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer teardownTestDB(t, db)

	repo := database.NewUserRepository(db)

	// Test Create
	user := &models.User{
		Mail:      "test@example.com",
		Password:  "hashed-password",
		Nom:       "Dupont",
		Prenom:    "Jean",
		Question:  "What is your favorite color?",
		Reponse:   "hashed-answer",
		IsValid:   false,
		SendInfos: false,
		Obsolete:  false,
		DateCreation: time.Now(),
		Guid:      "test-guid-123",
	}

	err := repo.Create(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if user.ID == 0 {
		t.Error("User ID should be set after creation")
	}

	// Test GetByEmail
	retrievedUser, err := repo.GetByEmail("test@example.com")
	if err != nil {
		t.Fatalf("Failed to get user by email: %v", err)
	}
	if retrievedUser == nil {
		t.Error("User should exist")
	}
	if retrievedUser.Mail != user.Mail {
		t.Errorf("Expected email %s, got %s", user.Mail, retrievedUser.Mail)
	}

	// Test GetByGuid
	retrievedUser2, err := repo.GetByGuid("test-guid-123", "test@example.com")
	if err != nil {
		t.Fatalf("Failed to get user by guid: %v", err)
	}
	if retrievedUser2 == nil {
		t.Error("User should exist")
	}

	// Test Update
	user.Nom = "Martin"
	err = repo.Update(user)
	if err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}

	updatedUser, err := repo.GetByID(user.ID)
	if err != nil {
		t.Fatalf("Failed to get updated user: %v", err)
	}
	if updatedUser.Nom != "Martin" {
		t.Errorf("Expected name Martin, got %s", updatedUser.Nom)
	}

	// Test CheckEmailExists
	exists, err := repo.CheckEmailExists("test@example.com")
	if err != nil {
		t.Fatalf("Failed to check email exists: %v", err)
	}
	if !exists {
		t.Error("Email should exist")
	}

	exists, err = repo.CheckEmailExists("nonexistent@example.com")
	if err != nil {
		t.Fatalf("Failed to check email exists: %v", err)
	}
	if exists {
		t.Error("Email should not exist")
	}
}

// TestMachineRepository tests machine repository operations
func TestMachineRepository(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer teardownTestDB(t, db)

	repo := database.NewMachineRepository(db)

	// Test Create
	machine := &models.Machine{
		NoSerie:        "TEST003",
		Version:        "V1",
		Pkey:           "test-key-32-chars-long-123456",
		HashedPkey:     "hashed-key",
		AutoriseAlarme: true,
		IsActive:       true,
		DateCreation:   time.Now(),
		DateModification: time.Now(),
	}

	err := repo.Create(machine)
	if err != nil {
		t.Fatalf("Failed to create machine: %v", err)
	}
	if machine.ID == 0 {
		t.Error("Machine ID should be set after creation")
	}

	// Test GetByNoSerie
	retrievedMachine, err := repo.GetByNoSerie("TEST003")
	if err != nil {
		t.Fatalf("Failed to get machine by no_serie: %v", err)
	}
	if retrievedMachine == nil {
		t.Error("Machine should exist")
	}
	if retrievedMachine.NoSerie != machine.NoSerie {
		t.Errorf("Expected no_serie %s, got %s", machine.NoSerie, retrievedMachine.NoSerie)
	}

	// Test Update
	machine.Version = "V2"
	err = repo.Update(machine)
	if err != nil {
		t.Fatalf("Failed to update machine: %v", err)
	}

	updatedMachine, err := repo.GetByID(machine.ID)
	if err != nil {
		t.Fatalf("Failed to get updated machine: %v", err)
	}
	if updatedMachine.Version != "V2" {
		t.Errorf("Expected version V2, got %s", updatedMachine.Version)
	}
}




