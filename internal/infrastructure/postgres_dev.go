package infrastructure

func NewDevDBConfig() *BaseDBConfig {
	return &BaseDBConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "game_user",
		Password: "game_password",
		DBName:   "game_db",
	}
}
