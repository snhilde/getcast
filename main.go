package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
)

var (
	// DebugMode signals whether or not we will print debug statements.
	DebugMode bool

	// LogFile is the file where we will write all log/debug statements.
	LogFile *os.File
)

func main() {
	urlArg := flag.String("u", "", "Required. URL of show's RSS feed")
	dirArg := flag.String("d", "", "Required. Main download directory for all podcasts")
	numArg := flag.String("n", "", "Optional. Episode number to download. If podcast also has season, specify the episode like this: seasonNum-episodeNum, e.g. 3-5 to download episode 5 of season 3.")
	logArg := flag.String("l", "", "Optional. Path to log, for writing all debug and non-debug statements")
	debugFlag := flag.Bool("v", false, "Enable debug mode")
	flag.Parse()

	if *debugFlag {
		DebugMode = true
		Debug("Debug mode enabled")
	}

	if *logArg != "" {
		if file, err := os.Create(*logArg); err != nil {
			Log("Error creating log file:", err)
		} else {
			LogFile = file
			defer LogFile.Close()
		}
	}

	if *urlArg == "" {
		Log("No show specified")
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	u, err := url.Parse(strings.ToLower(*urlArg))
	if err != nil {
		Log("Invalid URL:", err)
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}
	show := Show{URL: u}

	// Validate (or create) the download directory.
	dir := path.Clean(*dirArg)
	if dir == "" {
		Log("No download directory specified")
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}
	if err := ValidateDir(dir); err != nil {
		Log(err)
		os.Exit(1)
	}

	// And sync the show.
	Log("Beginning sync process for", show.URL)
	n, err := show.Sync(dir, *numArg)
	Log("")
	Log("Synced", n, "episodes")

	if err != nil {
		Log(err)
		os.Exit(1)
	}
}
