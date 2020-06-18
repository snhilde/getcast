package main

import (
	"errors"
	"bytes"
	"io"
	"strings"
	"golang.org/x/text/encoding/unicode"
)


var (
	// badMeta is the error returned when a nil meta object is used.
	badMeta = errors.New("Invalid meta object")
)


// Type Meta is the main type used. It holds all the information related to the metadata.
type Meta struct {
	buffer      *bytes.Buffer // buffer to store filedata between successive Write operations
	buffered     bool         // whether or not all metadata is present in the buffer
	noMeta       bool         // whether or not the file has any metadata
	readFrames   bool         // whether or not the metadata frames have been read and parsed.
	frames     []Frame        // list of frames
}

// Type Frame is used to store information about a metadata frame.
type Frame struct {
	id      string
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
// writing to the buffer and return (n, ErrShortWrite), with n designating how many bytes were consumed in this
// operation.
func (m *Meta) Write(p []byte) (int, error) {
	if m == nil {
		return 0, badMeta
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

// ReadFrom reads from the io.Reader into the Meta object.
func (m *Meta) ReadFrom(r io.Reader) (int, error) {
	if m == nil {
		return 0, badMeta
	}

	n, err := io.Copy(m, r)
	return int(n), err
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
	if m == nil {
		return nil
	}

	return m.buffer.Bytes()
}

// Len returns the number of bytes currently buffered.
func (m *Meta) Len() int {
	if m == nil {
		return 0
	}

	return m.buffer.Len()
}

// Version returns the version of ID3v2 metadata in use, or 0 if not found.
func (m *Meta) Version() byte {
	if m == nil || m.noMeta || m.buffer.Len() < 4 {
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

// GetValues returns all values for the given frame ID. The name will be matched in a case-sensitive comparison.
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
// to have this frame ID.
func (m *Meta) SetValue(id string, value []byte, multiple bool) {
	if m == nil || !m.Buffered() {
		return
	}

	if len(id) == 4 {
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
	} else {
		Debug("Invalid frame ID:", id)
	}
}

// Build constructs the metadata for the episode's file. If the metadata cannot be constructed, this will return nil.
func (m *Meta) Build() []byte {
	if m == nil {
		return nil
	}
	Debug("Building metadata")

	version := m.Version()
	if version == 0 {
		version = 4
	}

	// Build out the frames first so we know how long the metadata is.
	frames := m.buildFrames(version)
	if frames == nil {
		Debug("No track information exists")
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
	length := writeLen(len(frames), version)
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
		if len(frame.id) != 4 {
			continue
		}

		// Write ID.
		buf.WriteString(strings.ToUpper(frame.id))

		// Write length. (+2 for encoding bytes around value.)
		length := writeLen(len(frame.value) + 2, version)
		buf.Write(length)

		// Write flags.
		buf.Write([]byte{0x00, 0x00})

		// Write value. 0x03 header with 0x00 footer indicates that the value is UTF-8. (We store everything as UTF-8.)
		buf.WriteByte(0x03)
		buf.Write(frame.value)
		buf.WriteByte(0x00)
	}

	return buf.Bytes()
}

// parseFrames creates the internal list of all frames (represented as id/value pairs) in the metadata.
func (m *Meta) parseFrames() {
	if !m.buffered || m.readFrames {
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

	// Skip past the extended header, if present.
	if flags & (1 << 6) > 0 {
		length := readLen(buf.Next(4), version)
		buf.Next(length - 4)
	}

	// If we encounter any error while reading the metadata, we have to bail out with what we've got, because we don't
	// know how to continue parsing the rest of the frames. A good area for future development would be to enhance this.
	for buf.Len() > 0 {
		// Read frame ID. The ID must be uppercase letters or numbers.
		id := buf.Next(4)
		valid := true
		for _, char := range id {
			if char < '0' || char > 'Z' || (char > '9' && char < 'A') {
				valid = false
				break
			}
		}
		if !valid {
			Debug("Stopping frame parse early: Invalid frame ID")
			break
		}

		size := readLen(buf.Next(4), version)
		if size <= 0 {
			Debug("Stopping frame parse early: Invalid length for", string(id), "-", size)
			break
		}

		flags := buf.Next(2)
		if len(flags) != 2 {
			Debug("Stopping frame parse early: Error reading frame flags")
			break
		}

		value := buf.Next(size)
		if len(value) != size {
			Debug("Stopping frame parse early: Error reading frame value")
			break
		}

		// We only want the frame if these flags are not set.
		if flags[1] & 0x0C > 0 {
			Debug("Skipping frame")
			continue
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

		Debug("Found", string(id), "-", string(value))
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
	length := readLen(buf.Next(4), version)
	if length < 0 {
		return -1
	}

	// Add 10 bytes for the header.
	return length + 10
}


// readLen reads a big-endian length out of the bytes. If the version is 4, then the length will be read as synch-safe
// bytes (meaning that only the first 7 bits of each byte are used for counting, with the high bit ignored).
func readLen(buf []byte, version byte) int {
	width := 8
	if version == 4 {
		width = 7
	}

	num := int(0)
	for _, b := range buf {
		num <<= width
		num |= int(b)
	}

	return num
}

// writeLen converts the integer into a byte slice, big-endian. If the version is 4, then the length will be written out
// as synch-safe bytes (meaning that only the first 7 bits of each byte are used for counting, with the high bit ignored).
// TODO: implement version-based length
func writeLen(n int, version byte) []byte {
	buf := new(bytes.Buffer)
	for i := 0; i < 4; i++ {
		tmp := n
		tmp >>= (3 - i) * 8
		buf.WriteByte(byte(tmp & 0xFF))
	}

	return buf.Bytes()
}
