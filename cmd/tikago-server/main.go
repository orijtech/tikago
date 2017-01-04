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

var (
	maxAllowedBodyBytes  = int64(1 << 10)
	maxMultipartFormSize = int64(1 << 20)
)

const MultipartFormFileKey = "file"

var errEmptyURLInGET = errors.New("empty \"url\" field")

func parseFromMultipartForm(req *http.Request) (*tikago.Request, error) {
	// If it is a multipart upload,
	// assume the body is entirely the upload
	if err := req.ParseMultipartForm(maxMultipartFormSize); err != nil {
		return nil, err
	}
	mpartFile, mpartHeader, err := req.FormFile(MultipartFormFileKey)
	if err != nil {
		return nil, err
	}
	tReq := &tikago.Request{
		Stdin:   mpartFile,
		Headers: http.Header(mpartHeader.Header),
	}
	tReq.SetDone(req.MultipartForm.RemoveAll)
	return tReq, nil
}

func parseTikagoRequest(req *http.Request) (*tikago.Request, error) {
	defer req.Body.Close()

	log.Printf("req: %v\n", req)
	switch {
	//  At times, the ContentType will be sent "multipart/form-data; boundary="
	// for example: Content-Type:[multipart/form-data; boundary=------------------------6c9a36cd5c43e3fd]
	// We should still be able to recognize that request as a multipart upload.
	case strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data"):
		return parseFromMultipartForm(req)
	}

	lr := io.LimitReader(req.Body, maxAllowedBodyBytes)
	blob, err := ioutil.ReadAll(lr)
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
		default:
			return nil, fmt.Errorf("no body passed in for method %q", req.Method)
		}
	}

	tikagoReq := new(tikago.Request)
	if err := json.Unmarshal(blob, tikagoReq); err != nil {
		return nil, err
	}

	return tikagoReq, nil
}

func extract(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Trailer", "X-Tikago-Extras")
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
