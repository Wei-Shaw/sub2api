package traffic

import (
	"net/http"
	"strings"
)

// ContentEncoding combines every field line in wire order. Content-Encoding is
// list-valued: looking only at Header.Get's first field could decode half of a
// stacked representation (or mistake identity followed by gzip for identity).
func ContentEncoding(header http.Header) string {
	return strings.Join(header.Values("Content-Encoding"), ",")
}
