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
	Image      string  `xml:"channel>image,href"`
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

	if len(s.Episodes) == 0 {
		if specificEp != "" {
			return 0, fmt.Errorf("Episode %v not found", specificEp)
		} else {
			return 0, fmt.Errorf("No new episodes")
		}
	}

	fmt.Println("Downloading", len(s.Episodes), "episodes")
	success := 0
	for i, episode := range s.Episodes {
		fmt.Println("\n--- Downloading", episode.Title, "---")
		// Try up to 3 times to download the episode properly.
		for j := 1; j <= 3; j++ {
			if err := episode.Download(s.Dir); err == errDownload {
				if j < 3 {
					fmt.Println("Download attempt", j, "of 3 failed, trying again")
				} else {
					return i, fmt.Errorf("ERROR: All 3 download attempts failed")
				}
			} else if err != nil {
				fmt.Println("Error downloading episode:", err)
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
		if _, err := io.Copy(meta, file); err != nil && err != io.EOF {
			return err
		}

		// We only want episodes from this show.
		if value := getFirstValue(meta, "TALB"); value != s.Title {
			Debug("Episode is from a different show:", value)
			return nil
		}

		season := 0
		if value := getFirstValue(meta, "TPOS"); value != "" {
			if num, err := strconv.Atoi(value); err == nil {
				season = num
				if season > latestSeason {
					latestSeason = season
				}
			}
		}

		episode := 0
		if value := getFirstValue(meta, "TRCK"); value != "" {
			if num, err := strconv.Atoi(value); err == nil {
				episode = num
				if season == latestSeason && episode > latestEpisode {
					latestEpisode = episode
				}
			}
		}

		have[filename] = episode

		return nil
	}

	want := []Episode{}
	if specificEp != "" {
		fmt.Println("Looking for specified episode")
		ep := findSpecific(s.Episodes, specificEp)
		if ep == (Episode{}) {
			want = nil
		} else {
			want = []Episode{ep}
		}
	} else {
		fmt.Println("Looking for most current episode already synced")
		if err := filepath.Walk(s.Dir, walkFunc); err != nil {
			return err
		}

		if latestEpisode > 0 {
			want = findNewer(s.Episodes, latestSeason, latestEpisode)
		} else {
			want = findUnsynced(s.Episodes, have)
		}

		// Feed will list episodes newest to oldest. We'll reverse that here to make error handling easier later on.
		length := len(want)
		for i := 0; i < length/2; i++ {
			want[i], want[length - 1 - i] = want[length - 1 - i], want[i]
		}
	}

	s.Episodes = want

	return nil
}


// findSpecific finds the specified episode among the episodes available for download. A season can also be specified by
// separating the season and episode numbers with a "-".
func findSpecific(episodes []Episode, specified string) Episode {
	if specified == "" {
		return Episode{}
	}

	specificSeason := 0
	specificEpisode := 0

	parts := strings.Split(specified, "-")
	switch len(parts) {
	case 1:
		// Only an episode was specified.
		if num, err := strconv.Atoi(parts[0]); err != nil {
			fmt.Println("Error parsing specified episode:", err)
			return Episode{}
		} else {
			specificEpisode = num
		}
	case 2:
		// An episode and a season were specified.
		if num, err := strconv.Atoi(parts[0]); err != nil {
			fmt.Println("Error parsing specified season:", err)
			return Episode{}
		} else {
			specificSeason = num
		}

		if num, err := strconv.Atoi(parts[1]); err != nil {
			fmt.Println("Error parsing specified episode:", err)
			return Episode{}
		} else {
			specificEpisode = num
		}
	default:
		fmt.Println("Error parsing specified episode/season")
		return Episode{}
	}

	for _, episode := range episodes {
		season, _ := strconv.Atoi(episode.Season)
		number, _ := strconv.Atoi(episode.Number)
		if season == specificSeason && number == specificEpisode {
			if specificSeason > 0 {
				fmt.Println("Found episode", specificEpisode, "of season", specificSeason)
			} else {
				fmt.Println("Found episode", specificEpisode)
			}
			return episode
		}
	}

	// If we're here, then we didn't find anything.
	return Episode{}
}

// findNewer sifts through the episodes available for download and finds any that are newer than the most recent one
// already downloaded.
func findNewer(episodes []Episode, latestSeason int, latestEpisode int) []Episode {
	newer := []Episode{}

	if latestSeason > 0 {
		fmt.Println("Latest episode found is episode", latestEpisode, "of season", latestSeason)
	} else {
		fmt.Println("Latest episode found is episode", latestEpisode)
	}

	for _, episode := range episodes {
		season, _ := strconv.Atoi(episode.Season)
		number, _ := strconv.Atoi(episode.Number)
		if season == latestSeason && number > latestEpisode {
			newer = append(newer, episode)
		}
	}

	return newer
}

// findUnsynced compares the list of episodes already downloaded to the list of episodes available for download and removes any
// matches. It returns the episodes available for download but not yet downloaded.
func findUnsynced(episodes []Episode, have map[string]int) []Episode {
	unsynced := []Episode{}

	fmt.Println("Could not determine latest episode, syncing everything")

	for _, episode := range episodes {
		if _, ok := have[episode.Title]; !ok {
			unsynced = append(unsynced, episode)
		}
	}

	return unsynced
}

// getFirstValue gets the first value for the given frame ID. This is a convenience function for dealing with frame IDs
// that should have only one occurrence.
func getFirstValue(meta *Meta, id string) string {
	values := meta.GetValues("TALB")
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
