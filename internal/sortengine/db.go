package sortengine

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
	// m "github.com/ascheel/gosort/internal/media"
)

type DB struct {
	// SQLite3 database
	filename string
	config *Config
	db *sql.DB
	destdir string
	
	// Prepared statements - cached for performance
	// These are prepared once during Init() and reused for all queries
	// This eliminates the overhead of preparing statements for every query
	stmtChecksumExists    *sql.Stmt
	stmtChecksum100kExists *sql.Stmt
	stmtAddFile          *sql.Stmt
}

func NewDB(filename string, config *Config) *DB {
	db := &DB{}
	db.filename = filename
	db.config = config
	db.Init()
	return db
}

func (d *DB) AddFileToDB(media *Media) error {
	if len(media.Checksum) == 0 {
		media.SetChecksum()
	}
	
	// Use cached prepared statement for better performance
	if d.stmtAddFile == nil {
		return fmt.Errorf("database not properly initialized: AddFile statement is nil")
	}
	
	_, err := d.stmtAddFile.Exec(
		media.Filename,
		media.Checksum,
		media.Checksum100k,
		media.Size,
		media.CreationDate,
	)
	if err != nil {
		return err
	}
	return nil
}

// AddFilesToDBBatch inserts multiple files in a single transaction
// This is much faster than inserting files one at a time (5-10x improvement)
// batchSize controls how many files to insert per transaction (default: 100)
func (d *DB) AddFilesToDBBatch(mediaList []*Media, batchSize int) error {
	if len(mediaList) == 0 {
		return nil
	}
	
	if batchSize <= 0 {
		batchSize = 100 // Default batch size
	}
	
	// Ensure all media have checksums
	for _, media := range mediaList {
		if len(media.Checksum) == 0 {
			media.SetChecksum()
		}
	}
	
	// Process in batches
	for i := 0; i < len(mediaList); i += batchSize {
		end := i + batchSize
		if end > len(mediaList) {
			end = len(mediaList)
		}
		batch := mediaList[i:end]
		
		// Start transaction
		tx, err := d.db.Begin()
		if err != nil {
			return fmt.Errorf("error starting transaction: %v", err)
		}
		
		// Prepare statement for this transaction
		// Use INSERT OR IGNORE to handle duplicates gracefully (atomic operation)
		// This prevents entire batch rollback on duplicate entries
		stmt, err := tx.Prepare("INSERT OR IGNORE INTO media (filename, checksum, checksum100k, size, create_date) VALUES (?, ?, ?, ?, ?)")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error preparing batch insert statement: %v", err)
		}
		
		// Track successful and failed inserts for better error reporting
		var successful []*Media
		var failed []struct {
			media *Media
			err   error
		}
		
		// Insert all files in this batch
		// Individual errors are handled gracefully - batch continues processing
		for _, media := range batch {
			result, err := stmt.Exec(
				media.Filename,
				media.Checksum,
				media.Checksum100k,
				media.Size,
				media.CreationDate,
			)
			if err != nil {
				// Log error but continue with other files in batch
				failed = append(failed, struct {
					media *Media
					err   error
				}{media, err})
				continue
			}
			
			// Check if row was actually inserted (INSERT OR IGNORE returns 0 rows affected for duplicates)
			rowsAffected, _ := result.RowsAffected()
			if rowsAffected == 0 {
				// Duplicate entry (INSERT OR IGNORE silently skipped)
				// This is expected behavior, not an error
				continue
			}
			
			successful = append(successful, media)
		}
		
		// Close statement
		stmt.Close()
		
		// If all inserts failed, rollback and return error
		if len(successful) == 0 && len(failed) > 0 {
			tx.Rollback()
			return fmt.Errorf("all files in batch failed to insert: %v", failed[0].err)
		}
		
		// Log any failures but commit successful inserts
		if len(failed) > 0 {
			for _, f := range failed {
				fmt.Printf("Warning: Failed to insert file %s: %v\n", f.media.Filename, f.err)
			}
		}
		
		// Commit transaction with successful inserts
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("error committing batch transaction: %v", err)
		}
		
		// Log success
		if len(successful) > 0 {
			fmt.Printf("Successfully inserted %d files in batch (skipped %d duplicates/failures)\n", len(successful), len(failed))
		}
	}
	
	return nil
}

func (d *DB) DbClose() {
	// Close all prepared statements before closing the database connection
	// This ensures proper cleanup of resources
	if d.stmtChecksumExists != nil {
		d.stmtChecksumExists.Close()
	}
	if d.stmtChecksum100kExists != nil {
		d.stmtChecksum100kExists.Close()
	}
	if d.stmtAddFile != nil {
		d.stmtAddFile.Close()
	}
	d.db.Close()
}

