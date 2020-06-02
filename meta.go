package main

import (
	"os"
	"bytes"
	"fmt"
)


type Meta struct {
	filedata *bytes.Buffer      // buffer to store filedata between successive Write operations
	exFields  map[string]string // additional fields to write to the metadata, as passed in via AddFields
	hasMeta   bool              // whether or not the file has any metadata.
}


func NewMeta() *Meta {
	return new(Meta)
}


// AddFields allows additional fields to be added to the metadata. If a field already exists, it will be overwritten
// with the supplied data.
func (m *Meta) AddFields(fields map[string]string) {
	if m == nil {
		return
	}

	m.exFields = fields
}

// Write buffers metadata into the internal buffer. When the metadata has been completely written, Write will stop
// writing to the buffer and return (n, ErrShortWrite), with n designating how many bytes were consumed in this
// operation.
func (m *Meta) Write(p []byte) (int, error) {
	if m == nil {
		return 0, fmt.Errorf("Invalid meta object")
	}

	if m.filedata == nil {
		m.filedata = new(bytes.Buffer)
	}

	if !m.hasMeta || (m.hasMeta && m.Buffered()) {
		// All metadata has already been written.
		return 0, io.ErrShortWrite
	}

	tmp := bytes.NewBuffer(m.filedata.Bytes())
	tmp.Write(p)

	// Let's see how many more bytes we need to write to complete the metadata.
	length := metaLen(tmp)
	if length == 0 {
		// The file doesn't start with any metadata.
		m.hasMeta = false
		return 0, io.ErrShortWrite
	}

	need := length - m.filedata.Len()
	if len(p) <= need {
		// We need all of these bytes.
		return m.filedata.Write(p)
	}

	// We only need some of the bytes offered.
	if n, err := m.filedata.Write(p[:need]); err != nil {
		return n, err
	} else if n != need {
		return n, fmt.Errorf("Failed to write metadata")
	}

	return need, io.ErrShortWrite
}

// Fields pulls out the metadata fields currently in the file and builds a dictionary of id/field pairs from the file's
// metadata. If no metadata exists, this will return an empty, non-nil dictionary. This does not affect the buffered
// fieldata.
func (m *Meta) Fields() map[string]string {
	tags := make(map[string]string)
	if m.exFields != nil {
		tags = m.exFields
	}

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

		if m.exFields != nil && _, ok := m.exFields[id]; !ok {
			tags[id] = field
		}
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

	length := metaLen(m.filedata)
	if length < 0 {
		return false
	}

	return m.filedata.Len() >= length
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

// metaLen returns the reported length in bytes of the entire metadata, or -1 if the metadata could not be successfully
// parsed (possibly indicating that more metadata is needed). It is not necessary to have the entire metadata buffered.
// If no metadata exists in the file's contents, this will return 0.
func metaLen(buf *bytes.Buffer) int {
	if buf == nil {
		return false
	}

	tmp := bytes.NewBuffer(buf.Bytes())
	if tmp.Len() < 3 {
		// Need more metadata.
		return -1
	}

	if string(tmp.Next(3)) != "ID3" {
		// The file does not contain any metadata.
		return 0
	}

	// Read Major Version.
	major, err := tmp.ReadByte()
	if err != nil {
		return -1
	}
	// TODO: change logic based on major version

	// Skip minor version.
	if _, err := tmp.ReadByte(); err != nil {
		return -1
	}

	// Skips flags.
	// TODO: do we need to check for a footer here?
	if _, err := tmp.ReadByte(); err != nil {
		return -1
	}

	// Read metadata length.
	length := readLen(tmp, 4)
	if length < 0 {
		return -1
	}

	// Add 10 bytes for the header.
	return length + 10
}
