// Parse the links of a given URL
// Links must correspond to the same domain (option -domain)
// the program follows the links up to a maximum depth (option -depth)
// Write the bulk result to stdout by default, or to a XML formatted file (option -outfile)
// Note : URLs are sorted and in absolute format (https://domain/...)
package main

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"

	. "../links"

	"golang.org/x/net/html"
)

var (
	schemes  = [...]string{"https://", "http://"}
	mLinks   = make(map[string]Link, 0)
	maxDepth = -1
)

// Check if the provided string s begins with one of the defined schemes
func hasScheme(s, withOption string) bool {
	for _, p := range schemes {
		if strings.HasPrefix(s, p+withOption) {
			return true
		}
	}
	return false
}

// Load the page corresponding to the given URL as []byte
func loadPage(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// Filter the links with a predicate function
func filterLinks(links []Link, f func(Link) bool) []Link {
	res := make([]Link, 0)
	for _, l := range links {
		if f(l) {
			res = append(res, l)
		}
	}
	return res
}

// Update the links with a mapping function (link => f(link) Link)
func mapLinks(links []Link, f func(Link) Link) []Link {
	res := make([]Link, 0)
	for _, l := range links {
		res = append(res, f(l))
	}
	return res
}

// Check if the given string is of the correct domain => s = http://domain/... or /...
func hasDomain(s, domain string) bool {
	return len(s) > 1 && (string(s[0]) == "/" || hasScheme(s, domain+"/"))
}

// Suppress twin links.
// Return a sorted slice of unique Links
func purgeTwins(links []Link) []Link {
	uniqLinks := make([]Link, 0)
	sort.Slice(links, func(i1, i2 int) bool {
		return links[i1].Href < links[i2].Href
	})
	for _, l := range links {
		i := sort.Search(len(uniqLinks), func(i int) bool {
			return l.Href == uniqLinks[i].Href
		})
		if i >= len(uniqLinks) {
			uniqLinks = append(uniqLinks, l)
		}
	}
	return uniqLinks
}

// Parse the page at url, extracting the links with same domain, normalized and unique
func parsePage(url, domain string) ([]Link, error) {
	body, err := loadPage(url)
	if err != nil {
		return nil, err
	}
	// Load the page
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	// Find all links in the page
	var links []Link
	Parse(doc, &links)
	// Suppress links with a different domain
	filteredLinks := filterLinks(links, func(l Link) bool {
		ref := strings.TrimSpace(l.Href)
		return ref != "" && hasDomain(ref, domain)
	})
	// Normalize all links (/blabla => http://domain/blabla)
	normalizedLinks := mapLinks(filteredLinks, func(l Link) Link {
		if string(l.Href[0]) == "/" {
			return Link{Href: schemes[0] + domain + l.Href, Text: l.Text}
		}
		return l
	})
	// Suppress twins
	uniqLinks := purgeTwins(normalizedLinks)

	return uniqLinks, nil
}

// Recursive : check if each link page is already parsed, else parse it and add the url to the global map
func findAll(links []Link, domain string, depth int) error {
	for _, link := range links {
		if _, found := mLinks[link.Href]; found {
			continue
		}
		// Link not found : add it to the map
		mLinks[link.Href] = link
		// Follow the link according to depth options
		if maxDepth == -1 || depth < maxDepth {
			newLinks, err := parsePage(link.Href, domain)
			if err != nil {
				return err
			}
			findAll(newLinks, domain, depth+1)
		}
	}
	return nil
}

type url struct {
	Loc string `xml:"loc"`
}

//  Write the links in XML format in the provided filename
func writeXml(filename string) error {
	urls := make([]url, 0)
	for _, l := range mLinks {
		urls = append(urls, url{Loc: l.Href})
	}
	sort.Slice(urls, func(i1, i2 int) bool {
		return urls[i1].Loc < urls[i2].Loc
	})
	// Output to XML encoding
	output, err := xml.MarshalIndent(&urls, "  ", "   ")
	if err != nil {
		return err
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	f.Write([]byte(xml.Header))
	f.Write([]byte("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">"))
	f.Write(output)
	f.Write([]byte("</urlset>"))
	return nil
}

// Write the urls to stdout
func writeBuf() {
	fmt.Printf("Number of links : %d\n", len(mLinks))
	urls := make([]string, 0)
	for _, l := range mLinks {
		urls = append(urls, l.Href)
	}
	sort.Strings(urls)
	w := bufio.NewWriter(os.Stdout)
	fmt.Fprint(w, urls)
	w.Flush()
}

func main() {
	domain := flag.String("domain", "www.calhoun.io", "domain to parse")
	outfile := flag.String("outfile", "", "XML output file (if empty, output is written to stdout)")
	depth := flag.Int("depth", -1, "max depth number of links to follow from root page")
	flag.Parse()

	maxDepth = *depth

	fulldomain := *domain
	if !hasScheme(*domain, "") {
		fulldomain = schemes[0] + *domain
	}

	// The first link to enter the process
	links := []Link{{Href: fulldomain, Text: "Root link"}}

	// find all links
	err := findAll(links, *domain, 0)
	if err != nil {
		panic(err)
	}

	// Output : print only the link's href
	if strings.TrimSpace(*outfile) == "" {
		writeBuf()
	} else {
		writeXml(*outfile)
	}
}
