//go:build !js

// The native build runs a .nex headless for a number of frames and writes
// the last picture, for tests and for looking at a program without a page:
//
//	nexdirect program.nex frames out.png [taps...]
//
// A tap is a frame number at which SPACE is pressed for five frames, or
// frame:row:mask for another key of the keyboard matrix. The folder named
// by NEXDIRECT_FILES is the card: its files are offered to the program, and
// what the program writes lands there.
package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type tap struct{ row, mask int }

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: nexdirect program.nex frames out.png [frame | frame:row:mask ...]")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	count, _ := strconv.Atoi(os.Args[2])
	taps := map[int]tap{}
	for _, arg := range os.Args[4:] {
		parts := strings.Split(arg, ":")
		at, _ := strconv.Atoi(parts[0])
		t := tap{7, 1} // SPACE
		if len(parts) == 3 {
			t.row, _ = strconv.Atoi(parts[1])
			t.mask, _ = strconv.Atoi(parts[2])
		}
		taps[at] = t
	}
	files := map[string][]byte{}
	folder := os.Getenv("NEXDIRECT_FILES")
	if folder != "" {
		entries, _ := os.ReadDir(folder)
		for _, e := range entries {
			if content, err := os.ReadFile(filepath.Join(folder, e.Name())); err == nil {
				files[e.Name()] = content
			}
		}
	}
	m, err := start(data, files)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	m.card.Saved = func(name string, content []byte) {
		fmt.Printf("saved %s, %d bytes\n", name, len(content))
		if folder != "" {
			_ = os.WriteFile(filepath.Join(folder, name), content, 0o644)
		}
	}
	stop := startProfile()
	began := time.Now()
	for done := 0; done < count; done++ {
		if t, ok := taps[done]; ok {
			m.kbd.PressMatrixKey(t.row, byte(t.mask), true)
		}
		if t, ok := taps[done-5]; ok {
			m.kbd.PressMatrixKey(t.row, byte(t.mask), false)
		}
		m.frames(1)
		m.ula.RenderAudioFrame()
		m.ula.Render()
	}
	stop()
	fmt.Printf("%d frames in %v, PC %04x, SP %04x\n", count, time.Since(began), m.cpu.PC, m.cpu.SP)
	img := m.ula.Render()
	out, err := os.Create(os.Args[3])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer out.Close()
	if err := png.Encode(out, img); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
