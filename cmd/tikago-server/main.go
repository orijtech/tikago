package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/odeke-em/tikago"
)

func envOrDefault(envVar string, alternates ...string) string {
	value := strings.TrimSpace(os.Getenv(envVar))
	if value != "" {
		return value
	}

	for _, alt := range alternates {
		trimmed := strings.TrimSpace(alt)
		if trimmed != "" {
			value = trimmed
			break
		}
	}
	return value
}

var maxAllowedBodyBytes = int64(1 << 10)
var errEmptyURLInGET = errors.New("empty \"url\" field")

func parseTikagoRequest(req *http.Request) (*tikago.Request, error) {
	defer req.Body.Close()

	lr := io.LimitReader(req.Body, maxAllowedBodyBytes)
	blob, err := ioutil.ReadAll(lr)
	log.Printf("blob: %s err: %v", blob, err)
	if err != nil {
		return nil, err
	}

	if len(blob) < 1 {
		switch req.Method {
		case "GET":
			query := req.URL.Query()
			uri := strings.TrimSpace(query.Get("url"))
			if uri == "" {
				return nil, errEmptyURLInGET
			}
			blob = []byte(fmt.Sprintf(`{"url": %q}`, uri))
		}
	}

	tikagoReq := new(tikago.Request)
	if err := json.Unmarshal(blob, tikagoReq); err != nil {
		return nil, err
	}

	return tikagoReq, nil
}

func extract(rw http.ResponseWriter, req *http.Request) {
	tikaReq, err := parseTikagoRequest(req)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	sr, err := tikaReq.Extract()
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Trailer", "X-Tikago-Extras")
	n, err := io.Copy(rw, sr)
	if n < 1 {
		if err := <-sr.Errors(); err != nil {
			rw.Header().Set("X-Tikago-Extras", err.Error())
		}
	}
}

func main() {
	http.HandleFunc("/", extract)

	server := &http.Server{
		Addr: envOrDefault("TIKAGO_SERVER_PORT", ":8899"),
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	keyFile := envOrDefault("TIKAGO_KEYFILE", "./keys/key.pem")
	certFile := envOrDefault("TIKAGO_CERTFILE", "./keys/cert.pem")

	var err error
	if envOrDefault("TIKAGO_HTTP1") != "" {
		err = server.ListenAndServe()
	} else {
		err = server.ListenAndServeTLS(certFile, keyFile)
	}

	if err != nil {
		log.Fatal(err)
	}
}
