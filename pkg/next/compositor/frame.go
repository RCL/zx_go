package compositor

import "github.com/conorarmstrong/zx_go/pkg/next/palette"

// FullHeight is the Next's whole frame, borders included: 320x256, the paper
// at (32, 32).
const FullHeight = 256

// ComposeFrameRow composes one row of the whole 320x256 frame. base is the
// ULA's output for that row as RGBA (the paper's pixels, or the border
// colour, or the fallback when the ULA is off), and every Next layer is
// drawn over it in NextReg 0x15 order: Layer 2 at 256x192 covers the paper
// only, at 320x256 the whole row; the tilemap and the sprites are
// frame-relative anyway. The paper path (ComposeScanlineRange plus the
// border patches) composes the paper in that order and then paints the
// borders a layer at a time; with Layer 2 at 320x256 every layer reaches
// every pixel, so the order matters everywhere, and this is what the display
// path uses then.
func (c *Compositor) ComposeFrameRow(frameY int, base, dst []byte) {
	if len(dst) < FullWidth*4 || len(base) < FullWidth*4 {
		return
	}
	if c.pal == nil {
		copy(dst[:FullWidth*4], base[:FullWidth*4])
		return
	}
	r := rowLayers{ulaRGBA: base, dst: dst}
	if c.l2 != nil && c.l2.Enabled() {
		if pal := c.pal.PaletteForLayer(palette.LayerLayer2); pal != nil {
			switch c.l2.LineWidth() {
			case FullWidth:
				c.l2.RenderScanlineEnabled(frameY, c.frameL2Scratch[:], c.frameL2Enabled[:])
				r.l2Scan, r.l2En, r.l2Pal = c.frameL2Scratch[:], c.frameL2Enabled[:], pal
			case Width:
				// 256x192: the paper's rows only, at the paper's columns
				for i := range c.frameL2Enabled {
					c.frameL2Enabled[i] = 0
				}
				if paperY := frameY - SpriteFrameYTop; paperY >= 0 && paperY < paperHeight {
					c.l2.RenderScanlineEnabled(paperY, c.frameL2Scratch[BorderOffsetX:BorderOffsetX+Width],
						c.frameL2Enabled[BorderOffsetX:BorderOffsetX+Width])
				}
				r.l2Scan, r.l2En, r.l2Pal = c.frameL2Scratch[:], c.frameL2Enabled[:], pal
			}
		}
	}
	if c.tilemap != nil && c.tilemap.Enabled() && !c.tilemap.Is80Col() {
		if pal := c.pal.PaletteForLayer(palette.LayerTilemap); pal != nil {
			c.tilemap.RenderScanlineFlags(frameY, c.tilemapScratch[:], c.tilemapEn[:], c.tilemapBelow[:])
			r.tmScan, r.tmEn, r.tmBelow, r.tmPal = c.tilemapScratch[:], c.tilemapEn[:], c.tilemapBelow[:], pal
		}
	}
	if c.sprites != nil && c.sprites.Enabled() {
		if pal := c.pal.PaletteForLayer(palette.LayerSprites); pal != nil {
			for i := range c.spriteScratch {
				c.spriteScratch[i] = 0
			}
			c.sprites.RenderScanline(frameY, c.spriteScratch[:], FullWidth)
			r.spScan, r.spCovered, r.spPal = c.spriteScratch[:], c.sprites.Covered(), pal
		}
	}
	if r.l2Pal == nil && r.tmPal == nil && r.spPal == nil {
		copy(dst[:FullWidth*4], base[:FullWidth*4])
		return
	}
	c.composePixels(&r, 0, FullWidth)
}

// paperHeight is the classic paper's rows, which Layer 2 at 256x192 covers.
const paperHeight = 192
