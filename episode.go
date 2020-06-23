package main

import (
	"io"
	"fmt"
	"os"
	"net/http"
	"strconv"
	"time"
	"strings"
	"path/filepath"
	"net/url"
	"io/ioutil"
)


// Episode represents internal data related to each episode of the podcast.
type Episode  struct {
	// Show information
	showTitle   string
	showArtist  string
	showImage   string

	// Episode information
	Title       string    `xml:"title"`
	Season      string    `xml:"season"`
	Number      string    `xml:"episode"`
	Image       string    `xml:"image,href"`
	Desc        string    `xml:"description"`
	Date        string    `xml:"pubDate"`
	Enclosure   struct {
		URL         string    `xml:"url,attr"`
		Size        string    `xml:"length,attr"`
		Type        string    `xml:"type,attr"`
	} `xml:"enclosure"`

	// Objects to handle reading/writing
	meta       *Meta      // Metadata object
	w           io.Writer // Writer that will handle writing the file.
}


// Download downloads the episode. The bytes will stream through this path from web to disk:
// Internet -> http object -> Episode object -> Disk
//             \-> Progress object   \-> Meta object
func (e *Episode) Download(showDir string) error {
	if showDir == "" {
		return fmt.Errorf("Missing download directory")
	}

	if err := e.validateData(); err != nil {
		return err
	}

	filename := e.buildFilename(showDir)
	Debug("Saving episode to", filename)

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	resp, err := http.Get(e.Enclosure.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("%v", resp.Status)
	}

	size, err := strconv.Atoi(e.Enclosure.Size)
	if err == nil && int(resp.ContentLength) != size {
		fmt.Println("Warning: RSS feed is reporting episode size different than currently exists")
		Debug("RSS feed size: ", size, "bytes")
		Debug("Available size:", resp.ContentLength, "bytes")
	}

	bar := Progress{total: int(resp.ContentLength), totalString: Reduce(int(resp.ContentLength))}
	tee := io.TeeReader(resp.Body, &bar)

	// Connect the episode on both ends of the flow.
	e.meta = NewMeta(nil)
	e.w = file

	Debug("Beginning download process")
	_, err = io.Copy(e, tee)
	if err != nil {
		return err
	}

	return bar.Finish()
}

// Write first constructs and then writes the episode's metadata and then passes all remaining data on to the next layer.
func (e *Episode) Write(p []byte) (int, error) {
	if e == nil {
		return 0, fmt.Errorf("Invalid episode object")
	} else if e.w == nil {
		return 0, fmt.Errorf("Invalid writer")
	}

	consumed := 0
	if !e.meta.Buffered() {
		// Continue buffering metadata.
		if n, err := e.meta.Write(p); err != nil && err != io.EOF {
			// Either more data is needed or there was an error writing the metadata.
			return n, err
		} else {
			// All metadata has been written. The rest of the bytes are filedata.
			consumed = n
		}

		// Now that we have all of the metadata, let's build it with the additional data from the episode and write
		// everything to disk.
		e.addFrames()
		metadata := e.meta.Build()
		if n, err := e.w.Write(metadata); err != nil {
			return consumed, err
		} else if n != len(metadata) {
			return consumed, fmt.Errorf("Failed to write complete metadata")
		}
	}

	// If we're here, then all metadata has been successfully written. We can resume with writing the file data now.
	n, err := e.w.Write(p[consumed:])
	return consumed + n, err
}

// SetShowTitle sets the title of the episode's show.
func (e *Episode) SetShowTitle(title string) {
	if e != nil {
		e.showTitle = title
	}
}

// SetShowArtist sets the artist of the episode's show.
func (e *Episode) SetShowArtist(artist string) {
	if e != nil {
		e.showArtist = artist
	}
}

// SetShowImage sets the image link of the episode's show. If no image is found for the episode, it will default to the
// value set here.
func (e *Episode) SetShowImage(image string) {
	if e != nil {
		e.showImage = image
	}
}


