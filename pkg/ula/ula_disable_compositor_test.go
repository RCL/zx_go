package ula

import "testing"

type disableAwareCompositor struct {
	frameMock
	disabled bool
}

func (c *disableAwareCompositor) SetULAOutputDisabled(v bool) { c.disabled = v }
func TestCompositorTracksULAOutputDisable(t *testing.T) {
	u := &ULA{}
	u.SetULAOutputDisabled(true)
	c := &disableAwareCompositor{}
	u.SetNextCompositor(c)
	if !c.disabled {
		t.Fatal("attachment lost disabled state")
	}
	u.SetULAOutputDisabled(false)
	if c.disabled {
		t.Fatal("enable not propagated")
	}
	u.SetULAOutputDisabled(true)
	if !c.disabled {
		t.Fatal("disable not propagated")
	}
	u.SetNextCompositor(nil)
	u.SetULAOutputDisabled(false)
}
