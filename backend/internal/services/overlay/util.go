package overlay

import (
	"image"
	"image/color"
	"math"
	"strings"
	"unicode"

	"golang.org/x/image/font"
)

func clampToBounds(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func prepareDrawableBox(x1, y1, x2, y2 int) (int, int, int, int, bool) {
	return prepareDrawableBoxWithShrink(x1, y1, x2, y2, overlayBboxShrinkPx)
}

func prepareDrawableBoxWithShrink(x1, y1, x2, y2 int, shrinkPx int) (int, int, int, int, bool) {
	if shrinkPx < 0 {
		shrinkPx = 0
	}
	x1 += shrinkPx
	y1 += shrinkPx
	x2 -= shrinkPx
	y2 -= shrinkPx

	if x2 <= x1 || y2 <= y1 {
		return 0, 0, 0, 0, false
	}
	if (x2-x1) < overlayMinDrawWidth || (y2-y1) < overlayMinDrawHeight {
		return 0, 0, 0, 0, false
	}
	return x1, y1, x2, y2, true
}

func shouldSkipForOverlap(candidate image.Rectangle, existing []image.Rectangle) bool {
	candidateArea := rectArea(candidate)
	if candidateArea <= 0 {
		return true
	}
	for _, box := range existing {
		overlap := rectArea(candidate.Intersect(box))
		if overlap == 0 {
			continue
		}
		overlapRatio := float64(overlap) / float64(candidateArea)
		if overlapRatio >= overlayMaxOverlapPct {
			return true
		}
	}
	return false
}

func detectShadowOffset(img *image.RGBA, box image.Rectangle) shadowHint {
	bounds := img.Bounds()
	bg := medianRegionColor(img, box)
	bgLum := colorLuminance(bg)

	candidates := [][2]int{{1, 1}, {1, 0}, {0, 1}, {-1, 1}, {1, -1}, {-1, -1}, {0, -1}, {-1, 0}}

	bestDx, bestDy := 1, 1
	bestDarkness := 0.0
	for _, d := range candidates {
		dx, dy := d[0], d[1]
		fringe := box.Add(image.Point{X: dx, Y: dy}).Intersect(bounds)
		if fringe.Empty() {
			continue
		}
		fringeLum := avgRegionLuminance(img, fringe)
		darkness := bgLum - fringeLum
		if darkness > bestDarkness {
			bestDarkness = darkness
			bestDx, bestDy = dx, dy
		}
	}

	if bestDarkness < overlayDetectedShadowMinDarkness {
		return shadowHint{}
	}
	scaled := float64(overlayShadowAlpha) * bestDarkness / 0.15
	alpha := uint8(math.Min(float64(overlayDetectedShadowMaxAlpha), math.Max(60, scaled)))
	return shadowHint{dx: bestDx, dy: bestDy, alpha: alpha}
}

func shadowColorFor(ink color.Color) color.Color {
	r, g, b, _ := ink.RGBA()
	inkR := uint8(r >> 8)
	inkG := uint8(g >> 8)
	inkB := uint8(b >> 8)
	inv := func(c uint8) uint8 { return 255 - c }
	blend := func(inverted, ink uint8) uint8 {
		return uint8((int(inverted)*8 + int(ink)*2) / 10)
	}
	return color.RGBA{
		R: blend(inv(inkR), inkR),
		G: blend(inv(inkG), inkG),
		B: blend(inv(inkB), inkB),
		A: overlayShadowAlpha,
	}
}

func baselineScriptOffset(text string, fontSize float64) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	total := 0
	letters := 0
	descenders := 0
	cjk := 0
	rtl := 0
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if unicode.IsLetter(r) {
			letters++
			if isDescenderRune(r) {
				descenders++
			}
		}
		if isCJKRune(r) {
			cjk++
		}
		if isRTLRune(r) {
			rtl++
		}
	}
	if total == 0 {
		return 0
	}

	offset := -max(1, int(math.Round(fontSize*0.07)))

	if letters > 0 && descenders > 0 {
		ratio := float64(descenders) / float64(letters)
		normalized := ratio / 0.55
		if normalized > 1.0 {
			normalized = 1.0
		}
		offset += int(math.Round(fontSize * 0.05 * normalized))
	}

	if cjk*10 >= total*7 {
		offset -= max(1, int(math.Round(fontSize*0.02)))
	}

	if rtl*2 >= total {
		offset += max(1, int(math.Round(fontSize*0.04)))
	}

	return offset
}

func isDescenderRune(r rune) bool {
	switch unicode.ToLower(r) {
	case 'g', 'j', 'p', 'q', 'y':
		return true
	}
	return false
}

