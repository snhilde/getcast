package getcast

import (
	"fmt"
	"github.com/kennygrant/sanitize"
	"os"
	"path/filepath"
	"syscall"
	"regexp"
	"strconv"
	"net/http"
	"io"
	"strings"
	"math"
)


type Podcast interface {
	Build() error             // Build fetches and parses data about the show and its episodes.
	Title() string            // Title returns the title of the show.
	Available() int           // Available returns the number of episodes available for download.
	TitleOf(index int) string // TitleOf returns the title of the episode at the provided index.
	NumberOf(index int) int   // NumberOf returns the episode number of the episode at the provided index.
	LinkOf(index int) string  // LinkOf returns the download URL for the episode at the provided index.
}

// episode represents internal data related to each episode of the podcast.
type episode struct {
	number int    // Episode number
	title  string // Title of the episode. If the standard title does not include an episode number, the module should
	              // add one, preferably as a prefix.
	link   string // Link used to download the episode
}


// Sync checks for and downloads new episodes. The returned number is the number of episodes actually downloaded.
func Sync(path string, shows ...Podcast) (int, error) {
	// If no directory was specified, we'll assume Podcasts in the current directory.
	if path == "" {
		path = "./Podcasts"
	}

	total := 0
	for i, show := range shows {
		if show == nil {
			return total, fmt.Errorf("Missing show %v", i)
		}

		// Ask the chosen module to collect data for the show and its episodes.
		if err := show.Build(); err != nil {
			return total, err
		}

		// Get show's title, and sanitize it so that it can safely be used in a filename.
		title := sanitize.BaseName(show.Title())
		titleSan := sanitize.BaseName(title)
		if titleSan == "" {
			return total, fmt.Errorf("Missing show title")
		}

		// Validate (or create) the download directory.
		dir, err := validateDir(path, titleSan)
		if err != nil {
			return total, err
		}
		fmt.Println("Syncing", title, "episodes in", dir)

		// Build the list of available episodes.
		num := show.Available()
		available := make([]episode, num)
		for i := 0; i < num; i++ {
			epNumber := show.NumberOf(i)
			epTitle := sanitize.BaseName(show.TitleOf(i)) + ".mp3"
			epLink := show.LinkOf(i)
			available[i] = episode{number: epNumber, title: epTitle, link: epLink}
		}

		// Figure out which episodes we want to download.
		want, err := selectEps(available, dir)
		if err != nil {
			return total, err
		}
		if len(want) == 0 {
			fmt.Println("No new episodes available")
			return total, nil
		}

		// Download those episodes.
		got, err := downloadEps(want, dir)
		total += got
		if err != nil {
			return total, err
		}
	}

	return total, nil
}


// validateDir checks that these things are true about the provided download directory:
// - Path is an existing directory. If it isn't, we'll create it.
// - Directory is either the main directory or the show's directory.
// - Directory has read permissions
// - Directory has write permissions
func validateDir(path string, title string) (string, error) {
	// Make sure the path is valid.
	info, err := os.Stat(path)
	if err != nil {
		// We'll assume the error is because the directory does not exist. We'll try to create it here and let other
		// possible errors flow from that.
		if err := os.MkdirAll(path, 0755); err != nil {
			return "", err
		}
		info, _ = os.Stat(path)
	}

	// Make sure the path is a directory.
	if !info.IsDir() {
		return "", fmt.Errorf("%v is not a directory", filepath.Base(path))
	}

	// Let's see if we were provided the path to the show's directory.
	if filepath.Base(path) != title {
		// We were not. Let's see if the show's directory is a subdirectory.
		path = filepath.Join(path, title)
		info, err = os.Stat(path)
		if err != nil {
			// The show's directory does not exist. Let's create it and return to the caller.
			return path, os.Mkdir(path, 0755)
		}
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
			return "", fmt.Errorf("Cannot read %v", path)
		}

		// Find out which of the directory's user, group, and other write bits are set.
		uWrite := perms & (1 << 7) > 0
		gWrite := perms & (1 << 4) > 0
		oWrite := perms & (1 << 1) > 0

		// Check for write permission.
		if !(isUser && uWrite) && !(isGroup && gWrite) && !oWrite {
			return "", fmt.Errorf("Cannot write to %v", path)
		}
	}

	return path, nil
}

