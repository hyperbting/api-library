package database

func NewDevDBConfig() DBConfig {
	return DBConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "game_user",
		Password: "game_password",
		DBName:   "game_db",
	}
}
