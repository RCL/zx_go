//go:build js

// The browser side. The page gets one global object, nexdirect:
//
//	nexdirect.start(nex, files) -> null | error   a fresh machine running the .nex;
//	                                              files is {name: Uint8Array}, the card
//	nexdirect.frame(rgba) -> {w, h}               one ULA frame; the picture into rgba (w*h*4)
//	nexdirect.audio(buf) -> n                     drains queued stereo int16 samples into buf
//	nexdirect.key(row, mask, down)                a key of the keyboard matrix
//	nexdirect.joystick(mask, down)                Kempston 1 bits (--FUDLR, fire on 4, 6, 5)
//	nexdirect.peek(addr) / poke(addr, value)      the CPU's view of memory
//	nexdirect.state() -> {pc, sp, halted, frames}
//	nexdirect.onSave = function(name, bytes)       called when the program writes a file
//	nexdirect.ready                               true once the object is installed
//
// The page owns the clock: call frame() fifty times a second by the wall
// clock, and audio() when it has room. The picture is 320x256 RGBA when
// Layer 2 runs at 320x256 or sprites are on, 320x240 otherwise.
package main

import (
	"fmt"
	"syscall/js"
)

var (
	cur        *machine
	framesRun  int
	audioQueue []int16
	uint8Array = js.Global().Get("Uint8Array")
	api        = js.Global().Get("Object").New()
)

// audioQueueCap bounds the undrained audio: about eight frames of stereo.
const audioQueueCap = 2 * 882 * 8

func expose(name string, fn func([]js.Value) any) {
	api.Set(name, js.FuncOf(func(_ js.Value, args []js.Value) (result any) {
		defer func() {
			if r := recover(); r != nil {
				result = "panic: " + fmt.Sprint(r)
			}
		}()
		return fn(args)
	}))
}

func bytesOf(v js.Value) []byte {
	out := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(out, v)
	return out
}

func main() {
	expose("start", func(a []js.Value) any {
		if len(a) == 0 || !a[0].InstanceOf(uint8Array) {
			return "start wants the .nex as a Uint8Array"
		}
		files := map[string][]byte{}
		if len(a) > 1 && a[1].Type() == js.TypeObject {
			keys := js.Global().Get("Object").Call("keys", a[1])
			for i := 0; i < keys.Length(); i++ {
				name := keys.Index(i).String()
				if value := a[1].Get(name); value.InstanceOf(uint8Array) {
					files[name] = bytesOf(value)
				}
			}
		}
		m, err := start(bytesOf(a[0]), files)
		if err != nil {
			return err.Error()
		}
		m.card.Saved = func(name string, content []byte) {
			fn := api.Get("onSave")
			if fn.Type() != js.TypeFunction {
				return
			}
			out := uint8Array.New(len(content))
			js.CopyBytesToJS(out, content)
			fn.Invoke(name, out)
		}
		cur = m
		framesRun = 0
		audioQueue = audioQueue[:0]
		return nil
	})
	expose("frame", func(a []js.Value) any {
		if cur == nil {
			return nil
		}
		cur.frames(1)
		framesRun++
		audioQueue = append(audioQueue, cur.ula.RenderAudioFrame()...)
		if len(audioQueue) > audioQueueCap {
			audioQueue = audioQueue[len(audioQueue)-audioQueueCap:]
		}
		img := cur.ula.Render()
		if len(a) > 0 && a[0].InstanceOf(uint8Array) {
			js.CopyBytesToJS(a[0], img.Pix)
		}
		return map[string]any{"w": img.Bounds().Dx(), "h": img.Bounds().Dy()}
	})
	expose("audio", func(a []js.Value) any {
		if len(a) == 0 || !a[0].InstanceOf(uint8Array) {
			return 0
		}
		n := a[0].Get("length").Int() / 2
		if n > len(audioQueue) {
			n = len(audioQueue)
		}
		out := make([]byte, n*2)
		for i := 0; i < n; i++ {
			out[i*2], out[i*2+1] = byte(audioQueue[i]), byte(uint16(audioQueue[i])>>8)
		}
		js.CopyBytesToJS(a[0], out)
		audioQueue = append(audioQueue[:0], audioQueue[n:]...)
		return n
	})
	expose("key", func(a []js.Value) any {
		if cur != nil && len(a) == 3 {
			cur.kbd.PressMatrixKey(a[0].Int(), byte(a[1].Int()), a[2].Bool())
		}
		return nil
	})
	expose("joystick", func(a []js.Value) any {
		if cur != nil && len(a) == 2 {
			cur.ula.SetKempstonButton(byte(a[0].Int()), a[1].Bool())
		}
		return nil
	})
	expose("peek", func(a []js.Value) any {
		if cur == nil || len(a) == 0 {
			return nil
		}
		return int(cur.mem.Read(uint16(a[0].Int())))
	})
	expose("poke", func(a []js.Value) any {
		if cur != nil && len(a) == 2 {
			cur.mem.Write(uint16(a[0].Int()), byte(a[1].Int()))
		}
		return nil
	})
	expose("state", func(a []js.Value) any {
		if cur == nil {
			return nil
		}
		return map[string]any{"pc": int(cur.cpu.PC), "sp": int(cur.cpu.SP), "halted": cur.cpu.Halted, "frames": framesRun}
	})
	api.Set("ready", true)
	js.Global().Set("nexdirect", api)
	select {}
}
