package dbhelper

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
)

// DBConnectionParameter ...
type DBConnectionParameter struct {
	dbUsername string
	dbPassword string
	dbURL      string
	dbPort     string
	dbName     string
}

var db *sql.DB
var dbParameter DBConnectionParameter

// Setup General Init of DB parameter
func Setup(dbuname string, dbpass string, targetURL string, targetPort string, dbtable string) {
	dbParameter.dbUsername = dbuname
	dbParameter.dbPassword = dbpass
	dbParameter.dbURL = targetURL
	dbParameter.dbPort = targetPort
	dbParameter.dbName = dbtable
}

// InitDB General Init of DB connection
func InitDB() {

	var err error
	//dnsStr := fmt.Sprintf("postgres://%s:%s@%s/testdb", "rdsadmin", url.PathEscape(authToken), "something.eu-west-3.rds.amazonaws.com")
	db, err = sql.Open("mysql", dbParameter.dbUsername+":"+dbParameter.dbPassword+"@tcp("+dbParameter.dbURL+":"+dbParameter.dbPort+")/"+dbParameter.dbName)
	if err != nil {
		log.Fatal(err)
		panic(err.Error())
		//fmt.Printf("Cannot open db: %s\n", err)
	}

	// Open doesn't open a connection. Validate DSN data:
	err = db.Ping()
	if err != nil {
		panic(err.Error()) // proper error handling instead of panic in your app
	}

	//defer db.Close()
}

// SteupnInitDB setup parameters and run init at the same time
func SteupnInitDB(dbuname string, dbpass string, targetURL string, targetPort string, dbtable string) {
	Setup(dbuname, dbpass, targetURL, targetPort, dbtable)
	InitDB()
}

// CloseDB close DB connection, not used in general
func CloseDB() {
	defer db.Close()
}

// GetDB Direct usage og db connection
func GetDB() *sql.DB {
	if db == nil {
		InitDB()
	}

	return db
}

// TesterFunc close DB connection, not used in general
func TesterFunc() (result string) {
	return "ok"
}
