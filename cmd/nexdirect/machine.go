// Package main is nexdirect: a Spectrum Next that runs one .nex file with
// no operating system. It is the emulator's Next hardware (Z80N, MMU,
// Layer 2, tilemap, sprites, Copper, zxnDMA, AY, DAC) assembled without
// the FPGA boot ROM, the divMMC ROM, an SD card or NextZXOS: the .nex is
// parsed and its banks copied into RAM, the CPU is started the way NEXLOAD
// leaves it, and the esxDOS file calls a program makes through RST 8 are
// answered from a card kept in memory. A browser page (GOOS=js) drives it a
// frame at a time; a native build runs it headless for tests.
//
// What this cannot do: anything that needs the OS (dot commands, the
// browser, +3DOS, most of esxDOS) or a ROM routine. A program that does
// only its own thing, with at most a config file to read and write, is what
// it is for.
package main

import (
	"fmt"
	"os"

	"github.com/conorarmstrong/zx_go/pkg/ay"
	"github.com/conorarmstrong/zx_go/pkg/keyboard"
	"github.com/conorarmstrong/zx_go/pkg/memory"
	"github.com/conorarmstrong/zx_go/pkg/next"
	"github.com/conorarmstrong/zx_go/pkg/next/compositor"
	"github.com/conorarmstrong/zx_go/pkg/next/copper"
	"github.com/conorarmstrong/zx_go/pkg/next/dac"
	"github.com/conorarmstrong/zx_go/pkg/next/divmmc"
	"github.com/conorarmstrong/zx_go/pkg/next/dma"
	"github.com/conorarmstrong/zx_go/pkg/next/keymap"
	"github.com/conorarmstrong/zx_go/pkg/next/layer2"
	"github.com/conorarmstrong/zx_go/pkg/next/nextregs"
	"github.com/conorarmstrong/zx_go/pkg/next/palette"
	rtcpkg "github.com/conorarmstrong/zx_go/pkg/next/rtc"
	"github.com/conorarmstrong/zx_go/pkg/next/sprite"
	"github.com/conorarmstrong/zx_go/pkg/next/tilemap"
	uartpkg "github.com/conorarmstrong/zx_go/pkg/next/uart"
	"github.com/conorarmstrong/zx_go/pkg/peripherals"
	"github.com/conorarmstrong/zx_go/pkg/roms"
	"github.com/conorarmstrong/zx_go/pkg/ula"
	"github.com/conorarmstrong/zx_go/pkg/z80"
)

// frameTStates is the ULA frame at 3.5 MHz in the 128K/+3 timing the Next
// boots in: (456 columns * 311 lines) / 2, as cmd/zx_go uses.
const frameTStates = 70908

// machine is one Spectrum Next with a program in it.
type machine struct {
	cpu    *z80.CPU
	mem    *memory.Memory
	ula    *ula.ULA
	kbd    *keyboard.Keyboard
	disp   *nextregs.Dispatcher
	im2    *next.IM2Driver
	card   *card
	frameT int
}

type dmaPortBus struct{ u *ula.ULA }

func (b dmaPortBus) WritePort(port uint16, val byte) { b.u.WritePort(port, val) }
func (b dmaPortBus) ReadPort(port uint16) byte {
	v, _ := b.u.ReadPort(port)
	return v
}

