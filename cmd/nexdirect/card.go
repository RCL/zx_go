package main

import (
	"strings"

	"github.com/conorarmstrong/zx_go/pkg/next/esxdos"
	"github.com/conorarmstrong/zx_go/pkg/z80"
)

// card stands in for the SD card: a few whole files in memory, answering the
// esxDOS calls a program makes through RST 8. A file written and closed is
// handed to Saved, which the page keeps in the browser's storage.
type card struct {
	files map[string][]byte
	open  map[byte]*openFile
	next  byte
	Saved func(name string, data []byte)
}

type openFile struct {
	name  string
	data  []byte
	pos   int
	write bool
}

const (
	esxFOpen  = 0x9a
	esxFClose = 0x9b
	esxFRead  = 0x9d
	esxFWrite = 0x9e
	esxWrite  = 0x02 // F_OPEN's B: write access; the program creates or truncates
)

func newCard() *card {
	return &card{files: map[string][]byte{}, open: map[byte]*openFile{}}
}

// Put stores a file the program may open for reading.
func (c *card) Put(name string, data []byte) {
	c.files[strings.ToLower(name)] = append([]byte(nil), data...)
}

func (c *card) Register(esx *esxdos.Dispatcher) {
	esx.Register(esxFOpen, c.fOpen)
	esx.Register(esxFClose, c.fClose)
	esx.Register(esxFRead, c.fRead)
	esx.Register(esxFWrite, c.fWrite)
}

func readName(mem esxdos.Memory, at uint16) string {
	var name []byte
	for len(name) < 64 {
		b := mem.Read(at)
		if b == 0 {
			break
		}
		name = append(name, b)
		at++
	}
	return strings.ToLower(string(name))
}

// F_OPEN: HL = name, B = access, A = drive. Returns A = handle.
func (c *card) fOpen(cpu *z80.CPU, mem esxdos.Memory) error {
	name := readName(mem, cpu.HL())
	f := &openFile{name: name, write: cpu.B&esxWrite != 0}
	if !f.write {
		data, ok := c.files[name]
		if !ok {
			return esxdos.NewError(esxdos.ESX_ENOENT, name)
		}
		f.data = data
	}
	for tries := 0; tries < 127; tries++ {
		c.next = c.next%127 + 1
		if _, used := c.open[c.next]; !used {
			c.open[c.next] = f
			cpu.A = c.next
			return nil
		}
	}
	return esxdos.NewError(esxdos.ESX_EIO, "no free handles")
}

func (c *card) handle(cpu *z80.CPU) (*openFile, error) {
	f, ok := c.open[cpu.A]
	if !ok {
		return nil, esxdos.NewError(esxdos.ESX_EBADF, "bad handle")
	}
	return f, nil
}

// F_READ: A = handle, HL = address, BC = bytes. Returns BC = bytes read.
func (c *card) fRead(cpu *z80.CPU, mem esxdos.Memory) error {
	f, err := c.handle(cpu)
	if err != nil {
		return err
	}
	at := cpu.HL()
	count := 0
	for count < int(cpu.BC()) && f.pos < len(f.data) {
		mem.Write(at, f.data[f.pos])
		at++
		f.pos++
		count++
	}
	cpu.SetBC(uint16(count))
	cpu.SetHL(at)
	return nil
}

// F_WRITE: A = handle, HL = address, BC = bytes. Returns BC = bytes written.
func (c *card) fWrite(cpu *z80.CPU, mem esxdos.Memory) error {
	f, err := c.handle(cpu)
	if err != nil {
		return err
	}
	if !f.write {
		return esxdos.NewError(esxdos.ESX_EBADF, "read only")
	}
	at := cpu.HL()
	for n := 0; n < int(cpu.BC()); n++ {
		f.data = append(f.data, mem.Read(at))
		at++
	}
	cpu.SetHL(at)
	return nil
}

// F_CLOSE: A = handle. A written file is kept and handed out.
func (c *card) fClose(cpu *z80.CPU, _ esxdos.Memory) error {
	f, err := c.handle(cpu)
	if err != nil {
		return err
	}
	delete(c.open, cpu.A)
	if f.write {
		c.files[f.name] = f.data
		if c.Saved != nil {
			c.Saved(f.name, f.data)
		}
	}
	return nil
}
