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
	"os"
)


// Show is the main type. It holds information about the podcast and its episodes.
type Show struct {
	URL       *url.URL
	Path       string
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

	s.Path = filepath.Join(mainDir, s.Title)
	if err := ValidateDir(s.Path); err != nil {
		fmt.Println("Invalid show directory:", err)
		return 0
	}

	if err := s.Filter(); err != nil {
		fmt.Println("Error selecting episodes:", err)
		return 0
	}

	if len(s.Episodes) == 0 {
		fmt.Println("No new episodes")
		return 0
	}

	for i, v := range s.Episodes {
		if err := v.Download(s.Path); err != nil {
			fmt.Println("Error downloading episode:", err)
			return i
		}
	}

	return len(s.Episodes)
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

	for i, v := range s.Episodes {
		num := GuessNum(v.Title)
		if num < 0 && v.Number > 0 {
			// Prepend episode number if we have one but can't find it in the title.
			v.Title = fmt.Sprintf("%v-%v", v.Number, v.Title)
		} else if num > 0 && v.Number == 0 {
			// Save episode number if we have one in the title but not in the episode notes.
			s.Episodes[i].Number = num
		}

		// Add the file extension.
		s.Episodes[i].Title = v.Title + v.Ext
	}

	return nil
}

// Filter chooses which episodes need to be downloaded and discards the rest.
func (s *Show) Filter() error {
	have := make(map[string]string)
	latest := 0

	walkFunc := func(path string, info os.FileInfo, err error) error {
		filename := info.Name()
		if err != nil {
			return err
		} else if !isAudio(filename) {
			return nil
		}

		have[filename] = filename
		if num := GuessNum(filename); num > latest {
			latest = num
		}

		return nil
	}
	if err := filepath.Walk(s.Path, walkFunc); err != nil {
		return err
	}

	want := []Episode{}
	if latest > 0 {
		for _, v := range s.Episodes {
			if v.Number > latest {
				want = append(want, v)
			}
		}
	} else {
		// We weren't able to determine the latest episode. We'll grab everything we don't already have.
		for _, v := range s.Episodes {
			if _, ok := have[v.Title]; !ok {
				want = append(want, v)
			}
		}
	}

	s.Episodes = want

	return nil
}


// isAudio determines if the provided file is an audio file or not.
func isAudio(filename string) bool {
	switch filepath.Ext(filename) {
	case ".aac":
		return true
	case ".midi":
		return true
	case ".mp3":
		return true
	case ".oga":
		return true
	case ".opus":
		return true
	case ".wav":
		return true
	case ".weba":
		return true
	}

	return false
}
