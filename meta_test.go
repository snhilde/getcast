package main

import (
	"testing"
	"os/exec"
	"strings"
	"os"
	"bytes"
	"io/ioutil"
	"errors"
	"net/url"
	"path"
)


// localData is used to hold the information about a test file on disk, including its location, metadata size, and frames.
type localData struct {
	name       string
	path       string
	metasize   int // (header length + frames length)
	frames   []refFrame
}

// remoteData is used to hold the information about a podcast and one specific episode.
type remoteData struct {
	name   string
	url    string
	number string
	data   localData
}

// refFrame holds information about an individual frame in the metadata.
type refFrame struct {
	id    string // standard frame ID
	name  string // human-readable frame name
	value string // frame value
}


// We're going to use these generated audio files to test our ability to read metadata.
var localFiles = []localData {
	{ "White", "./audio/white.mp3", 10 + 291, []refFrame{
		{ "TIT2", "title",          "White Title"                       },
		{ "TPE1", "artist",         "White Artist"                      },
		{ "TALB", "album",          "White Album"                       },
		{ "TRCK", "track",          "1"                                 },
		{ "TDRC", "date",           "1111"                              }, // ID3v2.4 only
		{ "TCON", "genre",          "White Noise"                       },
		{ "COMM", "comment",        "White noise generated by Audacity" },
		{ "TXXX", "Additional Tag", "Additional white noise tag"        },
	} },
	{ "Pink", "./audio/pink.mp3", 10 + 209, []refFrame{
		{ "TIT2", "title",          "Pink Title"                        },
		{ "TPE1", "artist",         "Pink Artist"                       },
		{ "TALB", "album",          "Pink Album"                        },
		{ "TRCK", "track",          "2"                                 },
		{ "TDRC", "date",           "22222"                             }, // ID3v2.4 only
		{ "TCON", "genre",          "Country"                           },
		{ "COMM", "comment",        "Pink noise"                        },
		{ "TYER", "TYER",           "22222"                             },
		{ "TXXX", "Pink",           "Noise"                             },
	} },
	{ "Brown", "./audio/brown.mp3", 10 + 117, []refFrame{
		{ "TIT2", "title",          "Brown Title"                       },
		{ "TPE1", "artist",         "Brown Artist"                      },
		{ "TALB", "album",          "Brown Album"                       },
		{ "TRCK", "track",          "3"                                 },
		{ "TDRC", "date",           "33"                                }, // ID3v2.4 only
		{ "TYER", "TYER",           "33"                                },
	} },
}

// We're going to use these podcast episodes to test our ability to download an episode and read and write the correct
// metadata. We're going to use podcasts in which we have reasonable confidence that the files will remain online for a
// long time and the metadata will not change.
var onlineFiles = []remoteData {
	{ "The Joe Rogan Experience", "http://joeroganexp.joerogan.libsynpro.com/rss", "1000",
		localData { "Joe Rogan",      "#1000 - Joey Diaz & Tom Segura.mp3", 10 + 383990, []refFrame {
			{ "TPE1", "artist",       "Joe Rogan"                          },
			{ "TPE2", "album_artist", "Joe Rogan"                          },
			{ "TALB", "album",        "The Joe Rogan Experience"           },
			{ "TIT2", "title",        "#1000 - Joey Diaz & Tom Segura.mp3" },
			{ "TCON", "genre",        "Podcast"                            },
			{ "TRCK", "track",        "1000"                               },
			{ "TDRC", "date",         "2017-08-18 23:43"                   },
	} } },

	{ "Go Time", "https://changelog.com/gotime/feed", "1",
		localData { "Go Time", "1 - It's Go Time!.mp3", 10 + 838, []refFrame {
			{ "TPE1", "artist",          "Changelog Media"       },
			{ "TPE2", "album_artist",    "Changelog Media"       },
			{ "TALB", "album",           "Go Time"               },
			{ "TIT2", "title",           "1 - It's Go Time!.mp3" },
			{ "TSSE", "encoder",         "Lavf56.25.101"         },
			{ "TDES", "description",     "In this inaugural show Erik, Brian, and Carlisia kick things off by sharing some recent Go news that caught their attention, what to expect from this show, ways to get in touch, and more." },
			{ "TCON", "genre",           "Podcast"               },
			{ "WOAF", "url",             "https://cdn.changelog.com/uploads/gotime/1/go-time-1.mp3" },
			{ "TPUB", "publisher",       "Changelog Media"       },
			{ "TDRC", "date",            "2016"                  },
			{ "PCST", "podcast episode", "1"                     },
	} } },

		// localData { "The Daily", "https://rss.art19.com/episodes/ee819b27-9640-445c-8743-85b3dcec8db5.mp3", 10 + 306428, []refFrame {
		// { "TIT2", "title",          "Our Fear Facer Makes a New Friend" },
		// { "TPE1", "artist",         "The Daily"                         },
		// { "TALB", "album",          "The Daily"                         },
		// { "TCON", "genre",          "News"                              },
		// { "TPUB", "publisher",      "The New York Times"                },
		// { "TLAN", "language",       "English"                           },
		// { "TENC", "encoding",       "ART19, Inc."                       },
	// } } },
}


