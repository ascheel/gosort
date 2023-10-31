package main

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"os"
	"path/filepath"
	"fmt"
)

type Sort struct {
	dbFilename string
	db *sql.DB
	destdir string
}

type Sorter interface {
	dbInit()
	dbExec()
	dbClose()
	getDestination()
}

func (s *Sort) getSetting(setting string) (string, error) {
	var err error
	var dst string
	var stmt *sql.Stmt
	stmt, err = s.db.Prepare("SELECT value FROM settings WHERE setting = ?")
	if err != nil {
		return "", err
	}
	defer stmt.Close()
	stmt.QueryRow(setting).Scan(&dst)
	return dst, nil
}

func (s *Sort) getDestination() (string, error) {
	dst, err := s.getSetting("destination")
	return dst, err
}

func (s *Sort) Close () (error) {
	var err error
	s.db.Close()
	if err != nil {
		return err
	}
	return nil
}

func (s *Sort) Exec(stmt string) error {
	_, err := s.db.Exec(stmt)
	if err != nil {
		return err
	}
	return nil
}

func (s *Sort) Init() (error) {
	var err error
	s.db, err = sql.Open("sqlite3", s.dbFilename)
	if err != nil {
		return err
	}

	stmt := `
	CREATE TABLE IF NOT EXISTS
		settings (
			setting CHAR UNIQUE,
			value CHAR
		)
	`
	err = s.Exec(stmt)
	if err != nil {
		return err
	}

	stmt = `
	CREATE TABLE IF NOT EXISTS
		media (
			filename_original CHAR,
			filename_new CHAR UNIQUE,
			sha256sum CHAR UNIQUE,
			size INT,
			create_date TIMESTAMP
		)
	`
	err = s.Exec(stmt)
	if err != nil {
		return err
	}

	var dst string
	dst, err = s.getDestination()
	if err != nil {
		log.Fatal("Unable to get Destination.")
	}
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
		tx, err := s.db.Begin()
		if err != nil {
			log.Fatalln("Cannot insert.")
		}
		stmt, err := tx.Prepare("INSERT INTO settings (setting, value) VALUES (?, ?)")
		if err != nil {
			log.Fatalln("Cannot insert (2).")
		}
		_, err = stmt.Exec("destination", dst)
		if err != nil {
			log.Fatalln("Failed to insert.")
		}
		err = tx.Commit()
		if err != nil {
			log.Fatalln("Unable to commit destination directory.")
		}
	}
	s.destdir = dst

	return nil
}
