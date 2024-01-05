package main

import (
	"time"
	"fmt"
	"os"
	"github.com/kolesa-team/goexiv"
	"log"
	"image"
	_ "image/jpeg"
	_ "image/png"
	_ "image/gif"
)

var TimeFormat string = "%Y:%m:%d %H:%M:%S"

type Media struct {
	Filename string
	Sha256sum string
	Size int64
	ModifiedDate time.Time
	CreationDate time.Time
	Width int
	Height int
	Metadata map[string]string
}

type Mediaer interface {
	Init()
	GetNewFilename()
	GetStatData()
	GetMetadata()
	GetBounds()
	Print()
}

func NewMediaFile(filename string) *Media {
	mediaInstance := &Media{
		Filename: filename,
	}
	mediaInstance.Init()
	return mediaInstance
}

func (m *Media) GetBounds () (int, int, error) {
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
	fileInfo, err := os.Stat(m.Filename)
	if err != nil {
		return err
	}

	m.Size = fileInfo.Size()
	m.ModifiedDate = fileInfo.ModTime()
	m.Sha256sum, err = sha256sum(m.Filename)
	if err != nil {
		return err
	}
	m.Width, m.Height, err = m.GetBounds()
	m.CreationDate = m.GetDate()
	m.GetMetadata()
	
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
			theDate2, err := time.Parse(time.RFC3339, theDate)
			if err != nil {
				panic(err)
			}
			return theDate2
		}
	}
	// No exif data?  Then return the Modified Date.
	// Linux timestamps do not store creation time.
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
