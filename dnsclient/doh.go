package dnsclient

import (
	"bytes"
	"encoding/json"
	"github.com/go-resty/resty/v2"
	"github.com/gogf/gf/util/gconv"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"net"
	"strconv"
	"strings"
)

type DoH struct {
	option *Option
}

func (c *DoH) Lookup(name string, rType uint16) *dns.Msg {

	ret := new(dns.Msg)
	ret.SetQuestion(name, rType)
	c.LookupAppend(ret, name, rType)

	return ret
}

func (c *DoH) getClient() *resty.Client {
	return resty.
		NewWithClient(c.option.Client).
		SetDebug(true)
}

func (c *DoH) handlerRR(item *DohCommon) (tmp dns.RR) {

	switch gconv.Uint16(item.Type) {
	case dns.TypeA:
		tmp = &dns.A{
			A: net.ParseIP(item.Data),
		}
	case dns.TypeAAAA:
		tmp = &dns.AAAA{
			AAAA: net.ParseIP(item.Data),
		}
	case dns.TypeTXT:
		txt, err := strconv.Unquote(item.Data)
		if err != nil {
			logrus.Error(err)
			return
		}
		tmp = &dns.TXT{
			Txt: []string{txt},
		}
	case dns.TypeCNAME:
		tmp = &dns.CNAME{
			Target: item.Data,
		}
	case dns.TypeSOA:
		s := strings.Split(item.Data, " ")
		if len(s) < 7 {
			return
		}
		tmp = &dns.SOA{
			Ns:      s[0],
			Mbox:    s[1],
			Serial:  gconv.Uint32(s[2]),
			Refresh: gconv.Uint32(s[3]),
			Retry:   gconv.Uint32(s[4]),
			Expire:  gconv.Uint32(s[5]),
			Minttl:  gconv.Uint32(s[6]),
		}

	}

	return
}

func (c *DoH) LookupAppend(r *dns.Msg, name string, rType uint16) {

	res, err := c.getClient().R().
		EnableTrace().
		SetHeaders(map[string]string{
			"accept": "application/dns-json",
		}).
		SetQueryParams(map[string]string{
			"name": name,
			"type": dns.TypeToString[rType],
			"cd":   "false", // ignore DNSSEC
			"do":   "false", // ignore DNSSEC
		}).
		Get(c.option.Endpoint)
	if err != nil {
		logrus.Fatal(err)
	}

	var resp DohResponse
	if err := json.NewDecoder(bytes.NewReader(res.Body())).Decode(&resp); err != nil {
		logrus.Fatal(err)
	}

	for _, item := range resp.Answer {
		if tmp := c.handlerRR(item); tmp != nil {
			tmp.Header().Name = dns.Fqdn(item.Name)
			tmp.Header().Rrtype = gconv.Uint16(item.Type)
			tmp.Header().Class = dns.ClassINET
			tmp.Header().Ttl = gconv.Uint32(item.TTL)
			r.Answer = append(r.Answer, tmp)
		}
	}

	for _, item := range resp.Authority {
		if tmp := c.handlerRR(item); tmp != nil {
			tmp.Header().Name = dns.Fqdn(item.Name)
			tmp.Header().Rrtype = gconv.Uint16(item.Type)
			tmp.Header().Class = dns.ClassINET
			tmp.Header().Ttl = gconv.Uint32(item.TTL)
			r.Ns = append(r.Ns, tmp)
		}
	}

}

func (c *DoH) LookupTXT(name string) *dns.TXT {
	return nil
}

func (c *DoH) LookupA(name string) (result []*dns.A) {

	result = make([]*dns.A, 0)

	res, err := resty.NewWithClient(c.option.Client).
		SetDebug(true).
		R().
		EnableTrace().
		SetHeaders(map[string]string{
			"accept": "application/dns-json",
		}).
		SetQueryParams(map[string]string{
			"name": name,
			"type": "A",
			"cd":   "false", // ignore DNSSEC
			"do":   "false", // ignore DNSSEC
		}).
		Get(c.option.Endpoint)
	if err != nil {
		logrus.Fatal(err)
	}

	var resp DohResponse
	if err := json.NewDecoder(bytes.NewReader(res.Body())).Decode(&resp); err != nil {
		logrus.Fatal(err)
	}

	for _, answer := range resp.Answer {

		if gconv.Uint16(answer.Type) != dns.TypeA {
			continue
		}

		result = append(result, &dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(answer.Name),
				Rrtype: gconv.Uint16(answer.Type),
				Class:  dns.ClassINET,
				Ttl:    gconv.Uint32(answer.TTL),
			},
			A: net.ParseIP(answer.Data),
		})
	}

	return
}

// requestResponse contains the response from a DNS query.
// Both Google and Cloudflare seem to share a scheme here. As in:
//	https://tools.ietf.org/id/draft-bortzmeyer-dns-json-01.html
//
// https://developers.google.com/speed/public-dns/docs/dns-over-https#dns_response_in_json
// https://developers.cloudflare.com/1.1.1.1/dns-over-https/json-format/
type DohResponse struct {
	Status   int  `json:"Status"` // 0=NOERROR, 2=SERVFAIL - Standard DNS response code (32 bit integer)
	TC       bool `json:"TC"`     // Whether the response is truncated
	RD       bool `json:"RD"`     // Always true for Google Public DNS
	RA       bool `json:"RA"`     // Always true for Google Public DNS
	AD       bool `json:"AD"`     // Whether all response data was validated with DNSSEC
	CD       bool `json:"CD"`     // Whether the dnsclient asked to disable DNSSEC
	Question []struct {
		Name string `json:"name"` // FQDN with trailing dot
		Type int    `json:"type"` // Standard DNS RR type
	} `json:"Question"`
	Answer           []*DohCommon  `json:"Answer"`
	Authority        []*DohCommon  `json:"Authority"`
	Additional       []interface{} `json:"Additional"`
	EdnsClientSubnet string        `json:"edns_client_subnet"` // IP address / scope prefix-length
	Comment          string        `json:"Comment"`            // Diagnostics information in case of an error
}

type DohCommon struct {
	Name string `json:"name"` // Always matches name in the Question section
	Type int    `json:"type"` // Standard DNS RR type
	TTL  int    `json:"TTL"`  // Record's time-to-live in seconds
	Data string `json:"data"` // Data
}