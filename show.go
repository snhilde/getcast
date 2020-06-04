package main

import (
	"net/url"
	"net/http"
	"fmt"
	"io"
	"io/ioutil"
	"encoding/xml"
	"path/filepath"
	"os"
	"strconv"
)


// Show is the main type. It holds information about the podcast and its episodes.
type Show struct {
	URL       *url.URL
	Dir        string  // show's directory on disk
	Title      string  `xml:"channel>title"`
	Author     string  `xml:"channel>author"`
	Image      string  `xml:"channel>image"`
	Episodes []Episode `xml:"channel>item"`
}


// Sync gets the current list of available episodes, determines which of them need to be downloaded, and then gets them.
func (s *Show) Sync(mainDir string) int {
	resp, err := http.Get(s.URL.String())
	if err != nil {
		fmt.Println("Invalid RSS feed:", err)
		return 0
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading RSS feed:", err)
		return 0
	}

	if err := xml.Unmarshal(data, s); err != nil {
		fmt.Println("Error reading RSS feed:", err)
		return 0
	}

	s.Title = Sanitize(s.Title)
	if s.Title == "" {
		fmt.Println("Error parsing RSS feed: No show title found")
		return 0
	}

	for i, e := range s.Episodes {
		e.SetShowTitle(s.Title)
		e.SetShowArtist(s.Author)
		e.Title = Sanitize(e.Title) + mimeToExt(e.Type)
	}

	s.Dir = filepath.Join(mainDir, s.Title)
	if err := ValidateDir(s.Dir); err != nil {
		fmt.Println("Invalid show directory:", err)
		return 0
	}

	if err := s.filter(); err != nil {
		fmt.Println("Error selecting episodes:", err)
		return 0
	}

	if len(s.Episodes) == 0 {
		fmt.Println("No new episodes")
		return 0
	}

	for i, e := range s.Episodes {
		e.SetShowTitle(s.Title)
		e.SetShowArtist(s.Author)
		if err := e.Download(s.Dir); err != nil {
			fmt.Println("Error downloading episode:", err)
			return i
		}
	}

	return len(s.Episodes)
}

// filter filters out the episodes we don't want to download.
func (s *Show) filter() error {
	have := make(map[string]int)
	latestSeason := 0
	latestEpisode := 0

	walkFunc := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		filename := info.Name()
		if !isAudio(filename) {
			return nil
		}

		file, err := os.Open(filepath.Join(path, filename))
		if err != nil {
			return err
		}
		defer file.Close()

		meta := NewMeta(nil)
		if _, err := io.Copy(meta, file); err != nil && err != io.ErrShortWrite {
			return err
		}

		season := 0
		if value := meta.GetField("TPOS"); value != "" {
			if num, err := strconv.Atoi(season); err == nil {
				season = num
				if season > latestSeason {
					latestSeason = season
				}
			}
		}

		episode := 0
		if value := meta.GetField("TRCK"); value != "" {
			if num, err := strconv.Atoi(episode); err == nil {
				episode = num
				if episode > latestEpisode && season == latestSeason {
					latestEpisode = episode
				}
			}
		}

		have[filename] = episode

		return nil
	}
	if err := filepath.Walk(s.Dir, walkFunc); err != nil {
		return err
	}

	want := []Episode{}
	if latestEpisode > 0 {
		for _, episode := range s.Episodes {
			if episode.Season == latestSeason && episode.Number > latestEpisode {
				want = append(want, episode)
			}
		}
	} else {
		// We weren't able to determine the latest episode. We'll grab everything we don't already have.
		for _, episode := range s.Episodes {
			if _, ok := have[episode.Title]; !ok {
				want = append(want, episode)
			}
		}
	}

	// Feed will list episodes newest to oldest. We'll reverse that here to make error handling easier later on.
	length := len(want)
	for i := 0; i < length/2; i++ {
		want[i], want[length - 1 - i] = want[length - 1 - i], want[i]
	}

	s.Episodes = want

	return nil
}


// mimeToExt finds the appropriate file extension based on the MIME type.
func mimeToExt(mime string) string {
	switch mime {
	case "audio/aac":
		return ".aac"
	case "audio/midi", "audio/x-midi":
		return ".midi"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/ogg":
		return ".oga"
	case "audio/opus":
		return ".opus"
	case "audio/wav":
		return ".wav"
	case "audio/webm":
		return ".weba"
	}

	// If we can't match a specific type, we'll default to mp3.
	return ".mp3"
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
