package tikago

import (
	"net/http"
	"os"
	"path/filepath"
)

var defaultClient = new(http.Client)

func StatusOK(status int) bool {
	return status >= 200 && status <= 299
}

type relToRootFS struct{}

var _ http.FileSystem = (*relToRootFS)(nil)

func (rfs *relToRootFS) Open(uri string) (http.File, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	abspath := filepath.Join(cwd, uri)
	return os.Open(abspath)
}

func FileAndHTTPRoundTripper() http.RoundTripper {
	transport := new(http.Transport)
	transport.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	transport.RegisterProtocol("", http.NewFileTransport(&relToRootFS{}))

	return transport
}
