package sortengine

import (
	"github.com/barasher/go-exiftool"
	"sync"
	"fmt"
)

var lock = &sync.Mutex{}
type Exiftool struct {
	et *exiftool.Exiftool
}

var et *Exiftool

func GetExiftool() *Exiftool {
	// This is our singleton
	if et == nil {
		lock.Lock()
		defer lock.Unlock()

		et = &Exiftool{}
		var err error
		et.et, err = exiftool.NewExiftool()
		if err != nil {
			fmt.Printf("Error creating exiftool: %s\n", err)
			panic(err)
		} else {
			fmt.Println("Exiftool created.")
		}
	}
	return et
}

func (e *Exiftool) ReadMetadata(filename string) map[string]string {
	output := make(map[string]string)
	metadata := e.et.ExtractMetadata(filename)
	for _, fileInfo := range metadata {
		for k, v := range fileInfo.Fields {
			output[k] = fmt.Sprintf("%v", v)
		}
	}
	return output
}
