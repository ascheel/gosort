package sortengine

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"fmt"
	"io"
	"crypto/sha256"
	"crypto/md5"
	"hash"
)

func FileOrDirExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	} else {
		return true
	}
}

func NewEngine() *Engine {
	configPath, err := GetDefaultConfigPath()
	if err != nil {
		log.Fatalf("Unable to get default config path: %v", err)
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Unable to load config: %v", err)
	}
	return NewEngineWithConfig(config)
}

func NewEngineWithConfig(config *Config) *Engine {
	engine := &Engine{}
	engine.Config = config
	//engine.DbInit()
	engine.dbFilename          = engine.Config.Server.DBFile
	engine.DB                  = NewDB(engine.dbFilename, engine.Config)
	engine.report              = make(map[string][]string)
	engine.report["image"]     = make([]string, 0)
	engine.report["video"]     = make([]string, 0)
	engine.report["duplicate"] = make([]string, 0)
	engine.report["unsorted"]  = make([]string, 0)
	engine.count               = 0
	return engine
}

type Engine struct {
	dbFilename string
	DB *DB
	report map[string][]string
	count uint64
	Config *Config
}

func (e *Engine) GetNewFilename(m *Media) (string) {
	// fmt.Printf("  Getting new filename: %s\n",
	dst := e.Config.Server.SaveDir

	TimeDirFormat := "2006-01"
	TimeFormat := "2006-01-02 15.04.05"
	num := 0

	dirname := filepath.Join(dst, m.CreationDate.Format(TimeDirFormat))
	
	// Ensure directory exists
	if err := os.MkdirAll(dirname, 0755); err != nil {
		panic(fmt.Sprintf("Cannot create directory %s: %v", dirname, err))
	}
	
	for {
		shortname := m.CreationDate.Format(TimeFormat)
		if num > 0 {
			shortname = fmt.Sprintf("%s.%d", shortname, num)
		}
		shortname = fmt.Sprintf("%s.%s", shortname, m.Ext())
		filename := filepath.Join(dirname, shortname)
		
		// CRITICAL: Validate path to prevent path traversal attacks
		// Ensure the generated path is within the save directory
		absFilename, err := filepath.Abs(filename)
		if err != nil {
			panic(fmt.Sprintf("Cannot get absolute path for %s: %v", filename, err))
		}
		absSaveDir, err := filepath.Abs(dst)
		if err != nil {
			panic(fmt.Sprintf("Cannot get absolute path for save directory %s: %v", dst, err))
		}
		if !strings.HasPrefix(absFilename, absSaveDir) {
			panic(fmt.Sprintf("Path traversal detected: %s is outside save directory %s", absFilename, absSaveDir))
		}

		if FileOrDirExists(filename) {
			sum, err := checksum(filename)
			if err != nil {
				panic(err)
			}
			if m.Checksum == sum {
				panic("Shouldn't be able to hit this.  Existing checksum should have been found in the DB.")
			}
			num += 1
			continue
		} else {
			return filename
		}
	}
}

// func (e *Engine) ProcessFile(m *Media) (string, error) {
// 	//m.Print()
// 	p := message.NewPrinter(language.AmericanEnglish)
// 	e.count += 1
// 	p.Printf("%10d: %s... ", e.count, m.Filename)
// 	if ! m.IsRecognized() {
// 		e.report["unsorted"] = append(e.report["unsorted"], m.Filename)
// 		fmt.Printf("\n")
// 		return "", errors.New("is not a picture or video")
// 	} else if m.IsImage() {
// 		e.report["image"] = append(e.report["image"], m.Filename)
// 	} else if m.IsVideo() {
// 		e.report["video"] = append(e.report["video"], m.Filename)
// 	} else {
// 		panic("You shouldn't hit this.")
// 	}

// 	if e.FileIsInDB(m) {
// 		e.report["duplicate"] = append(e.report["duplicate"], m.Filename)
// 		fmt.Println("  Exists.")
// 		return "", nil
// 	}
// 	fmt.Printf("\n")
	
// 	m.Filename = e.GetNewFilename(m)
// 	err := e.AddFileToDB(m)
// 	if err != nil {
// 		fmt.Printf("    Unable to insert into database.\n")
// 		fmt.Println(err)
// 		return "", err
// 	}
// 	dirname := filepath.Dir(m.Filename)
// 	if ! FileOrDirExists(dirname) {
// 		os.MkdirAll(dirname, 0755)
// 	}
// 	fixthisshit
// 	err = copyFile(m.Filename, m.Filename)
// 	if err != nil {
// 		return "", err
// 	}
// 	err = os.Chtimes(m.Filename, m.ModifiedDate, m.ModifiedDate)
// 	if err != nil {
// 		return "", err
// 	}
// 	return m.Filename, nil
// }

// func copyFile(src string, dst string) error {
// 	srcFile, err := os.Open(src)
// 	if err != nil {
// 		return err
// 	}
// 	defer srcFile.Close()

// 	dstFile, err := os.Create(dst)
// 	if err != nil {
// 		return err
// 	}
// 	defer dstFile.Close()

// 	_, err = io.Copy(dstFile, srcFile)
// 	return err
// }

// func (e *Engine) visit(path string, info os.FileInfo, err error) error {
// 	if err != nil {
// 		fmt.Println(err)
// 		return nil
// 	}
// 	if ! info.IsDir() {
// 		absPath, err := filepath.Abs(path)
// 		if err != nil {
// 			panic(err)
// 		}
// 		mediaFile := NewMediaFile(absPath)
// 		e.ProcessFile(mediaFile)
// 		//fmt.Printf("%s is a file.  Abs: %s\n", path, absPath)
// 	}
// 	return nil
// }

// func (e *Engine) Sort(root string) error {
// 	//count := 0
// 	err := filepath.Walk(root, e.visit)
// 	if err != nil {
// 		fmt.Printf("Error walking path %v: %v\n", root, err)
// 		return err
// 	}
// 	return nil
// }

func (e *Engine) Report() {
	for k, v := range e.report {
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

