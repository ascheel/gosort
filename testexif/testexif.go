package main

import (
	"fmt"
	"slices"
	"github.com/barasher/go-exiftool"
)

func SortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0)
	for k, _ := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func main() {
	et, err := exiftool.NewExiftool()
	if err != nil {
		fmt.Printf("Error creating exiftool: %s\n", err)
		panic(err)
	}
	filename := "/home/scheel/pics/2015/20150802_222506.jpg"

	metadata := et.ExtractMetadata(filename)
	keys := SortedKeys(metadata[0].Fields)
	for _, fileInfo := range metadata {
		for _, k := range keys {
			fmt.Printf("%-25s: %v\n", k, fileInfo.Fields[k])
		}
	}
}
