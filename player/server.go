package player

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/aybabtme/iocontrol"
	"github.com/gin-gonic/gin"
)

var ThrottleScale float64 = 1.5
var ThrottleSwitch = true

// errNoOverlap is returned by serveContent's parseRange if first-byte-pos of
// all of the byte-range-spec values is greater than the content size.
var errNoOverlap = errors.New("invalid range: failed to overlap")

// ServeStream replies to the request using the content in the
// provided ReadSeeker. The main benefit of ServeStream over io.Copy
// is that it handles Range requests properly, sets the MIME type, and
// handles If-Match, If-Unmodified-Since, If-None-Match, If-Modified-Since,
// and If-Range requests.
//
// The content's Seek method must work: ServeStream uses
// a seek to the end of the content to determine its size.
//
// If the caller has set w's ETag header formatted per RFC 7232, section 2.3,
// ServeStream uses it to handle requests using If-Match, If-None-Match, or If-Range.
//
// content must be seeked to the beginning of the file.
func ServeStream(c *gin.Context, content *Stream) {
	code := http.StatusOK
	size := int64(content.Size)

	// handle Content-Range header.
	sendSize := size
	var sendContent io.Reader = content
	if size >= 0 {
		ranges, err := parseRange(c.GetHeader("Range"), size)
		if err != nil {
			if err == errNoOverlap {
				c.Header("Content-Range", fmt.Sprintf("bytes */%d", size))
			}
			Error(c, err.Error(), http.StatusRequestedRangeNotSatisfiable)
			return
		}

		if sumRangesSize(ranges) > size {
			// The total number of bytes in all the ranges
			// is larger than the size of the file by
			// itself, so this is probably an attack, or a
			// dumb client. Ignore the range request.
			ranges = nil
		}

		if len(ranges) == 1 {
			// RFC 7233, Section 4.1:
			// "If a single part is being transferred, the server
			// generating the 206 response MUST generate a
			// Content-Range header field, describing what range
			// of the selected representation is enclosed, and a
			// payload consisting of the range.
			// ...
			// A server MUST NOT generate a multipart response to
			// a request for a single range, since a client that
			// does not request multiple parts might not support
			// multipart responses."
			ra := ranges[0]
			if _, err := content.Seek(ra.start, io.SeekStart); err != nil {
				Error(c, err.Error(), http.StatusRequestedRangeNotSatisfiable)
				return
			}

			if c.Request.Method != http.MethodHead {
				_, err = content.GetChunk(int(getRange(ra.start, 1).FirstChunkIdx))
				if err != nil {
					Error(c, err.Error(), http.StatusRequestedRangeNotSatisfiable)
					return
				}
			}

			sendSize = ra.length
			code = http.StatusPartialContent
			c.Header("Content-Range", ra.contentRange(size))
		}

		c.Header("Accept-Ranges", "bytes")
		if c.GetHeader("Content-Encoding") == "" {
			c.Header("Content-Length", strconv.FormatInt(sendSize, 10))
		}
	}

	c.Status(code)

	if c.Request.Method != http.MethodHead {
		if ThrottleSwitch {
			throttledW := iocontrol.ThrottledWriter(c.Writer, int(ThrottleScale*iocontrol.MiB), 1*time.Second)
			io.CopyN(throttledW, sendContent, sendSize)
		} else {
			io.CopyN(c.Writer, sendContent, sendSize)
		}
	}
}

// Error replies to the request with the specified error message and HTTP code.
// It does not otherwise end the request; the caller should ensure no further
// writes are done to w.
// The error message should be plain text.
func Error(c *gin.Context, error string, code int) {
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(code)
	fmt.Fprintln(c.Writer, error)
}

// parseRange parses a Range header string as per RFC 7233.
// errNoOverlap is returned if none of the ranges overlap.
func parseRange(s string, size int64) ([]httpRange, error) {
	if s == "" {
		return nil, nil // header not present
	}
	const b = "bytes="
	if !strings.HasPrefix(s, b) {
		return nil, errors.New("invalid range")
	}
	var ranges []httpRange
	noOverlap := false
	for _, ra := range strings.Split(s[len(b):], ",") {
		ra = strings.TrimSpace(ra)
		if ra == "" {
			continue
		}
		i := strings.Index(ra, "-")
		if i < 0 {
			return nil, errors.New("invalid range")
		}
		start, end := strings.TrimSpace(ra[:i]), strings.TrimSpace(ra[i+1:])
		var r httpRange
		if start == "" {
			// If no start is specified, end specifies the
			// range start relative to the end of the file.
			i, err := strconv.ParseInt(end, 10, 64)
			if err != nil {
				return nil, errors.New("invalid range")
			}
			if i > size {
				i = size
			}
			r.start = size - i
			r.length = size - r.start
		} else {
			i, err := strconv.ParseInt(start, 10, 64)
			if err != nil || i < 0 {
				return nil, errors.New("invalid range")
			}
			if i >= size {
				// If the range begins after the size of the content,
				// then it does not overlap.
				noOverlap = true
				continue
			}
			r.start = i
			if end == "" {
				// If no end is specified, range extends to end of the file.
				r.length = size - r.start
			} else {
				i, err := strconv.ParseInt(end, 10, 64)
				if err != nil || r.start > i {
					return nil, errors.New("invalid range")
				}
				if i >= size {
					i = size - 1
				}
				r.length = i - r.start + 1
			}
		}
		ranges = append(ranges, r)
	}
	if noOverlap && len(ranges) == 0 {
		// The specified ranges did not overlap with the content.
		return nil, errNoOverlap
	}
	return ranges, nil
}

// httpRange specifies the byte range to be sent to the client.
type httpRange struct {
	start, length int64
}

func sumRangesSize(ranges []httpRange) (size int64) {
	for _, ra := range ranges {
		size += ra.length
	}
	return
}

func (r httpRange) contentRange(size int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", r.start, r.start+r.length-1, size)
}

func (r httpRange) mimeHeader(contentType string, size int64) textproto.MIMEHeader {
	return textproto.MIMEHeader{
		"Content-Range": {r.contentRange(size)},
		"Content-Type":  {contentType},
	}
}
