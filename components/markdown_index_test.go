package components

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	"github.com/vivaui/viva/base"
	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/markdown"
)

// linearOffsetAtReference mirrors the pre-index linear scan over v.pieces so
// tests can verify exact equivalence across dense grids of pointer positions.
func linearOffsetAtReference(v *MarkdownView, point gfx.Point) (int, bool) {
	if !v.painted || v.measure == nil || len(v.pieces) == 0 {
		return 0, false
	}
	x, y := point.X-v.paintX, point.Y-v.paintY
	best, bestScore := -1, float32(0)
	var previous *mdPiece
	for index := range v.pieces {
		piece := &v.pieces[index]
		if piece.plainLo < 0 {
			continue
		}
		if previous != nil && previous.y == piece.y && previous.plainHi < piece.plainLo && y >= piece.y && y <= piece.y+piece.h {
			start := previous.x + v.caretAdvance(previous, int(previous.plainHi-previous.plainLo))
			end := piece.x + v.caretAdvance(piece, 0)
			if end > start && x >= start && x <= end {
				gap := int(piece.plainLo - previous.plainHi)
				position := int(float32(gap)*(x-start)/(end-start) + 0.5)
				return gfx.GraphemeStart(unsafe.String(unsafe.SliceData(v.plain), len(v.plain)), int(previous.plainHi)+min(max(position, 0), gap)), true
			}
		}
		previous = piece
		dx, dy := float32(0), float32(0)
		if x < piece.x {
			dx = piece.x - x
		} else if x > piece.x+piece.w {
			dx = x - piece.x - piece.w
		}
		if y < piece.y {
			dy = piece.y - y
		} else if y > piece.y+piece.h {
			dy = y - piece.y - piece.h
		}
		score := dy*1_000_000 + dx
		if best < 0 || score < bestScore {
			best, bestScore = index, score
		}
	}
	if best < 0 {
		return 0, false
	}
	piece := &v.pieces[best]
	localX := min(max(x-piece.x, 0), piece.w)
	var local int
	if piece.mono {
		local = v.codeOffsetAt(piece, localX)
	} else {
		local = v.measure.TextOffsetAtStyle(piece.text, gfx.TextStyle{Size: piece.size, Weight: piece.weight, Italic: piece.italic}, localX)
	}
	local = min(max(local, 0), int(piece.plainHi-piece.plainLo))
	return int(piece.plainLo) + local, true
}

// linearCaretAtReference mirrors the pre-index linear scan over v.pieces so
// tests can verify exact equivalence for every byte offset.
func linearCaretAtReference(v *MarkdownView, offset int) (gfx.Rect, bool) {
	if !v.painted || v.measure == nil || len(v.pieces) == 0 {
		return gfx.Rect{}, false
	}
	value := unsafe.String(unsafe.SliceData(v.plain), len(v.plain))
	offset = gfx.GraphemeStart(value, offset)
	var chosen, before, after *mdPiece
	for index := range v.pieces {
		piece := &v.pieces[index]
		if piece.plainLo < 0 {
			continue
		}
		switch {
		case offset >= int(piece.plainLo) && offset <= int(piece.plainHi):
			if chosen == nil || piece.plainLo == int32(offset) && chosen.plainLo < int32(offset) {
				chosen = piece
			}
		case int(piece.plainHi) < offset:
			if before == nil || piece.plainHi > before.plainHi {
				before = piece
			}
		case int(piece.plainLo) > offset:
			if after == nil || piece.plainLo < after.plainLo {
				after = piece
			}
		}
	}
	if chosen == nil {
		if before != nil && after != nil && before.y == after.y {
			start := before.x + v.caretAdvance(before, int(before.plainHi-before.plainLo))
			end := after.x + v.caretAdvance(after, 0)
			fraction := float32(offset-int(before.plainHi)) / float32(after.plainLo-before.plainHi)
			return gfx.Rect{X: v.paintX + start + (end-start)*fraction, Y: v.paintY + before.y, Width: 1, Height: before.h}, true
		}
		if before == nil && after == nil {
			return gfx.Rect{}, false
		}
		if before == nil || after != nil && int(after.plainLo)-offset <= offset-int(before.plainHi) {
			chosen = after
			offset = int(after.plainLo)
		} else {
			chosen = before
			offset = int(before.plainHi)
		}
	}
	local := min(max(offset-int(chosen.plainLo), 0), int(chosen.plainHi-chosen.plainLo))
	advance := v.caretAdvance(chosen, local)
	return gfx.Rect{X: v.paintX + chosen.x + advance, Y: v.paintY + chosen.y, Width: 1, Height: chosen.h}, true
}

