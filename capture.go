package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/headless"
	"github.com/vivaui/viva/ui"
)

func main() {
	for _, dark := range []bool{false, true} {
		for _, kind := range []string{"popover", "alert", "drawer", "nonmodal-drawer"} {
			if err := capture(kind, dark); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
	}
}

func capture(kind string, dark bool) error {
	theme, themeName := gfx.LightTheme(), "light"
	if dark {
		theme, themeName = gfx.DarkTheme(), "dark"
	}
	var tree gfx.Tree
	scope := tree.Sub(tree.Feature("wheel-evidence"), tree.Name("alert"))
	rows := make(ui.TableRows, 60)
	for i := range rows {
		rows[i] = []string{fmt.Sprintf("Background row %02d", i)}
	}
	var table ui.TableState
	var area ui.Box
	open := true
	invalidator := new(app.Invalidator)
	page := ui.NewPage(ui.PageConfig{Tree: &tree, Theme: &theme, View: func(c ui.C) {
		body := func() {
			area = ui.Scroll(c).FillWidth().Height(150).Children(func() {
				for i := range 30 {
					ui.Text(c, fmt.Sprintf("Panel row %02d", i)).Height(30).Shrink(0)
				}
			})
		}
		ui.Column(c).Fill().Children(func() {
			anchor := ui.Button(c, "Projects")
			if kind == "popover" {
				ui.PopoverWith(c, anchor, &open, ui.PopoverSpec{Width: 240}, body)
			}
			ui.Table(c, &table, ui.TableSpec{
				Columns: []components.TableColumn{{}}, Rows: tree.Sub(scope, tree.Name("rows")), Source: &rows,
			}).FillWidth().Height(420).Shrink(0)
		})
		switch kind {
		case "alert":
			ui.AlertDialog(c, &open, ui.AlertDialogSpec{
				Title: "Discard changes?", Description: "This cannot be undone.",
				Scope: scope.At(0), ActionID: scope.At(1), CancelID: scope.At(2),
				ActionLabel: "Discard", CancelLabel: "Cancel",
			})
		case "drawer", "nonmodal-drawer":
			ui.Drawer(c, &open, "Inspector", ui.DrawerSpec{Size: 280, NonModal: kind == "nonmodal-drawer"}, body, func() { ui.Button(c, "Done") })
		}
	}})
	session, err := headless.Open(ui.Configure(page, app.Config{
		Title: "Wheel panel routing", Width: 640, Height: 480, Invalidator: invalidator,
	}), nil, headless.Options{Scale: 1})
	if err != nil {
		return err
	}
	defer session.Close()
	if err := session.Advance(time.Second); err != nil {
		return err
	}
	wheel := func(x, y, dy float32) error {
		if err := session.Scroll(gfx.ScrollEvent{X: x, Y: y, DeltaY: dy, Precise: true}); err != nil {
			return err
		}
		invalidator.Invalidate()
		return session.Driver().Pump()
	}
	insideOffset := float32(0)
	if kind == "alert" {
		if err := wheel(320, 240, 60); err != nil {
			return err
		}
	} else {
		r := area.Rect()
		x, y := r.X+r.Width/2, r.Y+r.Height/2
		if err := wheel(x, y, 50000); err != nil {
			return err
		}
		if err := wheel(x, y, 60); err != nil {
			return err
		}
		insideOffset = area.ScrollState().Offset
	}
	if table.Scroll.Offset != 0 {
		return fmt.Errorf("%s/%s background moved through the panel: %v", kind, themeName, table.Scroll.Offset)
	}
	if kind != "alert" {
		x, y := float32(10), float32(240)
		if kind == "popover" {
			x = 620
		}
		if err := wheel(x, y, 30); err != nil {
			return err
		}
	}
	want := float32(0)
	if kind == "popover" || kind == "nonmodal-drawer" {
		want = 30
	}
	if table.Scroll.Offset != want {
		return fmt.Errorf("%s/%s beside panel: %v, want %v", kind, themeName, table.Scroll.Offset, want)
	}
	path, err := filepath.Abs(filepath.Join("build", "pr349-"+kind+"-"+themeName+".png"))
	if err != nil {
		return err
	}
	if err := session.Screenshot(path); err != nil {
		return err
	}
	fmt.Printf("%s panelOffset=%v backgroundOffset=%v\n", path, insideOffset, table.Scroll.Offset)
	return nil
}
