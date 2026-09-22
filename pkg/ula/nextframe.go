package ula

import "image"

// FrameComposer is what the whole-frame render path needs of a Next
// compositor: one call composes a 320-wide row of the 320x256 frame over the
// ULA's output for that row. pkg/next/compositor.Compositor satisfies it.
type FrameComposer interface {
	ComposeFrameRow(frameY int, base, dst []byte)
}

// wantsFrameRender reports whether the Next compositor wants the display
// composed a whole frame row at a time: Layer 2 at 320x256, where every
// layer covers the borders too and the paper-then-patches path cannot keep
// NextReg 0x15's order. (640x256 keeps the wide path.)
func (u *ULA) wantsFrameRender() bool {
	if u.nextCompositor == nil || !u.nextCompositor.HiResLayer2Active() {
		return false
	}
	_, ok := u.nextCompositor.(FrameComposer)
	return ok && u.nextCompositor.Layer2Width() == TotalWidth
}

// renderNextFrame builds the Next's whole 320x256 frame a row at a time
// (ComposeFrameRow), with the ULA's paper and border, or its off-state
// fill, as the base of every row. The Copper is stepped a line at a time
// here, ahead of composing the line, in raster order: the paper and the
// visible bottom border first, then the lines in the vertical blank, then
// the top border, which the beam draws last. A Copper write inside a line
// is seen from that line's start (the paper path composes a column at a
// time, this path a line at a time).
func (u *ULA) renderNextFrame() *image.RGBA {
	composer := u.nextCompositor.(FrameComposer)
	const top = (FullHeight - TotalHeight) / 2 // the image is the middle of the frame
	if u.nextFullImg == nil {
		u.nextFullImg = image.NewRGBA(image.Rect(0, 0, TotalWidth, FullHeight))
	}
	dst := u.nextFullImg
	if u.compositorRow == nil {
		u.compositorRow = make([]byte, TotalWidth*4)
	}
	base := u.compositorRow
	edge := u.palette[u.BorderColour&0x0F]
	if u.ulaOutputDisabled {
		edge = u.ulaDisabledFill()
	}
	columnsPerLine := TStatesPerLineFor(u.mem.GetCurrentModel()) * 2
	lines := LinesPerFrameFor(u.mem.GetCurrentModel())
	journal := u.nextRasterLog
	if journal != nil {
		journal.BeginReplay()
	}
	stepping := u.nextCopper != nil && !u.nextCopper.Idle()
	for line := 0; line < lines; line++ {
		if journal != nil && line < ScreenHeight {
			journal.ApplyThrough(line)
		}
		if stepping {
			for hc := 0; hc < columnsPerLine; hc++ {
				u.copperDebt = u.stepCopperColumn(line, hc, u.copperDebt)
			}
		}
		// video line 0 is the paper's first row, frame row 32; the top
		// border's rows come round at the end of the frame
		row := line + BorderTop + top
		if row >= lines {
			row -= lines
		}
		if row >= FullHeight {
			continue
		}
		if row >= top && row < top+TotalHeight {
			start := (row - top) * u.img.Stride
			copy(base, u.img.Pix[start:start+TotalWidth*4])
		} else {
			for x := 0; x < TotalWidth*4; x += 4 {
				base[x], base[x+1], base[x+2], base[x+3] = edge.R, edge.G, edge.B, 0xFF
			}
		}
		composer.ComposeFrameRow(row, base, dst.Pix[row*dst.Stride:row*dst.Stride+TotalWidth*4])
	}
	if journal != nil {
		journal.EndReplay()
	}
	return dst
}

// FullHeight is the Next's whole frame: 256 lines, the paper at row 32.
const FullHeight = 256
