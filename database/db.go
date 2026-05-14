package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
}

type Group struct {
	ChatID    int64
	Title     string
	LastMsgID int
	LastSentAt time.Time
}

func Init(path string) *DB {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		log.Fatal("Failed to open DB:", err)
	}
	if err := migrate(db); err != nil {
		log.Fatal("DB migration failed:", err)
	}
	return &DB{db}
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS monitored_groups (
			chat_id      INTEGER PRIMARY KEY,
			title        TEXT NOT NULL,
			last_msg_id  INTEGER DEFAULT 0,
			last_sent_at TEXT DEFAULT '',
			added_at     DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		);
		INSERT OR IGNORE INTO settings (key, value) VALUES ('target_group', '');
		INSERT OR IGNORE INTO settings (key, value) VALUES ('bot_enabled',  'true');
	`)
	if err != nil {
		return err
	}
	// Add columns to existing tables (safe to run multiple times)
	db.Exec(`ALTER TABLE monitored_groups ADD COLUMN last_msg_id INTEGER DEFAULT 0`)
	db.Exec(`ALTER TABLE monitored_groups ADD COLUMN last_sent_at TEXT DEFAULT ''`)
	return nil
}

func (db *DB) IsMonitored(chatID int64) bool {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM monitored_groups WHERE chat_id = ?", chatID).Scan(&n)
	return n > 0
}

func (db *DB) AddGroup(chatID int64, title string) error {
	_, err := db.Exec(
		"INSERT OR REPLACE INTO monitored_groups (chat_id, title) VALUES (?, ?)",
		chatID, title,
	)
	return err
}

func (db *DB) RemoveGroup(chatID int64) error {
	_, err := db.Exec("DELETE FROM monitored_groups WHERE chat_id = ?", chatID)
	return err
}

func (db *DB) ListGroups() ([]Group, error) {
	rows, err := db.Query("SELECT chat_id, title, last_msg_id, last_sent_at FROM monitored_groups")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []Group
	for rows.Next() {
		var g Group
		var sentAt string
		if err := rows.Scan(&g.ChatID, &g.Title, &g.LastMsgID, &sentAt); err != nil {
			return nil, err
		}
		if sentAt != "" {
			g.LastSentAt, _ = time.Parse(time.RFC3339, sentAt)
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (db *DB) UpdateLastMsg(chatID int64, msgID int, sentAt time.Time) error {
	_, err := db.Exec(
		"UPDATE monitored_groups SET last_msg_id = ?, last_sent_at = ? WHERE chat_id = ?",
		msgID, sentAt.Format(time.RFC3339), chatID,
	)
	return err
}

func (db *DB) GetTargetGroup() int64 {
	var v string
	db.QueryRow("SELECT value FROM settings WHERE key = 'target_group'").Scan(&v)
	if v == "" {
		return 0
	}
	var id int64
	fmt.Sscanf(v, "%d", &id)
	return id
}

func (db *DB) SetTargetGroup(chatID int64) error {
	_, err := db.Exec(
		"INSERT OR REPLACE INTO settings (key, value) VALUES ('target_group', ?)",
		fmt.Sprintf("%d", chatID),
	)
	return err
}

func (db *DB) IsBotEnabled() bool {
	var v string
	db.QueryRow("SELECT value FROM settings WHERE key = 'bot_enabled'").Scan(&v)
	return v == "true"
}

func (db *DB) SetBotEnabled(enabled bool) error {
	v := "false"
	if enabled {
		v = "true"
	}
	_, err := db.Exec(
		"INSERT OR REPLACE INTO settings (key, value) VALUES ('bot_enabled', ?)", v,
	)
	return err
}