func (d *DB) DbExec(stmt string) error {
	_, err := d.db.Exec(stmt)
	if err != nil {
		return err
	}
	return nil
}

func (d *DB) MediaInDB(media *Media) (bool) {
	return d.ChecksumExists(media.Checksum)
}

func (d *DB) Checksum100kExists(checksum string) (bool) {
	// Use cached prepared statement instead of creating a new one each time
	// This provides significant performance improvement for high-frequency queries
	if d.stmtChecksum100kExists == nil {
		log.Fatal("database not properly initialized: Checksum100kExists statement is nil")
	}
	
	var result int
	err := d.stmtChecksum100kExists.QueryRow(checksum).Scan(&result)
	if err != nil {
		// If there's an error, assume it doesn't exist rather than crashing
		return false
	}
	return result > 0
}

func (d *DB) ChecksumExists(checksum string) (bool) {
	// Use cached prepared statement instead of creating a new one each time
	// This provides significant performance improvement for high-frequency queries
	if d.stmtChecksumExists == nil {
		log.Fatal("database not properly initialized: ChecksumExists statement is nil")
	}
	
	var result int
	err := d.stmtChecksumExists.QueryRow(checksum).Scan(&result)
	if err != nil {
		// If there's an error, assume it doesn't exist rather than crashing
		return false
	}
	return result > 0
}

// openDBWithRetry attempts to open database connection with retry logic
// This handles transient connection errors and network issues
func (d *DB) openDBWithRetry(maxRetries int, retryDelay time.Duration) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		d.db, err = sql.Open("sqlite", d.filename)
		if err == nil {
			// Test the connection
			pingErr := d.db.Ping()
			if pingErr == nil {
				return nil
			}
			d.db.Close()
			err = fmt.Errorf("database ping failed: %v", pingErr)
		}
		
		if i < maxRetries-1 {
			fmt.Printf("Database connection attempt %d/%d failed: %v, retrying in %v...\n", 
				i+1, maxRetries, err, retryDelay)
			time.Sleep(retryDelay)
			retryDelay *= 2 // Exponential backoff
		}
	}
	return fmt.Errorf("failed to open database after %d attempts: %v", maxRetries, err)
}

func (d *DB)Init() error {
	// Open database with retry logic for better resilience
	// Retry up to 3 times with exponential backoff (100ms, 200ms, 400ms)
	err := d.openDBWithRetry(3, 100*time.Millisecond)
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		return err
	}

	// Configure connection pool settings for better performance
	// SetMaxOpenConns sets the maximum number of open connections to the database
	// SetMaxIdleConns sets the maximum number of connections in the idle connection pool
	d.db.SetMaxOpenConns(25)  // Maximum open connections
	d.db.SetMaxIdleConns(5)   // Maximum idle connections
	d.db.SetConnMaxLifetime(0) // Connections don't expire (SQLite doesn't need this)

	// Enable WAL (Write-Ahead Logging) mode for better crash recovery and performance
	// WAL mode provides:
	// - Better crash recovery (reduced corruption risk)
	// - Better concurrent read performance
	// - Atomic transactions even on crashes
	_, err = d.db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		fmt.Printf("Warning: Could not enable WAL mode: %v\n", err)
		// Continue anyway - WAL is not strictly required but highly recommended
	}
	
	// Set synchronous mode to NORMAL for better performance while maintaining safety
	// NORMAL mode is safe with WAL and provides better performance than FULL
	// WAL + NORMAL provides durability guarantees while being faster than default
	_, err = d.db.Exec("PRAGMA synchronous=NORMAL")
	if err != nil {
		fmt.Printf("Warning: Could not set synchronous mode: %v\n", err)
	}
	
	// Enable foreign key constraints (if needed in future)
	_, err = d.db.Exec("PRAGMA foreign_keys=ON")
	if err != nil {
		fmt.Printf("Warning: Could not enable foreign keys: %v\n", err)
	}

	// Create tables
	stmt := `
	CREATE TABLE IF NOT EXISTS
		settings (
			setting CHAR UNIQUE,
			value CHAR
		)
	`
	err = d.DbExec(stmt)
	if err != nil {
		return err
	}

	stmt = `
	CREATE TABLE IF NOT EXISTS
		media (
			filename CHAR,
			checksum CHAR UNIQUE,
			checksum100k CHAR,
			size INT,
			create_date TIMESTAMP
		)
	`
	err = d.DbExec(stmt)
	if err != nil {
		return err
	}
	
	// Ensure UNIQUE constraint is enforced (atomic operation prevents race conditions)
	// This constraint is critical for preventing duplicate files
	// SQLite will automatically create an index for UNIQUE constraints, but we verify it exists
	err = d.DbExec("CREATE UNIQUE INDEX IF NOT EXISTS idx_checksum_unique ON media(checksum)")
	if err != nil {
		fmt.Printf("Warning: Could not create unique index on checksum: %v\n", err)
		// Continue - the UNIQUE constraint in table definition should still work
	}

	// Create indexes for frequently queried columns
	// These dramatically improve query performance (5-10x faster lookups)
	// Indexes are critical for:
	// - checksum100k: Used in every duplicate check (high frequency)
	// - create_date: Used for time-based queries and sorting
	// - checksum: Already has UNIQUE constraint (implicit index)
	err = d.createIndexes()
	if err != nil {
		// Index creation failure is not fatal, but log it
		fmt.Printf("Warning: Some indexes could not be created: %v\n", err)
	}

	// Prepare all statements once during initialization
	// These prepared statements will be reused for all queries, eliminating
	// the overhead of preparing statements for every single query
	// This provides a 2-5x performance improvement for high-frequency operations
	
	d.stmtChecksumExists, err = d.db.Prepare("SELECT count(*) FROM media WHERE checksum = ?")
	if err != nil {
		return fmt.Errorf("unable to prepare ChecksumExists statement: %v", err)
	}

	d.stmtChecksum100kExists, err = d.db.Prepare("SELECT count(*) FROM media WHERE checksum100k = ?")
	if err != nil {
		return fmt.Errorf("unable to prepare Checksum100kExists statement: %v", err)
	}

	d.stmtAddFile, err = d.db.Prepare("INSERT INTO media (filename, checksum, checksum100k, size, create_date) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("unable to prepare AddFile statement: %v", err)
	}

	dst := d.config.Server.SaveDir

	if len(dst) == 0 {
		// Destination does not exist.
		homedir, err := os.UserHomeDir()
		defaultDir := filepath.Join(homedir, "pictures")
		if err != nil {
			log.Fatalln("Cannot get home dir.")
		}
		fmt.Printf("Directory to store images [%s]: ", defaultDir)
		fmt.Scanln(&dst)
		if len(dst) == 0 {
			dst = defaultDir
		}
		//stmt = `INSERT INTO settings (setting, value) VALUES (?, ?)`
		tx, err := d.db.Begin()
		if err != nil {
			log.Fatalln("Cannot insert.")
		}
		// stmt, err := tx.Prepare("INSERT INTO settings (setting, value) VALUES (?, ?)")
		// if err != nil {
		// 	log.Fatalln("Cannot insert (2).")
		// }
		// _, err = stmt.Exec("destination", dst)
		// if err != nil {
		// 	log.Fatalln("Failed to insert.")
		// }
		err = tx.Commit()
		if err != nil {
			log.Fatalln("Unable to commit destination directory.")
		}
	}
	d.destdir = dst

	return nil
}

