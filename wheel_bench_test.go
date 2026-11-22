package base

import (
	"testing"
	"unsafe"

	"github.com/vivaui/viva/gfx"
)

func BenchmarkPR349Wheel(b *testing.B) {
	for _, panel := range []bool{false, true} {
		name := "page"
		if panel {
			name = "panel"
		}
		b.Run(name, func(b *testing.B) {
			var view View
			canvas := testCanvas()
			f := view.Begin(canvas, gfx.LightTheme(), Rect{Width: 1000, Height: 300})
			f.Row().Fill().Children(func() {
				for i := range 10 {
					f.ScrollArea(gfx.Item(i + 1)).Width(100).Children(func() {
						for range 50 {
							f.Slot().Height(30)
						}
					})
				}
			})
			if panel {
				anchor := f.Slot().Absolute().Top(0).Left(0).Size(1, 1)
				f.Popover(anchor, PopoverSpec{Width: 100, Instant: true, Scope: gfx.Item(12)}).Children(func() {
					f.ScrollArea(gfx.Item(11)).FillWidth().Height(150).Children(func() { f.Slot().Height(1500) })
				})
			}
			f.End()
			event := gfx.ScrollEvent{X: 50, Y: 50, DeltaY: 1, Precise: true}
			if result := view.Wheel(event); !result.Handled || !result.Repaint {
				b.Fatalf("wheel has no scroll area: %+v", result)
			}
			b.ReportAllocs()
			for b.Loop() {
				event.DeltaY = -event.DeltaY
				view.Wheel(event)
			}
		})
	}
}

func TestPR349RoutingSizes(t *testing.T) {
	t.Logf("fieldEntry=%d tableRoute=%d drawerState=%d", unsafe.Sizeof(fieldEntry{}), unsafe.Sizeof(tableRoute{}), unsafe.Sizeof(drawerState{}))
}
