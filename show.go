package main

import (
	"net/url"
	"net/http"
	"fmt"
	"io"
	"io/ioutil"
	"encoding/xml"
	"github.com/kennygrant/sanitize"
	"path/filepath"
)


// Show is the main type. It holds information about the podcast and its episodes.
type Show struct {
	URL       *url.URL
	Title      string  `xml:"channel>title"`
	Episodes []Episode `xml:"channel>item"`
}


func (s *Show) Sync(mainDir string) int {
	resp, err := http.Get(s.URL.String())
	if err != nil {
		fmt.Println("Invalid RSS feed:", err)
		return 0
	}
	defer resp.Body.Close()

	if err := s.ReadFrom(resp.Body); err != nil {
		fmt.Println("Error reading RSS feed:", err)
		return 0
	}

	showDir := filepath.Join(mainDir, s.Title)
	if err := ValidateDir(showDir); err != nil {
		fmt.Println("Invalid show directory:", err)
		return 0
	}

	// TODO: figure out which episodes need to be downloaded.

	for i, v := range episodes {
		if err := v.Download(showDir); err != nil {
			fmt.Println("Error downloading episode:", err)
			return i
		}
	}

	return len(episodes)
}

// ReadFrom reads and parses feed data from the provided io.Reader.
func (s *Show) ReadFrom(r io.Reader) error {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	if err := xml.Unmarshal(data, s); err != nil {
		return err
	}

	s.Title = sanitize.BaseName(s.Title)

	return nil
}