// selectEps builds a list of episodes that we want to download, either by determining which episodes are newer than
// what we already have or by determining what we don't have.
func selectEps(available []episode, dir string) ([]episode, error) {
	latestEp := -1
	have := make(map[string]int)

	// Find the latest episode we have.
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		} else if filepath.Ext(path) != ".mp3" {
			return nil
		}

		// Parse out the episode's number.
		filename := filepath.Base(path)
		number := findEpNum(filename)
		if number > latestEp {
			latestEp = number
		}
		have[filename] = number

		return nil
	}
	if err := filepath.Walk(dir, walkFn); err != nil {
		return nil, fmt.Errorf("Error in current episodes: %v", err)
	}

	// We now know what episodes we already have. Let's figure out which ones we need.
	need := []episode{}
	if latestEp >= 0 {
		// We know the number of the newest episode we currently have. Let's grab everything that is newer than it.
		for _, v := range available {
			if v.number > latestEp {
				need = append(need, v)
			}
		}
	} else {
		// We either don't have any episodes or can't determine which is the most recent by episode number prefix. We'll
		// compare what we have to what's available and download everything we don't already have.
		fmt.Println("Cannot determine latest episode already downloaded. Syncing all available episodes.")
		for _, v := range available {
			if _, ok := have[v.title]; !ok {
				// We don't have this episode yet.
				need = append(need, v)
			}
		}
	}

	return need, nil
}

// downloadEps downloads the provided episodes and returns how many were actually downloaded.
func downloadEps(want []episode, dir string) (int, error) {
	if len(want) == 0 || dir == "" {
		return 0, fmt.Errorf("Invalid call")
	}

	fmt.Println("Downloading", len(want), "episodes")

	for i, ep := range want {
		// Create a save point.
		filename := filepath.Join(dir, ep.title)
		fmt.Println(filename)

		file, err := os.Create(filename)
		if err != nil {
			return i, err
		}
		defer file.Close()

		// Grab the file's data.
		resp, err := http.Get(ep.link)
		if err != nil {
			return i, err
		}
		defer resp.Body.Close()

		// Make sure we accessed everything correctly.
		if resp.StatusCode != 200 {
			return i, fmt.Errorf("%v", resp.Status)
		}

		// Set up our progress bar.
		p := progress{total: int(resp.ContentLength), totalString: reduce(int(resp.ContentLength))}
		t := io.TeeReader(resp.Body, &p)

		// Save the file.
		_, err = io.Copy(file, t)
		if err != nil {
			return i, err
		}

		// Because we've been mucking around with carriage returns, we need to manually move down a row.
		fmt.Println()
	}

	return len(want), nil
}


// findEpNum parses out and return the episode's number, or returns -1 if not found. Currently, it will grab the first
// number it sees. Enhancing this would be a good area for future development.
var reNum = regexp.MustCompile("^[[:print:]]*?([[:digit:]]+)[[:print:]]*$")
func findEpNum(title string) int {
	matches := reNum.FindStringSubmatch(title)
	if len(matches) < 2 {
		return -1
	}

	// The first item will be the matching title, and the second will be the number found.
	number, err := strconv.Atoi(matches[1])
	if err != nil {
		return -1
	}

	return number
}


// progress is used to display a progress bar during the download operation.
type progress struct {
	total       int    // total number of bytes to be downloaded
	totalString string // size of file to be downloaded, ready for printing
	have        int    // number of bytes we currently have
	count       int    // running count of write operations, for determining if we should print or not
}

func (pr *progress) Write(p []byte) (int, error) {
	n := len(p)
	pr.have += n

	// We don't need to do expensive print operations that often.
	pr.count++
	if pr.count % 50 > 0 {
		return n, nil
	}

	// Clear the line.
	fmt.Printf("\r%s", strings.Repeat(" ", 50))

	// Print the current transfer status.
	fmt.Printf("\rReceived %v of %v total (%v%%)", reduce(pr.have), pr.totalString, ((pr.have * 100) / pr.total))

	return n, nil
}


// reduce will convert the number of bytes into its human-readable value (less than 1024) with SI unit suffix appended.
var units = []string{"B", "K", "M", "G"}
func reduce(n int) string {
	if n <= 0 {
		return "0B"
	}

	index := int(math.Log2(float64(n))) / 10
	n >>= (10 * index)

	return strconv.Itoa(n) + units[index]
}
