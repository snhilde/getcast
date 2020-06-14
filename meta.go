package main

import (
	"errors"
	"bytes"
	"fmt"
	"io"
	"strings"
	"strconv"
	"golang.org/x/text/encoding/unicode"
	"unsafe"
)


var (
	badMeta = errors.New("Invalid meta object")
)


type Meta struct {
	buffer   *bytes.Buffer      // buffer to store filedata between successive Write operations
	buffered  bool              // whether or not all metadata is present in the buffer
	noMeta    bool              // whether or not the file has any metadata
	frames    map[string][]byte // cached dictionary of frames
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

// GetFrame returns the value of the specified frame, or nil if no frame exists with that name. The name will be matched
// in a case-sensitive comparison.
func (m *Meta) GetFrame(frame string) []byte {
	if m == nil || !m.Buffered() {
		return nil
	}

	// If we haven't cached the frames yet, do so now.
	if m.frames == nil {
		if err := m.parseFrames(); err != nil {
			Debug("Failed to parse frames:", err)
			return nil
		}
		if m.frames == nil {
			Debug("Didn't find any frames")
			m.frames = make(map[string][]byte)
		}
	}

	return m.frames[frame]
}

// SetFrame sets the frame with the value. Value should be UTF-8 encoded.
func (m *Meta) SetFrame(frame string, value []byte) {
	if m == nil || !m.Buffered() {
		return
	}

	// If we haven't cached the frames yet, do so now.
	if m.frames == nil {
		if err := m.parseFrames(); err != nil {
			Debug("Failed to parse frames:", err)
			return
		}
		if m.frames == nil {
			Debug("Didn't find any frames")
			m.frames = make(map[string][]byte)
		}
	}

	if len(frame) == 4 {
		frame = strings.ToUpper(frame)
		m.frames[frame] = value
		Debug("Set frame", frame, "to", string(value))
	}
}

// Build constructs the metadata for the episode's file. If the metadata cannot be constructed, this will return nil.
func (m *Meta) Build() []byte {
	if m == nil {
		return nil
	}
	Debug("Building metadata")

	// Build out the frames first so we know how long the metadata is.
	frames := m.buildFrames()
	if frames == nil {
		Debug("No track information exists")
		return nil
	}

	metadata := new(bytes.Buffer)

	// Write ID.
	metadata.WriteString("ID3")

	// Write major version.
	version := m.Version()
	if version == "" {
		version = "4"
	}
	v, _ := strconv.Atoi(version)
	metadata.WriteByte(byte(v))

	// Write minor version.
	metadata.WriteByte(0x00)

	// Write flags.
	metadata.WriteByte(0x00)

	// Write length.
	length := writeLen(len(frames))
	metadata.Write(length)

	// Write frames.
	metadata.Write(frames)

	return metadata.Bytes()
}


// buildFrames builds only the frames of the episode's metadata from the internal dictionary of id/value pairs.
func (m *Meta) buildFrames() []byte {
	if m == nil || !m.Buffered() {
		return nil
	}
	Debug("Building metadata frames")

	if m.frames == nil {
		if err := m.parseFrames(); err != nil {
			Debug("Failed to parse existing frames")
			return nil
		}
	}

	if len(m.frames) == 0 {
		Debug("No frames to build")
		return nil
	}

	buf := new(bytes.Buffer)
	for id, value := range m.frames {
		if len(id) != 4 {
			continue
		}

		// Write ID.
		buf.WriteString(strings.ToUpper(id))

		// Write length. (+2 for encoding bytes around value.)
		length := writeLen(len(value) + 2)
		buf.Write(length)

		// Write flags.
		buf.Write([]byte{0x00, 0x00})

		// Write value. (0x03 header with 0x00 footer indicates that the value is UTF-8. We store everything as UTF-8)
		buf.WriteByte(0x03)
		buf.Write(value)
		buf.WriteByte(0x00)
	}

	return buf.Bytes()
}

// parseFrames creates the internal dictionary of all frames (represented as id/value pairs) in the metadata. If no
// metadata is present or metadata is present but no frames exist, this will create an empty, non-nil dictionary.
func (m *Meta) parseFrames() error {
	if !m.Buffered() {
		return fmt.Errorf("Missing metadata to parse")
	} else if m.noMeta {
		return nil
	} else if m.frames != nil {
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
		length := readNum(buf.Next(4))
		buf.Next(length - 4)
	}

	// If we encounter any error while reading the metadata, we have to bail out with what we've got because we dont'
	// know how to continue parsing the rest of the frames. A good area for future development would be to enhance this.
	m.frames = make(map[string][]byte)
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

		size := readNum(buf.Next(4))
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

		Debug("Found", string(id), "-", string(value))
		fmt.Println(string(id), size, string(value))
		m.frames[string(id)] = value
	}

	return nil
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

	// Skip major version.
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
	length := readNum(buf.Next(4))
	if length < 0 {
		return -1
	}

	// Add 10 bytes for the header.
	return length + 10
}


// readNum reads a big-endian number out of the bytes. If the number of bytes is greater than the memory size of int,
// this will return -1.
func readNum(buf []byte) int {
	num := int(0)
	if len(buf) > int(unsafe.Sizeof(num)) {
		return -1
	}

	for _, b := range buf {
		num <<= 8
		num |= int(b)
	}

	return num
}

// writeLen converts the integer into a byte slice, big-endian.
func writeLen(n int) []byte {
	buf := new(bytes.Buffer)
	for i := 0; i < 4; i++ {
		tmp := n
		tmp >>= (3 - i) * 8
		buf.WriteByte(byte(tmp & 0xFF))
	}

	return buf.Bytes()
}
