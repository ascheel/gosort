package main

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"os"
	"path/filepath"
	"fmt"
	"errors"
	"io"
	"github.com/ascheel/gosort/media"
)

func FileOrDirExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	} else {
		return true
	}
}

func NewSort(filename string) *Sort {
	sort := &Sort{dbFilename: filename}
	sort.DbInit()
	return sort
}

type Sort struct {
	dbFilename string
	db *sql.DB
	destdir string
}

type Sorter interface {
	DbInit()
	DbExec()
	DbClose()
	GetDestination()
	GetSetting()
	Sort()
	ProcessFile()
	FileIsInDB()
}

func (s *Sort) DbInit() (error) {
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
	err = s.DbExec(stmt)
	if err != nil {
		return err
	}

	stmt = `
	CREATE TABLE IF NOT EXISTS
		media (
			filename_original CHAR,
			filename_new CHAR UNIQUE,
			checksum CHAR UNIQUE,
			size INT,
			create_date TIMESTAMP
		)
	`
	err = s.DbExec(stmt)
	if err != nil {
		return err
	}

	var dst string
	dst, err = s.GetDestination()
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

func (s *Sort) DbExec(stmt string) error {
	_, err := s.db.Exec(stmt)
	if err != nil {
		return err
	}
	return nil
}

func (s *Sort) DbClose() (error) {
	var err error
	s.db.Close()
	if err != nil {
		return err
	}
	return nil
}

// Get destination directory from config file.
func (s *Sort) GetDestination() (string, error) {
	dst, err := s.GetSetting("destination")
	return dst, err
}

func (s *Sort) GetSetting(setting string) (string, error) {
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

func (s *Sort) AddImageToDB(m *Media) error {
	stmt := `
	INSERT INTO
		media (
			filename_original,
			filename_new,
			checksum,
			size,
			create_date
		) VALUES (?, ?, ?, ?, ?)
	`
	// fmt.Println(stmt)
	// fmt.Printf("%s - %s - %s - %d", m.Filename, m.FilenameNew, m.Sha256sum, m.Size)
	_, err := s.db.Exec(stmt, m.Filename, m.FilenameNew, m.Checksum, m.Size, m.CreationDate)
	if err != nil {
		return err
	}

	return nil
}

func (s *Sort) FileIsInDB(m *Media) (bool) {
	var err error
	var result uint8
	var stmt *sql.Stmt
	stmt, err = s.db.Prepare("SELECT count(*) FROM media WHERE checksum = ?")
	if err != nil {
		return false
	}
	defer stmt.Close()
	stmt.QueryRow(m.Checksum).Scan(&result)
	return result > 0
}

func (s *Sort) GetNewFilename(m *Media) (string) {
	dst, err := s.GetDestination()
	if err != nil {
		return ""
	}
	// _date = self.date
	// _datedir = _date.strftime("%Y-%m")
	// _newname = _date.strftime("%Y-%m-%d %H.%M.%S") + "."
	TimeDirFormat := "2006-01"
	TimeFormat := "2006-01-02 15.04.05"
	num := 0
	dirname := filepath.Join(dst, m.CreationDate.Format(TimeDirFormat))
	for {
		shortname := m.CreationDate.Format(TimeFormat)
		if num > 0 {
			shortname = fmt.Sprintf("%s.%d", shortname, num)
		}
		shortname = fmt.Sprintf("%s.%s", shortname, m.Ext())
		filename := filepath.Join(dirname, shortname)
		fmt.Printf("Checking filename: %s\n", filename)
		if FileOrDirExists(filename) {
			sum, err := checksum(filename)
			if err != nil {
				panic(err)
			}
			if m.Checksum == sum {
				panic("Shouldn't be able to hit this. Existing checksum should have been found in the DB.")
			}
			num += 1
			continue
		} else {
			return filename
		}
	}
}

func (s *Sort) ProcessFile(m *Media) (string, error) {
	fmt.Printf("Processing file %s\n", m.Filename)
	if s.FileIsInDB(m) {
		fmt.Printf("  File is already in database. Returning. (%s)\n", m.Checksum)
		return "", nil
	}
	if m.IsImage() {
		fmt.Printf("  File is an image.\n")
		m.FilenameNew = s.GetNewFilename(m)
		err := s.AddImageToDB(m)
		if err != nil {
			fmt.Printf("    Unable to insert into database.\n")
			fmt.Println(err)
			return "", err
		}
		fmt.Printf("  New filename: %s\n", m.FilenameNew)
		dirname := filepath.Dir(m.FilenameNew)
		if ! FileOrDirExists(dirname) {
			fmt.Printf("    Creating dir: %s\n", dirname)
			os.MkdirAll(dirname, 0755)
		}
		fmt.Printf("Copying file: %s -> %s\n", m.Filename, m.FilenameNew)
		copyFile(m.Filename, m.FilenameNew)
		// Now copy the modification time
		err = os.Chtimes(m.FilenameNew, m.ModifiedDate, m.ModifiedDate)
		if err != nil {
			panic(err)
		}
		return m.FilenameNew, nil
	} else if m.IsVideo() {
		fmt.Printf("  File is a video.  Not yet supported.\n")
		return "", errors.New("video not yet supported")
	} else {
		return "", errors.New("is not picture or video")
	}
}

func copyFile(src string, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		panic(err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		panic(err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func (s *Sort) visit(path string, info os.FileInfo, err error) error {
	if err != nil {
		fmt.Println(err)
		return nil
	}
	if ! info.IsDir() {
		absPath, err := filepath.Abs(path)
		if err != nil {
			panic(err)
		}
		mediaFile := NewMediaFile(absPath)
		s.ProcessFile(mediaFile)
		//fmt.Printf("%s is a file.  Abs: %s\n", path, absPath)
	}
	return nil
}

func (s *Sort) Sort(root string) error {
	//count := 0
	err := filepath.Walk(root, s.visit)
	if err != nil {
		fmt.Printf("Error walking path %v: %v\n", root, err)
	}
	return nil
}
