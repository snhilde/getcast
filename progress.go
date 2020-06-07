package main

import (
	"fmt"
	"strings"
)


// Progress is used to keep track during the download process and to display a progress bar during the operation.
type Progress struct {
	total       int    // total number of bytes to be downloaded
	totalString string // size of file to be downloaded, ready for printing
	have        int    // number of bytes we currently have
	writeCount  int    // running count of write operations, for determining if we should print or not
}


// Write prints the number of bytes written to stdout.
func (pr *Progress) Write(p []byte) (int, error) {
	n := len(p)
	pr.have += n

	// We don't need to do expensive print operations that often.
	pr.writeCount++
	if pr.writeCount % 50 > 0 {
		return n, nil
	}

	// Clear the line.
	fmt.Printf("\r%s", strings.Repeat(" ", 50))

	// Print the current transfer status.
	fmt.Printf("\rReceived %v of %v total (%v%%)", Reduce(pr.have), pr.totalString, ((pr.have * 100) / pr.total))

	return n, nil
}
