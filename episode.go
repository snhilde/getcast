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
	Title       string    `xml:">title"`
	Season      string    `xml:">season"`
	Number      string    `xml:">episode"`
	Desc        string    `xml:">description"`
	Date        string    `xml:">pubDate"`
	URL         string    `xml:">enclosure,url"`
	Size        string    `xml:">enclosure,length"` // TODO: currently unused
	Type        string    `xml:">enclosure,type"`

	// Objects to handle reading/writing
	meta       *Meta      // Metadata object
	w           io.Writer // Writer that will handle writing the file.
}


// Download downloads the episode. The bytes will stream through this path from web to disk:
// Internet -> http object -> Meta object -> Episode object -> Disk
//                 \-> Progress Bar object
func (e *Episode) Download(showDir string) error {
	if showDir == "" {
		return fmt.Errorf("Invalid call")
	}

	filename := filepath.Join(showDir, e.Title)
	fmt.Println(filename)

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	resp, err := http.Get(e.URL)
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
	return e.w.Write(p[consumed:])
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


// addFrames fleshes out the metadata with information from the episode. If a field already exists in the metadata, it
// will not be overwritten with data from the RSS feed.
func (e *Episode) addFrames() {
	frames := []struct {
		field string
		value string
	}{
		// Show information
		{ "TALB", e.showTitle  },
		{ "TPE1", e.showArtist }, // Artist
		{ "TPE2", e.showArtist }, // Album Artist

		// Episode information
		{ "TIT2", e.Title      },
		{ "TRCK", e.Number     },
		{ "TDES", e.Desc       },
		{ "TPOS", e.Season     },
		{ "WOAF", e.URL        },

		// Defaults
		{ "TCON", "Podcast"    },
		{ "PCST", "1"          },
	}

	for _, frame := range frames {
		if e.meta.GetField(frame.field) == "" {
			e.meta.SetField(frame.field, frame.value)
		}
	}

	if e.Date != "" {
		if time, err := time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", e.Date); err == nil {
			if e.meta.Version() == "3" {
				if e.meta.GetField("TYER") == "" {
					e.meta.SetField("TYER", time.Format("2006")) // YYYY
				}
				if e.meta.GetField("TDAT") == "" {
					e.meta.SetField("TDAT", time.Format("0201")) // DDMM
				}
				if e.meta.GetField("TIME") == "" {
					e.meta.SetField("TIME", time.Format("1504")) // HHMM
				}
			} else {
				if e.meta.GetField("TDRC") == "" {
					e.meta.SetField("TDRC", time.Format("20060102T150405")) // YYYYMMDDTHHMMSS
				}
			}
		}
	}
}