func verticalTextPadding(boxWidth int, opts OverlayOptions) int {
	if boxWidth <= 0 {
		return opts.TextPadding
	}
	target := max(overlayVerticalMinPadding, boxWidth/10)
	return min(opts.TextPadding, target)
}

func verticalMinFontSize(opts OverlayOptions, availW int) int {
	minVerticalFont := opts.MinFontSize + overlayVerticalMinFontBoost
	if availW <= overlayVerticalNarrowWidthThreshold {
		minVerticalFont += overlayVerticalNarrowFontBoost
	}
	if minVerticalFont > opts.MaxFontSize {
		return opts.MaxFontSize
	}
	return minVerticalFont
}

func verticalExtraStrokeWidth() int {
	return overlayVerticalExtraStrokePx
}

func measureStringPx(face font.Face, s string) int {
	return (&font.Drawer{Face: face}).MeasureString(s).Ceil()
}

func fitWordToWidth(word string, maxWidth int, face font.Face) (string, string) {
	if measureStringPx(face, word) <= maxWidth {
		return word, ""
	}
	runes := []rune(word)
	lo, hi := 1, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if measureStringPx(face, string(runes[:mid])) <= maxWidth {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo]), string(runes[lo:])
}

func normalizeOverlayText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func isNearbyBox(r1, r2 image.Rectangle, maxDist int) bool {
	dx := 0
	if r1.Max.X < r2.Min.X {
		dx = r2.Min.X - r1.Max.X
	} else if r2.Max.X < r1.Min.X {
		dx = r1.Min.X - r2.Max.X
	}
	dy := 0
	if r1.Max.Y < r2.Min.Y {
		dy = r2.Min.Y - r1.Max.Y
	} else if r2.Max.Y < r1.Min.Y {
		dy = r1.Min.Y - r2.Max.Y
	}
	return dx <= maxDist && dy <= maxDist
}

func isDuplicateNearbyText(candidate overlayCandidate, drawn []overlayCandidate) bool {
	norm := normalizeOverlayText(candidate.text)
	if norm == "" {
		return false
	}
	for _, d := range drawn {
		if !isNearbyBox(candidate.box, d.box, overlayNearbyBoxDist) {
			continue
		}
		dNorm := normalizeOverlayText(d.text)
		if dNorm == "" {
			continue
		}
		if norm == dNorm || strings.Contains(dNorm, norm) || strings.Contains(norm, dNorm) {
			return true
		}
	}
	return false
}

func rectArea(rect image.Rectangle) int {
	if rect.Dx() <= 0 || rect.Dy() <= 0 {
		return 0
	}
	return rect.Dx() * rect.Dy()
}

func avgRegionLuminance(img *image.RGBA, r image.Rectangle) float64 {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return 1.0
	}
	step := max(1, min(r.Dx(), r.Dy())/8)
	var sumLum float64
	samples := 0
	for y := r.Min.Y; y < r.Max.Y; y += step {
		for x := r.Min.X; x < r.Max.X; x += step {
			c := img.RGBAAt(x, y)
			sumLum += 0.299*float64(c.R)/255 + 0.587*float64(c.G)/255 + 0.114*float64(c.B)/255
			samples++
		}
	}
	if samples == 0 {
		return 1.0
	}
	return sumLum / float64(samples)
}

