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
	debugFlag := flag.Bool("debug", false, "Enable debug mode")
	urlArg := flag.String("u", "", "Required, URL of show's RSS feed")
	dirArg := flag.String("d", "", "Required, Main download directory for all podcasts")
	numArg := flag.String("n", "", "(Optional) Episode number to download. If podcast also has season, specify the episode like this: seasonNum-episodeNum, e.g. 3-5 to download episode 5 of season 3.")
	flag.Parse()

	if (*debugFlag) {
		DebugMode = true
		Debug("Debug mode enabled")
	}

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
	if *dirArg == "" {
		fmt.Println("No download directory specified")
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}
	if err := ValidateDir(*dirArg); err != nil {
		fmt.Println(err)
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// And sync the show.
	n, err := show.Sync(*dirArg, *numArg)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Synced", n, "episodes")
}
