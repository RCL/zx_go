package compositor

import (
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/next/layer2"
	"github.com/conorarmstrong/zx_go/pkg/next/palette"
	"github.com/conorarmstrong/zx_go/pkg/next/sprite"
)

// A 320x256 Layer 2 (column-major: byte x*256+y of the 80K frame, 64
// columns to a 16K bank) with a sprite over it, composed as whole-frame
// rows: the sprite's transparent cells show Layer 2 in SLU, Layer 2 hides
// the sprite in LSU, and both reach the border rows, which the paper
// path could not order.
func TestComposeFrameRowOrdersLayersEverywhere(t *testing.T) {
	pal := palette.NewBank()
	pal.Select(palette.PaletteLayer2First)
	pal.Active().Set(5, 0b1_1100_0000) // L2 index 5 = red
	pal.Select(palette.PaletteSpritesFirst)
	pal.Active().Set(9, 0b0_0011_1000) // sprite index 9 = green
	pal.Select(0)

	// the whole frame index 5, in five banks of 64 columns
	banks := map[int][]byte{}
	for b := 0; b < 5; b++ {
		page := make([]byte, 16384)
		for i := range page {
			page[i] = 5
		}
		banks[b] = page
	}
	l2 := layer2.New(&fakeBanks{banks: banks})
	l2.SetActiveBank(0)
	l2.SetResolution(1)
	l2.SetClip(0, 159, 0, 255) // NR$18 for the whole 320x256 (x in pairs); reset is the paper
	l2.SetEnabled(true)

	// one 8bpp sprite at the top left corner of the frame (0, 0): its first
	// row is 9, E3, 9, E3, ...: green then a hole
	e := sprite.New()
	e.SetEnabled(true)
	e.SetOverBorder(true)
	e.SetPatternAddr(0)
	for i := 0; i < 256; i++ {
		v := byte(0xE3)
		if i < 16 && i%2 == 0 {
			v = 9
		}
		e.WritePatternByte(v)
	}
	e.Set(0, sprite.Attr{X: 0, Y: 0, Pattern: 0, Visible: true})

	c := New(pal, l2)
	c.SetSprites(e)
	base := make([]byte, FullWidth*4) // the border, black
	dst := make([]byte, FullWidth*4)
	red := func(x int) bool { return dst[x*4] != 0 && dst[x*4+1] == 0 && dst[x*4+2] == 0 }
	green := func(x int) bool { return dst[x*4] == 0 && dst[x*4+1] != 0 && dst[x*4+2] == 0 }

	c.SetPrioritySource(fixedPriority{ModeSLU})
	c.ComposeFrameRow(0, base, dst)
	if !green(0) || !red(1) || !green(2) || !red(100) || !red(319) {
		t.Errorf("SLU, frame row 0: sprite over Layer 2 with holes: %v %v %v, L2 elsewhere %v %v",
			dst[0:3], dst[4:7], dst[8:11], dst[400:403], dst[319*4:319*4+3])
	}
	c.SetPrioritySource(fixedPriority{ModeLSU})
	c.ComposeFrameRow(0, base, dst)
	if !red(0) || !red(1) {
		t.Errorf("LSU, frame row 0: Layer 2 over the sprite: %v %v", dst[0:3], dst[4:7])
	}
	// the bottom border row is Layer 2 too, and out of the sprite
	c.ComposeFrameRow(255, base, dst)
	if !red(0) || !red(160) {
		t.Errorf("frame row 255 is Layer 2: %v %v", dst[0:3], dst[640:643])
	}
	// Layer 2 off: the base shows, the sprite where it is
	l2.SetEnabled(false)
	c.SetPrioritySource(fixedPriority{ModeSLU})
	c.ComposeFrameRow(0, base, dst)
	if !green(0) || dst[4] != 0 || dst[5] != 0 || dst[6] != 0 {
		t.Errorf("Layer 2 off: sprite %v, base %v", dst[0:3], dst[4:7])
	}
}

// Layer 2 at 256x192 covers the paper only: frame rows above it and the
// columns beside it show the base.
func TestComposeFrameRowLayer2PaperOnly(t *testing.T) {
	pal := palette.NewBank()
	pal.Select(palette.PaletteLayer2First)
	pal.Active().Set(5, 0b1_1100_0000)
	pal.Select(0)
	bank := make([]byte, 16384)
	for i := range bank {
		bank[i] = 5
	}
	l2 := layer2.New(&fakeBanks{banks: map[int][]byte{0: bank, 1: bank, 2: bank}})
	l2.SetActiveBank(0)
	l2.SetEnabled(true)
	c := New(pal, l2)
	base := make([]byte, FullWidth*4)
	dst := make([]byte, FullWidth*4)
	c.ComposeFrameRow(SpriteFrameYTop, base, dst) // the paper's first row
	if dst[BorderOffsetX*4] == 0 || dst[(BorderOffsetX-1)*4] != 0 || dst[(BorderOffsetX+Width)*4] != 0 {
		t.Errorf("paper row: L2 at the paper's columns only: %v %v %v",
			dst[(BorderOffsetX-1)*4:(BorderOffsetX-1)*4+3], dst[BorderOffsetX*4:BorderOffsetX*4+3], dst[(BorderOffsetX+Width)*4:(BorderOffsetX+Width)*4+3])
	}
	c.ComposeFrameRow(SpriteFrameYTop-1, base, dst)
	if dst[BorderOffsetX*4] != 0 {
		t.Errorf("the row above the paper has no Layer 2: %v", dst[BorderOffsetX*4:BorderOffsetX*4+3])
	}
}
