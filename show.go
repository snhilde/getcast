package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Show is the main type. It holds information about the podcast and its episodes.
type Show struct {
	URL      *url.URL
	Dir      string    // show's directory on disk
	Title    string    `xml:"channel>title"`
	Author   string    `xml:"channel>author"`
	Image    string    `xml:"channel>image,href"`
	Episodes []Episode `xml:"channel>item"`
}

// Sync gets the current list of available episodes, determines which of them need to be downloaded, and then gets them.
func (s *Show) Sync(mainDir string, specificEp string) (int, error) {
	resp, err := http.Get(s.URL.String())
	if err != nil {
		return 0, fmt.Errorf("Error getting RSS feed: %v", err)
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

	// The feed will list episodes newest to oldest. We'll reverse that here to make error handling easier later on.
	length := len(s.Episodes)
	for i := 0; i < length/2; i++ {
		s.Episodes[i], s.Episodes[length-1-i] = s.Episodes[length-1-i], s.Episodes[i]
	}

	// Make sure we can create directories and files with the names that were parsed earlier from the RSS feed.
	s.Title = SanitizeTitle(s.Title)
	Debug("Setting show title to", s.Title)
	Debug("Setting show artist to", s.Author)
	for i := range s.Episodes {
		s.Episodes[i].SetShowTitle(s.Title)
		s.Episodes[i].SetShowArtist(s.Author)
		s.Episodes[i].SetShowImage(s.Image)
	}

	// Validate (or create) this show's directory.
	s.Dir = filepath.Join(mainDir, s.Title)
	if err := ValidateDir(s.Dir); err != nil {
		return 0, fmt.Errorf("Invalid show directory: %v", err)
	}

	// Choose which episodes we want to download.
	if err := s.filter(specificEp); err != nil {
		return 0, fmt.Errorf("Error selecting episodes: %v", err)
	}

	switch len(s.Episodes) {
	case 0:
		if specificEp != "" {
			return 0, fmt.Errorf("Episode %v not found", specificEp)
		}
		Log("No new episodes")
		return 0, nil
	case 1:
		Log("Downloading 1 episode")
	default:
		Log("Downloading", len(s.Episodes), "episodes")
	}

	success := 0
	for _, episode := range s.Episodes {
		message := fmt.Sprintf("\n--- Downloading %s", episode.Title)
		if episode.Season != "" && episode.Number != "" {
			message += fmt.Sprintf(" (%s-%s)", episode.Season, episode.Number)
		} else if episode.Number != "" {
			message += fmt.Sprintf(" (%s)", episode.Number)
		}
		message += " ---"
		Log(message)
		// Try up to 3 times to download the episode properly.
		for j := 1; j <= 3; j++ {
			if err := episode.Download(s.Dir); err == errDownload {
				if j < 3 {
					Log("Download attempt", j, "of 3 failed, trying again")
				} else {
					Log("ERROR: All 3 download attempts failed")
					break
				}
			} else if err != nil {
				Log("Error downloading episode:", err)
				if errors.Is(err, syscall.ENOSPC) {
					// If there's no space left for writing, then we'll stop the entire process.
					return success, errors.New("No space left on disk, stopping process")
				}
				break
			} else {
				success++
				break
			}
		}
	}

	return success, nil
}

// filter filters out the episodes we don't want to download.
func (s *Show) filter(specificEp string) error {
	have := make(map[string]bool)

	// We're going to use this function to inspect all the episodes we currently have in the show's directory.
	walkFunc := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		filename := info.Name()
		if strings.HasPrefix(filename, ".") {
			Debug("Skipping hidden file:", filename)
			return nil
		} else if !isAudio(filename) {
			Debug("Skipping non-audio file:", filename)
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Build the metadata object so we can inspect the tag contents.
		// (We're temporarily turning off Debug Mode so we don't spam print all the metadata frames. They'll still get
		// written to the log.)
		tmpDebug := DebugMode
		DebugMode = false
		meta := NewMeta(nil)
		if _, err := io.Copy(meta, file); err != nil && err != io.EOF {
			Debug("Stopping walk check early")
			return err
		}
		DebugMode = tmpDebug

		titleID := "TIT2"
		if meta.Version() == 2 {
			titleID = "TT2"
		}
		title := getFirstValue(meta, titleID)
		have[title] = true

		return nil
	}

	if specificEp != "" {
		Log("\nLooking for specified episode")
		if ep, found := findSpecific(s.Episodes, specificEp); found {
			s.Episodes = []Episode{ep}
		} else {
			s.Episodes = nil
		}
	} else {
		Log("Building list of unsynced episodes")
		// Get all the metadata titles of the episodes we already have.
		if err := filepath.Walk(s.Dir, walkFunc); err != nil {
			return err
		}

		// Compare that list to what's available to find the episodes we need to download.
		want := []Episode{}
		for _, episode := range s.Episodes {
			if _, ok := have[episode.Title]; !ok {
				Debug("Need", episode.Title)
				want = append(want, episode)
			}
		}

		s.Episodes = want
	}

	return nil
}

// findSpecific finds the specified episode among the episodes available for download. A season can also be specified by
// separating the season and episode numbers with a "-".
func findSpecific(episodes []Episode, specified string) (Episode, bool) {
	if specified == "" {
		return Episode{}, false
	}

	specificSeason := 0
	specificEpisode := 0

	parts := strings.Split(specified, "-")
	switch len(parts) {
	case 1:
		// Only an episode was specified.
		num, err := strconv.Atoi(parts[0])
		if err != nil {
			Log("Error parsing specified episode:", err)
			return Episode{}, false
		}
		specificEpisode = num
	case 2:
		// An episode and a season were specified.
		num, err := strconv.Atoi(parts[0])
		if err != nil {
			Log("Error parsing specified season:", err)
			return Episode{}, false
		}
		specificSeason = num

		num, err = strconv.Atoi(parts[1])
		if err != nil {
			Log("Error parsing specified episode:", err)
			return Episode{}, false
		}
		specificEpisode = num
	default:
		Log("Error parsing specified episode/season")
		return Episode{}, false
	}

	for _, episode := range episodes {
		season, _ := strconv.Atoi(episode.Season)
		number, _ := strconv.Atoi(episode.Number)
		if season == specificSeason && number == specificEpisode {
			if specificSeason > 0 {
				Log("Found episode", specificEpisode, "of season", specificSeason)
			} else {
				Log("Found episode", specificEpisode)
			}
			return episode, true
		}
	}

	// If we're here, then we didn't find anything.
	return Episode{}, false
}

// getFirstValue gets the first value for the given frame ID. This is a convenience function for dealing with frame IDs
// that should have only one occurrence.
func getFirstValue(meta *Meta, id string) string {
	values := meta.GetValues(id)
	if values == nil || len(values) == 0 {
		return ""
	}

	return string(values[0])
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
