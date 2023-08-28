package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/miekg/dns"
)

func main() {
	server := &dns.Server{Addr: ":53", Net: "udp"}
	dns.HandleFunc("xkcd.", handleXKCDRequest)

	// Run the DNS Server
	fmt.Println("Starting DNS Server")
	err := server.ListenAndServe()
	if err != nil {
		fmt.Println(err)
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
	fmt.Println(req)

	if req == "xkcd." {
		// Display Random Comic
		fmt.Println("Random Comic")
		handleRandomComic(w, m)
	} else {
		breakdown := strings.Split(req[:len(req)-1], ".")
		fmt.Println(breakdown, len(breakdown))

		if len(breakdown) == 2 {
			// Display Comic Number
			fmt.Println("Comic Number")

			comicNum, err := strconv.Atoi(breakdown[0])
			if err != nil {
				fmt.Println(err)
				handleRefused(w, m)
				return
			}

			handleComicNumber(comicNum, w, m)

		} else {
			// Invalid Request
			fmt.Println("Invalid Request")
			handleRefused(w, m)
		}
	}
}

func handleRefused(w dns.ResponseWriter, m *dns.Msg) {

	resp := new(dns.Msg)
	resp.SetReply(m)
	resp.SetRcode(m, dns.RcodeNameError)

	w.WriteMsg(resp)
}
func handleServerError(w dns.ResponseWriter, m *dns.Msg) {

	resp := new(dns.Msg)
	resp.SetReply(m)
	resp.SetRcode(m, dns.RcodeServerFailure)

	w.WriteMsg(resp)
}

func handleComicNumber(num int, w dns.ResponseWriter, m *dns.Msg) {
	// Make a GET request to fetch the comic page
	response, err := http.Get(fmt.Sprintf("https://xkcd.com/%d/", num))
	if err != nil {
		fmt.Println(err)
		handleServerError(w, m)
		return
	}
	defer response.Body.Close()

	// Check if the response was successful
	if response.StatusCode != http.StatusOK {
		handleRefused(w, m)
		return
	}

	comicResponse(response.Body, w, m)
}

func handleRandomComic(w dns.ResponseWriter, m *dns.Msg) {

	// Make a GET request to fetch the comic page
	response, err := http.Get("https://c.xkcd.com/random/comic/")
	if err != nil {
		fmt.Println(err)
		handleServerError(w, m)
		return
	}
	defer response.Body.Close()

	// Check if the response was successful
	if response.StatusCode != http.StatusOK {
		handleRefused(w, m)
		return
	}

	comicResponse(response.Body, w, m)
}

func comicResponse(data io.Reader, w dns.ResponseWriter, m *dns.Msg) {
	// Parse the response body using goquery
	document, err := goquery.NewDocumentFromReader(data)
	if err != nil {
		log.Fatal(err)
	}

	// Find the comic image URL and alt text
	comicImageURL, _ := document.Find("#comic img").Attr("src")
	comicAltText := document.Find("#comic img").AttrOr("title", "")
	comicTitle := document.Find("#ctitle").Text()

	// Print the comic data
	fmt.Printf("Comic Title: %s\n", comicTitle)
	fmt.Printf("Alt Text: %s\n", comicAltText)

	// Create a new DNS response message
	resp := new(dns.Msg)
	resp.SetReply(m)

	// Add the TXT record to the response message
	resp.Answer = append(resp.Answer,
		buildResponse(m.Question[0].Name, comicTitle),
		buildResponse(m.Question[0].Name, fmt.Sprintf("https:%s", comicImageURL)),
		buildResponse(m.Question[0].Name, comicAltText),
	)

	// Write the response message
	w.WriteMsg(resp)
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
