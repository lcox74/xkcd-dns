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

type ComicData uint8

const (
	ComicTitle ComicData = iota
	ComicImageURL
	ComicAltText
	ComicAll
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.InfoLevel)

	server := &dns.Server{Addr: ":53", Net: "udp"}
	dns.HandleFunc("xkcd.", handleXKCDRequest)

	// Run the DNS Server
	log.Info("Starting DNS Server")
	err := server.ListenAndServe()
	if err != nil {
		log.WithError(err).Fatal("DNS Server Shutdown")
	}
}

func handleXKCDRequest(w dns.ResponseWriter, m *dns.Msg) {
	if m.MsgHdr.Response {
		return
	}

	if len(m.Question) > 1 {
		return
	}

	req := m.Question[0].Name
	resp := new(dns.Msg)
	resp.SetReply(m)

	err := parseRequest(req, m, resp)

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

func parseRequest(req string, m *dns.Msg, r *dns.Msg) error {
	// Handle Random Request
	switch req {
	case "xkcd.":
		// Display Random Comic
		return handleRandomComic(m, ComicAll, r)
	case "title.xkcd.":
		return handleRandomComic(m, ComicTitle, r)
	case "img.xkcd.":
		return handleRandomComic(m, ComicImageURL, r)
	case "alt.xkcd.":
		return handleRandomComic(m, ComicAltText, r)
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

		handleComicNumber(comicNum, m, ComicAll, r)
	}

	comicNum, err := strconv.Atoi(breakdown[1])
	if err != nil {
		handleRefused(m, r)
		return err
	}

	switch breakdown[0] {
	case "title":
		return handleComicNumber(comicNum, m, ComicTitle, r)
	case "img":
		return handleComicNumber(comicNum, m, ComicImageURL, r)
	case "alt":
		return handleComicNumber(comicNum, m, ComicAltText, r)
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

func handleComicNumber(num int, m *dns.Msg, d ComicData, r *dns.Msg) error {
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

	comicResponse(response.Body, m, d, r)
	return nil
}

func handleRandomComic(m *dns.Msg, d ComicData, r *dns.Msg) error {

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

	return comicResponse(response.Body, m, d, r)
}

func comicResponse(data io.Reader, m *dns.Msg, d ComicData, r *dns.Msg) error {
	// Parse the response body using goquery
	document, err := goquery.NewDocumentFromReader(data)
	if err != nil {
		handleServerError(m, r)
		return err
	}

	// Find the comic image URL and alt text
	comicImageURL, _ := document.Find("#comic img").Attr("src")
	comicAltText := document.Find("#comic img").AttrOr("title", "")
	comicTitle := document.Find("#ctitle").Text()

	// Add the TXT record to the response message
	switch d {
	case ComicTitle:
		r.Answer = append(r.Answer, buildResponse(m.Question[0].Name, comicTitle))
	case ComicImageURL:
		r.Answer = append(r.Answer, buildResponse(m.Question[0].Name, fmt.Sprintf("https:%s", comicImageURL)))
	case ComicAltText:
		r.Answer = append(r.Answer, buildResponse(m.Question[0].Name, comicAltText))
	case ComicAll:
		r.Answer = append(r.Answer,
			buildResponse(m.Question[0].Name, comicTitle),
			buildResponse(m.Question[0].Name, fmt.Sprintf("https:%s", comicImageURL)),
			buildResponse(m.Question[0].Name, comicAltText),
		)
	}

	return nil
}

func buildResponse(name, data string) dns.RR {
	return &dns.TXT{
		Hdr: dns.RR_Header{
			Name:   name,
			Rrtype: dns.TypeTXT,
			Class:  dns.ClassINET,
			Ttl:    0,
		},
		Txt: []string{data},
	}
}
