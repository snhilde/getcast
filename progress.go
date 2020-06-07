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
	if pr == nil {
		return 0, fmt.Errorf("Invalid Progress object")
	}

	n := len(p)
	pr.have += n

	// We don't need to do expensive print operations that often.
	pr.writeCount++
	if pr.writeCount % 50 > 0 {
		return n, nil
	}

	// Clear the line and print the current status.
	fmt.Printf("\r%s", strings.Repeat(" ", 50))
	fmt.Printf("%v", pr.String())

	return n, nil
}

// String shows the current transfer status.
func (pr *Progress) String() string {
	if pr == nil {
		return "<nil>"
	}

	return fmt.Sprintf("\rReceived %v of %v total (%v%%)", Reduce(pr.have), pr.totalString, ((pr.have * 100) / pr.total))
}

// Finish cleans up the terminal line and prints the overall success of the download operation.
func (pr *Progress) Finish() {
	if pr != nil {
		// Print the final status.
		fmt.Printf("\r%s", strings.Repeat(" ", 50))
		fmt.Printf("%v", pr.String())

		// Because we've been mucking around with carriage returns, we need to manually move down a row.
		fmt.Println()

		if pr.have == pr.total {
			fmt.Println("Episode successfully downloaded")
		} else {
			if pr.have < pr.total {
				fmt.Println("Failed to download entire episode")
			} else {
				fmt.Println("Downloaded more bytes than expected")
			}
			Debug("Expected", pr.total, "bytes, Received", pr.have, "bytes")
		}
	}
}