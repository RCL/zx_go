# nexdirect

A Spectrum Next that runs one `.nex` file with no operating system, for a
web page. The Next hardware of this emulator is assembled without the FPGA
boot ROM, the divMMC ROM, an SD card or NextZXOS; the `.nex` is parsed and
its banks copied into RAM; the CPU starts the way NEXLOAD leaves it; and
the esxDOS file calls the program makes through RST 8 (F_OPEN, F_READ,
F_WRITE, F_CLOSE) are answered from a card kept in memory, whose files the
page supplies and receives. A program is running a few milliseconds after
the call, and nothing licensed is shipped.

It is for programs that do only their own thing: a game that reads a
config file and writes a score table, say. Anything that needs the OS (dot
commands, the file browser, +3DOS, the rest of esxDOS) or calls into a ROM
is out of its reach; use `cmd/zxwasm`-style booting from an SD image for
those.

## Building

    GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o web/nexdirect.wasm ./cmd/nexdirect
    cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/

The page loads `wasm_exec.js`, instantiates the module, runs it, and waits
for `nexdirect.ready`.

## The page's side

One global object, `nexdirect`:

| call | does |
|---|---|
| `start(nex, files)` | a fresh machine running the `.nex` (a `Uint8Array`); `files` is `{name: Uint8Array}`, the card's contents. Returns `null`, or an error string |
| `frame(rgba)` | runs one ULA frame and writes the picture into `rgba`; returns `{w, h}` (320x256 with Layer 2 at 320x256 or sprites on, 320x240 otherwise) |
| `audio(buf)` | drains queued audio into `buf` as interleaved stereo int16 at 44100 Hz; returns the number of samples |
| `key(row, mask, down)` | a key of the keyboard matrix (row 0 = CAPS..V, 7 = SPACE..B) |
| `joystick(mask, down)` | Kempston 1 bits: right 1, left 2, down 4, up 8, fire 16, fire 3 32, fire 2 64 |
| `peek(addr)`, `poke(addr, value)` | the CPU's view of memory |
| `state()` | `{pc, sp, halted, frames}` |
| `onSave` | assign a function `(name, bytes)`; called when the program writes and closes a file |

The page owns the clock: call `frame` fifty times a second by the wall
clock (skipping or doubling when the display's rate differs), and `audio`
when the audio graph has room. Audio in the browser needs a user gesture
before it will play, so start on a click.

## The native build

    go build ./cmd/nexdirect
    NEXDIRECT_FILES=cardfolder ./nexdirect program.nex 600 out.png 300 350:1:1

runs a program headless for 600 frames and writes the last picture; the
extra arguments press SPACE at frame 300 and A (row 1, mask 1) at 350, each
for five frames. The folder is the card: its files are offered to the
program, and what the program writes lands there.

## What is and is not modelled

- Layer 2 at all three resolutions, the tilemap, sprites, the Copper,
  zxnDMA, Z80N, 28 MHz, the 8K MMU, AY and the DAC bank: the emulator's
  Next hardware, as `cmd/zx_go` wires it.
- Hardware IM2 (NR $C0): the frame interrupt is the one source, with the
  vector the chain forms from NR $C0's top bits and the ULA's index.
- NEXLOAD's border, palette, loading screen and start delay are not
  reproduced.
- The card has no directories, no dates and no sizes beyond what it holds;
  F_OPEN for writing always creates or truncates, reading an absent file is
  "not found".
