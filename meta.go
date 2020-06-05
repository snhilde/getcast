package main

import (
	"os"
	"bytes"
	"fmt"
	"golang.org/x/text/encoding/unicode"
)


type Meta struct {
	buffer   *bytes.Buffer      // buffer to store filedata between successive Write operations
	buffered  bool              // whether or not all metadata is present in the buffer
	noMeta    bool              // whether or not the file has any metadata
	fields    map[string]string // cached dictionary of fields
}


// NewMeta creates a new Meta object. If file data is passed in, NewMeta will read as much of the metadata from it as possible.
func NewMeta(file []byte) *Meta {
	m := new(Meta)

	if file != nil {
		m.Write(file)
	}

	return m
}


// Write buffers metadata into the internal buffer. When the metadata has been completely written, Write will stop
// writing to the buffer and return (n, ErrShortWrite), with n designating how many bytes were consumed in this
// operation.
func (m *Meta) Write(p []byte) (int, error) {
	if m == nil {
		return 0, fmt.Errorf("Invalid meta object")
	}

	if m.buffer == nil {
		m.buffer = new(bytes.Buffer)
	}

	if m.Buffered() {
		// All metadata has already been written.
		return 0, io.ErrShortWrite
	}

	// We don't know how many of the provided bytes we need to finish buffering the metadata. Let's add everything we're
	// given to our internal buffer now. Later, we'll drop any bytes that we don't need.
	m.buffer.Write(p)

	length := m.length()
	if length < 0 {
		// Need more data.
		return len(p), nil
	}

	if length == 0 {
		// The file has data but not any metadata.
		m.buffer.Truncate(0)
		return 0, nil
	}

	if m.buffer.Len() <= length {
		// We need all of the data from this write.
		return len(p), nil
	}

	// If we're here, then we wrote too many bytes to our buffer. Let's back it up a bit and return how many bytes we
	// actually need.
	need := length - (m.buffer.Len() - len(p))
	m.buffer.Truncate(length)
	return need, io.ErrShortWrite
}

// Buffered checks if all of the metadata for the episode's file has been fully buffered or not. If the file doesn't
// have any metadata, then this will return true.
func (m * Meta) Buffered() bool {
	if m == nil || m.buffer == nil {
		return false
	}

	if m.buffered || m.noMeta {
		return true
	}

	length := m.length()
	if length < 0 {
		return false
	}

	// A length of 0 means that the file has data but not any metadata.
	if length == 0 {
		return true
	}

	if m.buffer.Len() >= length {
		m.buffered = true
	}

	return m.buffered
}

// Version returns the version of ID3v2 metadata in use, or "" if not found.
func (m *Meta) Version() string {
	if m == nil || m.noMeta || m.buffer.Len() < 4 {
		return ""
	}

	data := m.buffer.Bytes()
	return fmt.Sprintf("%v", data[3])
}

// GetField returns the value of the specified field, or "" if no field exists with that name. The name will be matched
// in a case-sensitive comparison.
func (m *Meta) GetField(field string) string {
	if m == nil {
		return ""
	}

	// If we haven't cached the fields yet, do so now.
	if m.fields == nil {
		m.fields = m.parseFields()
	}

	if m.fields == nil {
		return ""
	}

	return m.fields[field]
}

// SetField sets the field with the value.
func (m *Meta) SetField(field, value string) {
	if m == nil {
		return
	}

	// If we haven't cached the fields yet, do so now.
	if m.fields == nil {
		m.fields = m.parseFields()
	}

	if m.fields != nil {
		m.fields[field] = value
	}
}

// Build constructs the metadata for the episode's file.
func (m *Meta) Build() []byte {
	if m == nil {
		return nil
	}

	// TODO
	frames := new(bytes.Buffer)
	return frames.Bytes()
}


// parseFields returns a dictionary of all fields (represented as id/value pairs) in the metadata. On error or if there
// is no metadata present, this will return nil. If metadata is present but no fields exist, this will return an empty,
// non-nil dictionary.
func (m *Meta) parseFields() map[string]string {
	if !m.Buffered() || m.noMeta {
		return nil
	}

	buf := bytes.NewBuffer(m.buffer.Bytes())

	// Skip past ID.
	buf.Next(3)

	// Skip major version.
	buf.ReadByte()

	// Skip minor version.
	buf.ReadByte()

	flags, _ := buf.ReadByte()

	// Skip past the length.
	buf.Next(4)

	// Skip past the extended header, if present.
	if flags & (1 << 6) > 0 {
		length := readLen(buf, 4)
		buf.Next(length - 4)
	}

	fields := make(map[string]string)
	for buf.Len() > 0 {
		id := buf.Next(4)
		size := readLen(buf, 4)
		flags := buf.Next(2)
		value := buf.Next(size)

		// We only want the frame if these flags are not set.
		if flags[1] & 0x0C == 0 {
			switch value[0] {
			case 0x00:
				// ASCII characters. Remove the first and last bytes.
				value = value[1:len(tmp)-1]
			case 0x01:
				// UTF-16 with BOM. Remove the first byte and the last 2 bytes and decode to UTF-8.
				value = value[1:len(tmp)-2]
				decoder := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder()
				value, _ = decoder.Bytes(value)
			case 0x02:
				// UTF-16 Big Endian without BOM. Remove the first byte and the last 2 bytes and decode to UTF-8.
				value = value[1:len(tmp)-2]
				decoder := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder()
				value, _ = decoder.Bytes(value)
			case 0x03:
				// UTF-8 (Unicode). Remove the first and last bytes.
				value = value[1:len(tmp)-1]
			}

			fields[string(id)] = string(value)
		}
	}

	return fields
}


// length returns the reported length in bytes of the entire metadata, or -1 if the metadata could not be successfully
// parsed (possibly indicating that more metadata is needed). It is not necessary to have the entire metadata buffered.
// If no metadata exists in the file's contents, this will return 0.
func (m *Meta) length() int {
	if m == nil || m.buffer == nil {
		return -1
	}

	buf := bytes.NewBuffer(m.buffer.Bytes())
	if buf.Len() < 3 {
		// Need more metadata to determine anything.
		return -1
	}

	if string(buf.Next(3)) != "ID3" {
		// The file has data but not any metadata.
		m.noMeta = true
		return 0
	}

	// Skip Major Version.
	if _, err := buf.ReadByte(); err != nil {
		return -1
	}

	// Skip minor version.
	if _, err := buf.ReadByte(); err != nil {
		return -1
	}

	// Skip flags.
	if _, err := buf.ReadByte(); err != nil {
		return -1
	}

	// Read metadata length.
	length := readLen(buf, 4)
	if length < 0 {
		return -1
	}

	// Add 10 bytes for the header.
	return length + 10
}

// Read a big-endian number of num bytes from the buffer. This will advance the buffer. If num bytes are not available,
// this will return -1.
func readLen(buf *bytes.Buffer, num int) int {
	if buf == nil || num < 0 || buf.Len() < num {
		return -1
	}

	bytes := buf.Next(num)
	if len(bytes) != num {
		return -1
	}

	length := int(0)
	for _, v := range bytes {
		length <<= 8
		length |= int(v)
	}

	return length
}
