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
	downDir := flag.String("d", "", "directory where shows will be synced")
	debugFlag := flag.Bool("debug", false, "enable debug mode")
	flag.Parse()

	if (*debugFlag) {
		DebugMode = true
	}

	// Validate (or create) the download directory.
	if err := ValidateDir(*downDir); err != nil {
		fmt.Println(err)
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Make sure we were provided something.
	urls := flag.Args()
	if len(urls) == 0 {
		fmt.Println("No shows specified")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Collect together the shows that we want to sync.
	shows := make([]Show, len(urls))
	for i, v := range urls {
		u, err := url.Parse(strings.ToLower(v))
		if err != nil {
			fmt.Println("Invalid URL:", v)
			fmt.Println(err)
			flag.PrintDefaults()
			os.Exit(1)
		}
		shows[i].URL = u
	}

	// And sync them.
	total := 0
	for _, v := range shows {
		total += v.Sync(*downDir)
	}
	fmt.Println("Synced", total, "episodes")
}
