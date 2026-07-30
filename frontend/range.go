package frontend

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ByteRange represents a single byte range
type ByteRange struct {
	Start int64
	End   int64
}

// ParseRange parses the Range header and returns requested ranges
// Format: "bytes=0-499" or "bytes=500-999" or "bytes=-500" or "bytes=500-"
func ParseRange(rangeHeader string, contentLength int64) ([]ByteRange, error) {
	if rangeHeader == "" {
		return nil, nil
	}

	// Must start with "bytes="
	const prefix = "bytes="
	if !strings.HasPrefix(rangeHeader, prefix) {
		return nil, errors.New("invalid range header format")
	}

	rangeSpec := strings.TrimPrefix(rangeHeader, prefix)
	ranges := []ByteRange{}

	// Split multiple ranges: "bytes=0-499,1000-1499"
	for _, part := range strings.Split(rangeSpec, ",") {
		part = strings.TrimSpace(part)

		r, err := parseByteRange(part, contentLength)
		if err != nil {
			return nil, err
		}
		ranges = append(ranges, r)
	}

	if len(ranges) == 0 {
		return nil, errors.New("no valid ranges")
	}

	return ranges, nil
}

func parseByteRange(spec string, contentLength int64) (ByteRange, error) {
	parts := strings.Split(spec, "-")
	if len(parts) != 2 {
		return ByteRange{}, errors.New("invalid range format")
	}

	start, end := parts[0], parts[1]

	// Case 1: "500-999" (both specified)
	if start != "" && end != "" {
		s, err := strconv.ParseInt(start, 10, 64)
		if err != nil {
			return ByteRange{}, err
		}
		e, err := strconv.ParseInt(end, 10, 64)
		if err != nil {
			return ByteRange{}, err
		}
		if s > e || s < 0 || e >= contentLength {
			return ByteRange{}, errors.New("invalid range values")
		}
		return ByteRange{Start: s, End: e}, nil
	}

	// Case 2: "-500" (last 500 bytes)
	if start == "" && end != "" {
		suffix, err := strconv.ParseInt(end, 10, 64)
		if err != nil {
			return ByteRange{}, err
		}
		if suffix <= 0 {
			return ByteRange{}, errors.New("invalid suffix length")
		}
		s := contentLength - suffix
		if s < 0 {
			s = 0
		}
		return ByteRange{Start: s, End: contentLength - 1}, nil
	}

	// Case 3: "500-" (from byte 500 to end)
	if start != "" && end == "" {
		s, err := strconv.ParseInt(start, 10, 64)
		if err != nil {
			return ByteRange{}, err
		}
		if s < 0 || s >= contentLength {
			return ByteRange{}, errors.New("invalid start position")
		}
		return ByteRange{Start: s, End: contentLength - 1}, nil
	}

	return ByteRange{}, errors.New("invalid range specification")
}

// ContentRange generates the Content-Range header value
func (r ByteRange) ContentRange(totalSize int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", r.Start, r.End, totalSize)
}

// Length returns the number of bytes in this range
func (r ByteRange) Length() int64 {
	return r.End - r.Start + 1
}
