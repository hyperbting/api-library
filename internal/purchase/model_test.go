package purchase

import (
	"api-library/internal/infrastructure"
	"log"
	"os"
	"testing"

	"gorm.io/gorm"
)

var gormDB *gorm.DB

func TestMain(m *testing.M) {
	// 1. Load test/dev database config
	cfg := infrastructure.NewDevDBConfig()

	// 2. Initialize database connection
	var err error
	gormDB, err = infrastructure.NewPostgreSQL(cfg)
	if err != nil {
		log.Fatalf("Failed to wrap *sql.DB with GORM: %v", err)
	}

	// 3. Run all tests in package
	code := m.Run()

	// 4. Manually close DB connection after tests complete
	sqlDB, err := gormDB.DB()
	if err != nil {
		log.Printf("Failed to get sql.DB for closing: %v", err)
	} else if err := sqlDB.Close(); err != nil {
		log.Printf("Failed to close database connection: %v", err)
	}

	// 5. Exit with test runner status code
	os.Exit(code)
}

func TestTransactionMigrate(t *testing.T) {

	t.Skip("Skipping the AutoMigrate part of the test.")

	t.Run("AutoMigrate", func(t *testing.T) {

		//Drop table if exists (will ignore or delete foreign key constraints when dropping)
		gormDB.Migrator().DropTable(
			&PlatformTransaction{},
			&CoinTransaction{},
			&PlayerBalance{},
		)

		err := gormDB.AutoMigrate(
			&PlatformTransaction{},
			&CoinTransaction{},
			&PlayerBalance{},
		)
		if err != nil {
			t.Fatalf("AutoMigrate failed: %v", err)
		}
	})
}
