package sortengine

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"fmt"
	"log"
	"os"
	"path/filepath"
	// m "github.com/ascheel/gosort/internal/media"
)

type DB struct {
	// SQLite3 database
	filename string
	config *Config
	db *sql.DB
	destdir string
}

func NewDB(filename string, config *Config) *DB {
	db := &DB{}
	db.filename = filename
	db.config = config
	db.Init()
	return db
}

func (d *DB) AddFileToDB(media *Media) error {
	stmt := `INSERT INTO media (filename, checksum, checksum100k, size, create_date) VALUES (?, ?, ?, ?, ?)`
	if len(media.Checksum) == 0 {
		media.SetChecksum()
	}
	_, err := d.db.Exec(
		stmt,
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

func (d *DB) DbClose() {
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
	stmt, err := d.db.Prepare("SELECT count(*) FROM media WHERE checksum100k = ?")
	if err != nil {
		log.Fatal("Unable to prepare checksum100k statement.")
	}
	defer stmt.Close()
	var result int
	stmt.QueryRow(checksum).Scan(&result)
	if result == 0 {
		return false
	} else {
		return true
	}
}

func (d *DB) ChecksumExists(checksum string) (bool) {
	stmt, err := d.db.Prepare("SELECT count(*) FROM media WHERE checksum = ?")
	if err != nil {
		log.Fatal("Unable to prepare checksum statement.")
	}
	defer stmt.Close()
	var result int
	stmt.QueryRow(checksum).Scan(&result)
	if result == 0 {
		return false
	} else {
		return true
	}
}

func (d *DB)Init() error {
	var err error
	d.db, err = sql.Open("sqlite", d.filename)
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		return err
	}

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

func (d *DB) Open() (error) {
	var err error
	d.db, err = sql.Open("sqlite3", d.filename)
	if err != nil {
		return err
	}
	return nil
}

