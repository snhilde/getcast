package main

import (
	"flag"
	"fmt"
	"os"
	"net/url"
	"strings"
)

var (
	DebugMode bool
)


func main() {
	dirArg := flag.String("d", "", "required, main download directory for all podcasts")
	urlArg := flag.String("u", "", "required, URL of show's RSS feed")
	numArg := flag.Int("n", 0, "optional, episode number to download")
	debugFlag := flag.Bool("debug", false, "enable debug mode")
	flag.Parse()

	if (*debugFlag) {
		DebugMode = true
		Debug("Debug mode enabled")
	}

	// Make sure we were provided a URL.
	if *urlArg == "" {
		fmt.Println("No show specified")
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}
	fmt.Println("Beginning sync process")

	u, err := url.Parse(strings.ToLower(*urlArg))
	if err != nil {
		fmt.Println("Invalid URL:", err)
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}
	show := Show{URL: u}

	// Validate (or create) the download directory.
	if err := ValidateDir(*dirArg); err != nil {
		fmt.Println(err)
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// And sync them.
	n, err := show.Sync(*dirArg)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Synced", n, "episodes")
}
