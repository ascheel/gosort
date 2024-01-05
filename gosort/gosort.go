package main

import (
	"flag"
	"github.com/ascheel/gosort/media"
)

var (
	scanDir string
)

func init() {
	flag.StringVar(&scanDir, "dir", ".", "Directory to scan images.")
	flag.Parse()
}

func main() {
	// imageFile := "omelette.jpg"
	// image := NewMediaFile(imageFile)
	
	// //image.GetMetadata()
	// image.Print()
	// os.Exit(0)
	sort := NewSort("./gosort.db")
	sort.Sort(scanDir)
}
