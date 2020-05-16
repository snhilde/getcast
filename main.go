package main

import (
	"flag"
	"os"
	"syscall"
	"fmt"
	"path/filepath"
	"io/ioutil"
	"net/http"
	"encoding/xml"
)


// Show is the main type. It holds information about the podcast and all of the available episodes.
type Show struct {
	Title      string  `xml:"channel>title"`
	Episodes []episode `xml:"channel>item"`
}

// episode represents internal data related to each episode of the podcast.
type episode struct {
	Title  string `xml:"title"`
	Number string `xml:"episode"` // full namespace: itunes:episode
	Link   string `xml:"link"`
}


func main() {
	url := flag.String("u", "", "URL of the podcast's main Libsyn page")
	dir := flag.String("d", "", "Optional, main getcast download directory")
	eps := flag.Args()

	flag.Parse()

	// Validate (or create) the download directory.
	if *dir != "" {
		if err := validateDir(*dir); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	// Let's see what data we have for this podcast.
	show, err := getShowInfo(*url)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// validateDir validates the provided download directory in these ways:
// - path is an existing directory (if not, we'll create it)
// - path points to Podcasts directory
// - directory has read permissions
// - directory has write permissions
func validateDir(path string) error {
	// Make sure the path is valid.
	info, err := os.Stat(path)
	if err != nil {
		// We'll assume the error is because the directory does not exist. We'll try to create it here and let other
		// possible errors flow from that.
		return os.MkdirAll(path, 0755)
	}

	// Make sure the path is a directory, and is specifically named Podcasts.
	if !info.IsDir() {
		return fmt.Errorf("%v is not a directory", path)
	} else if filepath.Base(path) != "Podcasts" {
		return fmt.Errorf("Specified directory must be Podcasts")
	}

	// Make sure we have read and write permissions to the directory. This is more of an early sanity check to get a
	// better idea of what could be wrong and not an actual perms check. We won't fail here if anything goes wrong
	// getting the permissions values, but we will fail if the perms don't match.
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		// Check if we match the directory's user or group.
		isUser := os.Getuid() == int(stat.Uid)
		isGroup := os.Getgid() == int(stat.Gid)

		// Find out which of the directory's user, group, and other read bits are set.
		perms := info.Mode().Perm() & os.ModePerm
		uRead := perms & (1 << 8) > 0
		gRead := perms & (1 << 5) > 0
		oRead := perms & (1 << 2) > 0

		// Check for read permission.
		if !(isUser && uRead) && !(isGroup && gRead) && !oRead {
			return fmt.Errorf("Cannot read %v", path)
		}

		// Find out which of the directory's user, group, and other write bits are set.
		uWrite := perms & (1 << 7) > 0
		gWrite := perms & (1 << 4) > 0
		oWrite := perms & (1 << 1) > 0

		// Check for write permission.
		if !(isUser && uWrite) && !(isGroup && gWrite) && !oWrite {
			return fmt.Errorf("Cannot write to %v", path)
		}
	}

	return nil
}

// getShowInfo will parse the metadata and episode data for the show specified.
func getShowInfo(url string) (Show, error) {
	// Grab the show's RSS feed.
	resp, err := http.Get(url + "/rss")
	if err != nil {
		return Show{}, fmt.Errorf("Invalid show homepage: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return Show{}, fmt.Errorf("Failed to read data: %v", err)
	}

	// Parse the RSS feed into an XML doc.
	show := Show{}
	if err := xml.Unmarshal(body, &show); err != nil {
		return Show{}, fmt.Errorf("Error parsing xml: %v", err)
	}

	return show, nil
}