func TestMarkdownIndexMatchesLinearReferenceOnGrid(t *testing.T) {
	// Parse-built document combining blank lines inside code blocks, horizontal
	// rules, tables (including empty and wrapped cells), nested quotes, lists,
	// links, inline code, and multi-byte UTF-8.
	parsedSrc := strings.Join([]string{
		"# Overview",
		"",
		"before text with [link](https://example.com) and `code` 中文",
		"",
		"```go",
		"firstLine := 1",
		"",
		"",
		"",
		"lastLine := 2",
		"```",
		"",
		"after text",
		"",
		"---",
		"",
		"```",
		"",
		"",
		"",
		"onlyTrailing()",
		"```",
		"",
		"> outer quote line one",
		">> nested quote line two",
		"",
		"| Col A | Col B | Col C |",
		"| --- | :---: | ---: |",
		"| alpha |  | gamma wrapped across multiple words in a narrow column |",
		"|  |  |  |",
		"| delta | epsilon | zeta |",
		"",
		"1. First item",
		"2. Second item with **bold** and *italic*",
	}, "\n")

	var parsedDoc markdown.Document
	parsedDoc.Parse(parsedSrc)

	// Programmatic document with Labeled blocks and mixed block kinds.
	var labeledDoc markdown.Document
	labeledDoc.Reset()
	labeledDoc.AppendParagraph(0, 0, []markdown.Span{{Text: "Lead paragraph before labeled rows."}})
	labeledDoc.AppendCode(0, "java", "a();\n\n\n\nb();")
	labeledDoc.AppendRule(0)
	labeledDoc.AppendLabeled(0, []markdown.Span{{Text: "Since:"}}, []markdown.Span{{Text: "1.2"}})
	labeledDoc.AppendLabeled(0, []markdown.Span{{Text: "See also:"}}, []markdown.Span{
		{Text: "Collection", Style: markdown.Mono, Link: "symbol:Collection"},
		{Text: ", "},
		{Text: "List", Style: markdown.Mono, Link: "symbol:List"},
	})
	labeledDoc.AppendLabeled(0, []markdown.Span{{Text: "Type parameters:"}}, []markdown.Span{
		{Text: "<E>", Style: markdown.Mono},
		{Text: " – the type of elements in this list wrapped onto another line"},
	})

	cases := []struct {
		name  string
		doc   *markdown.Document
		width float32
	}{
		{name: "parsed-full-features", doc: &parsedDoc, width: 320},
		{name: "programmatic-labeled", doc: &labeledDoc, width: 300},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canvas := markdownCanvas()
			var view MarkdownView
			h := view.Layout(canvas, MarkdownStyle{Size: 13, LineHeight: 20}, tc.doc, tc.width)
			if h <= 0 {
				t.Fatalf("expected positive height, got %v", h)
			}
			const originX, originY float32 = 16, 24
			canvas.Reset()
			view.Paint(canvas, MarkdownPaint{
				Look: MarkdownLookFor(gfx.Theme{}),
				X:    originX,
				Y:    originY,
			})

			// Verify OffsetAt on a dense grid covering margins, gaps, blank code
			// lines, rules, table cell gaps, and boundaries.
			for y := float32(-10); y <= h+20; y += 5 {
				for x := float32(-10); x <= tc.width+20; x += 17 {
					pt := gfx.Point{X: originX + x, Y: originY + y}
					gotOff, gotOK := view.OffsetAt(pt)
					wantOff, wantOK := linearOffsetAtReference(&view, pt)
					if gotOff != wantOff || gotOK != wantOK {
						t.Fatalf("OffsetAt(%v) = (%d, %v), want linear (%d, %v)", pt, gotOff, gotOK, wantOff, wantOK)
					}
				}
			}

			// Verify CaretAt for every byte offset in [0, TextLen].
			n := view.TextLen()
			for off := 0; off <= n; off++ {
				gotRect, gotOK := view.CaretAt(off)
				wantRect, wantOK := linearCaretAtReference(&view, off)
				if gotRect != wantRect || gotOK != wantOK {
					t.Fatalf("CaretAt(%d) = (%+v, %v), want linear (%+v, %v)", off, gotRect, gotOK, wantRect, wantOK)
				}
			}
		})
	}
}

