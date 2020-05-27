package main

import (
	"fmt"
	"strings"
)


// ProgressBar is used to display a progress bar during the download operation.
type ProgressBar struct {
	total       int    // total number of bytes to be downloaded
	totalString string // size of file to be downloaded, ready for printing
	have        int    // number of bytes we currently have
	count       int    // running count of write operations, for determining if we should print or not
}


// Write prints the number of bytes written to stdout.
func (pr *ProgressBar) Write(p []byte) (int, error) {
	n := len(p)
	pr.have += n

	// We don't need to do expensive print operations that often.
	pr.count++
	if pr.count % 50 > 0 {
		return n, nil
	}

	// Clear the line.
	fmt.Printf("\r%s", strings.Repeat(" ", 50))

	// Print the current transfer status.
	fmt.Printf("\rReceived %v of %v total (%v%%)", Reduce(pr.have), pr.totalString, ((pr.have * 100) / pr.total))

	return n, nil
}
