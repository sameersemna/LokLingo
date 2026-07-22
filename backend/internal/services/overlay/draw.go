package overlay

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

func drawTextLine(dst *image.RGBA, fontSize float64, line string, dotX, baselineY int, inkColor color.Color, clipRect image.Rectangle) {
	drawTextLineStyled(dst, fontSize, line, dotX, baselineY, inkColor, clipRect, 0)
}

func drawTextLineWithStyle(dst *image.RGBA, fontSize float64, line string, dotX, baselineY int, inkColor color.Color, clipRect image.Rectangle, style BlockFontStyle, letterSpacing fixed.Int26_6) {
	strokes := computeStrokeOffsets(style, fontSize)
	drawTextLineStyledWithFont(dst, fontSize, line, dotX, baselineY, inkColor, clipRect, strokes, style, letterSpacing)
}

func drawTextLineStyled(dst *image.RGBA, fontSize float64, line string, dotX, baselineY int, inkColor color.Color, clipRect image.Rectangle, extraInkX int) {
	var strokes []strokeOffset
	if extraInkX > 0 {
		strokes = []strokeOffset{{extraInkX, 0}}
	}
	drawTextLineStyledWithFont(dst, fontSize, line, dotX, baselineY, inkColor, clipRect, strokes, BlockFontStyle{}, 0)
}

func drawTextLineStyledWithFont(dst *image.RGBA, fontSize float64, line string, dotX, baselineY int, inkColor color.Color, clipRect image.Rectangle, strokes []strokeOffset, style BlockFontStyle, letterSpacing fixed.Int26_6) {
	clipped := dst.SubImage(clipRect).(*image.RGBA)
	shadow := shadowColorFor(inkColor)
	shDx, shDy := style.Shadow.dx, style.Shadow.dy
	if shDx == 0 && shDy == 0 {
		shDx, shDy = 1, 1
	}
	if style.Shadow.alpha != 0 {
		sr, sg, sb, _ := shadow.RGBA()
		shadow = color.RGBA{
			R: uint8(sr >> 8),
			G: uint8(sg >> 8),
			B: uint8(sb >> 8),
			A: style.Shadow.alpha,
		}
	}
	dot := fixed.P(dotX, baselineY)
	shadowSrc := image.NewUniform(shadow)
	inkSrc := image.NewUniform(inkColor)
	segs := splitIntoScriptSegments(line)
	totalRunes := 0
	for _, seg := range segs {
		totalRunes += len([]rune(seg.text))
	}
	runesDone := 0
	for _, seg := range segs {
		face, err := segFaceStyled(seg, fontSize, style)
		if err != nil {
			continue
		}
		if letterSpacing == 0 {
			(&font.Drawer{Dst: clipped, Src: shadowSrc, Face: face, Dot: fixed.P(dot.X.Ceil()+shDx, baselineY+shDy)}).DrawString(seg.text)
			d := &font.Drawer{Dst: clipped, Src: inkSrc, Face: face, Dot: dot}
			d.DrawString(seg.text)
			for _, so := range strokes {
				(&font.Drawer{Dst: clipped, Src: inkSrc, Face: face, Dot: fixed.P(dot.X.Ceil()+so.dx, baselineY+so.dy)}).DrawString(seg.text)
			}
			runesDone += len([]rune(seg.text))
			dot = d.Dot
		} else {
			for _, r := range []rune(seg.text) {
				runesDone++
				rs := string(r)
				(&font.Drawer{Dst: clipped, Src: shadowSrc, Face: face, Dot: fixed.P(dot.X.Ceil()+shDx, baselineY+shDy)}).DrawString(rs)
				d := &font.Drawer{Dst: clipped, Src: inkSrc, Face: face, Dot: dot}
				d.DrawString(rs)
				for _, so := range strokes {
					(&font.Drawer{Dst: clipped, Src: inkSrc, Face: face, Dot: fixed.P(dot.X.Ceil()+so.dx, baselineY+so.dy)}).DrawString(rs)
				}
				dot = d.Dot
				if runesDone < totalRunes {
					dot.X += letterSpacing
				}
			}
		}
	}
}