func TestMarkdownEmptyCodeLinesStayInCodeBlock(t *testing.T) {
	// Direct regression for blank lines inside a code block: clicking an empty
	// line between code lines must resolve to the code block, not before/after text.
	src := "before text\n\n```\nline1\n\n\n\nline5\n```\n\nafter text\n"
	var doc markdown.Document
	doc.Parse(src)

	canvas := markdownCanvas()
	var view MarkdownView
	h := view.Layout(canvas, MarkdownStyle{Size: 13, LineHeight: 20}, &doc, 400)
	canvas.Reset()
	view.Paint(canvas, MarkdownPaint{Look: MarkdownLookFor(gfx.Theme{})})

	plain := view.SelectedText(base.TextRange{Anchor: 0, Focus: view.TextLen()})
	codeLo := strings.Index(plain, "line1")
	codeHi := strings.Index(plain, "line5") + len("line5")
	if codeLo < 0 || codeHi <= codeLo {
		t.Fatalf("expected code block text in plain %q", plain)
	}
	r1, ok1 := view.CaretAt(codeLo)
	r5, ok5 := view.CaretAt(codeHi)
	if !ok1 || !ok5 || r5.Y <= r1.Y+r1.Height {
		t.Fatalf("expected empty lines between line1 (%+v) and line5 (%+v), height=%v", r1, r5, h)
	}
	for y := r1.Y + r1.Height + 2; y < r5.Y; y += 6 {
		off, ok := view.OffsetAt(gfx.Point{X: 40, Y: y})
		if !ok || off < codeLo || off > codeHi {
			t.Fatalf("empty code line at y=%v resolved to offset %d (plain=%q), want inside [%d,%d]", y, off, plain, codeLo, codeHi)
		}
	}
}

func TestMarkdownBuilderLabeledAndOpaqueLinks(t *testing.T) {
	var doc markdown.Document
	doc.Reset()
	doc.AppendParagraph(0, 0, []markdown.Span{
		{Text: "Iterators returned by "},
		{Text: "iterator", Style: markdown.Mono, Link: "symbol:#iterator()"},
		{Text: " are "},
		{Text: "fail-fast", Style: markdown.Italic},
		{Text: "."},
	})
	doc.AppendLabeled(0, []markdown.Span{{Text: "Since:"}}, []markdown.Span{{Text: "1.2"}})
	doc.AppendLabeled(0, []markdown.Span{{Text: "See also:"}}, []markdown.Span{
		{Text: "Collection", Style: markdown.Mono, Link: "symbol:Collection"},
		{Text: ", "},
		{Text: "List", Style: markdown.Mono, Link: "symbol:List"},
	})
	doc.AppendLabeled(0, []markdown.Span{{Text: "Type parameters:"}}, []markdown.Span{
		{Text: "<E>", Style: markdown.Mono},
		{Text: " – the type of elements"},
	})
	doc.AppendLabeled(0, []markdown.Span{{Text: "Author:"}}, []markdown.Span{{Text: "Josh"}})

	canvas := markdownCanvas()
	var view MarkdownView
	h := view.Layout(canvas, MarkdownStyle{Size: 13, LineHeight: 20}, &doc, 460)
	if h <= 0 {
		t.Fatalf("expected positive height, got %v", h)
	}
	if view.LinkCount() != 3 {
		t.Fatalf("LinkCount = %d, want 3", view.LinkCount())
	}
	if got := view.Link(0); got != "symbol:#iterator()" {
		t.Fatalf("Link(0) = %q, want opaque symbol target", got)
	}
	if view.AllocatedBytes() <= 0 || doc.AllocatedBytes() <= 0 {
		t.Fatalf("expected positive AllocatedBytes: view=%d doc=%d", view.AllocatedBytes(), doc.AllocatedBytes())
	}

	// Verify aligned Labeled value columns start at the same X using public CaretAt
	// (comparing plain-text rows with each other and inline-code rows with each other,
	// since inline code pills include their internal horizontal padding).
	canvas.Reset()
	view.Paint(canvas, MarkdownPaint{Look: MarkdownLookFor(gfx.Theme{})})
	plain := view.SelectedText(base.TextRange{Anchor: 0, Focus: view.TextLen()})
	caretX := func(needle string) float32 {
		t.Helper()
		off := strings.Index(plain, needle)
		if off < 0 {
			t.Fatalf("missing %q in rendered text %q", needle, plain)
		}
		rect, ok := view.CaretAt(off)
		if !ok {
			t.Fatalf("CaretAt(%q=%d) returned ok=false", needle, off)
		}
		return rect.X
	}
	plainA, plainB := caretX("1.2"), caretX("Josh")
	monoA, monoB := caretX("Collection"), caretX("<E>")
	if plainA != plainB {
		t.Fatalf("labeled plain value column misaligned: %v vs %v", plainA, plainB)
	}
	if monoA != monoB {
		t.Fatalf("labeled mono value column misaligned: %v vs %v", monoA, monoB)
	}
	if monoA < plainA {
		t.Fatalf("expected inline-code value X (%v) >= plain value X (%v)", monoA, plainA)
	}
}

