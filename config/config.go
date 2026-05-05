package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	BotToken string
	AdminIDs []int64
	DBPath   string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file, using environment variables")
	}

	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("BOT_TOKEN is required")
	}

	var adminIDs []int64
	for _, s := range strings.Split(os.Getenv("ADMIN_IDS"), ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			log.Printf("Invalid admin ID %q: %v", s, err)
			continue
		}
		adminIDs = append(adminIDs, id)
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "taxibot.db"
	}

	return &Config{
		BotToken: token,
		AdminIDs: adminIDs,
		DBPath:   dbPath,
	}
}
