package nameserver

import (
	"errors"
	"fmt"
	"github.com/miekg/dns"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

// Parse a URL of the form PUT /name/<identifier>/<ip-address>
func parseUrl(url string) (identifier string, ipaddr string, err error) {
	parts := strings.Split(url, "/")
	if len(parts) != 4 {
		return "", "", errors.New("Unable to parse url: " + url)
	}
	return parts[2], parts[3], nil
}

func httpErrorAndLog(level *log.Logger, w http.ResponseWriter, msg string,
	status int, logmsg string, logargs ...interface{}) {
	http.Error(w, msg, status)
	level.Printf(logmsg, logargs...)
}

func ListenHttp(domain string, db Zone, port int) {

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		} else {
			io.WriteString(w, "ok")
		}
	})

	http.HandleFunc("/name/", func(w http.ResponseWriter, r *http.Request) {

		reqError := func(msg string, logmsg string, logargs ...interface{}) {
			httpErrorAndLog(Warning, w, msg, http.StatusBadRequest,
				logmsg, logargs...)
		}

		switch r.Method {
		case "PUT":
			ident, weaveIPstr, err := parseUrl(r.URL.Path)
			name := r.FormValue("fqdn")
			if ident == "" || weaveIPstr == "" || name == "" {
				reqError("Invalid request", "Invalid request: %s, %s", r.URL, r.Form)
				return
			}

			weaveIP := net.ParseIP(weaveIPstr)
			if weaveIP == nil {
				reqError("Invalid weave IP", "Invalid weave IP in request: %s", weaveIPstr)
				return
			}

			if dns.IsSubDomain(domain, name) {
				Info.Printf("Adding %s -> %s", name, weaveIPstr)
				if err = db.AddRecord(ident, name, weaveIP); err != nil {
					if dup, ok := err.(DuplicateError); !ok {
						httpErrorAndLog(
							Error, w, "Internal error", http.StatusInternalServerError,
							"Unexpected error from DB: %s", err)
						return
					} else if dup.Ident != ident {
						http.Error(w, err.Error(), http.StatusConflict)
						return
					} // else we are golden
				}
			} else {
				Info.Printf("Ignoring name %s, not in %s", name, domain)
			}

		case "DELETE":
			ident, weaveIPstr, err := parseUrl(r.URL.Path)
			if ident == "" || weaveIPstr == "" {
				reqError("Invalid Request", "Invalid request: %s, %s", r.URL, r.Form)
				return
			}

			weaveIP := net.ParseIP(weaveIPstr)
			if weaveIP == nil {
				reqError("Invalid IP in request", "Invalid IP in request: %s", weaveIPstr)
				return
			}
			Info.Printf("Deleting %s (%s)", ident, weaveIPstr)
			if err = db.DeleteRecord(ident, weaveIP); err != nil {
				if _, ok := err.(LookupError); !ok {
					httpErrorAndLog(
						Error, w, "Internal error", http.StatusInternalServerError,
						"Unexpected error from DB: %s", err)
					return
				}
			}
		default:
			msg := "Unexpected http method: " + r.Method
			reqError(msg, msg)
			return
		}
	})

	address := fmt.Sprintf(":%d", port)
	if err := http.ListenAndServe(address, nil); err != nil {
		Error.Fatal("Unable to create http listener: ", err)
	}
}
