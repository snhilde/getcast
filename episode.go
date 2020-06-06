package main

import (
	"io"
	"fmt"
	"path/filepath"
	"os"
	"net/http"
	"time"
)


// Episode represents internal data related to each episode of the podcast.
type Episode  struct {
	// Show information
	showTitle   string
	showArtist  string

	// Episode information
	Title       string    `xml:"title"`
	Season      string    `xml:"season"`
	Number      string    `xml:"episode"`
	Desc        string    `xml:"description"`
	Date        string    `xml:"pubDate"`
	Enclosure   struct {
		URL         string    `xml:"url,attr"`
		Size        string    `xml:"length,attr"` // TODO: currently unused
		Type        string    `xml:"type,attr"`
	} `xml:"enclosure"`

	// Objects to handle reading/writing
	meta       *Meta      // Metadata object
	w           io.Writer // Writer that will handle writing the file.
}


// Download downloads the episode. The bytes will stream through this path from web to disk:
// Internet -> http object -> Episode object -> Disk
//             \-> Progress Bar object   \-> Meta object
func (e *Episode) Download(showDir string) error {
	if showDir == "" {
		return fmt.Errorf("Invalid call")
	}

	if err := e.validateData(); err != nil {
		return err
	}

	e.Title = Sanitize(e.Title) + mimeToExt(e.Enclosure.Type)
	filename := filepath.Join(showDir, e.Title)

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

	bar := ProgressBar{total: int(resp.ContentLength), totalString: Reduce(int(resp.ContentLength))}
	tee := io.TeeReader(resp.Body, &bar)

	// Connect the episode on both ends of the flow.
	e.meta = NewMeta(nil)
	e.w = file

	_, err = io.Copy(e, tee)
	if err != nil {
		return err
	}

	// Because we've been mucking around with carriage returns, we need to manually move down a row.
	fmt.Println()

	return nil
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
		if n, err := e.meta.Write(p); err != io.ErrShortWrite {
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


// addFrames fleshes out the metadata with information from the episode. If a frame already exists in the metadata, it
// will not be overwritten with data from the RSS feed. The only exceptions to this rule are the show and episode
// titles, which must match the data from the RSS feed to sync properly.
func (e *Episode) addFrames() {
	frames := []struct {
		frame string
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

	// We always want the show and episode titles to match the contents of the RSS feed.
	e.meta.SetFrame("TALB", []byte(e.showTitle))
	e.meta.SetFrame("TIT2", []byte(e.Title))

	for _, frame := range frames {
		if e.meta.GetFrame(frame.frame) == nil {
			e.meta.SetFrame(frame.frame, []byte(frame.value))
		}
	}

	if e.Date != "" {
		if time, err := time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", e.Date); err == nil {
			switch e.meta.Version() {
			case "3":
				if e.meta.GetFrame("TYER") == nil {
					e.meta.SetFrame("TYER", []byte(time.Format("2006"))) // YYYY
				}
				if e.meta.GetFrame("TDAT") == nil {
					e.meta.SetFrame("TDAT", []byte(time.Format("0201"))) // DDMM
				}
				if e.meta.GetFrame("TIME") == nil {
					e.meta.SetFrame("TIME", []byte(time.Format("1504"))) // HHMM
				}
			case "4":
				if e.meta.GetFrame("TDRC") == nil {
					e.meta.SetFrame("TDRC", []byte(time.Format("20060102T150405"))) // YYYYMMDDTHHMMSS
				}
			}
		}
	}
}

// validateData checks that we have all of the required fields from the RSS feed.
func (e *Episode) validateData() error {
	if e == nil {
		return fmt.Errorf("Cannot validata data: Bad episode object")
	}

	if e.Title == "" {
		return fmt.Errorf("Missing episode title")
	}

	if e.Enclosure.URL == "" {
		return fmt.Errorf("Missing download link for %v", e.Title)
	}

	if e.Number == "" {
		Debug("No episode number found for", e.Title)
	}

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
