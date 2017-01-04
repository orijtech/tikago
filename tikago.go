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
	"sync"
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
	sync.RWMutex
	URL string `json:"url"`

	Stdin   io.Reader
	Headers http.Header

	doneFn func() error

	RoundTripper http.RoundTripper `json:"-"`
}

func (req *Request) Done() error {
	req.RLock()
	defer req.RUnlock()
	if fn := req.doneFn; fn != nil {
		return fn()
	}
	return nil
}

func (req *Request) SetDone(fn func() error) (oldDone func() error) {
	req.Lock()
	defer req.Unlock()

	oldDone = req.doneFn
	req.doneFn = fn

	return oldDone
}

func signature(fn func() error) string {
	return fmt.Sprintf("%p", fn)
}

func (req *Request) SetAndChainDone(fn func() error) {
	if fn == nil {
		return
	}

	req.Lock()
	defer req.Unlock()

	oldFn := req.doneFn
	oldFnSig, newFnSig := signature(oldFn), signature(fn)
	switch {
	case req.doneFn == nil:
		req.doneFn = fn
	case oldFnSig != newFnSig:
		req.doneFn = func() error {
			err := oldFn()
			if fErr := fn(); fErr != nil {
				err = fErr
			}
			return err
		}
	}
}

type Response struct {
	Text     string `json:"text,omitempty"`
	Language string `json:"language"`
}

func (req *Request) HasStdin() bool {
	return req.Stdin != nil
}

// Validate checks if a Request has the
// necessary attributes set for it to be used.
func (req *Request) Validate() error {
	source := strings.TrimSpace(req.URL)
	if req.HasStdin() {
		return nil
	}

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

	var stdin io.Reader
	var done func() error

	if req.HasStdin() {
		stdin = req.Stdin
		if closer, ok := stdin.(io.Closer); ok {
			done = closer.Close
		}
	} else {
		res, err := req.fetch()
		if err != nil {
			return nil, err
		}

		if !StatusOK(res.StatusCode) {
			return nil, fmt.Errorf("Status: %s. Headers: %v", res.Status, res.Header)
		}
		stdin = res.Body
		done = res.Body.Close
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
	cmd.Stdin = stdin
	prc, pwc := io.Pipe()
	cmd.Stdout = pwc
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	execErrs := make(chan error)
	go func() {
		defer close(execErrs)
		if done != nil {
			defer done()
		}
		err := cmd.Wait()
		_ = pwc.Close()
		execErrs <- err
	}()

	return &StreamResult{r: prc, execErrs: execErrs}, nil
}
