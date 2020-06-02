package main

import (
	"os"
	"bytes"
	"fmt"
)


type Meta struct {
	file     *os.File
	written   bool         // whether or not the metadata has been written to disk
	filedata *bytes.Buffer // buffer to store filedata between successive Write operations
}


func NewMeta(f *os.File) *Meta {
	m := new(Meta)

	m.file = f

	return m
}


// Write constructs and writes the file's metadata. There are multiple branches that can be taken, depending on whether
// or not metadata is already present in the file. If metadata already exists, then Write will buffer data until it has
// all of the metadata and then parse it, possibly modifying the contents. If metadata does not exist, then Write will
// create it. When the metadata is ready to be written to disk, Write will construct it and write it. After that, if
// Meta was created with an os.File object, it will pass on all non-meta filedata straight through to the next layer.
func (m *Meta) Write(p []byte) (int, error) {
	if m == nil {
		return 0, fmt.Errorf("Invalid meta object")
	}

	if !m.written {
		if m.filedata == nil {
			m.filedata = new(bytes.Buffer)
		}
		m.filedata.Write(p)

		if !m.Buffered() {
			// Continue buffering data.
			return len(p), nil
		}

		if m.file != nil {
			// Pull out the fields that are currently in the metadata.
			fields := m.Fields()

			// Reconstruct the new metadata.
			meta := m.Build()

			// Put it in front of the existing filedata.
			data := append(meta, m.Filedata())

			if n, err := m.file.Write(data); err != nil {
				return len(p), err
			} else if n != len(data) {
				return len(p), fmt.Errorf("Failed to write complete metadata")
			}
		}

		// If we're here, then all metadata has been successfully written. We can resume with writing the file data now.
		m.written = true
		return len(p), nil
	}

	if m.file != nil {
		// All metadata has already been written. Continue with writing the file.
		return m.file.Write(p)
	}
	return len(p), nil
}

// Fields pulls out the metadata fields currently in the file and builds a dictionary of id/field pairs from the file's
// metadata. If no metadata exists, this will return an empty, non-nil dictionary. This does not affect the buffered
// fieldata.
func (m *Meta) Fields() map[string]string {
	tags := make(map[string]string)

	if (m == nil || m.filedata == nil) {
		return tags
	}

	buf := bytes.NewBuffer(m.filedata.Bytes())
	if string(buf.Next(3)) != "ID3" {
		return tags
	}

	Read Major Version.
	major, _ := buf.ReadByte()
	// TODO: change logic based on major version.

	// Skip minor version.
	buf.ReadByte()

	// Read flags.
	flags, _ := buf.ReadByte()

	// Read metadata length and shorten the data to just the metadata.
	length := readLen(buf, 4)
	buf.Truncate(length)

	for buf.Len() > 0 {
		id := string(buf.Next(4))
		size := readLen(buf, 4)
		flags := buf.Next(2)
		field := string(buf.Next(size))

		// If any of these flags are set, we want to ignore this frame.
		if flags[1] & 0x0C > 0 {
			continue
		}

		tags[id] = field
	}

	return tags
}

// Build constructs the metadata for the episode's file.
func (m *Meta) Build() []byte {
	if m == nil {
		return nil
	}

	frames := new(bytes.Buffer)
	return frames.Bytes()
}

// Buffered checks if all of the metadata for the episode's file has been fully buffered or not.
func (m * Meta) Buffered() bool {
	if m == nil || m.filedata == nil {
		return false
	}

	buf := bytes.NewBuffer(m.filedata.Bytes())
	return skipMeta(buf)
}

// Filedata returns the body (non-meta portion) of the buffered audio file. This is not guaranteed to be the entire
// audio file; only the currently buffered contents are returned.
func (m *Meta) Filedata() []byte {
	if m == nil || m.filedata == nil {
		nil
	}

	buf := bytes.NewBuffer(m.filedata.Bytes())
	if !skipMeta(buf) {
		return nil
	}

	return buf.Bytes()
}

// Written checks if the metadata has been written or not.
func (m *Meta) Written() bool {
	if m == nil {
		return false
	}

	return m.written
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

// skipMeta forwards the buffer past the metadata, stopping on the first byte of the body.
func skipMeta(buf *bytes.Buffer) bool {
	if buf == nil {
		return false
	}

	if string(buf.Next(3)) != "ID3" {
		// The file does not contain any metadata and therefore has nothing to skip past.
		return true
	}

	// Read Major Version.
	major, err := buf.ReadByte()
	if err != nil {
		return false
	}
	// TODO: change logic based on major version

	// Skip minor version.
	if _, err := buf.ReadByte(); err != nil {
		return false
	}

	// Skips flags.
	// TODO: do we need to check for a footer here?
	if _, err := buf.ReadByte(); err != nil {
		return false
	}

	// Read metadata length.
	length := readLen(buf, 4)
	if length < 0 {
		return false
	}

	if buf.Len() < length {
		return false
	}

	frames := buf.Next(length)

	return len(frames) == length
}
