package dnsclient

import (
	"crypto/tls"
	"github.com/sirupsen/logrus"
	"net/http"
	"testing"
)

func TestNewGoogleDNS(t *testing.T) {

	client := NewGoogleDNS(func(option *Option) {
		option.ClientIP = "60.186.195.38/32"
		option.Client = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	})
	msg := client.LookupA("www.iqiyi.com")

	logrus.Info(msg)
}