package main

import (
	"github.com/barasher/go-exiftool"
	"time"
	"fmt"
	"os"
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
	et, err := exiftool.NewExiftool()
	if err != nil {
		fmt.Printf("Error initializing: %v\n", err)
		os.Exit(1)
	}
	defer et.Close()

	fileInfos := et.ExtractMetadata(m.filename_original)
	for _, fileInfo := range fileInfos {
		if fileInfo.Err != nil {
			fmt.Printf("Error opening file %v: %v\n", fileInfo.File, fileInfo.Err)
			continue
		}

		for k, v := range fileInfo.Fields {
			fmt.Printf("[%v] %v\n", k, v)
			metadata[k] = v.GetString()
		}
	}
	return metadata
}
