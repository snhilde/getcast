![Maintenance Badge](https://img.shields.io/badge/Maintained-yes-success)
![Version Badge](https://img.shields.io/badge/Version-1.3-informational)
[![GoReportCard example](https://goreportcard.com/badge/github.com/snhilde/getcast)](https://goreportcard.com/report/github.com/snhilde/getcast)

# getcast
`getcast` is a utility for archiving podcasts.

## Introduction
`getcast` syncs local show repositories with episodes currently available online. You tell it where the podcasts are synced locally and supply it with a show's RSS feed, and it either grabs the latest episodes or syncs all episodes if it can't determine episode numbers. `getcast` includes native support for ID3v2 metadata (version 2.2, 2.3, and 2.4) and augments the file information with information skimmed from the RSS feed.

## Usage
1. Download the repository:
`git clone https://github.com/snhilde/getcast`
2. Build and install:
`cd getcast`
`go install`
(If you need the go tools, you can [grab them here](https://golang.org/doc/install)).
3. Run the program:
`getcast -d [path to podcasts] -u [URL of RSS feed]`

### Options
* `-d` Main download directory for all podcasts (Required)
* `-h` Help screen
* `-l` Log file for logging all regular and debug messages
* `-n` Episode number to download, or `x-y` to download episode `y` of season `x`
* `-u` URL of show's RSS feed (Required)
* `-v` Verbose mode
