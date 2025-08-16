package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"gorm.io/driver/postgres"
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

// initConfigDB initializes the configuration database using SQLite and GORM.
// It creates or opens the "config.db" file in the current directory, performs
// auto-migration for the Connection model, and assigns the database instance
// to the App's configDB field. Returns an error if database initialization or
// migration fails.
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

// saveConnection saves a new database connection configuration with the specified
// name, database type, and DSN (Data Source Name) into the application's configuration
// database. It returns an error if the operation fails.
// Parameters:
//   - name:   the name to identify the connection
//   - dbType: the type of the database (e.g., "mysql", "postgres")
//   - dsn:    the data source name containing connection details
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

// GetConnections retrieves all Connection records from the configDB.
// It returns a slice of Connection and an error if the operation fails.
func (a *App) GetConnections() ([]Connection, error) {
	var connections []Connection

	if err := a.configDB.Find(&connections).Error; err != nil {
		return nil, fmt.Errorf("failed to get connections: %v", err)
	}

	return connections, nil
}

// DeleteConnection deletes a connection from the configuration database by its ID.
// If the deleted connection is currently active, it closes the active connection.
// Returns an error if the deletion fails.
func (a *App) DeleteConnection(id uint) error {
	if err := a.configDB.Delete(&Connection{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete connection: %v", err)
	}

	// Close active connection if it is the one being deleted
	if a.activeConnID == id {
		a.activeDB = nil
		a.activeConnID = 0
	}

	log.Printf("[Config] Deleted connection with ID: %d", id)
	return nil
}

// ConnectToDatabase establishes a connection to a database specified by the given connection ID.
// It retrieves the connection configuration from the configDB, opens the database using GORM based on the connection type,
// and tests the connection by pinging the database. Supported database types are "postgres" and "sqlite".
// On success, it sets the activeDB and activeConnID fields of the App.
// Returns an error if the connection configuration is not found, the database type is unsupported,
// or if any step in the connection process fails.
func (a *App) ConnectToDatabase(id uint) error {
	var connection Connection

	if err := a.configDB.Find(&connection, id).Error; err != nil {
		return fmt.Errorf("connection not found: %v", err)
	}

	var db *gorm.DB
	var err error

	switch connection.Type {
	case "postgres":
		db, err = gorm.Open(postgres.Open(connection.DSN), &gorm.Config{})
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(connection.DSN), &gorm.Config{})
	default:
		return fmt.Errorf("unsupported database type: %s", connection.Type)
	}

	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	// Test the connection to the database
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %v", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	a.activeDB = db
	a.activeConnID = id

	log.Printf("[DB] Connected to database: %s", connection.Name)
	return nil
}

// GetTables retrieves a list of tables from the currently active database connection.
// For each table, it returns its name and the number of rows it contains.
// Supports PostgreSQL and SQLite databases. Returns an error if there is no active
// database connection, if the connection type is unsupported, or if any query fails.
//
// Returns:
//   - []TableInfo: Slice containing information about each table (name and row count).
//   - error: Non-nil if an error occurs during retrieval or querying.
func (a *App) GetTables() ([]TableInfo, error) {
	if a.activeDB == nil {
		return nil, fmt.Errorf("no active database connection")
	}

	var tables []TableInfo
	sqlDB, err := a.activeDB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %v", err)
	}

	// Get connection information
	var connection Connection
	if err := a.configDB.First(&connection, a.activeConnID).Error; err != nil {
		return nil, fmt.Errorf("failed to get connection info: %v", err)
	}

	var rows *sql.Rows

	switch connection.Type {
	case "postgres":
		rows, err = sqlDB.Query(`
			SELECT table_name
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		`)
	case "sqlite":
		rows, err = sqlDB.Query(`
			SELECT name 
			FROM sqlite_master 
			WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		`)
	default:
		return nil, fmt.Errorf("unsupported database type for table listing: %s", connection.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			continue
		}

		// Get row count for each table
		var count int64
		conutQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
		if err := sqlDB.QueryRow(conutQuery).Scan(&count); err != nil {
			count = 0 // set count to 0 if unable to get row count
		}

		tables = append(tables, TableInfo{
			Name:     tableName,
			RowCount: count,
		})
	}

	return tables, nil
}

