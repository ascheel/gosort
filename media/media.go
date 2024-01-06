package media

import (
	"time"
	"fmt"
	"os"
	"github.com/kolesa-team/goexiv"
	"log"
	"image"
	"errors"
	"strings"
	_ "image/jpeg"
	_ "image/png"
	_ "image/gif"
	"path/filepath"
	"crypto/sha256"
	"crypto/md5"
	"io"
	"hash"
)

var TimeFormat string = "%Y:%m:%d %H:%M:%S"

var ImageExtensions []string = []string{"jpg", "jpeg", "png", "gif", "tif", "tiff", "bmp"}
var VideoExtensions []string = []string{"mpg", "mp4", "mkv", "avi", "mkv", "m4v"}

type Media struct {
	Filename     string
	FilenameNew  string
	Checksum    string
	Size         int64
	ModifiedDate time.Time
	CreationDate time.Time
	Width        int
	Height       int
	Metadata     map[string]string
}

type Mediaer interface {
	Init()
	GetStatData()
	GetMetadata()
	GetBounds()
	Print()
	IsImage()
	IsVideo()
	IsRecognized()
	Exists()
	Ext()
}

func (m *Media) Ext() string {
	ext := filepath.Ext(m.Filename)
	if len(ext) < 2 {
		return ""
	} else {
		return ext[1:]
	}
}

func NewMediaFile(filename string) *Media {
	fullPathName, err := filepath.Abs(filename)
	if err != nil {
		panic(err)
	}
	mediaInstance := &Media{
		Filename: fullPathName,
	}
	mediaInstance.Init()
	return mediaInstance
}

func (m *Media) Exists() bool {
	_, err := os.Stat(m.Filename)
	return !errors.Is(err, os.ErrNotExist)
}

func MatchesExtensions(filename string, exts []string) bool {
	_, err := os.Stat(filename)
	if errors.Is(err, os.ErrNotExist) {
		// File doesn't exist.  Can't be an image or video.
		return false
	}
	for _, ext := range exts {
		ext2 := filepath.Ext(filename)
		if len(ext2) == 0 {
			// No extension.  Automatic reject.
			return false
		}
		if strings.EqualFold(ext2[1:], ext) {
			return true
		}
	}
	return false
}

func (m *Media) IsImage() (bool) {
	return MatchesExtensions(m.Filename, ImageExtensions)
}

func (m *Media) IsVideo() (bool) {
	return MatchesExtensions(m.Filename, VideoExtensions)
}

func (m *Media) IsRecognized() (bool) {
	return m.IsImage() || m.IsVideo()
}

func (m *Media) GetBounds() (int, int, error) {
	reader, err := os.Open(m.Filename)
	if err != nil {
		return -1, -1, err
	}
	defer reader.Close()

	img, _, err := image.Decode(reader)
	if err != nil {
		return -1, -1, err
	}

	width  := img.Bounds().Dx()
	height := img.Bounds().Dy()
	return width, height, nil
}

func (m *Media) Init() (error) {
	m.GetMetadata()
	fileInfo, err := os.Stat(m.Filename)
	if err != nil {
		return err
	}
	if ! m.IsRecognized() {
		return errors.New("file is neither picture or video")
	}
	m.Size = fileInfo.Size()
	m.ModifiedDate = fileInfo.ModTime()
	m.Checksum, err = checksum(m.Filename)
	if err != nil {
		return err
	}
	m.Width, m.Height, err = m.GetBounds()
	if err != nil {
		return err
	}
	m.CreationDate = m.GetDate()
	
	return nil
}

func (m *Media) Print() {
	var w int = 0
	for k := range m.Metadata {
			if len(k) > w {
					w = len(k)
			}
	}
	for k, v := range m.Metadata {
			fmt.Printf("%-*s: %s\n", w, k, v)
	}
}

func (m *Media) GetDate() time.Time {
	// First, attempt to get the date from EXIF data.
	// If that doesn't work...
	fields := []string{
		"Exif.Photo.DateTimeDigitized",
		"Exif.Photo.DateTimeOriginal",
		"Exif.Image.DateTime",
	}
	for _, field := range fields {
		theDate, ok := m.Metadata[field]

		if ok {
			theDate2, err := time.Parse("2006:01:02 15:04:05", theDate)
			if err != nil {
				panic(err)
			}
			return theDate2
		}
	}
	// No exif data?  Then return the Modified Date.
	// Linux timestamps do not store creation time.
	fmt.Printf("  Found no others.  Returning file mod date.  %s\n", m.ModifiedDate.Format("2006-01-02 15.04.05"))
	return m.ModifiedDate
}

func (m *Media) GetMetadata() {
	img, err := goexiv.Open(m.Filename)
	if err != nil {
		log.Fatal(err)
	}

	err = img.ReadMetadata()
	if err != nil {
		log.Fatal(err)
	}

	m.Metadata = img.GetExifData().AllTags()
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