// Test the ability to read metadata correctly. The mp3 files to read are local files.
func TestReadMetaLocal(t *testing.T) {
	// First, let's make sure that ffprobe is finding the correct metadata in our test files. If that looks good, then
	// we can see how our solution is looking.
	for _, file := range localFiles {
		checkRefMeta(t, file.name, file.path, file.frames)
	}

	// Now, let's see how our reader stacks up.
	for _, file := range localFiles {
		meta, _, err := readAudioFile(file.path)
		if err != nil {
			t.Error(file.name, "-", err)
			return
		}

		if num := checkRefFile(t, meta, file.frames); num > 0 {
			t.Error(file.name, "-", num, "errors")
		}
	}
}

// Test the ability to write metadata and files correctly. The files to copy and write are the same files in
// TestReadMetaLocal.
func TestWriteMetaLocal(t *testing.T) {
	// Read the reference files into memory, copy them, and write them back out. If they're equal, then the write
	// operation is good.
	for _, file := range localFiles {
		filepath := file.path
		meta, audio, err := readAudioFile(file.path)
		if err != nil {
			t.Error(file.name, "-", err)
			continue
		}

		// Check that we copied the correct amount of metadata.
		if len(meta.Bytes()) != file.metasize {
			t.Error(file.name, "- Metadata sizes do not match")
			t.Log("\tExpected:", file.metasize)
			t.Log("\tReceived:", len(meta.Bytes()))
		}

		// If we read the correct amount of metadata out, then the first byte in the audio data should be 0xFF.
		if audio[0] != 0xFF {
			t.Error(file.name, "- Audio data does not start with 0xFF")
		}

		// Test writing everything to disk.
		filepath += "_tmp"
		testWrite(t, file.name, filepath, meta, audio, file.frames)
	}
}

// Test the ability to download and save a podcast episode with the correct file information and metadata.
func TestDownload(t *testing.T) {
	for _, podcast := range onlineFiles {
		u, err := url.Parse(podcast.url)
		if err != nil {
			t.Error(podcast.name, "- URL error:", err)
			continue
		}
		show := Show{URL: u}
		if n, err := show.Sync("./audio", podcast.number); err != nil {
			t.Error(podcast.name, "- Error syncing:", err)
		} else if n != 1 {
			t.Error(podcast.name, "- Failed to download episode")
		} else {
			filepath := path.Join("./audio", podcast.name, podcast.data.path)
			checkRefMeta(t, podcast.data.name, filepath, podcast.data.frames)

			meta, audio, err := readAudioFile(filepath)
			if err != nil {
				t.Error(podcast.data.name, "-", err)
				continue
			}

			// Check that we copied the correct amount of metadata.
			if len(meta.Bytes()) != podcast.data.metasize {
				t.Error(podcast.data.name, "- Metadata sizes do not match")
				t.Log("\tExpected:", podcast.data.metasize)
				t.Log("\tReceived:", len(meta.Bytes()))
			}

			// If we read the correct amount of metadata out, then the first byte in the audio data should be 0xFF.
			if audio[0] != 0xFF {
				t.Error(podcast.data.name, "- Audio data does not start with 0xFF")
			}
		}
	}
}