// GetTableData retrieves paginated data from the specified table in the active database connection.
// It returns a TableData struct containing column information, rows of data, and the total row count.
// The function takes the table name, offset, and limit as parameters for pagination.
// Returns an error if there is no active database connection, if column info cannot be retrieved,
// or if any query fails.
func (a *App) GetTableData(tableName string, offset, limit int) (*TableData, error) {
	if a.activeDB == nil {
		return nil, fmt.Errorf("no active database connection")
	}

	sqlDB, err := a.activeDB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %v", err)
	}

	columns, err := a.getColumnInfo(tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to get column info: %v", err)
	}

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	if err := sqlDB.QueryRow(countQuery).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to get total count: %v", err)
	}

	// Get data with pagination
	dataQuery := fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", tableName, limit, offset)
	rows, err := sqlDB.Query(dataQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query table data: %v", err)
	}
	defer rows.Close()

	columnNames, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get column names: %v", err)
	}

	var data []map[string]interface{}

	for rows.Next() {
		values := make([]interface{}, len(columnNames))
		valuePtrs := make([]interface{}, len(columnNames))

		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{})
		for i, colName := range columnNames {
			if values[i] != nil {
				switch v := values[i].(type) {
				case []byte:
					row[colName] = string(v)
				default:
					row[colName] = v
				}
			} else {
				row[colName] = nil
			}
		}

		data = append(data, row)
	}

	return &TableData{
		Columns: columns,
		Rows:    data,
		Total:   total,
	}, nil

}

// getColumnInfo retrieves metadata about the columns of a specified table from the active database connection.
// It supports both PostgreSQL and SQLite databases, returning a slice of ColumnInfo structs containing details
// such as column name, data type, nullability, default value, and primary key status.
//
// Parameters:
//   - tableName: The name of the table for which column information is requested.
//
// Returns:
//   - []ColumnInfo: A slice containing metadata for each column in the specified table.
//   - error: An error if the operation fails or the database type is unsupported.
func (a *App) getColumnInfo(tableName string) ([]ColumnInfo, error) {
	sqlDB, err := a.activeDB.DB()
	if err != nil {
		return nil, err
	}

	// Get current connection info to determine database type
	var connection Connection
	if err := a.configDB.First(&connection, a.activeConnID).Error; err != nil {
		return nil, err
	}

	var columns []ColumnInfo
	var rows *sql.Rows

	switch connection.Type {
	case "postgres":
		rows, err = sqlDB.Query(`
			SELECT column_name, data_type, is_nullable, column_default,
				   CASE WHEN constraint_type = 'PRIMARY KEY' THEN true ELSE false END as is_primary
			FROM information_schema.columns c
			LEFT JOIN information_schema.key_column_usage kcu ON c.table_name = kcu.table_name AND c.column_name = kcu.column_name
			LEFT JOIN information_schema.table_constraints tc ON kcu.constraint_name = tc.constraint_name
			WHERE c.table_name = $1
			ORDER BY c.ordinal_position
		`, tableName)
	case "sqlite":
		rows, err = sqlDB.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	default:
		return nil, fmt.Errorf("unsupported database type: %s", connection.Type)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if connection.Type == "postgres" {
		for rows.Next() {
			var col ColumnInfo
			var nullable, defaultVal sql.NullString
			if err := rows.Scan(&col.Name, &col.Type, &nullable, &defaultVal, &col.IsPrimaryKey); err != nil {
				continue
			}
			col.Nullable = nullable.String == "YES"
			if defaultVal.Valid {
				col.DefaultValue = defaultVal.String
			}
			columns = append(columns, col)
		}
	} else if connection.Type == "sqlite" {
		for rows.Next() {
			var cid int
			var col ColumnInfo
			var notNull int
			var defaultVal sql.NullString
			var pk int
			if err := rows.Scan(&cid, &col.Name, &col.Type, &notNull, &defaultVal, &pk); err != nil {
				continue
			}
			col.Nullable = notNull == 0
			col.IsPrimaryKey = pk == 1
			if defaultVal.Valid {
				col.DefaultValue = defaultVal.String
			}
			columns = append(columns, col)
		}
	}

	return columns, nil
}

// TestConnection attempts to establish a connection to a database using the provided
// database type and DSN (Data Source Name). Supported database types are "postgres"
// and "sqlite". It returns an error if the connection cannot be established, the
// underlying sql.DB cannot be retrieved, or the database cannot be pinged successfully.
// If the connection is successful, it returns nil.
func (a *App) TestConnection(dbType, dsn string) error {
	var db *gorm.DB
	var err error

	switch dbType {
	case "postgres":
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	default:
		return fmt.Errorf("unsupported database type: %s", dbType)
	}

	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %v", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	return nil
}
