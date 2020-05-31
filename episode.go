package main

import (
	"io"
	"fmt"
	"path/filepath"
	"os"
	"net/http"
	"encoding/xml"
	"github.com/kennygrant/sanitize"
	"strconv"
)


// Episode represents internal data related to each episode of the podcast.
type Episode struct {
	Number   int       // episode number
	Title    string    // title of the episode
	Link     string    // link used to download the episode
	Length   int       // episode size in bytes
	Ext      string    // file extension

	metaDone bool      // true after metadata has been written to disk
	meta     Meta       // buffer that will hold metadata as it's built
	w        io.Writer // writer that will write the episode to disk
}


// Download downloads the episode.
func (e *Episode) Download(showDir string) error {
	if showDir == "" {
		return fmt.Errorf("Invalid call")
	}

	filename := filepath.Join(showDir, e.Title)
	fmt.Println(filename)

	// Create a save point.
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

	// Wrap the file writer in an episode writer so we can add/modify the tag data.
	wrapper := e.NewWrapper(file)

	// Save the file.
	_, err = io.Copy(wrapper, tee)
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

		// When we find a start tag, we'll see if we have one of the tags that we want to save.
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
						e.Ext = mimeToExt(v.Value)
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

func (e *Episode) NewWrapper(w io.Writer) io.Writer {
	e.w = w

	return e
}

// Write is used as a wrapper around another io.Writer to add or modify ID3v2 metadata.
func (e *Episode) Write(p []byte) (int, error) {
	if e == nil || e.w == nil {
		return 0, io.ErrClosedPipe
	}

	if !e.metaDone {
		meta, err := e.meta.Build(p)
		if err != nil {
			return len(p), err
		}

		if meta == nil {
			// We need more file data to write out the complete metadata.
			return len(p), err
		}

		// We have all the metadata. Let's write it to disk (and any other file data included in this chunk).
		if n, err := e.w.Write(meta); err != nil {
			return len(p), err
		} else if n != len(meta) {
			return len(p), fmt.Errorf("Failed to write complete metadata")
		}

		// If we're here, then all metadata has been successfully written to disk. We can resume with writing the
		// file data now.
		e.metaDone = true
		return len(p), nil
	}

	return e.w.Write(p)
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
