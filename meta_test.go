package main

import (
	"testing"
	"path"
	"os/exec"
	"strings"
	"os"
	"io"
	"bytes"
	"encoding/hex"
	"io/ioutil"
)


type refFrame struct {
	name  string
	id    string
	value string
}


// We're going to use these generated audio files to test our ability to read metadata.
var refData = []struct {
	filepath   string
	metasize   int // (header length + frames length)
	frames   []refFrame
}{
	{ "./audio/white.mp3", 10 + 291, []refFrame{
		{ "title",          "TIT2", "White Title"                       },
		{ "artist",         "TPE1", "White Artist"                      },
		{ "album",          "TALB", "White Album"                       },
		{ "track",          "TRCK", "1"                                 },
		{ "date",           "TDRC", "1111"                              }, // ID3v2.4 only
		{ "genre",          "TCON", "White Noise"                       },
		{ "comment",        "COMM", "White noise generated by Audacity" },
		{ "Additional Tag", "TXXX", "Additional white noise tag"        },
	} },
	{ "./audio/pink.mp3", 10 + 209, []refFrame{
		{ "title",          "TIT2", "Pink Title"                        },
		{ "artist",         "TPE1", "Pink Artist"                       },
		{ "album",          "TALB", "Pink Album"                        },
		{ "track",          "TRCK", "2"                                 },
		{ "date",           "TDRC", "22222"                             }, // ID3v2.4 only
		{ "genre",          "TCON", "Country"                           },
		{ "comment",        "COMM", "Pink noise"                        },
		{ "TYER",           "TYER", "22222"                             },
		{ "Pink",           "TXXX", "Noise"                             },
	} },
	{ "./audio/brown.mp3", 10 + 117, []refFrame{
		{ "title",          "TIT2", "Brown Title"                       },
		{ "artist",         "TPE1", "Brown Artist"                      },
		{ "album",          "TALB", "Brown Album"                       },
		{ "track",          "TRCK", "3"                                 },
		{ "date",           "TDRC", "33"                                }, // ID3v2.4 only
		{ "TYER",           "TYER", "33"                                },
	} },
}


// Test the ability to read metadata correctly. The mp3 files to read are local files.
func TestReadMetaLocal(t *testing.T) {
	// First, let's make sure that ffprobe is finding the correct metadata in our test files. If that looks good, then
	// we can see how our solution is looking.
	for _, ref := range refData {
		filename := path.Base(ref.filepath)
		probeMeta, err := runProbe(ref.filepath)
		if err != nil {
			t.Error(err)
			continue
		}

		for _, frame := range ref.frames {
			want := frame.value
			have := probeMeta[frame.name]
			if want != have {
				t.Error(filename, "- Values do not match for id:", frame.name, "/", frame.id)
				t.Log("\tExpected:", want)
				t.Log("\tFound:", have)
			}
			delete(probeMeta, frame.name)
		}

		// Make sure we found everything that we expected to find.
		if len(probeMeta) != 0 {
			t.Error(len(probeMeta), "keys remain in metadata for", filename)
			t.Log("Keys remaining:", probeMeta)
		}
	}

	// Now, let's see how our reader stacks up.
	for _, ref := range refData {
		meta := NewMeta(nil)
		filename := path.Base(ref.filepath)

		file, err := os.Open(ref.filepath)
		if err != nil {
			t.Error(filename, "-", err)
		}

		if _, err := meta.ReadFrom(file); err != io.ErrShortWrite {
			t.Error(filename, "-", err)
		}
		file.Close()

		// Go through all of the known frames and make sure our meta reader found the same values.
		for _, frame := range ref.frames {
			found := false

			// Look for a match in all of the values for this frame ID in the metadata.
			values := meta.GetValues(frame.id)
			for _, value := range values {
				switch frame.id {
				case "COMM":
					// If this frame is present, then there are usually 2 instances of it: one that starts with 3 null
					// bytes, and one that starts with three 'X' bytes. Either way, the next byte is a null separator
					// followed by the value.
					value = bytes.TrimLeft(value, string([]byte{0x00, 'X'}))
					if string(value) == frame.value {
						found = true
					}
				case "TXXX":
					// This is the user-defined field. The frame name and frame value are separated by a null byte.
					fields := bytes.SplitN(value, []byte{0x00}, 2)
					if len(fields) == 2 && string(fields[0]) == frame.name && string(fields[1]) == frame.value {
						found = true
					}
				default:
					if string(value) == frame.value {
						found = true
					}
				}
				if found {
					break
				}
			}
			if !found {
				t.Error(filename, "- value not found for id:", frame.id)
				t.Log("\tExpected:", frame.value)
				for _, value := range values {
				t.Log("\tFound:", hex.Dump(value))
				}
			}
		}
	}
}


// runProbe runs ffprobe on the specified file and returns the metadata read as key/value pairs. Note that ffprobe does
// not return the actual tag name; it returns a human-readable format. For example, it returns "title" instead of "TIT2".
func runProbe(path string) (map[string]string, error) {
	cmd := exec.Command("ffprobe", "-hide_banner", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	// To find our metadata, we're going to read everything between the header (Metadata:) and trailer (Duration:).
	start := false
	meta := make(map[string]string)

	lines := strings.Split(string(output), "\n")
	for _, v := range lines {
		if strings.Contains(v, "Metadata:") {
			start = true
		} else if strings.Contains(v, "Duration:") {
			break
		} else if start {
			fields := strings.SplitN(v, ":", 2)
			id := strings.TrimSpace(fields[0])
			value := strings.TrimSpace(fields[1])
			meta[id] = value
		}
	}

	return meta, nil
}