// newMachine assembles the Next hardware with nothing to boot: no FPGA boot
// ROM, no divMMC ROM, no card. The classic 48K and 128K ROM images the
// emulator embeds are loaded into the ROM banks as always; a .nex program
// does not run them, but it may map them.
func newMachine() (*machine, error) {
	// memory.New would look for a NextZXOS image on disk otherwise
	_ = os.Setenv("ZX_GO_SKIP_DISTRO_PRELOAD", "1")
	kbd := keyboard.New()
	mem, err := memory.New("roms", roms.ModelNext)
	if err != nil {
		return nil, fmt.Errorf("nexdirect: memory: %w", err)
	}
	u := ula.New(mem, kbd)
	u.EnableAudioCapture() // no audio device here; the host pulls the frames
	cpu := z80.New(mem, u)
	cpu.Variant = z80.VariantZ80N
	assert, pulse := next.FrameIntTiming(0x03, false)
	cpu.IntAssertTstate = uint64(assert)
	cpu.IntPulseTstates = uint64(pulse)

	pm := peripherals.NewPeripheralManager(mem, "roms")
	u.SetPeripherals(pm)

	disp := nextregs.New()
	ayEngine := ay.NewEngine()
	if existing := u.AY(); existing != nil {
		ayEngine.SetChip(0, existing)
	}
	l2 := layer2.New(mem)
	pal := palette.NewBank()
	prio := next.NewLayerPriority()
	sprites := sprite.New()
	sprites.SetLineClockBudget(ula.TStatesPerLineFor(roms.ModelNext) * 8)
	cop := copper.New()
	cop.SetRegWriter(disp)
	rtcEngine := rtcpkg.New()
	uartEngine := uartpkg.New()
	keymapEngine := keymap.New()
	tilemapLayer := tilemap.New(mem)
	dmaEngine := dma.New(mem)
	dacBank := dac.New()

	// The divMMC with no ROM: the automap logic is still there for the
	// esxDOS hook (see direct.go), the overlay reads as open bus.
	pager := divmmc.New(nil)
	mem.SetDivMMCRAM(pager)
	pager.SetMultifaceActiveFn(mem.MultifaceActive)
	pager.SetRom3Query(mem.DivMMCRom3Gate)

	next.Wire(next.WireOpts{
		Dispatcher:  disp,
		Memory:      mem,
		CPU:         cpu,
		AYEngine:    ayEngine,
		Layer2:      l2,
		Palette:     pal,
		Priority:    prio,
		Sprites:     sprites,
		Copper:      cop,
		RTC:         rtcEngine,
		UART:        uartEngine,
		Keymap:      keymapEngine,
		Tilemap:     tilemapLayer,
		DivMMCPager: pager,
	})
	cpu.NextRegs = disp
	u.SetNextRegs(disp)
	u.SetNextAY(ayEngine)

	comp := compositor.New(pal, l2)
	comp.SetSprites(sprites)
	u.SetNextSpritePort(sprites)
	comp.SetTilemap(tilemapLayer)
	comp.SetPrioritySource(prio)
	u.SetNextCompositor(comp)
	comp.SetULAPalette(u.Palette())

	// The live NextReg hooks next.Wire leaves to the host, as cmd/zx_go
	// installs them.
	disp.SetOnWrite(0x4C, func(d *nextregs.Dispatcher, val byte) {
		d.Store(0x4C, val&0x0F)
		comp.SetTilemapTransparency(val & 0x0F)
	})
	disp.SetOnRead(0x1F, func(*nextregs.Dispatcher) byte { return byte(u.ActiveVideoLine() & 0xFF) })
	disp.SetOnRead(0x1E, func(*nextregs.Dispatcher) byte { return byte((u.ActiveVideoLine() >> 8) & 0x01) })
	disp.SetOnWrite(0x2F, func(d *nextregs.Dispatcher, val byte) {
		d.Store(0x2F, val&0x03)
		tilemapLayer.SetScrollX(int(val&0x03)<<8 | int(d.ReadReg(0x30)))
	})
	disp.SetOnWrite(0x30, func(d *nextregs.Dispatcher, val byte) {
		d.Store(0x30, val)
		tilemapLayer.SetScrollX(int(d.ReadReg(0x2F)&0x03)<<8 | int(val))
	})
	disp.SetOnWrite(0x31, func(d *nextregs.Dispatcher, val byte) {
		d.Store(0x31, val)
		tilemapLayer.SetScrollY(int(val))
	})
	disp.SetOnWrite(0x14, func(d *nextregs.Dispatcher, val byte) {
		d.Store(0x14, val)
		comp.SetTransparency(val)
	})
	expandRGB332 := func(val byte) (byte, byte, byte) {
		r3, g3, b2 := (val>>5)&7, (val>>2)&7, val&3
		return r3<<5 | r3<<2 | r3>>1, g3<<5 | g3<<2 | g3>>1, b2<<6 | b2<<4 | b2<<2 | b2
	}
	disp.SetOnWrite(0x4A, func(d *nextregs.Dispatcher, val byte) {
		d.Store(0x4A, val)
		comp.SetFallbackColour(expandRGB332(val))
	})
	comp.SetFallbackColour(expandRGB332(0xE3))
	disp.SetOnWrite(0x4B, func(d *nextregs.Dispatcher, val byte) {
		d.Store(0x4B, val)
		comp.SetSpriteTransparency(val)
	})
	disp.SetOnWrite(0x26, func(d *nextregs.Dispatcher, val byte) {
		d.Store(0x26, val)
		u.SetULAScrollX(val)
	})
	disp.SetOnWrite(0x27, func(d *nextregs.Dispatcher, val byte) {
		d.Store(0x27, val)
		u.SetULAScrollY(val)
	})
	disp.SetOnWrite(0x68, func(d *nextregs.Dispatcher, val byte) {
		d.Store(0x68, val&^0x02)
		u.SetULAOutputDisabled(val&0x80 != 0)
		comp.SetBlendMode((val >> 5) & 0x03)
		comp.SetStencilMode(val&0x01 != 0)
		u.SetULAFineScrollX(val&0x04 != 0)
	})

	u.SetNextDMA(dmaEngine)
	dmaEngine.SetIOBus(dmaPortBus{u})
	dmaEngine.SetCycleSink(func(n uint64) { cpu.SetTstates(cpu.Tstates() + n) })
	dmaEngine.SetClock(func() uint64 { return cpu.Tstates() })
	dmaEngine.SetSpeedMultiplier(cpu.SpeedMultiplier)
	cpu.AddPreFetchHook("zxndma-step", func(uint16) { dmaEngine.Step(cpu.Tstates()) })

	u.SetNextI2C(rtcpkg.NewBus(rtcEngine))
	u.SetNextCopper(cop)
	u.SetNextDAC(dacBank)

	cpu.AddPreFetchHook("divmmc", pager.Step)
	cpu.SetRETNHook(func() {
		if mem.MultifaceActive() {
			mem.SetMultifaceActive(false)
		}
		pager.HandleRETN()
	})
	cpu.AddPostFetchHook("divmmc-pageout", pager.PostStep)
	u.SetNextDivMMC(pager)

	mem.PeripheralRead = func(addr uint16) (byte, bool) {
		if val, ok := pager.HandleRead(addr); ok {
			return val, true
		}
		return pm.HandleMemoryRead(addr)
	}
	mem.PeripheralWrite = func(addr uint16, val byte) bool {
		if pager.HandleWrite(addr, val) {
			return true
		}
		return pm.HandleMemoryWrite(addr, val)
	}

	m := &machine{cpu: cpu, mem: mem, ula: u, kbd: kbd, disp: disp, frameT: frameTStates}
	m.wireIM2()
	return m, nil
}

