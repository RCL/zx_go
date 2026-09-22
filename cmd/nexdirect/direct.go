package main

import (
	"bytes"
	"fmt"

	"github.com/conorarmstrong/zx_go/pkg/next/esxdos"
	"github.com/conorarmstrong/zx_go/pkg/next/nex"
)

// start parses a .nex, puts its banks into a fresh machine's RAM and leaves
// the CPU where NEXLOAD leaves it: the 128K map in the MMU with the entry
// bank at $C000, SP and PC from the header, interrupts off. files is what
// the in-memory card offers the program (see card.go).
//
// NEXLOAD would also set the border, load the palette and show the loading
// screen the header names, and honour its start delay; none of that is
// done here, since a program that runs in a page has no loader to wait on.
func start(data []byte, files map[string][]byte) (*machine, error) {
	n, err := nex.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	m, err := newMachine()
	if err != nil {
		return nil, err
	}
	for bank, page := range n.Banks {
		dst := m.mem.GetPage(bank)
		if dst == nil {
			return nil, fmt.Errorf("nexdirect: the machine has no bank %d", bank)
		}
		copy(dst, page)
	}

	m.card = newCard()
	for name, content := range files {
		m.card.Put(name, content)
	}
	esx := esxdos.New()
	m.card.Register(esx)
	// RST 8 is the esxDOS entry whenever the divMMC overlay is paged in on
	// hardware; there is no overlay here, so every RST 8 is one.
	m.cpu.AddPreFetchHook("esxdos-card", esx.HookFunc(m.cpu, m.mem, nil))

	entry := n.Header.EntryBank * 2
	for slot, page := range []byte{0xff, 0xff, 10, 11, 4, 5, entry, entry + 1} {
		m.disp.WriteReg(byte(0x50+slot), page)
	}
	m.cpu.SP = n.Header.SP
	m.cpu.PC = n.Header.PC
	m.cpu.IFF1, m.cpu.IFF2 = false, false
	return m, nil
}
