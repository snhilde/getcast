package main

import (
	"bytes"
	"fmt"
)


type Meta struct {
	buf *bytes.Buffer
}


// Build is in charge of building the metadata of the episode. There are multiple branches that can be taken, depending
// on whether or not metadata is already present in the file. If metadata already exists, then Build will buffer data
// until it has all of the metadata and then parse it, possibly modifying the contents. If metadata does not exist, then
// Build will create it. When the metadata is ready to be written to disk, it will return the complete data.
func (m *Meta) Build(p []byte) ([]byte, error) {
}