func avgRegionColor(img *image.RGBA, r image.Rectangle) color.RGBA {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	step := max(1, min(r.Dx(), r.Dy())/8)
	var sumR, sumG, sumB uint64
	samples := 0
	for y := r.Min.Y; y < r.Max.Y; y += step {
		for x := r.Min.X; x < r.Max.X; x += step {
			c := img.RGBAAt(x, y)
			sumR += uint64(c.R)
			sumG += uint64(c.G)
			sumB += uint64(c.B)
			samples++
		}
	}
	if samples == 0 {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	return color.RGBA{
		R: uint8(sumR / uint64(samples)),
		G: uint8(sumG / uint64(samples)),
		B: uint8(sumB / uint64(samples)),
		A: 255,
	}
}

func medianRegionColor(img *image.RGBA, r image.Rectangle) color.RGBA {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	step := max(1, min(r.Dx(), r.Dy())/16)
	var rs, gs, bs []uint8
	for y := r.Min.Y; y < r.Max.Y; y += step {
		for x := r.Min.X; x < r.Max.X; x += step {
			c := img.RGBAAt(x, y)
			rs = append(rs, c.R)
			gs = append(gs, c.G)
			bs = append(bs, c.B)
		}
	}
	if len(rs) == 0 {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	sortUint8(rs)
	sortUint8(gs)
	sortUint8(bs)
	mid := len(rs) / 2
	return color.RGBA{R: rs[mid], G: gs[mid], B: bs[mid], A: 255}
}

func medianRingColor(img *image.RGBA, outer image.Rectangle, inner image.Rectangle) color.RGBA {
	outer = outer.Intersect(img.Bounds())
	inner = inner.Intersect(outer)
	if outer.Empty() {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	step := max(1, min(outer.Dx(), outer.Dy())/16)
	var rs, gs, bs []uint8
	for y := outer.Min.Y; y < outer.Max.Y; y += step {
		for x := outer.Min.X; x < outer.Max.X; x += step {
			if x >= inner.Min.X && x < inner.Max.X && y >= inner.Min.Y && y < inner.Max.Y {
				continue
			}
			c := img.RGBAAt(x, y)
			rs = append(rs, c.R)
			gs = append(gs, c.G)
			bs = append(bs, c.B)
		}
	}
	if len(rs) == 0 {
		return medianRegionColor(img, outer)
	}
	sortUint8(rs)
	sortUint8(gs)
	sortUint8(bs)
	mid := len(rs) / 2
	return color.RGBA{R: rs[mid], G: gs[mid], B: bs[mid], A: 255}
}

func patchFillBBox(dst *image.RGBA, box image.Rectangle) {
	patchFillBBoxWithOptions(dst, box, DefaultOverlayOptions())
}

func boundaryContrast(dst *image.RGBA, r image.Rectangle) float64 {
	bounds := dst.Bounds()
	lumPx := func(x, y int) float64 {
		c := dst.RGBAAt(x, y)
		return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
	}
	var sumSq float64
	var count int
	for x := r.Min.X; x < r.Max.X; x++ {
		if r.Min.Y-1 >= bounds.Min.Y {
			d := lumPx(x, r.Min.Y-1) - lumPx(x, r.Min.Y)
			sumSq += d * d
			count++
		}
		if r.Max.Y < bounds.Max.Y {
			d := lumPx(x, r.Max.Y) - lumPx(x, r.Max.Y-1)
			sumSq += d * d
			count++
		}
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		if r.Min.X-1 >= bounds.Min.X {
			d := lumPx(r.Min.X-1, y) - lumPx(r.Min.X, y)
			sumSq += d * d
			count++
		}
		if r.Max.X < bounds.Max.X {
			d := lumPx(r.Max.X, y) - lumPx(r.Max.X-1, y)
			sumSq += d * d
			count++
		}
	}
	if count == 0 {
		return 0
	}
	rms := math.Sqrt(sumSq / float64(count))
	return math.Min(rms/128.0, 1.0)
}

func patchFillBBoxWithOptions(dst *image.RGBA, box image.Rectangle, opts OverlayOptions) {
	r := box.Intersect(dst.Bounds())
	if r.Empty() {
		return
	}

	if opts.StudioMode && opts.AntiHaloRadius > 0 {
		expanded := r.Inset(-opts.AntiHaloRadius)
		r = expanded.Intersect(dst.Bounds())
		if r.Empty() {
			return
		}
	}

	featherPx := opts.PatchFeatherPx
	if featherPx <= 0 {
		featherPx = overlayPatchFeatherPx
	}
	if featherPx < 1 {
		featherPx = 1
	}
	maxFeather := 3
	if opts.StudioMode {
		maxFeather = 8
	}
	if featherPx > maxFeather {
		featherPx = maxFeather
	}
	blurRadius := opts.PatchBlurRadius
	if blurRadius < 0 {
		blurRadius = overlayPatchBlurRadius
	}
	maxBlur := 2
	if opts.StudioMode {
		maxBlur = 4
	}
	if blurRadius > maxBlur {
		blurRadius = maxBlur
	}

	contrast := boundaryContrast(dst, r)
	if contrast > 0.05 {
		featherScale := 1.0 + 1.5*contrast
		blurScale := 1.0 + 1.0*contrast
		featherPx = int(math.Round(float64(featherPx) * featherScale))
		blurRadius = int(math.Round(float64(blurRadius) * blurScale))
		capFeather := 6
		if opts.StudioMode {
			capFeather = 12
		}
		if featherPx > capFeather {
			featherPx = capFeather
		}
		capBlur := 4
		if opts.StudioMode {
			capBlur = 6
		}
		if blurRadius > capBlur {
			blurRadius = capBlur
		}
	}

	const gridN = 4
	var cellColors [gridN][gridN]color.RGBA

	if opts.StudioMode && opts.TextureAwareFill {
		ringWidth := max(2, opts.AntiHaloRadius+1)
		for gy := 0; gy < gridN; gy++ {
			for gx := 0; gx < gridN; gx++ {
				x0 := r.Min.X + gx*r.Dx()/gridN
				y0 := r.Min.Y + gy*r.Dy()/gridN
				x1 := r.Min.X + (gx+1)*r.Dx()/gridN
				y1 := r.Min.Y + (gy+1)*r.Dy()/gridN
				cell := image.Rect(x0, y0, x1, y1)
				outer := cell.Inset(-ringWidth).Intersect(dst.Bounds())
				cellColors[gy][gx] = medianRingColor(dst, outer, cell)
			}
		}
	} else {
		for gy := 0; gy < gridN; gy++ {
			for gx := 0; gx < gridN; gx++ {
				x0 := r.Min.X + gx*r.Dx()/gridN
				y0 := r.Min.Y + gy*r.Dy()/gridN
				x1 := r.Min.X + (gx+1)*r.Dx()/gridN
				y1 := r.Min.Y + (gy+1)*r.Dy()/gridN
				cell := image.Rect(x0, y0, x1, y1)
				cellColors[gy][gx] = medianRegionColor(dst, cell)
			}
		}
	}

	w := r.Dx()
	h := r.Dy()
	if w < 2 {
		w = 2
	}
	if h < 2 {
		h = 2
	}

	patch := make([]color.RGBA, r.Dx()*r.Dy())
	idxAt := func(px, py int) int { return py*r.Dx() + px }

	for py := 0; py < r.Dy(); py++ {
		fy := float64(py) * float64(gridN-1) / float64(h-1)
		gy0 := int(fy)
		if gy0 > gridN-2 {
			gy0 = gridN - 2
		}
		gy1 := gy0 + 1
		ty := fy - float64(gy0)
		for px := 0; px < r.Dx(); px++ {
			fx := float64(px) * float64(gridN-1) / float64(w-1)
			gx0 := int(fx)
			if gx0 > gridN-2 {
				gx0 = gridN - 2
			}
			gx1 := gx0 + 1
			tx := fx - float64(gx0)
			c00 := cellColors[gy0][gx0]
			c10 := cellColors[gy0][gx1]
			c01 := cellColors[gy1][gx0]
			c11 := cellColors[gy1][gx1]
			r0 := float64(c00.R)*(1-tx) + float64(c10.R)*tx
			r1 := float64(c01.R)*(1-tx) + float64(c11.R)*tx
			g0 := float64(c00.G)*(1-tx) + float64(c10.G)*tx
			g1 := float64(c01.G)*(1-tx) + float64(c11.G)*tx
			b0 := float64(c00.B)*(1-tx) + float64(c10.B)*tx
			b1 := float64(c01.B)*(1-tx) + float64(c11.B)*tx
			patch[idxAt(px, py)] = color.RGBA{
				R: uint8(r0*(1-ty) + r1*ty),
				G: uint8(g0*(1-ty) + g1*ty),
				B: uint8(b0*(1-ty) + b1*ty),
				A: 255,
			}
		}
	}

	if blurRadius > 0 {
		band := featherPx + blurRadius
		blurred := make([]color.RGBA, len(patch))
		copy(blurred, patch)
		for py := 0; py < r.Dy(); py++ {
			for px := 0; px < r.Dx(); px++ {
				d := edgeDistance(px, py, r.Dx(), r.Dy())
				if d > band {
					continue
				}
				var sr, sg, sb, count int
				for oy := -blurRadius; oy <= blurRadius; oy++ {
					ny := py + oy
					if ny < 0 || ny >= r.Dy() {
						continue
					}
					for ox := -blurRadius; ox <= blurRadius; ox++ {
						nx := px + ox
						if nx < 0 || nx >= r.Dx() {
							continue
						}
						c := patch[idxAt(nx, ny)]
						sr += int(c.R)
						sg += int(c.G)
						sb += int(c.B)
						count++
					}
				}
				if count == 0 {
					continue
				}
				blurred[idxAt(px, py)] = color.RGBA{
					R: uint8(sr / count),
					G: uint8(sg / count),
					B: uint8(sb / count),
					A: 255,
				}
			}
		}
		patch = blurred
	}

	for py := 0; py < r.Dy(); py++ {
		for px := 0; px < r.Dx(); px++ {
			x := r.Min.X + px
			y := r.Min.Y + py
			orig := dst.RGBAAt(x, y)
			fill := patch[idxAt(px, py)]

			alpha := 1.0
			d := edgeDistance(px, py, r.Dx(), r.Dy())
			if d < featherPx {
				alpha = float64(d+1) / float64(featherPx+1)
			}

			dst.SetRGBA(x, y, blendRGBA(orig, fill, alpha))
		}
	}
}

func edgeDistance(x, y, w, h int) int {
	left := x
	right := w - 1 - x
	top := y
	bottom := h - 1 - y
	d := left
	if right < d {
		d = right
	}
	if top < d {
		d = top
	}
	if bottom < d {
		d = bottom
	}
	return d
}

func blendRGBA(base, over color.RGBA, alpha float64) color.RGBA {
	if alpha <= 0 {
		return base
	}
	if alpha >= 1 {
		return over
	}
	inv := 1.0 - alpha
	return color.RGBA{
		R: uint8(float64(base.R)*inv + float64(over.R)*alpha),
		G: uint8(float64(base.G)*inv + float64(over.G)*alpha),
		B: uint8(float64(base.B)*inv + float64(over.B)*alpha),
		A: 255,
	}
}

func inkColorForBackground(srcLum float64) color.Color {
	if srcLum >= overlayLumThreshold {
		return color.Black
	}
	return color.White
}

func colorLuminance(c color.RGBA) float64 {
	return 0.299*float64(c.R)/255 + 0.587*float64(c.G)/255 + 0.114*float64(c.B)/255
}

const (
	boostSaturationMax    = 0.55
	boostSaturationCurve = 1.5
	boostMinContrast     = 0.42
)

func boostSampledColor(c color.RGBA, bgLum float64) color.RGBA {
	h, s, l := rgbToHSL(c)
	adaptiveDelta := boostSaturationMax * math.Pow(1.0-s, boostSaturationCurve)
	s = math.Min(1.0, s+adaptiveDelta)
	inkLum := colorLuminance(c)
	if bgLum >= overlayLumThreshold {
		if diff := bgLum - inkLum; diff < boostMinContrast {
			l = math.Max(0.0, l-(boostMinContrast-diff))
		}
	} else {
		if diff := inkLum - bgLum; diff < boostMinContrast {
			l = math.Min(1.0, l+(boostMinContrast-diff))
		}
	}
	r, g, b := hslToRGB(h, s, l)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func rgbToHSL(c color.RGBA) (h, s, l float64) {
	r := float64(c.R) / 255
	g := float64(c.G) / 255
	b := float64(c.B) / 255
	cmax := math.Max(r, math.Max(g, b))
	cmin := math.Min(r, math.Min(g, b))
	delta := cmax - cmin
	l = (cmax + cmin) / 2
	if delta == 0 {
		return 0, 0, l
	}
	if l < 0.5 {
		s = delta / (cmax + cmin)
	} else {
		s = delta / (2 - cmax - cmin)
	}
	switch cmax {
	case r:
		h = math.Mod((g-b)/delta, 6)
	case g:
		h = (b-r)/delta + 2
	default:
		h = (r-g)/delta + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, l
}

func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return v, v, v
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	rv := hueToRGB(p, q, h/360+1.0/3)
	gv := hueToRGB(p, q, h/360)
	bv := hueToRGB(p, q, h/360-1.0/3)
	return uint8(math.Round(rv * 255)), uint8(math.Round(gv * 255)), uint8(math.Round(bv * 255))
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 1.0/2:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	}
	return p
}

func sampleDominantTextColor(img *image.RGBA, box image.Rectangle) color.Color {
	r := box.Intersect(img.Bounds())
	if r.Empty() {
		return inkColorForBackground(1.0)
	}
	bg := medianRegionColor(img, r)
	bgLum := colorLuminance(bg)
	step := max(1, min(r.Dx(), r.Dy())/16)
	var sumR, sumG, sumB uint64
	textPx, totalPx := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y += step {
		for x := r.Min.X; x < r.Max.X; x += step {
			totalPx++
			c := img.RGBAAt(x, y)
			if math.Abs(colorLuminance(c)-bgLum) >= overlayTextContrastThreshold {
				sumR += uint64(c.R)
				sumG += uint64(c.G)
				sumB += uint64(c.B)
				textPx++
			}
		}
	}
	if textPx < max(1, totalPx/10) {
		return inkColorForBackground(bgLum)
	}
	sampled := color.RGBA{
		R: uint8(sumR / uint64(textPx)),
		G: uint8(sumG / uint64(textPx)),
		B: uint8(sumB / uint64(textPx)),
		A: 255,
	}
	return boostSampledColor(sampled, bgLum)
}

func sortUint8(s []uint8) {
	for i := 0; i < len(s)-1; i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
