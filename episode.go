package main

import (
	"encoding/xml"
	"fmt"
	"github.com/kennygrant/sanitize"
	"strconv"
	"path/filepath"
	"os"
	"net/http"
	"io"
)


// Episode represents internal data related to each episode of the podcast.
type Episode struct {
	Number int    // Episode number
	Title  string // Title of the episode.
	Link   string // Link used to download the episode
	Length int    // Episode size in bytes
	MIME   string // MIME type
}


// Download downloads the episode.
func (e *Episode) Download(showDir string) error {
	if showDir == "" {
		return fmt.Errorf("Invalid call")
	}

	// Create a save point.
	filename := filepath.Join(showDir, e.Title)
	fmt.Println(filename)

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Grab the file's data.
	resp, err := http.Get(e.Link)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Make sure we accessed everything correctly.
	if resp.StatusCode != 200 {
		return fmt.Errorf("%v", resp.Status)
	}

	// Set up our progress bar.
	bar := ProgressBar{total: int(resp.ContentLength), totalString: Reduce(int(resp.ContentLength))}
	tee := io.TeeReader(resp.Body, &bar)

	// Save the file.
	_, err = io.Copy(file, tee)
	if err != nil {
		return err
	}

	// Because we've been mucking around with carriage returns, we need to manually move down a row.
	fmt.Println()

	return nil
}

// For the <title> tag, there are two tags returned: a general one, and one in the "itunes" namespace. The general tag
// usually has an episode number, while the other one does not. It will make everything easier if we save the podcast
// with the episode number in the filename, so we want to prefer the general tag. However, the parser reads the "itunes"
// namespace tag after the general one and overwrites the saved data, thus clobbering the desired title. This issue has
// been discussed as needing a fix in the xml library for some time. We'll grab the correct one here.
func (e *Episode) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	if e == nil {
		return fmt.Errorf("Invalid receiver")
	}

	// Scan through all of the tokens.
	for t, err := d.Token(); err != io.EOF; t, err = d.Token() {
		if err != nil {
			return err
		}

		// When we find a start tag, we'll see if we have one of the three tags that we want to save.
		if s, ok := t.(xml.StartElement); ok {
			// xml.StartElement has two fields: Name and Attr. We only care about the name, of which there are two
			// parts: Tag name (Local) and namespace (Space).
			switch s.Name.Local {
			case "title":
				// We only want the <title> tag without a namespace.
				if s.Name.Space == "" {
					// Grab the text between the opening and closing tags.
					n, err := d.Token()
					if err != nil {
						return err
					}
					if cd, ok := n.(xml.CharData); ok {
						e.Title = sanitize.BaseName(string(cd))
					}
				}
			case "enclosure":
				// We don't care about namespaces for this tag.
				for _, v := range s.Attr {
					switch v.Name.Local {
					case "url":
						e.Link = v.Value
					case "length":
						bytes, err := strconv.Atoi(v.Value)
						if err != nil {
							return err
						}
						e.Length = bytes
					case "type":
						e.MIME = v.Value
					}
				}
			case "episode":
				// For this tag, we actually want the namespace. Currently, I have only seen an itunes namespace, but
				// we'll allow any others that pop up.
				if s.Name.Space != "" {
					n, err := d.Token()
					if err != nil {
						return err
					}
					if cd, ok := n.(xml.CharData); ok {
						if num, err := strconv.Atoi(string(cd)); err != nil {
							return err
						} else {
							e.Number = num
						}
					}
				}
			}
		}
	}

	return nil
}
