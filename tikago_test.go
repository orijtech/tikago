package tikago_test

import (
	"io"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/odeke-em/tikago"
)

var fileFetchHTTPClient = tikago.FileAndHTTPRoundTripper()
var notYetSupported = int64(-1)

func TestRequestExtract(t *testing.T) {
	tests := [...]struct {
		uri          string
		wantErr      bool
		wantAtLeast  int64
		comment      string
		roundTripper http.RoundTripper
	}{
		0: {
			uri: "./testdata/resume.pdf", wantAtLeast: 100,
			roundTripper: fileFetchHTTPClient,
			comment:      "must work with custom roundTripper",
		},
		1: {
			uri: "./testdata/resume.pdf", wantErr: true,
			comment: "without a custom roundTripper, this should fail",
		},
		2: {
			uri:          "./testdata/Scanned_20150910-1044.pdf",
			roundTripper: fileFetchHTTPClient,

			wantAtLeast: notYetSupported,
			comment:     "handwritten notes cannot yet be parsed",
		},
		3: {
			uri:     "https://www.cs.cmu.edu/~dga/papers/cuckoo-conext2014.pdf",
			comment: "fetched from the web, should still be fetchable",

			wantAtLeast: 500,
		},
		4: {
			uri:     "./testdata/ch10-review-qns.docx",
			roundTripper: fileFetchHTTPClient,
			comment: "local docx file should be supported",

			wantAtLeast: 500,
		},

	}

	for i, tt := range tests {
		req := tikago.Request{
			URL:          tt.uri,
			RoundTripper: tt.roundTripper,
		}

		sr, err := req.Extract()
		if tt.wantErr {
			if err == nil {
				t.Errorf("#%d: want non-nil err", i)
			}
			continue
		}

		if err != nil {
			t.Errorf("#%d: err: %v", i, err)
			continue
		}

		n, err := io.Copy(ioutil.Discard, sr)
		if err != nil {
			t.Errorf("#%d: io.Copy err: %v", i, err)
		}

		if n < tt.wantAtLeast {
			t.Errorf("#%d: got %d want atleast %d bytes written", i, n, tt.wantAtLeast)
		}

		if tt.wantAtLeast == notYetSupported {
			t.Logf("#%d: not yet enabled; uri: %q %q", i, tt.uri, tt.comment)
		}

		if err := <-sr.Errors(); err != nil {
			t.Errorf("#%d: err: %v", i, err)
		}
	}
}