// checkRefMeta compares the metadata of a reference file using ffprobe to the expected metadata in the file table.
func checkRefMeta(t *testing.T, name string, filepath string, frames []refFrame) {
	// Get the frames from ffprobe's output.
	probeMeta, err := runProbe(filepath)
	if err != nil {
		t.Error(name, "- Error with ffprobe:", err)
		t.Log("\tUsed path", filepath)
		return
	}

	for _, frame := range frames {
		want := frame.value
		have := probeMeta[frame.name]
		if want != have {
			t.Error(name, "- Values do not match for id:", frame.name, "/", frame.id)
			t.Log("\tExpected:", want)
			t.Log("\tFound:", have)
		}
		delete(probeMeta, frame.name)
	}
}

// checkRefFile compares the metadata of a reference file using our meta reader to the expected metadata in the file table.
func checkRefFile(t *testing.T, meta *Meta, frames []refFrame) int {
	numErrors := 0

	// Go through all of the known frames and make sure our meta reader found the same values.
	for _, frame := range frames {
		found := false

		// Look for a match in all of the values for this frame ID in the metadata.
		values := meta.GetValues(frame.id)
		if len(values) == 0 {
			t.Error("No values for frame id " + frame.id + " (" + frame.name + ")")
			numErrors++
			continue
		}

		for _, value := range values {
			switch frame.id {
			case "COMM":
				// If this frame is present, then there are usually 2 instances of it: one that starts with 3 null
				// bytes, and one that starts with three 'X' bytes. Either way, the next byte is a null separator
				// followed by the value.
				value = bytes.TrimLeft(value, string([]byte{0x00, 'X'}))
				if strings.TrimSpace(string(value)) == frame.value {
					found = true
				}
			case "TXXX":
				// This is the user-defined field. The frame name and frame value are separated by a null byte.
				fields := bytes.SplitN(value, []byte{0x00}, 2)
				if len(fields) == 2 && string(fields[0]) == frame.name && string(fields[1]) == frame.value {
					found = true
				}
			default:
				if strings.TrimSpace(string(value)) == frame.value {
					found = true
				}
			}

			if found {
				break
			}
		}

		if !found {
			numErrors++
			t.Error(errors.New("Value not found for frame id " + frame.id + " (" + frame.name + ")"))
		}
	}

	return numErrors
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

// readAudioFile reads the data from the audio file and splits it into metadata and audio data.
func readAudioFile(path string) (*Meta, []byte, error) {
	// Open the reference file.
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	// Get all the data from the file.
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	// Copy the metadata and the rest of the file.
	meta := NewMeta(data)
	audio := data[meta.Len():]

	return meta, audio, nil
}

// testWrite tests the ability to write metadata correctly.
func testWrite(t *testing.T, name string, path string, meta *Meta, audio []byte, frames []refFrame) {
	if !writeData(t, name, path, meta.Build(), audio) {
		t.Error(name, "-", "Error while writing tmp file")
	}

	// Let's use ffprobe to see if all of the metadata was written correctly.
	checkRefMeta(t, name, path, frames)

	// And then do one more check with our reader.
	newMeta, _, err := readAudioFile(path)
	if err != nil {
		t.Error(name, "-", err)
	}
	if num := checkRefFile(t, newMeta, frames); num > 0 {
		t.Error(name, "-", num, "errors")
	}

	// Now that we're done, we can remove the temporary file.
	if err := os.Remove(path); err != nil {
		t.Error(name, "-", err)
	}
}

// writeData writes the metadata and audio data to the specified file.
func writeData(t *testing.T, name string, filepath string, meta, audio []byte) bool {
	file, err := os.Create(filepath)
	if err != nil {
		t.Error(name, "-", err)
		return false
	}
	defer file.Close()

	// Write the metadata first.
	if n, err := file.Write(meta); err != nil {
		t.Error(name, "-", err)
		return false
	} else if n != len(meta) {
		t.Error(name, "- Failed to write correct number of bytes")
		t.Log("\tExpected:", len(meta))
		t.Log("\tActual  :", n)
		return false
	}

	// Then right the audio data.
	if n, err := file.Write(audio); err != nil {
		t.Error(name, "-", err)
		return false
	} else if n != len(audio) {
		t.Error(name, "- Failed to write correct number of bytes")
		t.Log("\tExpected:", len(audio))
		t.Log("\tActual  :", n)
		return false
	}

	return true
}
