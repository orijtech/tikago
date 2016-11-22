package tikago

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func ThisFilepath() string {
	_, filepath, _, _ := runtime.Caller(0)
	return filepath
}

func ThisFileDir() string {
	absThisFilepath := ThisFilepath()
	return filepath.Dir(absThisFilepath)
}

type Request struct {
	URL string `json:"url"`

	RoundTripper http.RoundTripper `json:"-"`
}

type Response struct {
	Text     string `json:"text,omitempty"`
	Language string `json:"language"`
}

// Validate checks if a Request has the
// necessary attributes set for it to be used.
func (req *Request) Validate() error {
	source := strings.TrimSpace(req.URL)
	var errsList []string
	if source == "" {
		errsList = append(errsList, "expecting \"url\"")
	}

	if len(errsList) < 1 {
		return nil
	}

	msg := strings.Join(errsList, "\n")
	return errors.New(msg)
}

var tikaSource = filepath.Join(ThisFileDir(), "engine", "tika-app", "target", "tika-app-1.15-SNAPSHOT.jar")

func (req *Request) fetch() (*http.Response, error) {
	if req.RoundTripper == nil {
		return http.Get(req.URL)
	}

	httpReq, err := http.NewRequest("GET", req.URL, nil)
	if err != nil {
		return nil, err
	}

	return req.RoundTripper.RoundTrip(httpReq)
}

type StreamResult struct {
	r        io.Reader
	execErrs chan error
}

var _ io.Reader = (*StreamResult)(nil)

func (sr *StreamResult) Read(b []byte) (int, error) {
	return sr.r.Read(b)
}

func (sr *StreamResult) Errors() <-chan error {
	return sr.execErrs
}

func (req *Request) Extract() (*StreamResult, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	res, err := req.fetch()
	if err != nil {
		return nil, err
	}

	if !StatusOK(res.StatusCode) {
		return nil, fmt.Errorf("Status: %s. Headers: %v", res.Status, res.Header)
	}

	args := []string{
		"-jar",
		tikaSource,
		"--text",
		"--pretty-print",
		"-",
	}

	// It is imperative that we read the source from stdin
	cmd := exec.Command("java", args...)
	cmd.Stdin = res.Body
	prc, pwc := io.Pipe()
	cmd.Stdout = pwc
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	execErrs := make(chan error)
	go func() {
		defer close(execErrs)
		defer res.Body.Close()
		err := cmd.Wait()
		_ = pwc.Close()
		execErrs <- err
	}()

	return &StreamResult{r: prc, execErrs: execErrs}, nil
}
