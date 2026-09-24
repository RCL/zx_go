package compositor

import (
	"github.com/conorarmstrong/zx_go/pkg/next/layer2"
	"github.com/conorarmstrong/zx_go/pkg/next/palette"
	"testing"
)

// NR$68 disables the ULA independently of NR$14. A host's opaque fill
// must not cover lower layers, including black and the darkest blue.
func TestDisabledULADoesNotCoverLayer2(t *testing.T) {
	for _, wide := range []bool{false, true} {
		for mode := ModeSLU; mode <= ModeULS; mode++ {
			pal := palette.NewBank()
			pal.Select(palette.PaletteLayer2First)
			pal.Active().Set(5, 1) // RGB333 darkest blue: its top eight bits are zero
			bank := make([]byte, 16384)
			for i := range bank {
				bank[i] = 5
			}
			l2 := layer2.New(&fakeBanks{banks: map[int][]byte{0: bank, 1: bank, 2: bank, 3: bank, 4: bank}})
			l2.SetActiveBank(0)
			l2.SetEnabled(true)
			c := New(pal, l2)
			c.SetPrioritySource(fixedPriority{mode})
			c.SetTransparency(0xe3)
			c.SetFallbackColour(36, 73, 109)
			base := make([]byte, FullWidth*4)
			dst := make([]byte, len(base))
			for x := 0; x < FullWidth; x++ {
				base[x*4] = 255
				base[x*4+3] = 255
			}
			render := func() { c.ComposeScanline(0, base, dst) }
			if wide {
				l2.SetResolution(1)
				l2.SetClip(0, 159, 0, 255)
				render = func() { c.ComposeFrameRow(0, base, dst) }
			}
			c.SetULAOutputDisabled(true)
			render()
			if got := [4]byte{dst[0], dst[1], dst[2], dst[3]}; got != [4]byte{0, 0, 36, 255} {
				t.Fatalf("wide=%t mode=%d disabled ULA covered dark blue: %v", wide, mode, got)
			}
			c.SetULAOutputDisabled(false)
			render()
			ulaOnTop := mode == ModeSUL || mode == ModeUSL || mode == ModeULS
			if (dst[0] == 255) != ulaOnTop {
				t.Fatalf("wide=%t mode=%d re-enable ULA: %v", wide, mode, dst[:4])
			}
			// Colour-keying remains independent: NR$14=0 really does hide dark blue.
			c.SetULAOutputDisabled(true)
			c.SetTransparency(0)
			render()
			if got := [4]byte{dst[0], dst[1], dst[2], dst[3]}; got != [4]byte{36, 73, 109, 255} {
				t.Fatalf("wide=%t mode=%d fallback: %v", wide, mode, got)
			}
		}
	}
}

func TestDisabledULADoesNotContributeToBlend(t *testing.T) {
	pal := palette.NewBank()
	pal.Select(palette.PaletteLayer2First)
	pal.Active().Set(5, 1)
	bank := make([]byte, 16384)
	for i := range bank {
		bank[i] = 5
	}
	l2 := layer2.New(&fakeBanks{banks: map[int][]byte{0: bank, 1: bank, 2: bank}})
	l2.SetActiveBank(0)
	l2.SetEnabled(true)
	c := New(pal, l2)
	c.SetPrioritySource(fixedPriority{ModeBlend})
	c.SetTransparency(0xe3)
	base := make([]byte, Width*4)
	dst := make([]byte, len(base))
	for x := 0; x < Width; x++ {
		base[x*4] = 255
		base[x*4+3] = 255
	}
	c.SetULAOutputDisabled(true)
	c.ComposeScanline(0, base, dst)
	if dst[0] != 0 || dst[2] != 36 {
		t.Fatalf("disabled ULA contributed red to blend: %v", dst[:4])
	}
	c.SetULAOutputDisabled(false)
	c.ComposeScanline(0, base, dst)
	if dst[0] != 255 {
		t.Fatalf("enabled ULA missing from blend: %v", dst[:4])
	}
}
