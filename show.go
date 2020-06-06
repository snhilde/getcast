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
	"strings"
	"strconv"
)


// Show is the main type. It holds information about the podcast and its episodes.
type Show struct {
	URL       *url.URL
	Dir        string  // show's directory on disk
	Title      string  `xml:"channel>title"`
	Author     string  `xml:"channel>author"`
	Image      string  `xml:"channel>image"` // TODO: image/url?
	Episodes []Episode `xml:"channel>item"`
}


// Sync gets the current list of available episodes, determines which of them need to be downloaded, and then gets them.
func (s *Show) Sync(mainDir string) (int, error) {
	resp, err := http.Get(s.URL.String())
	if err != nil {
		return 0, fmt.Errorf("Invalid RSS feed: %v", err)
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("Error reading RSS feed: %v", err)
	}

	if err := xml.Unmarshal(data, s); err != nil {
		return 0, fmt.Errorf("Error reading RSS feed: %v", err)
	}
	if s.Title == "" {
		return 0, fmt.Errorf("Error parsing RSS feed: No show information found")
	} else if len(s.Episodes) == 0 {
		return 0, fmt.Errorf("Error parsing RSS feed: No episodes found")
	}

	// Make sure we can create directories and files with the names that were parsed earlier from the RSS feed.
	s.Title = SanitizeTitle(s.Title)
	Debug("Setting show title to", s.Title)
	Debug("Setting show artist to", s.Author)
	for i := range s.Episodes {
		s.Episodes[i].SetShowTitle(s.Title)
		s.Episodes[i].SetShowArtist(s.Author)
	}

	// Validate (or create) this show's directory.
	s.Dir = filepath.Join(mainDir, s.Title)
	if err := ValidateDir(s.Dir); err != nil {
		return 0, fmt.Errorf("Invalid show directory: %v", err)
	}

	// Choose which episodes we want to download.
	if err := s.filter(); err != nil {
		return 0, fmt.Errorf("Error selecting episodes: %v", err)
	}

	if len(s.Episodes) == 0 {
		return 0, fmt.Errorf("No new episodes")
	}

	Debug("Downloading", len(s.Episodes), "episodes")
	for i, episode := range s.Episodes {
		if err := episode.Download(s.Dir); err != nil {
			return i, fmt.Errorf("Error downloading episode:", err)
		}
	}

	return len(s.Episodes), nil
}

// filter filters out the episodes we don't want to download.
func (s *Show) filter() error {
	have := make(map[string]int)
	latestSeason := 0
	latestEpisode := 0

	// We're going to use this function to inspect all the episodes we currently have in the show's directory. We'll
	// compare every episode of this show to determine what the most recent episode is and then download only the
	// episodes that are newer than that.
	walkFunc := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		filename := info.Name()
		if strings.HasPrefix(filename, ".") {
			return nil
		} else if !isAudio(filename) {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Build the metadata object so we can inspect the tag contents.
		meta := NewMeta(nil)
		if _, err := io.Copy(meta, file); err != nil && err != io.ErrShortWrite {
			return err
		}

		// We only want episodes from this show.
		if value := meta.GetFrame("TALB"); string(value) != s.Title {
			Debug("Episode is from a different show:", string(value))
			return nil
		}

		season := 0
		if value := meta.GetFrame("TPOS"); value != nil {
			if num, err := strconv.Atoi(string(value)); err == nil {
				season = num
				if season > latestSeason {
					latestSeason = season
				}
			}
		}

		episode := 0
		if value := meta.GetFrame("TRCK"); value != nil {
			if num, err := strconv.Atoi(string(value)); err == nil {
				episode = num
				if season == latestSeason && episode > latestEpisode {
					latestEpisode = episode
				}
			}
		}

		have[filename] = episode

		return nil
	}
	Debug("Looking for most current episode already synced")
	if err := filepath.Walk(s.Dir, walkFunc); err != nil {
		return err
	}

	want := []Episode{}
	if latestEpisode > 0 {
		// Filter out any episodes that are older than the most recent one.
		if latestSeason > 0 {
			Debug("Latest episode found is episode", latestEpisode, "of season", latestSeason)
		} else {
			Debug("Latest episode found is episode", latestEpisode)
		}
		for _, episode := range s.Episodes {
			season, _ := strconv.Atoi(episode.Season)
			number, _ := strconv.Atoi(episode.Number)
			if season == latestSeason && number > latestEpisode {
				want = append(want, episode)
			}
		}
	} else {
		// We weren't able to determine the latest episode. We'll grab everything we don't already have.
		Debug("Could not determine latest episode, syncing everything")
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
