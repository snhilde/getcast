// For specifications on the 3 ID3 standards: https://id3.org/Developer%20Information
package main

import (
	"bytes"
	"fmt"
	"golang.org/x/text/encoding/unicode"
	"io"
	"strings"
)

// Meta is the main type used. It holds all the information related to the metadata.
type Meta struct {
	buffer     *bytes.Buffer // buffer to store filedata between successive Write operations
	buffered   bool          // whether or not all metadata is present in the buffer
	noMeta     bool          // whether or not the file has any metadata
	readFrames bool          // whether or not the metadata frames have been read and parsed.
	frames     []Frame       // list of frames
}

// Frame is used to store information about a metadata frame.
type Frame struct {
	id    string
	value []byte
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
// writing to the buffer and return (n, io.EOF), with n designating how many bytes were consumed in this operation.
func (m *Meta) Write(p []byte) (int, error) {
	if m == nil {
		return 0, fmt.Errorf("Invalid meta object")
	}

	if m.buffer == nil {
		m.buffer = new(bytes.Buffer)
	}

	if m.Buffered() {
		// All metadata has already been written.
		return 0, io.EOF
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
	m.Buffered()
	return need, io.EOF
}

// Buffered checks if all of the metadata for the episode's file has been fully buffered or not. If the file doesn't
// have any metadata, then this will return true.
func (m *Meta) Buffered() bool {
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
		m.noMeta = true
		return true
	}

	if m.buffer.Len() >= length {
		m.buffered = true
		m.parseFrames()
		m.readFrames = true
	}

	return m.buffered
}

// Bytes returns all the bytes currently buffered.
func (m *Meta) Bytes() []byte {
	if m == nil || m.buffer == nil {
		return nil
	}

	return m.buffer.Bytes()
}

// Len returns the number of bytes currently buffered.
func (m *Meta) Len() int {
	if m == nil || m.buffer == nil {
		return 0
	}

	return m.buffer.Len()
}

// Version returns the version of ID3v2 metadata in use, or 0 if not found.
func (m *Meta) Version() byte {
	if m == nil || m.noMeta || m.buffer == nil || m.buffer.Len() < 4 {
		return 0
	}

	data := m.buffer.Bytes()
	return data[3]
}

// NumFrames returns the number of frames in the metadata. If multiple frames have the same frame ID, each instance of
// the ID is counted separately.
func (m *Meta) NumFrames() int {
	if m == nil || m.noMeta || !m.Buffered() {
		return 0
	}

	return len(m.frames)
}

// GetValues returns all values for the given frame ID. The ID will be matched in a case-sensitive comparison.
func (m *Meta) GetValues(id string) [][]byte {
	if m == nil || !m.Buffered() {
		return nil
	}

	var values [][]byte
	for _, frame := range m.frames {
		if frame.id == id {
			values = append(values, frame.value)
		}
	}

	return values
}

// SetValue adds the value for this frame ID into the metadata. Value should be UTF-8 encoded. If multiple is true, the
// metadata is allowed to have multiple frames with the same frame ID. Otherwise, this frame is the only frame allowed
// to have this frame ID. ID3v2.2 frame IDs are 3 bytes long, while other versions have 4-byte IDs.
func (m *Meta) SetValue(id string, value []byte, multiple bool) {
	if m == nil || !m.Buffered() {
		return
	}

	if (m.Version() == 2 && len(id) != 3) || (m.Version() != 2 && len(id) != 4) {
		Debug("Invalid frame ID:", id)
		return
	}

	id = strings.ToUpper(id)

	if !multiple {
		// Remove all frames with matching ID.
		var frames []Frame
		for _, frame := range m.frames {
			if frame.id != id {
				frames = append(frames, frame)
			}
		}
		m.frames = frames
	}

	m.frames = append(m.frames, Frame{id, value})
	Debug("Set frame", id, "to", string(value))
}

// Build constructs the metadata for the episode's file. If the metadata cannot be constructed, this will return nil.
func (m *Meta) Build() []byte {
	if m == nil {
		return nil
	}

	version := m.Version()
	if version == 0 {
		version = 4
	}
	Debug("Building metadata to version", version, "standard")

	// Build out the frames first so we know how long the metadata is.
	frames := m.buildFrames(version)
	if frames == nil {
		Debug("No metadata frames available")
		return nil
	}

	metadata := new(bytes.Buffer)

	// Write ID.
	metadata.WriteString("ID3")

	// Write major version.
	metadata.WriteByte(version)

	// Write minor version.
	metadata.WriteByte(0x00)

	// Write flags.
	metadata.WriteByte(0x00)

	// Write length.
	length := writeLen(len(frames), version, true)
	metadata.Write(length)

	// Write frames.
	metadata.Write(frames)

	return metadata.Bytes()
}

// buildFrames builds only the frames of the episode's metadata from the internal list of id/value pairs.
func (m *Meta) buildFrames(version byte) []byte {
	if m == nil || !m.Buffered() {
		return nil
	}
	Debug("Building metadata frames")

	buf := new(bytes.Buffer)
	for _, frame := range m.frames {
		switch version := m.Version(); version {
		case 2:
			// ID3v2.2 frame headers are 3-byte IDs and 3-byte lengths.
			if len(frame.id) != 3 {
				continue
			}

			// Write ID.
			buf.WriteString(strings.ToUpper(frame.id))

			// Write length. (+2 for encoding bytes around value.)
			length := writeLen(len(frame.value)+2, version, false)
			buf.Write(length)

			// Write value. 0x03 header with 0x00 footer indicates that the value is UTF-8. (We store everything as UTF-8.)
			buf.WriteByte(0x03)
			buf.Write(frame.value)
			buf.WriteByte(0x00)

		default:
			// v2.3 and v2.4 frame headers are 4-byte IDs, 4-byte lengths, and 2 bytes of flags.
			if len(frame.id) != 4 {
				continue
			}

			// Write ID.
			buf.WriteString(strings.ToUpper(frame.id))

			// Write length. (+2 for encoding bytes around value.)
			length := writeLen(len(frame.value)+2, version, false)
			buf.Write(length)

			// Write flags.
			buf.Write([]byte{0x00, 0x00})

			// Write value. 0x03 header with 0x00 footer indicates that the value is UTF-8. (We store everything as UTF-8.)
			buf.WriteByte(0x03)
			buf.Write(frame.value)
			buf.WriteByte(0x00)
		}
	}

	if buf.Len() == 0 {
		return nil
	}

	return buf.Bytes()
}

// parseFrames creates the internal list of all frames (represented as id/value pairs) in the metadata.
func (m *Meta) parseFrames() {
	if m.noMeta || !m.buffered || m.readFrames {
		return
	}

	buf := bytes.NewBuffer(m.buffer.Bytes())

	// Skip past ID.
	buf.Next(3)

	// Read major version.
	version, _ := buf.ReadByte()

	// Skip minor version.
	buf.ReadByte()

	flags, _ := buf.ReadByte()

	// Skip past the length.
	buf.Next(4)

	// Skip past the extended header, if present (not needed for ID3v2.2).
	if version != 2 && flags&(1<<6) > 0 {
		length := readLen(buf, version, true)
		buf.Next(length - 4)
	}

	// If we encounter any error while reading the metadata, we won't know how to continue parsing the rest of the
	// frames and will have to bail out with what we've got.
	// TODO: A good area for future development would be to enhance this, perhaps by trying to continue on until the
	// next tag is found.
	for buf.Len() > 0 {
		// Read out the frame's ID.
		id := readID(buf, version)
		if id == nil {
			Debug("Stopping frame parse early: Invalid frame ID")
			break
		}

		// Read out the frame's length.
		size := readLen(buf, version, false)
		if size <= 0 {
			Debug("Stopping frame parse early: Invalid length for", string(id), "-", size)
			break
		}

		// ID3v2.2 does not have flags in the frame header.
		if version != 2 {
			flags := buf.Next(2)
			if len(flags) != 2 {
				Debug("Stopping frame parse early: Error reading frame flags")
				break
			}

			// We only want the frame if these flags are not set.
			if flags[1]&0x0C > 0 {
				buf.Next(size)
				Debug("Skipping frame")
				continue
			}
		}

		value := buf.Next(size)
		if len(value) != size {
			Debug("Stopping frame parse early: Error reading frame value")
			break
		}

		switch value[0] {
		case 0x00:
			// ASCII characters. Remove the first byte.
			value = value[1:]
		case 0x01:
			// UTF-16 with BOM. Remove the first byte and decode to UTF-8.
			value = value[1:]
			decoder := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder()
			value, _ = decoder.Bytes(value)
		case 0x02:
			// UTF-16 Big Endian without BOM. Remove the first byte and decode to UTF-8.
			value = value[1:]
			decoder := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder()
			value, _ = decoder.Bytes(value)
		case 0x03:
			// UTF-8 (Unicode). Remove the first byte.
			value = value[1:]
		}
		value = bytes.TrimSuffix(value, []byte{0x00})

		// Debug print everything but the image bytes.
		if string(id) != "PIC" && string(id) != "APIC" {
			Debug("Found", string(id), "-", string(value))
		}
		m.frames = append(m.frames, Frame{string(id), value})
	}
}

// length returns the reported length in bytes of the entire metadata, or -1 if the metadata could not be successfully
// parsed (possibly indicating that more metadata is needed). It is not necessary to have the entire metadata buffered.
// If no metadata exists in the file's contents, this will return 0.
func (m *Meta) length() int {
	if m == nil || m.buffer == nil {
		return -1
	}
	if m.noMeta {
		return 0
	}

	buf := bytes.NewBuffer(m.buffer.Bytes())
	if buf.Len() < 3 {
		// Need more metadata to determine anything.
		return -1
	}

	if string(buf.Next(3)) != "ID3" {
		// The file has data but not any metadata.
		return 0
	}

	// Read major version.
	version, err := buf.ReadByte()
	if err != nil {
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
	length := readLen(buf, version, true)
	if length < 0 {
		return -1
	}

	// Add 10 bytes for the header.
	return length + 10
}

// readID reads the appropriate ID out of the beginning of the buffer and validates the data. This advances the buffer.
// ID3v2.2 frame IDs are 3 bytes, while v2.3 and v2.4 frame IDs are 4 bytes.
func readID(buf *bytes.Buffer, version byte) []byte {
	idLen := 4
	if version == 2 {
		idLen = 3
	}

	// Read out the id bytes, and make sure all the bytes are there.
	id := buf.Next(idLen)
	if len(id) != idLen {
		return nil
	}

	// Frame IDs can be numbers and uppercase letters.
	for _, char := range id {
		if char < '0' || char > 'Z' || (char > '9' && char < 'A') {
			return nil
		}
	}

	return id
}

// readLen reads a big-endian length out of the bytes. Header lengths are always read as synch-safe bytes (meaning that
// only the first 7 bits of each byte are used for counting, with the high bit ignored). Frame lengths are read as
// synch-safe bytes for ID3v2.2 and ID3v2.4 and regular 8-bit bytes for ID3v2.3. Additionally, ID3v2.2 frame lengths are
// only 3 bytes long.
func readLen(buf *bytes.Buffer, version byte, header bool) int {
	bufLen := 4
	if version == 2 && !header {
		bufLen = 3
	}

	// Read out the length bytes, and make sure all the bytes were there.
	bytes := buf.Next(bufLen)
	if len(bytes) != bufLen {
		return -1
	}

	width := 7
	if version == 3 && !header {
		width = 8
	}

	num := int(0)
	for _, b := range bytes {
		num <<= width
		num |= int(b)
	}

	return num
}

// writeLen converts the integer into a byte slice, big-endian. Header lengths are always stored as synch-safe bytes
// (meaning that only the first 7 bits of each byte are used for counting, with the high bit ignored). Frame lengths are
// stored as synch-safe bytes for ID3v2.2 and ID3v2.4 and regular 8-bit bytes for ID3v2.3. Additionally, ID3v2.2 frame
// lengths are only 3 bytes long.
func writeLen(n int, version byte, header bool) []byte {
	bufLen := 4
	shiftWidth := 7
	refByte := byte(0x7F)

	if version == 2 && !header {
		bufLen = 3
	}

	if version == 3 && !header {
		shiftWidth = 8
		refByte = 0xFF
	}

	buf := make([]byte, bufLen)
	for i := range buf {
		buf[bufLen-1-i] = byte(n) & refByte
		n >>= shiftWidth
	}

	return buf
}
