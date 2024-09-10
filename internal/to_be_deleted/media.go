package main

import (
	"time"
	"fmt"
	"os"
	"github.com/kolesa-team/goexiv"
	"log"
)

var TimeFormat string = "%Y:%m:%d %H:%M:%S"

type Media struct {
	filename_original string
	filename_new string
	sha256sum string
	size int
	create_date time.Time
}

type Mediaer interface {
	Init()
	GetNewFilename()
	GetExif()
}

func (m *Media) GetNewFilename() (string, error) {
	return "", nil
}

func (m *Media) Init () error {
	var err error
	// Set sha256sum
	m.sha256sum, err = sha256sum(m.filename_original)
	if err != nil {
		fmt.Printf("Could not get checksum for file: %s\n", m.filename_original)
		os.Exit(1)
	}

	fmt.Println(m.GetExif())

	// Get date/time
	return nil
}

func (m *Media) GetExif() map[string]string {
	var metadata map[string]string

	img, err := goexiv.Open(m.filename_original)
	if err != nil {
		log.Fatal(err)
	}

	err = img.ReadMetadata()
	if err != nil {
		log.Fatal(err)
	}

	exif := img.GetExifData().AllTags()
	fmt.Println(exif)

	for key, value := range exif {
		fmt.Printf("%40s : %s\n", key,)
	}

	return metadata
}
