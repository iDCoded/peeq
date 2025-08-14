package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Represents a saved database connection
type Connection struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name"`
	Type      string    `json:"type"` // Postgres, SQLite, MySQL, etc...
	DSN       string    `json:"dsn"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TableInfo struct {
	Name     string `json:"name"`
	RowCount int64  `json:"row_count"`
	Schema   string `json:"schema,omitempty"`
}

type ColumnInfo struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Nullable     bool   `json:"nullable"`
	DefaultValue string `json:"default_value,omitempty"`
	IsPrimaryKey bool   `json:"is_primary_key"`
}

type TableData struct {
	Columns []ColumnInfo             `json:"columns"`
	Rows    []map[string]interface{} `json:"rows"`
	Total   int64                    `json:"total"`
}

// App struct
type App struct {
	ctx          context.Context
	configDB     *gorm.DB
	activeDB     *gorm.DB
	activeConnID uint
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	log.Println("[Peeq] Initializing application")

	if err := a.initConfigDB(); err != nil {
		log.Fatal("[Config] Failed to initialize config database:", err)
	}

	log.Println("[Peeq] Application initialized successfully")
}

func (a *App) initConfigDB() error {
	configPath := filepath.Join(".", "config.db")

	db, err := gorm.Open(sqlite.Open(configPath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open config database: %v", err)
	}

	// Auto-migrate the Connection model
	if err := db.AutoMigrate(&Connection{}); err != nil {
		return fmt.Errorf("failed to migrate config database: %v", err)
	}

	a.configDB = db
	log.Println("[Config] Config database initialized")
	return nil
}

func (a *App) saveConnection(name, dbType, dsn string) error {
	conn := Connection{
		Name: name,
		Type: dbType,
		DSN:  dsn,
	}

	if err := a.configDB.Create(&conn).Error; err != nil {
		return fmt.Errorf("failed to save connection: %v", err)
	}

	log.Printf("[Config] Saved connection: %s (%s)", name, dbType)
	return nil
}
