package libsyn

import (
	"net/http"
	"io/ioutil"
)


// Show is the main type. It holds information necessary to download the show's RSS feed.
type Show struct {
	url string
}


func Init(url string) *Show {
	s := new(Show)
	s.url = url

	return s
}

func (s *Show) Feed() []byte {
	// Grab the show's RSS feed.
	resp, err := http.Get(s.url + "/rss")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	// Pull out the content.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	return body
}
