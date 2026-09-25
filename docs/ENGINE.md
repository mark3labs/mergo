# mergo engine guide (for contributors)

mergo renders Mermaid diagrams natively in Go (no browser). Pipeline:

```
source ──diagram.Preprocess──▶ (front matter, %%{init}%%, comments stripped, type detected)
       ──<type>.Render──▶ *scene.Scene (vector display list, logical px)
       ──scene.Render(scale, viewport)──▶ *image.RGBA ──▶ kitty graphics / half blocks
```

## Packages

| package | purpose |
|---|---|
| `internal/theme` | Mermaid-compatible themes (`default`, `dark`, `forest`, `neutral`, `charm`), color parsing (`theme.ParseColor`, `theme.Hex`), helpers (`Mix`, `Lighten`, `ContrastText`, `PaletteColor(i)`). |
| `internal/scene` | Vector primitives: `Path` (MoveTo/LineTo/CubicTo/Rect/Ellipse/Arc/Polygon/BasisSpline...), `Text` (multi-line, anchor, valign, rotation), `Group`, markers (`scene.Marker`, `scene.Edge`), text metrics (`MeasureText`, `MeasureBlock`, `WrapText`), rasterizer. |
| `internal/layout` | Layered (Sugiyama) graph layout with clusters, edge labels, self loops. Node clip helpers (`ClipRect`, `ClipEllipse`, `ClipDiamond`, `ClipPolygon`). |
| `internal/diagram` | Registry (`diagram.Register`), preprocessing, `Config` (theme + raw config), label cleanup (`CleanLabel`), `Finish` (title + fit). |
| `internal/diagrams/<type>` | One package per diagram type. |
| `internal/mermaid` | Public API (`Render`, `RenderImage`, `RenderPNG`). Diagram packages are linked in via `register_<type>.go` blank imports. |
| `internal/devutil` | `devutil.ASCII(img, cols)` — a luminance ASCII preview for sanity-checking renders in tests/terminals without an image viewer. |

## Writing a diagram type

```go
package pie

func init() {
	diagram.Register(diagram.Type{
		Name:   "pie",
		Detect: diagram.Keyword("pie"),
		Render: Render,
	})
}

func Render(src string, cfg *diagram.Config) (*scene.Scene, error) {
	th := cfg.Theme
	sc := diagram.NewScene(th)
	// ... parse src, add paths/text in any coordinate system ...
	return diagram.Finish(sc, th, title), nil // adds title, fits with padding
}
```

Conventions:

* Coordinates are CSS-like pixels; base font size is `th.FontSize` (16).
* Colors in `scene` are straight (non-premultiplied) RGBA; alpha 0 = none.
* Use `scene.Style{Shadow: true}` on filled node shapes for a subtle drop shadow.
* Always measure text with `scene.MeasureText` / `scene.MeasureBlock` so the
  layout matches the rasterizer.
* Labels go through `diagram.CleanLabel` (quotes, `<br>`, entities, `#quot;`).
* Parsers must never panic; return `fmt.Errorf("line %d: ...")` on real
  syntax errors and silently ignore unknown-but-harmless statements
  (`click`, `accTitle`, styling you don't support yet...).
* Honor `style`/`classDef` colors where Mermaid supports them (`fill`,
  `stroke`, `color`, `stroke-width`, `stroke-dasharray`).

## Checking output

```sh
go run ./cmd/mmdpng -ascii 140 examples/flowchart/basic.mmd   # ASCII preview
go run ./cmd/mmdpng -theme dark examples/flowchart/basic.mmd /tmp/out.png
```
