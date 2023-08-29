package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/miekg/dns"

	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.InfoLevel)

	// New Cache
	comicCache := NewCache[Comic]()

	server := &dns.Server{Addr: ":53", Net: "udp"}
	dns.HandleFunc("xkcd.", func(w dns.ResponseWriter, m *dns.Msg) {
		handleXKCDRequest(comicCache, w, m)
	})

	// Run the DNS Server
	log.Info("Starting DNS Server")
	err := server.ListenAndServe()
	if err != nil {
		log.WithError(err).Fatal("DNS Server Shutdown")
	}
}

func handleXKCDRequest(cache *Cache, w dns.ResponseWriter, m *dns.Msg) {
	if m.MsgHdr.Response {
		return
	}

	if len(m.Question) > 1 {
		return
	}

	req := m.Question[0].Name
	resp := new(dns.Msg)
	resp.SetReply(m)

	err := parseRequest(cache, req, m, resp)

	if err != nil {
		log.WithFields(log.Fields{
			"question": m.Question[0].Name,
			"remote":   w.RemoteAddr().String(),
			"result":   resp.Rcode,
			"answer":   resp.Answer,
		}).WithError(err).Error("DNS Request")
	} else {
		log.WithFields(log.Fields{
			"question": m.Question[0].Name,
			"remote":   w.RemoteAddr().String(),
			"answer":   resp.Answer,
		}).Info("DNS Request")
	}

	// Write the response message
	w.WriteMsg(resp)
}

func parseRequest(cache *Cache, req string, m *dns.Msg, r *dns.Msg) error {
	// Handle Random Request
	switch req {
	case "xkcd.":
		// Display Random Comic
		return handleRandomComic(cache, m, ComicAll, r)
	case "title.xkcd.":
		return handleRandomComic(cache, m, ComicTitle, r)
	case "img.xkcd.":
		return handleRandomComic(cache, m, ComicImageURL, r)
	case "alt.xkcd.":
		return handleRandomComic(cache, m, ComicAltText, r)
	}

	// Check if Valid Comic Number Request
	regex := regexp.MustCompile(`^((title|img|alt)\.)?\d+\.xkcd\.`)
	if !regex.MatchString(req) {
		handleRefused(m, r)
		return fmt.Errorf("invalid request: %s", req)
	}

	breakdown := strings.Split(req[:len(req)-1], ".")
	if len(breakdown) == 2 {
		comicNum, err := strconv.Atoi(breakdown[0])
		if err != nil {
			handleRefused(m, r)
			return fmt.Errorf("invalid request: %s", req)
		}

		handleComicNumber(cache, comicNum, m, ComicAll, r)
	}

	comicNum, err := strconv.Atoi(breakdown[1])
	if err != nil {
		handleRefused(m, r)
		return err
	}

	switch breakdown[0] {
	case "title":
		return handleComicNumber(cache, comicNum, m, ComicTitle, r)
	case "img":
		return handleComicNumber(cache, comicNum, m, ComicImageURL, r)
	case "alt":
		return handleComicNumber(cache, comicNum, m, ComicAltText, r)
	default:
		handleRefused(m, r)
		return fmt.Errorf("invalid request: %s", req)
	}
}

func handleRefused(m *dns.Msg, r *dns.Msg) {
	r.SetRcode(m, dns.RcodeNameError)
}
func handleServerError(m *dns.Msg, r *dns.Msg) {
	r.SetRcode(m, dns.RcodeServerFailure)
}

func handleComicNumber(cache *Cache, num int, m *dns.Msg, d ComicDataReq, r *dns.Msg) error {

	if v, ok := cache.Get(num); ok {
		return v.GenerateReponse(m.Question[0].Name, d, r)
	}

	// Make a GET request to fetch the comic page
	response, err := http.Get(fmt.Sprintf("https://xkcd.com/%d/", num))
	if err != nil {
		handleServerError(m, r)
		return err
	}
	defer response.Body.Close()

	// Check if the response was successful
	if response.StatusCode != http.StatusOK {
		handleRefused(m, r)
		return err
	}

	comic, err := comicExtract(cache, response.Body)
	if err != nil {
		handleServerError(m, r)
		return err
	}

	return comic.GenerateReponse(m.Question[0].Name, d, r)
}

func handleRandomComic(cache *Cache, m *dns.Msg, d ComicDataReq, r *dns.Msg) error {

	// Make a GET request to fetch the comic page
	response, err := http.Get("https://c.xkcd.com/random/comic/")
	if err != nil {
		handleServerError(m, r)
		return err
	}
	defer response.Body.Close()

	// Check if the response was successful
	if response.StatusCode != http.StatusOK {
		handleRefused(m, r)
		return err
	}

	comic, err := comicExtract(cache, response.Body)
	if err != nil {
		handleServerError(m, r)
		return err
	}

	return comic.GenerateReponse(m.Question[0].Name, d, r)
}

func comicExtract(cache *Cache, data io.Reader) (Comic, error) {
	// Parse the response body using goquery
	document, err := goquery.NewDocumentFromReader(data)
	if err != nil {
		return Comic{}, err
	}

	// Find the comic image URL and alt text
	comicImageURL, _ := document.Find("#comic img").Attr("src")
	comicAltText := document.Find("#comic img").AttrOr("title", "")
	comicTitle := document.Find("#ctitle").Text()

	// Find the comic number
	comicNum := 0
	metaTag := document.Find(`meta[property="og:url"]`)
	for _, n := range metaTag.Nodes {
		for _, a := range n.Attr {
			if a.Key == "content" {
				// Extract the comic number from the URL
				// https://xkcd.com/614/ -> 614
				split := strings.Split(a.Val, "/")
				if len(split) != 5 {
					continue
				}

				comicNum, err = strconv.Atoi(strings.Split(a.Val, "/")[3])
				if err != nil {
					continue
				}

				break
			}
		}
	}

	if comicNum == 0 {
		return Comic{}, fmt.Errorf("unable to extract comic number")
	}

	// Cache the Comic Data
	if _, ok := cache.Get(comicNum); !ok {

		cache.Set(comicNum, Comic{
			Number: comicNum,
			Title:  fmt.Sprintf("%d: %s", comicNum, comicTitle),
			Alt:    comicAltText,
			Image:  fmt.Sprintf("https:%s", comicImageURL),
		})

		log.Infof("Cached Comic %d", comicNum)
	} else {
		log.Infoln("Comic already cached")
	}

	v, _ := cache.Get(comicNum)
	return v, nil
}