func TestMarkdownViewTallTableCellFourLevelIndex(t *testing.T) {
	// Single-row table where one cell wraps into >1000 visual lines: clipped
	// Paint and OffsetAt must visit only visible cell lines, not all 1000.
	tallSpans := make([]markdown.Span, 0, 2200)
	for i := 0; i < 1100; i++ {
		if i > 0 {
			tallSpans = append(tallSpans, markdown.Span{Text: markdown.Newline})
		}
		tallSpans = append(tallSpans, markdown.Span{Text: fmt.Sprintf("cell-line-%04d", i)})
	}
	var doc markdown.Document
	doc.AppendTable(0, 2, [][]markdown.TableCell{
		{
			{Spans: []markdown.Span{{Text: "Header A"}}},
			{Spans: []markdown.Span{{Text: "Header B"}}},
		},
		{
			{Spans: []markdown.Span{{Text: "Short"}}},
			{Spans: tallSpans},
		},
	})

	canvas := markdownCanvas()
	var view MarkdownView
	h := view.Layout(canvas, MarkdownStyle{Size: 13, LineHeight: 20}, &doc, 400)
	if h < 15000 {
		t.Fatalf("expected tall table height >= 15000, got %v", h)
	}

	// Count rows visited for a 200px viewport slice in the middle of the tall cell.
	clipTop, clipBottom := float32(10000), float32(10200)
	firstBlk, lastBlk := view.visibleBlockRange(true, clipTop, clipBottom)
	visited := 0
	view.visitVisibleRows(firstBlk, lastBlk, true, clipTop, clipBottom, func(*mdRowSpan) {
		visited++
	})
	if visited == 0 || visited > 20 {
		t.Fatalf("visited %d rows for 200px slice of 1100-line cell, want 1..20", visited)
	}

	// Paint the slice and verify OffsetAt + CaretAt round-trip inside the middle of the cell.
	canvas.Reset()
	view.Paint(canvas, MarkdownPaint{
		Look: MarkdownLookFor(gfx.Theme{}),
		X:    0,
		Y:    -clipTop,
		Clip: gfx.Rect{X: 0, Y: 0, Width: 400, Height: 200},
	})
	off, ok := view.OffsetAt(gfx.Point{X: 220, Y: 100})
	if !ok {
		t.Fatal("OffsetAt in tall cell returned ok=false")
	}
	rect, ok := view.CaretAt(off)
	if !ok {
		t.Fatalf("CaretAt(%d) returned ok=false", off)
	}
	if rect.Y < -25 || rect.Y > 225 {
		t.Fatalf("CaretAt(%d) Y=%v outside painted slice [0, 200]", off, rect.Y)
	}
}

func TestMarkdownViewLongQuoteBarAndShortBlocksIndex(t *testing.T) {
	// Counterexample check: one quote bar spanning many blocks does not force
	// visiting off-screen blocks or off-screen quote bars.
	var doc markdown.Document
	for i := 0; i < 600; i++ {
		doc.AppendParagraph(1, 0, []markdown.Span{{Text: fmt.Sprintf("Quoted paragraph %d", i)}})
	}
	canvas := markdownCanvas()
	var view MarkdownView
	h := view.Layout(canvas, MarkdownStyle{Size: 13, LineHeight: 20}, &doc, 420)
	if h < 10000 {
		t.Fatalf("expected tall quoted doc, got %v", h)
	}

	clipTop, clipBottom := float32(5000), float32(5200)
	firstBlk, lastBlk := view.visibleBlockRange(true, clipTop, clipBottom)
	if count := lastBlk - firstBlk; count == 0 || count > 20 {
		t.Fatalf("visible blocks = %d, want 1..20", count)
	}
	bars := view.visibleQuoteBars(0, true, clipTop, clipBottom)
	if len(bars) != 1 {
		t.Fatalf("visible quote bars at depth 0 = %d, want 1", len(bars))
	}
}

func BenchmarkMarkdownViewClippedPaintSteady(b *testing.B) {
	var doc markdown.Document
	for i := 0; i < 400; i++ {
		doc.AppendParagraph(1, 0, []markdown.Span{
			{Text: "Iterators returned by "},
			{Text: "iterator", Style: markdown.Mono, Link: "symbol:#iterator()"},
			{Text: " are "},
			{Text: "fail-fast", Style: markdown.Italic},
			{Text: " in paragraph."},
		})
	}
	canvas := markdownCanvas()
	var view MarkdownView
	view.Layout(canvas, MarkdownStyle{Size: 13, LineHeight: 20}, &doc, 500)
	look := MarkdownLookFor(gfx.Theme{})
	paint := MarkdownPaint{
		Look: look,
		X:    0,
		Y:    -4000,
		Clip: gfx.Rect{X: 0, Y: 0, Width: 500, Height: 300},
	}
	// Warm canvas command buffer and view scratch slices.
	canvas.Reset()
	view.Paint(canvas, paint)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		canvas.Reset()
		view.Paint(canvas, paint)
	}
}
