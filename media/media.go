package media

import (
	"time"
	"fmt"
	"os"
	"github.com/barasher/go-exiftool"
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
	"strconv"
	"reflect"
	"sort"
	"runtime"
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
	GetImageMetdata()
	GetFileMetadata()
	GetVideoMetadata()
	GetBounds()
	GetDate()
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
	// Note that this is a disgusting drain on CPU resources
	//   because it has to decode the entire JPG file.  And
	//   WE'RE NOT EVEN USING THE WIDTH x HEIGHT!
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

func lineno() string {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		panic(ok)
	}
	return fmt.Sprintf("%s: %d", file, line)
}

func (m *Media) Init() (error) {
	metadata, err := m.GetMetadata()
	if err != nil {
		return err
	}
	m.Metadata = metadata
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
	// m.Width, m.Height, err = m.GetBounds()
	if err != nil {
		m.Width = -1
		m.Height = -1
	}
	m.CreationDate = m.GetDate()
	
	return nil
}

func (m *Media) Print() {
	width := 0
	keys := make([]string, 0, len(m.Metadata))
	for key := range m.Metadata {
		keys = append(keys, key)
		if len(key) > width {
			width = len(key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("%-*s: %s\n", width, key, m.Metadata[key])
	}
	fmt.Printf("\n\n")
	fmt.Printf("Creation Date: %v\n", m.CreationDate)
	fmt.Printf("Size:          %d\n", m.Size)
	fmt.Printf("Checksum:      %s\n", m.Checksum)
}

func (m *Media) GetDate() time.Time {
	// First, attempt to get the date from EXIF data.
	// If that doesn't work...
	var fields []string
	if m.IsImage() {
		fields = []string{
			"Exif.Photo.DateTimeDigitized",
			"Exif.Photo.DateTimeOriginal",
			"Exif.Image.DateTime",
		}
	} else if m.IsVideo() {
		// Currently supports .mp4 only
		fields = []string{
			"CreateDate",
			"MediaCreateDate",
			"TrackCreateDate",
			"ModifyDate",
			"MediaModifyDate",
			"TrackModifyDate",
		}
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

func (m *Media) GetMetadata() (map[string]string, error) {
	if m.IsImage() {
		return m.GetImageMetadata()
	} else if m.IsVideo() {
		return m.GetVideoMetadata()
	} else {
		// Should probably handle this a little more gracefully.
		panic("Unsupported file type.")
	}
}

func (m *Media) GetImageMetadata() (map[string]string, error) {
	img, err := goexiv.Open(m.Filename)
	if err != nil {
		log.Fatal(err)
	}

	err = img.ReadMetadata()
	if err != nil {
		log.Fatal(err)
	}

	return img.GetExifData().AllTags(), nil
}

func (m *Media) GetFileMetadata() (map[string]string, error) {
	metadata := make(map[string]string)
	fileInfo, err := os.Stat(m.Filename)
	if err != nil {
		return make(map[string]string), nil
	}
	metadata["File.Size"]           = strconv.FormatInt(fileInfo.Size(), 10)
	metadata["File.ModifiedDate"]   = fileInfo.ModTime().Format("2006-01-02 16.04.05")
	metadata["File.MD5Sum"], err    = checksum(m.Filename, "md5")
	if err != nil {
		return make(map[string]string), err
	}
	metadata["File.Sha256sum"], err = checksum(m.Filename, "sha256")
	if err != nil {
		return make(map[string]string), err
	}
	return metadata, nil
}

func (m *Media) GetVideoMetadata() (map[string]string, error) {
	et, err := exiftool.NewExiftool()
	if err != nil {
		panic(err)
	}
	defer et.Close()

	metadata := make(map[string]string)

	fileInfos := et.ExtractMetadata(m.Filename)
	for _, fileInfo := range fileInfos {
		if fileInfo.Err != nil {
			fmt.Printf("Error concerning %v: %v\n", fileInfo.File, fileInfo.Err)
			continue
		}
		for key, value := range fileInfo.Fields {
			// metadata[k] = v
			// fmt.Printf("[%v] %v\n", k, v)
			switch v := value.(type) {
			case string:
				metadata[key] = v
			case int:
				metadata[key] = strconv.Itoa(v)
			case float64:
				metadata[key] = strconv.FormatFloat(v, 'f', -1, 64)
			case bool:
				metadata[key] = strconv.FormatBool(v)
			default:
				metadata[key] = fmt.Sprintf("<Unsupported field of type %s>", reflect.TypeOf(v))
			}
		}
	}

	return metadata, nil
}

func checksum(filename string, checksumFormat ...string) (string, error) {
	// Check arguments
	var cf string
	if len(checksumFormat) == 0 {
		cf = "md5"
	} else if len(checksumFormat) == 1 {
		cf = checksumFormat[0]
	} else {
		panic("Too many formats supplied.")
	}

	// Open file
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
	h := ChecksumFunctions[cf]()

	// Get the file's checksum
	_, err = io.Copy(h, f)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

