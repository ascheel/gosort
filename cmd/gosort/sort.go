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
	"github.com/ascheel/gosort/internal/media"
	"crypto/sha256"
	"crypto/md5"
	"hash"
    "golang.org/x/text/language"
    "golang.org/x/text/message"
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
	sort.report = make(map[string][]string)
	sort.report["image"] = make([]string, 0)
	sort.report["video"] = make([]string, 0)
	sort.report["duplicate"] = make([]string, 0)
	sort.report["unsorted"] = make([]string, 0)
	sort.count = 0
	return sort
}

type Sort struct {
	dbFilename string
	db *sql.DB
	destdir string
	report map[string][]string
	count uint64
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
			filename CHAR,
			checksum CHAR UNIQUE,
			checksum100k CHAR,
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

func (s *Sort) AddFileToDB(m *media.Media) error {
	stmt := `
	INSERT INTO
		media (
			filename,
			checksum,
			checksum100k,
			size,
			create_date
		) VALUES (?, ?, ?, ?, ?)
	`
	// fmt.Println(stmt)
	// fmt.Printf("%s - %s - %s - %d", m.Filename, m.FilenameNew, m.Sha256sum, m.Size)
	m.SetChecksum()
	_, err := s.db.Exec(stmt, m.FilenameNew, m.Checksum, m.Checksum100k, m.Size, m.CreationDate)
	if err != nil {
		return err
	}

	return nil
}

func (s *Sort) FileIsInDB(m *media.Media) (bool) {
	var err error
	var stmt *sql.Stmt
	stmt, err = s.db.Prepare("SELECT checksum FROM media WHERE checksum100k = ? ")
	if err != nil {
		return false
	}
	defer stmt.Close()

	rows, err := stmt.Query(m.Checksum100k)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	// Iterate
	for rows.Next() {
		var checksum string
		err := rows.Scan(&checksum)
		if err != nil {
			panic(err)
		}
		m.SetChecksum()

		// We have at least one...  so lets check the checksum.
		if checksum == m.Checksum {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	return false
}

func (s *Sort) GetNewFilename(m *media.Media) (string) {
	// fmt.Printf("  Getting new filename: %s\n", m.Filename)
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
	// fmt.Printf("    Dirname: %s\n", dirname)
	for {
		// fmt.Printf("      Creation date: %v\n", m.CreationDate)
		shortname := m.CreationDate.Format(TimeFormat)
		if num > 0 {
			shortname = fmt.Sprintf("%s.%d", shortname, num)
		}
		shortname = fmt.Sprintf("%s.%s", shortname, m.Ext())
		filename := filepath.Join(dirname, shortname)

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

func (s *Sort) ProcessFile(m *media.Media) (string, error) {
	//m.Print()
	p := message.NewPrinter(language.AmericanEnglish)
	s.count += 1
	p.Printf("%10d: %s... ", s.count, m.Filename)
	if ! m.IsRecognized() {
		s.report["unsorted"] = append(s.report["unsorted"], m.Filename)
		fmt.Printf("\n")
		return "", errors.New("is not a picture or video")
	} else if m.IsImage() {
		s.report["image"] = append(s.report["image"], m.Filename)
	} else if m.IsVideo() {
		s.report["video"] = append(s.report["video"], m.Filename)
	} else {
		panic("You shouldn't hit this.")
	}

	if s.FileIsInDB(m) {
		s.report["duplicate"] = append(s.report["duplicate"], m.Filename)
		fmt.Println("  Exists.")
		return "", nil
	}
	fmt.Printf("\n")
	
	m.FilenameNew = s.GetNewFilename(m)
	err := s.AddFileToDB(m)
	if err != nil {
		fmt.Printf("    Unable to insert into database.\n")
		fmt.Println(err)
		return "", err
	}
	dirname := filepath.Dir(m.FilenameNew)
	if ! FileOrDirExists(dirname) {
		os.MkdirAll(dirname, 0755)
	}
	err = copyFile(m.Filename, m.FilenameNew)
	if err != nil {
		return "", err
	}
	err = os.Chtimes(m.FilenameNew, m.ModifiedDate, m.ModifiedDate)
	if err != nil {
		return "", err
	}
	return m.FilenameNew, nil
}

func copyFile(src string, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
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
		mediaFile := media.NewMediaFile(absPath)
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

func (s *Sort) Report() {
	for k, v := range s.report {
		fmt.Printf("\n%s:\n", k)
		var count uint64 = 0
		for _, item := range v {
			count += 1
			fmt.Printf("%10d: %s\n", count, item)
		}
	}
}

func checksum100k(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Set the checksum function
	h := md5.New()

	// Get the file's checksum
	var BUFSIZE int64 = 102400
	_, err = io.CopyN(h, f, BUFSIZE)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func checksum(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Set the checksum function
	ChecksumFunctions := map[string]func() hash.Hash {
		"sha256": sha256.New,
		"md5":    md5.New,
	}
	checksumFormat := "sha256"
	h := ChecksumFunctions[checksumFormat]()

	// Get the file's checksum
	_, err = io.Copy(h, f)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

