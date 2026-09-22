package ula

import (
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/keyboard"
	"github.com/conorarmstrong/zx_go/pkg/memory"
	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// frameMock is a hi-res Layer 2 compositor that can compose whole-frame
// rows: it records each row's base and marks the row so the test can see
// what the display path fed it and where the result landed.
type frameMock struct {
	hiResL2Mock
	rows  int
	bases map[int][4]byte
}

func (m *frameMock) ComposeFrameRow(frameY int, base, dst []byte) {
	m.rows++
	m.bases[frameY] = [4]byte{base[0], base[1], base[2], base[3]}
	copy(dst, base)
	dst[4], dst[5], dst[6], dst[7] = byte(frameY), 0xCD, 0xEF, 0xFF // a marker in column 1
}

// With Layer 2 at 320x256 the display path composes all 256 frame rows: the
// middle 240 over the ULA's rows, the 8 above and below over the border
// colour, and the result is the 320x256 frame.
func TestRenderNextFrameComposesWholeFrame(t *testing.T) {
	testDir := "test_roms_next_frame"
	createTestROMs(t, testDir)
	defer cleanupTestROMs(testDir)
	mem, err := memory.New(testDir, roms.Model48K)
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	u := New(mem, keyboard.New())
	u.BorderColour = 2 // red
	m := &frameMock{hiResL2Mock: hiResL2Mock{width: 320}, bases: map[int][4]byte{}}
	u.SetNextCompositor(m)
	img := u.Render()
	if m.rows != FullHeight || m.wideL2Calls != 0 {
		t.Fatalf("composed %d frame rows and %d wide rows, want 256 and 0", m.rows, m.wideL2Calls)
	}
	if b := img.Bounds(); b.Dx() != TotalWidth || b.Dy() != FullHeight {
		t.Fatalf("frame is %dx%d, want 320x256", b.Dx(), b.Dy())
	}
	red := u.palette[2]
	for _, row := range []int{0, 7, 248, 255} {
		if b := m.bases[row]; b[0] != red.R || b[1] != red.G || b[2] != red.B {
			t.Errorf("frame row %d's base is the border colour: got %v", row, b)
		}
	}
	for _, row := range []int{8, 100, 247} {
		want := u.img.Pix[(row-8)*u.img.Stride : (row-8)*u.img.Stride+4]
		if b := m.bases[row]; b[0] != want[0] || b[1] != want[1] || b[2] != want[2] {
			t.Errorf("frame row %d's base is image row %d: got %v want %v", row, row-8, b, want)
		}
	}
	for _, row := range []int{0, 31, 32, 200, 255} {
		if p := img.Pix[row*img.Stride+4 : row*img.Stride+8]; p[0] != byte(row) || p[1] != 0xCD {
			t.Errorf("frame row %d landed elsewhere: %v", row, p)
		}
	}
	// 640 wide keeps the wide path
	w := &frameMock{hiResL2Mock: hiResL2Mock{width: 640}, bases: map[int][4]byte{}}
	u.SetNextCompositor(w)
	u.Render()
	if w.rows != 0 || w.wideL2Calls != TotalHeight {
		t.Errorf("640: %d frame rows, %d wide rows; want 0 and %d", w.rows, w.wideL2Calls, TotalHeight)
	}
}
