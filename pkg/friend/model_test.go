package friend

import (
	myCFG "api-library/config"
	myDB "api-library/internal/database"
	"database/sql"
	"log"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var db *sql.DB
var gormDB *gorm.DB

func TestMain(m *testing.M) {
	// 1. Load test/dev database config
	cfg := myCFG.NewDevDBConfig()

	// 2. Initialize database connection
	var err error
	db, err = myDB.NewPostgreSQL(*cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	gormDB, err = gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to wrap *sql.DB with GORM: %v", err)
	}

	// 3. Run all tests in package
	code := m.Run()

	// 4. Manually close DB connection after tests complete
	if err := db.Close(); err != nil {
		log.Printf("Failed to close database clean connection: %v", err)
	}

	// 5. Exit with test runner status code
	os.Exit(code)
}

func TestUserRelationshipMigrate(t *testing.T) {

	t.Skip("Skipping the AutoMigrate part of the test.")

	t.Run("AutoMigrate", func(t *testing.T) {

		//Drop table if exists (will ignore or delete foreign key constraints when dropping)
		gormDB.Migrator().DropTable(
			&UserRelationship{},
		)

		err := gormDB.AutoMigrate(
			&UserRelationship{},
		)
		if err != nil {
			t.Fatalf("AutoMigrate failed: %v", err)
		}
	})
}

// func TestUserRelationship_TableName(t *testing.T) {
// 	instance := UserRelationship{}
// 	typeOf := reflect.TypeOf(instance)
// 	actual := typeOf.MethodByName("TableName").Type()
// 	expectedType, _ := reflect.TypeOf(""), reflect.TypeOf(func() string { return "" })

// 	if actual.NumIn() != 1 || actual.In(0) != typeOf {
// 		t.Errorf("Method 'TableName' must have signature func() string, got %s", actual)
// 	}
// 	// Check return type
// 	if actual.NumOut() != 1 || actual.Out(0) != expectedType {
// 		t.Errorf("Method 'TableName' must return string, got %s", actual.Out(0))
// 	}
// }
