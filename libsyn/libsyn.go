package libsyn

import (
	"net/http"
	"fmt"
	"io"
	"io/ioutil"
	"encoding/xml"
	"strconv"
	"regexp"
)


// Show is the main type. It holds information about the podcast and all of the available episodes.
type Show struct {
	url         string  // This is the provided URL for the show's main Libsyn page.
	ShowTitle   string  `xml:"channel>title"`
	Episodes  []episode `xml:"channel>item"`
}

// Episode holds info for each episode of the podcast.
type episode struct {
	number int
	title  string
	link   string
}


// New returns a new object for the Libsyn podcast.
func New(url string) *Show {
	if url == "" {
		fmt.Println("Missing URL to show's main Libsyn page")
		return nil
	}

	show := new(Show)
	show.url = url

	return show
}


// Build reads the RSS feed for the show to determine metadata about the show and information for all of the available
// episodes.
func (s *Show) Build() error {
	// Grab the show's RSS feed.
	resp, err := http.Get(s.url + "/rss")
	if err != nil {
		return fmt.Errorf("Invalid show homepage: %v", err)
	}
	defer resp.Body.Close()

	// Pull out the content.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Failed to read data: %v", err)
	}

	// Parse the RSS feed into an XML doc.
	if err := xml.Unmarshal(body, s); err != nil {
		return fmt.Errorf("Error parsing xml: %v", err)
	}

	if len(s.Episodes) == 0 {
		return fmt.Errorf("Invalid show homepage: No episodes found")
	}

	// This might not be necessary, but let's just do a quick check on the episode numbers. If a number wasn't declared
	// in the RSS feed, we'll try to pull it out from the episode's title. This will make parsing easier in the main
	// module.
	for _, v := range s.Episodes {
		if v.number == 0 {
			// Could actually be episode number 0. Might want to rework this.
			v.guessNum()
			if v.number > 0 {
				v.title = fmt.Sprintf("%v-%v", v.number, v.title)
			}
		}
	}

	// In the RSS feed, the episodes are in reverse-sorted order. Let's reverse that to make downloading and error
	// handling easier later.
	num := len(s.Episodes)
	for i := 0; i < num/2; i++ {
		s.Episodes[i], s.Episodes[num - i - 1] = s.Episodes[num - i - 1], s.Episodes[i]
	}

	return nil
}

// Title returns the title of the show.
func (s *Show) Title() string {
	if s == nil {
		return ""
	}

	return s.ShowTitle
}

// Available returns the number of episodes currently available for download.
func (s *Show) Available() int {
	if s == nil {
		return 0
	}

	return len(s.Episodes)
}

// TitleOf returns the title of the episode at the provided index of the internal episode list.
func (s *Show) TitleOf(i int) string {
	if s == nil || i < 0 || i > len(s.Episodes) {
		return ""
	}

	return s.Episodes[i].title
}

// NumberOf returns the episode number of the episode at the provided index of the internal episode list.
func (s *Show) NumberOf(i int) int {
	if s == nil || i < 0 || i > len(s.Episodes) {
		return -1
	}

	return s.Episodes[i].number
}

// LinkOf returns the download URL for the episode at the provided index of the internal episode list.
func (s *Show) LinkOf(i int) string {
	if s == nil || i < 0 || i > len(s.Episodes) {
		return ""
	}

	return s.Episodes[i].link
}


// Due to namespaces, we have to manually unmarshal the episode data. Currently, this has only been observed with
// <title>. There are two title tags returned: a general one, and one in the "itunes" namespace. The general tag usually
// an episode number, while the other one does not. It will make everything easier if we save the podcast with the
// episode number in the filename, but the parser reads the "itunes" namespace tag after the general one, thus
// clobbering the desired title. We'll grab the correct one here.
func (e *episode) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	if e == nil {
		return fmt.Errorf("Invalid receiver")
	}

	// Scan through all of the tokens.
	for t, err := d.Token(); err != io.EOF; t, err = d.Token() {
		if err != nil {
			return err
		}

		// When we find a start tag, we'll see if we have one of the three tags that we want to save.
		if s, ok := t.(xml.StartElement); ok {
			// xml.StartElement has two fields: Name and Attr. We only care about the name, of which there are two
			// parts: Tag name (Local) and namespace (Space).
			switch (s.Name.Local) {
			case "title":
				// We only want the <title> tag without a namespace.
				if s.Name.Space == "" {
					// Grab the text between the opening and closing tags.
					n, err := d.Token()
					if err != nil {
						return err
					}
					if cd, ok := n.(xml.CharData); ok {
						e.title = string(cd)
					}
				}
			case "link":
				// We don't care about namespaces for this tag.
				n, err := d.Token()
				if err != nil {
					return err
				}
				if cd, ok := n.(xml.CharData); ok {
					e.link = string(cd)
				}
			case "episode":
				// For this tag, we actually want the namespace. Currently, I have only seen an itunes namespace, but
				// we'll allow any others that pop up.
				if s.Name.Space != "" {
					n, err := d.Token()
					if err != nil {
						return err
					}
					if cd, ok := n.(xml.CharData); ok {
						if num, err := strconv.Atoi(string(cd)); err != nil {
							return err
						} else {
							e.number = num
						}
					}
				}
			}
		}
	}

	return nil
}

// guessNum attempts to parse out the episode's number from it's title. If something is found, it will be saved in the
// object's number field.Currently, it will grab the first number it sees. Enhancing this would be a good area for
// future development.
var reNum = regexp.MustCompile("^[[:print:]]*?([[:digit:]]+)[[:print:]]*$")
func (e *episode) guessNum() {
	matches := reNum.FindStringSubmatch(e.title)
	if len(matches) < 2 {
		return
	}

	// The first item will be the match's title, and the second will be the number found.
	if number, err := strconv.Atoi(matches[1]); err != nil {
		e.number = number
	}
}
