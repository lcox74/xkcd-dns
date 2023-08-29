package main

import "github.com/miekg/dns"

type ComicDataReq uint8

const (
	ComicTitle ComicDataReq = iota
	ComicImageURL
	ComicAltText
	ComicAll
)

type Comic struct {
	Number int
	Title  string
	Alt    string
	Image  string
}

func (c Comic) GenerateReponse(domain string, d ComicDataReq, r *dns.Msg) error {

	// Add the TXT record to the response message
	switch d {
	case ComicTitle:
		r.Answer = append(r.Answer, buildResponse(domain, c.Title))
	case ComicImageURL:
		r.Answer = append(r.Answer, buildResponse(domain, c.Image))
	case ComicAltText:
		r.Answer = append(r.Answer, buildResponse(domain, c.Alt))
	case ComicAll:
		r.Answer = append(r.Answer,
			buildResponse(domain, c.Title),
			buildResponse(domain, c.Image),
			buildResponse(domain, c.Alt),
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