// wireIM2 gives the frame interrupt its vector in hardware IM2 mode (NR$C0
// bit 0). The daisy chain has one source here, the ULA, so every accepted
// interrupt is its request: the acknowledge answers with the chain's vector
// byte, NR$C0's top bits over the ULA's index, and RETI releases the slot.
// In pulse mode nothing drives the bus and the CPU keeps the floating $FF.
func (m *machine) wireIM2() {
	m.im2 = next.NewIM2Driver()
	m.im2.SetEnabled(next.IM2SourceULA, true)
	before := m.disp.OnWriteFn(0xC0)
	m.disp.SetOnWrite(0xC0, func(d *nextregs.Dispatcher, val byte) {
		if before != nil {
			before(d, val)
		} else {
			d.Store(0xC0, val)
		}
		m.im2.SetVectorBase(val)
		m.im2.SetMode(val&0x01 != 0)
	})
	m.cpu.SetINTAckHook(func() (byte, bool) {
		m.im2.Raise(next.IM2SourceULA)
		vec, ok := m.im2.Acknowledge()
		m.im2.Lower(next.IM2SourceULA)
		return vec, ok
	})
	m.cpu.SetRETISeenHook(m.im2.RETISeen)
}

// frames runs the machine for count ULA frames.
func (m *machine) frames(count int) {
	for i := 0; i < count; i++ {
		m.kbd.Tick()
		m.cpu.ExecuteFrame(m.frameT)
	}
}
