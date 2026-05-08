package services

// embedded_fonts.go bundles the four critical script fonts directly into the
// binary using go:embed.  This guarantees that Arabic, Devanagari, and Bengali
// text renders without tofu in every environment — including local dev machines
// and CI containers — regardless of which system fonts happen to be installed.
//
// The embedded bytes are used as a last-resort fallback inside
// fallbackFontEntry.load(): system paths are tried first so that, on a full
// production image where the apk/apt fonts are present, the on-disk file is
// used (no binary bloat at runtime).  On a bare dev machine, the embedded copy
// takes over transparently.

import _ "embed"

//go:embed fonts/NotoSansArabic-Regular.ttf
var embeddedNotoSansArabic []byte

//go:embed fonts/NotoNaskhArabic-Regular.ttf
var embeddedNotoNaskhArabic []byte

//go:embed fonts/NotoSansDevanagari-Regular.ttf
var embeddedNotoSansDevanagari []byte

//go:embed fonts/NotoSansBengali-Regular.ttf
var embeddedNotoSansBengali []byte
