package data

import _ "embed"

// CatalogJSON is the embedded fork-owned model catalog.
//
//go:embed catalog.json
var CatalogJSON []byte
