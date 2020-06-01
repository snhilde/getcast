package main

import (
	"bytes"
	"io"
	"fmt"
)


type Meta struct {
	e        *Episode
	w         io.Writer
	written   bool         // whether or not the metadata has been written to disk
	filedata *bytes.Buffer // buffer to store filedata between successive Write operations
}


func NewMeta(e *Episode, w io.Writer) *Meta {
	m := new(Meta)

	m.e = e
	m.w = w

	return m
}


// Write is in charge of constructing and writing the metadata of the episode. There are multiple branches that can be
// taken, depending on whether or not metadata is already present in the file. If metadata already exists, then Write
// will buffer data until it has all of the metadata and then parse it, possibly modifying the contents. If metadata
// does not exist, then Write will create it. When the metadata is ready to be written to disk, Write will construct it
// and write it. After that, it will pass on all data straight through to the next layer.
func (m *Meta) Write(p []byte) (int, error) {
	if m == nil {
		return 0, fmt.Errorf("Invalid meta object")
	}

	if !m.written {
		if m.filedata == nil {
			m.filedata = new(bytes.Buffer)
		}
		m.filedata.Write(p)

		if !isEntire(m.filedata) {
			// Continue buffering data.
			return len(p), nil
		}

		// Pull out the metadata that is currently in the file. After this, filedata will consist of only the episode's
		// audio data.
		fields := m.metadata()

		// Reconstruct the new metadata.
		meta := buildMetadata(fields, m.e)

		if n, err := m.w.Write(meta); err != nil {
			return len(p), err
		} else if n != len(meta) {
			return len(p), fmt.Errorf("Failed to write complete metadata")
		}

		// If we're here, then all metadata has been successfully written to disk. We can resume with writing the file
		// data now.
		m.written = true
		return len(p), nil
	}

	// All metadata has already been written. Contine with writing the file.
	return m.w.Write(p)
}

// metadata pulls out the metadata currently in the file and builds a dictionary of id/field pairs from the file's
// metadata. If no metadata exists, this will return an empty, non-nil dictionary. At the end of this operation,
// filedata will consist of all filedata minus the metadata.
func (m *Meta) metadata() map[string]string {
	tags := make(map[string]string)

	if string(m.filedata.Next(3)) != "ID3" {
		// The file does not have any metadata.
		for i := 0; i < 3; i++ {
			m.filedata.UnreadByte()
		}
		return tags
	}

	// Read Major Version.
	major, _ := m.filedata.ReadByte()

	// Skip minor version.
	m.filedata.ReadByte()

	// Read flags.
	flags, _ := m.filedata.ReadByte()

	// Read metadata length.
	length := readLen(m.filedata, 4)

	metadata := bytes.NewBuffer(m.filedata.Next(length))
	for metadata.Len() > 0 {
		id := string(metadata.Next(4))
		size := readLen(metadata, 4)
		flags := metadata.Next(2)
		field := string(metadata.Next(size))

		// If any of these flags are set, we want to ignore this frame.
		if flags[1] & 0x0C > 0 {
			continue
		}

		tags[id] = field
	}

	return tags
}

// Finished checks if the metadata was written out or not.
func (m *Meta) Finished() bool {
	if m == nil {
		return false
	}

	return m.written
}


// isEntire checks if all of the metadata for the episode's file has been fully buffered or not.
func isEntire(buf *bytes.Buffer) bool {
	if buf == nil {
		return false
	}

	if string(buf.Next(3)) != "ID3" {
		// The file does not have any metadata.
		return true
	}

	// Read Major Version.
	major, err := buf.ReadByte()
	if err != nil {
		return false
	}

	// Skip minor version.
	if _, err := buf.ReadByte(); err != nil {
		return false
	}

	// Skips flags.
	if _, err := buf.ReadByte(); err != nil {
		return false
	}

	// Read metadata length.
	length := readLen(buf, 4)
	if length < 0 {
		return false
	}

	if buf.Len() >= length {
		return true
	}

	return false
}

// buildMetadata constructs the metadata for the episode's file.
func (m *Meta) buildMetadata(fields map[string]string, e *Episode) ([]byte, error) {
	if m == nil {
		return nil, fmt.Errorf("Invalid meta object, cannot build metadata")
	}

	frames := new(bytes.Buffer)
}


// Read a big-endian number of num bytes from the buffer. This will advance the buffer.
func readLen(buf *bytes.Buffer, num int) int {
	length := int(0)
	bytes := buf.Next(num)
	for _, v := range bytes {
		length <<= 8
		length |= int(v)
	}

	return length
}
