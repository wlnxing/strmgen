package config

import "os"

type Config struct {
	ListenAddr    string
	DBPath        string
	CronTZ        string
	AdminUsername string
	AdminPassword string
}

func Load() Config {
	return Config{
		ListenAddr:    env("STRM_LISTEN_ADDR", ":18080"),
		DBPath:        env("STRM_DB_PATH", "data/strm.db"),
		CronTZ:        env("STRM_CRON_TZ", "Asia/Shanghai"),
		AdminUsername: env("STRM_ADMIN_USERNAME", "admin"),
		AdminPassword: env("STRM_ADMIN_PASSWORD", "admin"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
