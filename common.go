package main

import (
	"os"
	"fmt"
	"path/filepath"
	"syscall"
	"regexp"
	"strconv"
	"math"
)

// ValidateDir checks that these things are true about the provided directory:
// - Path is an existing directory. If it isn't, we'll create it.
// - Directory is either the main directory or the show's directory.
// - Directory has read permissions
// - Directory has write permissions
func ValidateDir(path string) error {
	// Make sure the path is valid.
	info, err := os.Stat(path)
	if err != nil {
		// We'll assume the error is because the directory does not exist. We'll try to create it here and let other
		// possible errors flow from that.
		return os.MkdirAll(path, 0755)
	}

	// Make sure the path is a directory.
	if !info.IsDir() {
		return fmt.Errorf("%v is not a directory", filepath.Base(path))
	}

	// Make sure we have read and write permissions to the directory. This is more of an early sanity check to get a
	// better idea of what could be wrong and not an actual perms check. We won't fail here if anything goes wrong
	// getting the permissions values, but we will fail if the perms don't match.
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		// Check if we match the directory's user or group.
		isUser := os.Getuid() == int(stat.Uid)
		isGroup := os.Getgid() == int(stat.Gid)

		// Find out which of the directory's user, group, and other read bits are set.
		perms := info.Mode().Perm() & os.ModePerm
		uRead := perms & (1 << 8) > 0
		gRead := perms & (1 << 5) > 0
		oRead := perms & (1 << 2) > 0

		// Check for read permission.
		if !(isUser && uRead) && !(isGroup && gRead) && !oRead {
			return fmt.Errorf("Cannot read %v", path)
		}

		// Find out which of the directory's user, group, and other write bits are set.
		uWrite := perms & (1 << 7) > 0
		gWrite := perms & (1 << 4) > 0
		oWrite := perms & (1 << 1) > 0

		// Check for write permission.
		if !(isUser && uWrite) && !(isGroup && gWrite) && !oWrite {
			return fmt.Errorf("Cannot write to %v", path)
		}
	}

	return nil
}

// GuessNum attempts to parse out the episode's number from its title. If something is found, it will be returned. If
// nothing is found, -1 will be returned. Currently, it will grab the first number it sees. Enhancing this would be a
// good area for future development.
var reNum = regexp.MustCompile("^[[:print:]]*?([[:digit:]]+)[[:print:]]*$")
func GuessNum(title string) int {
	matches := reNum.FindStringSubmatch(title)
	if len(matches) < 2 {
		return -1
	}

	// The first item will be the matching title, and the second will be the number found.
	number, err := strconv.Atoi(matches[1])
	if err != nil {
		return -1
	}

	return number
}

// Reduce converts the number of bytes into its human-readable value (less than 1024) with SI unit suffix appended.
var units = []string{"B", "K", "M", "G"}
func Reduce(n int) string {
	if n <= 0 {
		return "0B"
	}

	index := int(math.Log2(float64(n))) / 10
	n >>= (10 * index)

	return strconv.Itoa(n) + units[index]
}
