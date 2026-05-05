package main

import (
	"taxibot/bot"
	"taxibot/config"
	"taxibot/database"
)

func main() {
	cfg := config.Load()
	db := database.Init(cfg.DBPath)
	defer db.Close()

	b := bot.New(cfg, db)
	b.Start()
}