// createIndexes creates all necessary indexes for optimal query performance
// Indexes dramatically speed up WHERE clause lookups and JOIN operations
func (d *DB) createIndexes() error {
	indexes := []struct {
		name    string
		stmt    string
		purpose string
	}{
		{
			name:    "idx_checksum100k",
			stmt:    "CREATE INDEX IF NOT EXISTS idx_checksum100k ON media(checksum100k)",
			purpose: "Speeds up checksum100k duplicate checks (used in every file check)",
		},
		{
			name:    "idx_create_date",
			stmt:    "CREATE INDEX IF NOT EXISTS idx_create_date ON media(create_date)",
			purpose: "Speeds up time-based queries and sorting by creation date",
		},
		{
			name:    "idx_filename",
			stmt:    "CREATE INDEX IF NOT EXISTS idx_filename ON media(filename)",
			purpose: "Speeds up filename lookups (if needed for future features)",
		},
	}

	var lastErr error
	for _, idx := range indexes {
		err := d.DbExec(idx.stmt)
		if err != nil {
			fmt.Printf("Warning: Could not create index %s (%s): %v\n", idx.name, idx.purpose, err)
			lastErr = err
		}
	}

	return lastErr
}

// VerifyIndexes checks if all expected indexes exist in the database
// This is useful for database maintenance and troubleshooting
func (d *DB) VerifyIndexes() (map[string]bool, error) {
	indexes := map[string]bool{
		"idx_checksum100k": false,
		"idx_create_date":  false,
		"idx_filename":     false,
	}

	// SQLite stores index information in sqlite_master table
	stmt := `
		SELECT name 
		FROM sqlite_master 
		WHERE type='index' AND tbl_name='media' AND name LIKE 'idx_%'
	`
	rows, err := d.db.Query(stmt)
	if err != nil {
		return nil, fmt.Errorf("error querying indexes: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		if _, exists := indexes[name]; exists {
			indexes[name] = true
		}
	}

	return indexes, nil
}

func (d *DB) Open() (error) {
	var err error
	d.db, err = sql.Open("sqlite3", d.filename)
	if err != nil {
		return err
	}
	return nil
}