func drawVerticalTextBlock(dst *image.RGBA, candidate overlayCandidate, box image.Rectangle, opts OverlayOptions) (drew bool, err error) {
	runes := verticalCJKRunes(candidate.text)
	if len(runes) == 0 {
		return false, nil
	}

	pad := verticalTextPadding(box.Dx(), opts)
	availW := box.Dx() - (pad * 2)
	availH := box.Dy() - (pad * 2)
	if availW <= 0 || availH <= 0 {
		return false, nil
	}

	minVerticalFont := verticalMinFontSize(opts, availW)
	extraStrokeW := verticalExtraStrokeWidth()
	fontSize := min(opts.MaxFontSize, max(minVerticalFont, availW))
	if fontSize <= 0 {
		fontSize = minVerticalFont
	}
	if fontSize < minVerticalFont {
		fontSize = minVerticalFont
	}

	for fontSize >= minVerticalFont {
		lineHeight := approximateLineHeight(fontSize)
		topInset := approximateTopInset(fontSize)
		if lineHeight <= 0 {
			break
		}
		f, ferr := faceForText(string(runes), float64(fontSize))
		if ferr != nil {
			return false, ferr
		}
		ascentCache := &overlayAscentCache
		if needsFallbackFont(string(runes)) {
			ascentCache = &fallbackAscentCache
		}
		ascent := cachedAscent(f, ascentCache, float64(fontSize))
		textH := pad + ascent + topInset + (len(runes)-1)*lineHeight + pad

		maxGlyphW := 0
		for _, r := range runes {
			w := measureLinePx(string(r), float64(fontSize))
			if w > maxGlyphW {
				maxGlyphW = w
			}
		}
		maxGlyphW += extraStrokeW

		if textH <= box.Dy() && maxGlyphW <= availW {
			inkColor := color.Color(inkColorForBackground(avgRegionLuminance(dst, box)))
			if !opts.DisableColorSampling {
				inkColor = sampleDominantTextColor(dst, box)
			}
			if opts.EraseBBox {
				patchFillBBoxWithOptions(dst, box, opts)
			}

			nGlyphs := len(runes)
			availContentH := box.Dy() - 2*pad
			blockH := topInset + ascent + (nGlyphs-1)*lineHeight
			vertOffset := max(0, (availContentH-blockH)/2)
			dotX := box.Min.X + pad + max(0, (availW-maxGlyphW)/2)
			baselineY := box.Min.Y + pad + vertOffset + topInset + ascent
			baselineY += baselineScriptOffset(string(runes), float64(fontSize))
			for _, r := range runes {
				if baselineY > box.Max.Y-pad {
					break
				}
				drawTextLineStyled(dst, float64(fontSize), string(r), dotX, baselineY, inkColor, box, extraStrokeW)
				baselineY += lineHeight
			}
			return true, nil
		}
		fontSize--
	}

	return false, nil
}

func approximateLineHeight(fontSize int) int {
	if fontSize <= 0 {
		return 1
	}
	target := int(math.Round(float64(fontSize) * 1.18))
	minH := fontSize + overlayLineSpacing
	maxH := fontSize + 9
	if target < minH {
		return minH
	}
	if target > maxH {
		return maxH
	}
	return target
}

func approximateTopInset(fontSize int) int {
	return max(1, int(math.Round(float64(fontSize)*0.10)))
}

func approximateAscent(fontSize int) int {
	if fontSize <= 0 {
		return 1
	}
	return max(1, int(math.Round(float64(fontSize)*0.82)))
}

func blockHeight(topInset, ascent, lineHeight, nLines int) int {
	return topInset + ascent + (nLines-1)*lineHeight
}

func fitTextLayout(text string, maxWidth, maxHeight, minFontSize, maxFontSize int) textLayout {
	if stringsTrimSpace(text) == "" || maxWidth <= 0 || maxHeight <= 0 {
		return textLayout{}
	}

	toleratedHeight := int(math.Round(float64(maxHeight) * (1.0 + overlayFitOverflowPct)))

	wrapAndMeasure := func(fontSize int) (lines []string, lineHeight, topInset, ascent int) {
		measureFn := func(s string) int { return measureLinePx(s, float64(fontSize)) }
		splitFn := func(word string, maxW int) (string, string) {
			f, err := faceForText(word, float64(fontSize))
			if err != nil {
				return word, ""
			}
			return fitWordToWidth(word, maxW, f)
		}
		return wrapLines(text, maxWidth, measureFn, splitFn),
			approximateLineHeight(fontSize),
			approximateTopInset(fontSize),
			approximateAscent(fontSize)
	}

	startSize := min(maxFontSize, max(minFontSize, maxHeight))
	for fontSize := startSize; fontSize >= minFontSize; fontSize-- {
		lines, lineHeight, topInset, ascent := wrapAndMeasure(fontSize)
		if len(lines) == 0 {
			continue
		}
		if blockHeight(topInset, ascent, lineHeight, len(lines)) <= toleratedHeight {
			if fontSize < maxFontSize {
				largerLines, largerLH, largerTI, largerAscent := wrapAndMeasure(fontSize + 1)
				if len(largerLines) == len(lines) &&
					blockHeight(largerTI, largerAscent, largerLH, len(largerLines)) <= toleratedHeight {
					return textLayout{
						fontSize:   float64(fontSize + 1),
						lineHeight: largerLH,
						topInset:   largerTI,
						lines:      largerLines,
					}
				}
			}
			return textLayout{
				fontSize:   float64(fontSize),
				lineHeight: lineHeight,
				topInset:   topInset,
				lines:      lines,
			}
		}
	}

	fontSize := minFontSize
	lines, lineHeight, topInset, ascent := wrapAndMeasure(fontSize)
	measureFn := func(s string) int { return measureLinePx(s, float64(fontSize)) }
	maxLines := max(1, (toleratedHeight-topInset-ascent)/lineHeight+1)
	if len(lines) > maxLines {
		lines = truncateWithEllipsisFn(lines, maxLines, maxWidth, measureFn)
	}
	return textLayout{
		fontSize:   float64(fontSize),
		lineHeight: lineHeight,
		topInset:   topInset,
		lines:      lines,
	}
}

