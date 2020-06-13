package main

import (
	"testing"
	"path"
	"os/exec"
	"strings"
)


// We're going to use these generated audio files to test our ability to read metadata.
var testData = []struct {
	filepath string
	metadata map[string]string
}{
	{ "./audio/white.mp3",
		map[string]string{
			"title": "White Title",
			"artist": "White Artist",
			"album": "White Album",
			"track": "1",
			"date": "1111",
			"genre": "White Noise",
			"comment": "White noise generated by Audacity",
			"Additional Tag": "Additional white noise tag",
		},
	},
	{ "./audio/pink.mp3",
		map[string]string{
			"title": "Pink Title",
			"artist": "Pink Artist",
			"album": "Pink Album",
			"track": "2",
			"date": "22222",
			"genre": "Country",
			"comment": "Pink noise",
			"TYER": "22222",
		},
	},
	{ "./audio/brown.mp3",
		map[string]string{
			"title": "Brown Title",
			"artist": "Brown Artist",
			"album": "Brown Album",
			"track": "3",
			"date": "33",
			"TYER": "33",
		},
	},
}


// Test the ability to read metadata correctly. The mp3 files to read are local files.
func TestReadMetaLocal(t *testing.T) {
	for _, test := range testData {
		probeMeta, err := runProbe(test.filepath)
		if err != nil {
			t.Error(err)
			continue
		}

		filename := path.Base(test.filepath)
		for key, value := range test.metadata {
			if value != probeMeta[key] {
				t.Error("Mismatch with key", key, "for", filename)
				t.Log("\tExpected:", value)
				t.Log("\tFound:", probeMeta[key])
			}
			delete(probeMeta, key)
		}

		// Make sure we found everything that we expected to find.
		if len(probeMeta) != 0 {
			t.Error(len(probeMeta), "unexpected keys remain in metadata for", filename)
			t.Log("Keys remaining:", probeMeta)
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
			key := strings.TrimSpace(fields[0])
			value := strings.TrimSpace(fields[1])
			meta[key] = value
		}
	}

	return meta, nil
}
