package sortengine

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	//"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/barasher/go-exiftool"
	//"github.com/kolesa-team/goexiv"
)

var TimeFormat string = "%Y:%m:%d %H:%M:%S"

var ImageExtensions []string = []string{"jpg", "jpeg", "png", "gif", "tif", "tiff", "bmp"}
var VideoExtensions []string = []string{"mpg", "mp4", "mkv", "avi", "mkv", "m4v", "mpeg", "mpeg4"}

type Media struct {
	Path         string
	Filename     string
	Checksum     string
	Checksum100k string
	Size         int64
	ModifiedDate time.Time
	CreationDate time.Time
	Width        int
	Height       int
	Metadata     map[string]string
}

func (m *Media) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"filename":      m.Filename,
		"path":          m.Path,
		"checksum":      m.Checksum,
		"checksum100k":  m.Checksum100k,
		"size":          m.Size,
		"modified_time": m.ModifiedDate.Format("2006-01-02 15:04:05"),
		"creation_time": m.CreationDate.Format("2006-01-02 15:04:05"),
		"metadata":      m.Metadata,
	}
}

func (m *Media) ToJSON() string {
	j, err := json.Marshal(m.ToMap())
	if err != nil {
		fmt.Printf("Unable to Marshal JSON: %v\n", err)
		os.Exit(1)
	}
	return string(j)
	//return fmt.Sprintf(`{"filename":"%s","path":"%s","checksum":"%s","checksum100k":"%s","size":%d,"modified_time":"%s","creation_time":"%s","metadata":%v}`,
	//	m.Filename, m.Path, m.Checksum, m.Checksum100k, m.Size, m.ModifiedDate.Format("2006-01-02 15:04:05"), m.CreationDate.Format("2006-01-02 15:04:05"), m.Metadata)
}

func (m *Media) Ext() string {
	ext := filepath.Ext(m.Filename)
	if len(ext) < 2 {
		return ""
	} else {
		return ext[1:]
	}
}

func (m *Media) SetChecksum() error {
	cs, err := Checksum(m.Filename)
	if err != nil {
		fmt.Printf("Unable to get checksum: %s\v", err)
		return err
	}
	m.Checksum = cs
	return nil
}

func MediaFromJSON(j string) *Media {
	var m map[string]interface{}
	err := json.Unmarshal([]byte(j), &m)
	if err != nil {
		fmt.Printf("Unable to unmarshal JSON: %v\n", err)
		os.Exit(1)
	}
	mediaInstance := &Media{
		Filename:     m["filename"].(string),
		Path:         m["path"].(string),
		Checksum:     m["checksum"].(string),
		Checksum100k: m["checksum100k"].(string),
		Size:         int64(m["size"].(float64)),
		ModifiedDate: m["modified_time"].(time.Time),
		CreationDate: m["creation_time"].(time.Time),
		Metadata:     m["metadata"].(map[string]string),
	}
	return mediaInstance
}

// func MediaFromMap(m map[string]interface{}) *Media {
// 	mediaInstance :=  &Media{
// 		Filename:     m["filename"].(string),
// 		Path:         m["path"].(string),
// 		Checksum:     m["checksum"].(string),
// 		Checksum100k: m["checksum100k"].(string),
// 		Size:         int64(m["size"].(float64)),
// 		ModifiedDate: m["modified_time"].(time.Time),
// 		CreationDate: m["creation_time"].(time.Time),
// 		Metadata:     m["metadata"].(map[string]string),
// 	}
// 	return mediaInstance
// }

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

func (m *Media) IsImage() bool {
	return MatchesExtensions(m.Filename, ImageExtensions)
}

func (m *Media) IsVideo() bool {
	return MatchesExtensions(m.Filename, VideoExtensions)
}

func (m *Media) IsRecognized() bool {
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

	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	return width, height, nil
}

func (m *Media) Init() error {
	metadata, err := m.GetMetadata()
	if err != nil {
		return err
	}
	m.Metadata = metadata
	fileInfo, err := os.Stat(m.Filename)
	if err != nil {
		return err
	}
	if !m.IsRecognized() {
		return errors.New("file is neither picture or video")
	}
	m.Size = fileInfo.Size()
	m.ModifiedDate = fileInfo.ModTime()

	// Don't need to calculate it unless we're going to insert or check if it exists.  I hope.
	// m.Checksum, err = checksum(m.FilenameOld)

	m.Checksum100k, err = Checksum(m.Filename, true)
	if err != nil {
		return err
	}
	// m.Width, m.Height, err = m.GetBounds()
	// if err != nil {
	// 	m.Width = -1
	// 	m.Height = -1
	// }
	m.CreationDate, err = m.GetDate()
	if err != nil {
		return err
	}
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

func (m *Media) GetDate() (time.Time, error) {
	// First, attempt to get the date from EXIF data.
	// If that doesn't work...
	var fields []string
	if m.IsImage() {
		fields = []string{
			// "Exif.Photo.DateTimeDigitized",
			// "Exif.Photo.DateTimeOriginal",
			// "Exif.Image.DateTime",
			"DateTimeDigitized",
			"DateTimeOriginal",
			"DateTime",
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
				return m.ModifiedDate, err
			}
			return theDate2, err
		}
	}
	// No exif data?  Then return the Modified Date.
	// Linux timestamps do not store creation time.
	fmt.Printf("  Found no others.  Returning file mod date.  %s\n", m.ModifiedDate.Format("2006-01-02 15.04.05"))
	return m.ModifiedDate, nil
}

func (m *Media) GetMetadata() (map[string]string, error) {
	if m.IsImage() {
		return m.GetImageMetadata()
	} else if m.IsVideo() {
		return m.GetVideoMetadata()
	} else {
		// Should probably handle this a little more gracefully.
		//fmt.Printf("Unsupported file type: %s\n", m.Filename)
		return make(map[string]string), errors.New("unsupported filetype")
	}
}

func (m *Media) GetImageMetadata() (map[string]string, error) {
	et := GetExiftool()
	return et.ReadMetadata(m.Filename), nil
}

func (m *Media) GetFileMetadata() (map[string]string, error) {
	metadata := make(map[string]string)
	fileInfo, err := os.Stat(m.Filename)
	if err != nil {
		return make(map[string]string), nil
	}
	metadata["File.Size"] = strconv.FormatInt(fileInfo.Size(), 10)
	metadata["File.ModifiedDate"] = fileInfo.ModTime().Format("2006-01-02 16.04.05")
	metadata["File.MD5Sum"], err = Checksum(m.Filename)
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

func Checksum(filename string, short ...bool) (string, error) {
	var hundredk bool = false
	if len(short) > 0 {
		hundredk = short[0]
	}

	// Open file
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Set the checksum function
	h := md5.New()

	// Get the file's checksum
	var BUFSIZE int64 = 102400
	if hundredk {
		_, err = io.CopyN(h, f, BUFSIZE)
		if err != nil {
			return "", err
		}
	} else {
		_, err = io.Copy(h, f)
		if err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