func wrapTextToWidth(text string, maxWidth int, face font.Face) []string {
	measureFn := func(s string) int { return measureStringPx(face, s) }
	splitFn := func(word string, maxW int) (string, string) { return fitWordToWidth(word, maxW, face) }
	return wrapLines(text, maxWidth, measureFn, splitFn)
}

func wrapLines(text string, maxWidth int, measureFn func(string) int, splitFn func(string, int) (string, string)) []string {
	words := stringsFields(text)
	if len(words) == 0 {
		return nil
	}
	words = expandCJKTokens(words)
	if maxWidth <= 0 {
		return []string{stringsJoin(words, " ")}
	}

	lines := make([]string, 0, len(words))
	current := ""

	flushCurrent := func() {
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
	}

	for _, word := range words {
		for measureFn(word) > maxWidth {
			if current != "" {
				flushCurrent()
			}
			head, tail := splitFn(word, maxWidth)
			lines = append(lines, head)
			word = tail
		}
		if word == "" {
			continue
		}

		if current == "" {
			current = word
			continue
		}

		sep := " "
		if isCJKWord(word) || isCJKLastRune(current) {
			sep = ""
		}
		candidate := current + sep + word
		if measureFn(candidate) <= maxWidth {
			current = candidate
			continue
		}

		flushCurrent()
		current = word
	}

	flushCurrent()
	return rebalanceWrappedLines(lines, maxWidth, measureFn)
}

func rebalanceWrappedLines(lines []string, maxWidth int, measureFn func(string) int) []string {
	if len(lines) < 2 || maxWidth <= 0 {
		return lines
	}
	lastIdx := len(lines) - 1
	prev := stringsTrimSpace(lines[lastIdx-1])
	last := stringsTrimSpace(lines[lastIdx])
	if prev == "" || last == "" {
		return lines
	}
	if float64(measureFn(last)) >= float64(maxWidth)*0.45 {
		return lines
	}
	prevWords := stringsFields(prev)
	if len(prevWords) < 2 {
		return lines
	}
	absInt := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	bestPrev := prev
	bestLast := last
	bestDelta := absInt(measureFn(prev) - measureFn(last))
	for take := 1; take <= 2 && len(prevWords)-take >= 1; take++ {
		moved := stringsJoin(prevWords[len(prevWords)-take:], " ")
		newPrev := stringsJoin(prevWords[:len(prevWords)-take], " ")
		newLast := stringsTrimSpace(moved + " " + last)
		if measureFn(newPrev) > maxWidth || measureFn(newLast) > maxWidth {
			continue
		}
		delta := absInt(measureFn(newPrev) - measureFn(newLast))
		if delta < bestDelta {
			bestPrev = newPrev
			bestLast = newLast
			bestDelta = delta
		}
	}
	if bestPrev == prev && bestLast == last {
		return lines
	}
	out := append([]string(nil), lines...)
	out[lastIdx-1] = bestPrev
	out[lastIdx] = bestLast
	return out
}

func truncateWithEllipsis(lines []string, maxLines int, maxWidth int, face font.Face) []string {
	return truncateWithEllipsisFn(lines, maxLines, maxWidth, func(s string) int { return measureStringPx(face, s) })
}

func truncateWithEllipsisFn(lines []string, maxLines int, maxWidth int, measureFn func(string) int) []string {
	truncated := make([]string, maxLines)
	copy(truncated, lines[:maxLines])

	ellipsis := "…"
	bodyWidth := maxWidth - measureFn(ellipsis)
	if bodyWidth < 0 {
		bodyWidth = 0
	}

	last := truncated[len(truncated)-1]
	if measureFn(last) > bodyWidth {
		runes := []rune(last)
		fit := ""
		for i := len(runes); i > 0; i-- {
			prefix := stringsTrimRight(string(runes[:i]), " ")
			if measureFn(prefix) <= bodyWidth {
				fit = prefix
				break
			}
		}
		last = fit
	}
	truncated[len(truncated)-1] = last + ellipsis
	return truncated
}

func stringsTrimSpace(s string) string {
	return stringsTrimRight(stringsTrimLeft(s, " \t\n\r"), " \t\n\r")
}

func stringsTrimLeft(s, cutset string) string {
	for len(s) > 0 && containsRune(cutset, rune(s[0])) {
		s = s[1:]
	}
	return s
}

func stringsTrimRight(s, cutset string) string {
	for len(s) > 0 && containsRune(cutset, rune(s[len(s)-1])) {
		s = s[:len(s)-1]
	}
	return s
}

func stringsFields(s string) []string {
	var result []string
	current := ""
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func stringsJoin(elems []string, sep string) string {
	if len(elems) == 0 {
		return ""
	}
	n := len(sep) * (len(elems) - 1)
	for _, e := range elems {
		n += len(e)
	}
	b := make([]byte, n)
	pos := 0
	for i, e := range elems {
		if i > 0 {
			pos += copy(b[pos:], sep)
		}
		pos += copy(b[pos:], e)
	}
	return string(b)
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
