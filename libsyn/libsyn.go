package libsyn

import (
	"net/url"
	"strings"
	"net/http"
	"io/ioutil"
)


// Handles determines if the provided url should be handled by this module or not.
func Handles(u *url.URL) bool {
	// The hostname will look something like this:
	// <show name>.libsyn.com
	host := u.Hostname()
	parts := strings.Split(host, ".")
	if parts[len(parts) - 2] == "libsyn" {
		return true
	}

	return false
}

// Feed grabs the raw XML of the show's RSS feed.
func Feed(u *url.URL) ([]byte, error) {

}