// addFrames fleshes out the metadata with information from the episode. If a frame already exists in the metadata, it
// will not be overwritten with data from the RSS feed. The only exceptions to this rule are the show and episode
// titles, which must match the data from the RSS feed to sync properly.
func (e *Episode) addFrames() {
	Debug("Building metadata frames")

	frames := []struct {
		id    string
		value string
	}{
		// Show information
		{ "TPE1", e.showArtist    }, // Artist
		{ "TPE2", e.showArtist    }, // Album Artist

		// Episode information
		{ "TRCK", e.Number        },
		{ "TDES", e.Desc          },
		{ "TPOS", e.Season        },
		{ "WOAF", e.Enclosure.URL },

		// Defaults
		{ "TCON", "Podcast"       },
		{ "PCST", "1"             },
	}

	// Set these frames from the table above.
	for _, frame := range frames {
		if values := e.meta.GetValues(frame.id); values == nil || len(values) == 0 {
			e.meta.SetValue(frame.id, []byte(frame.value), false)
		}
	}

	// We always want the show and episode titles to match the contents of the RSS feed.
	e.meta.SetValue("TALB", []byte(e.showTitle), false)
	e.meta.SetValue("TIT2", []byte(e.Title), false)

	// We have to manually add the date and format it according the ID3v2 version we're using.
	if e.Date != "" {
		if time, err := time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", e.Date); err == nil {
			switch e.meta.Version() {
			case 3:
				if values := e.meta.GetValues("TYER"); values == nil || len(values) == 0 {
					e.meta.SetValue("TYER", []byte(time.Format("2006")), false) // YYYY
				}
				if values := e.meta.GetValues("TDAT"); values == nil || len(values) == 0 {
					e.meta.SetValue("TDAT", []byte(time.Format("0201")), false) // DDMM
				}
				if values := e.meta.GetValues("TIME"); values == nil || len(values) == 0 {
					e.meta.SetValue("TIME", []byte(time.Format("1504")), false) // HHMM
				}
			case 4:
				if values := e.meta.GetValues("TDRC"); values == nil || len(values) == 0 {
					e.meta.SetValue("TDRC", []byte(time.Format("20060102T150405")), false) // YYYYMMDDTHHMMSS
				}
			}
		}
	}

	// If the episode has an image, we'll add that. Otherwise, we'll try to get the default image of the show.
	image := e.downloadImage()
	if values := e.meta.GetValues("APIC"); values == nil || len(values) == 0 {
		e.meta.SetValue("APIC", image, false)
	}
}

// validateData checks that we have all of the required fields from the RSS feed.
func (e *Episode) validateData() error {
	if e == nil {
		return fmt.Errorf("Cannot validata data: Bad episode object")
	}

	Debug("Validating episode title:", e.Title)
	if e.Title == "" {
		return fmt.Errorf("Missing episode title")
	}

	e.Title = SanitizeTitle(e.Title)
	ext := mimeToExt(e.Enclosure.Type)
	if !strings.HasSuffix(e.Title, ext) {
		e.Title += ext
	}

	Debug("Validating episode link:", e.Enclosure.URL)
	if e.Enclosure.URL == "" {
		return fmt.Errorf("Missing download link for %v", e.Title)
	}

	Debug("Validating episode number:", e.Number)
	if e.Number == "" {
		Debug("No episode number found for", e.Title)
	}

	return nil
}

// buildFilename pieces together the different components of the episode into one absolute-path filename.
func (e *Episode) buildFilename(path string) string {
	// Let's first check if the title contains the episode number. If it doesn't, then we want to add it so as to
	// improve the filesystem sorting.
	if e.Number != "" && !strings.Contains(e.Title, e.Number) {
		e.Title = e.Number + " - " + e.Title
	}

	return filepath.Join(path, e.Title)
}

// downloadImage downloads either the episode (preferred) or show (fallback) image. If no link exists or there's any
// trouble downloading the image, this return nil.
func (e *Episode) downloadImage() []byte {
	if e == nil {
		return nil
	}
	Debug("Downloading image")

	var u *url.URL
	var err error
	if e.Image != "" {
		u, err = url.Parse(e.Image)
	} else if e.showImage != "" {
		u, err = url.Parse(e.showImage)
	} else {
		Debug("No episode or show image to download")
		return nil
	}

	if u == nil || err != nil {
		Debug("Error parsing episode/show image link")
		return nil
	}

	resp, err := http.Get(u.String())
	if err != nil {
		Debug("Error getting image information:", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		Debug("Error accessing image:", resp.StatusCode)
		return nil
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		Debug("Error retrieving image:", err)
		return nil
	}

	return data
}


// mimeToExt finds the appropriate file extension based on the MIME type.
func mimeToExt(mime string) string {
	var ext string
	switch mime {
	case "audio/aac":
		ext = ".aac"
	case "audio/midi", "audio/x-midi":
		ext = ".midi"
	case "audio/mpeg", "audio/mp3":
		ext = ".mp3"
	case "audio/ogg":
		ext = ".oga"
	case "audio/opus":
		ext = ".opus"
	case "audio/wav":
		ext = ".wav"
	case "audio/webm":
		ext = ".weba"
	default:
		// If we can't match a specific type, we'll default to mp3.
		ext = ".mp3"
	}

	Debug("Mapping MIME type", mime, "to extension", ext)
	return ext
}
