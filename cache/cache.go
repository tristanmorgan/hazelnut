package cache

import (
	"crypto/sha256"
	"net/http"
)

type ObjCore struct {
	Headers http.Header
	Body    []byte
}

// type Key string

// MakeKey takes a http.Request and a flag indicating whether to ignore the host,
// and returns a 32 byte sha256 hash of the request.
func MakeKey(r *http.Request, ignoreHost bool) string {
	sh := sha256.New()
	// Only include the host in the key if we're not ignoring it
	if !ignoreHost {
		_, _ = sh.Write([]byte(r.Host))
	}

	// Always include the path in the key
	_, _ = sh.Write([]byte(r.URL.Path))
	sum := sh.Sum(nil)
	// Return the key as a string
	return string(sum)
}
