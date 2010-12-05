/*

Copyright (c) 2010 Andrea Fazzi

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
"Software"), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

*/

package main

import (
	"spectrum"
	"spectrum/formats"
	"⚛sdl"
	"⚛sdl/ttf"
	"strings"
	"fmt"
	"flag"
	"os"
	"os/signal"
	"runtime"
	"clingon"
)

var (
	appSurface *sdl.Surface
	sdlScreen *spectrum.SDLScreen2x
	cliRenderer *clingon.SDLRenderer
	cli *clingon.Console
	font *ttf.Font
	sdlInitComplete, enableCLI, cliAnimation bool
	cliY int16 = 480
)

func render() {
	if sdlInitComplete {
		if sdlScreen != nil {
			if surface := sdlScreen.GetSurface(); surface != nil {
				appSurface.Blit(nil, sdlScreen.GetSurface(), nil)
			}
		}
		if enableCLI {
			if cliY > 240 {
				cliY--
			}
		} else {
			if cliY > 480 {
				cliRenderer = nil
			} else {
				cliY++
			}
		}
		if cliRenderer != nil {
			appSurface.Blit(&sdl.Rect{0, cliY, 0, 0}, cliRenderer.GetSurface(), &sdl.Rect{0,0, 640, 240})
		}
		appSurface.Flip()
	}
}

func toggleCLI() {
	enableCLI = !enableCLI
	if enableCLI {
		cliRenderer = clingon.NewSDLRenderer(sdl.CreateRGBSurface(sdl.SRCALPHA, 640, 240, 32, 0, 0, 0, 0), font)
		cliRenderer.GetSurface().SetAlpha(sdl.SRCALPHA, 0xaa)
		
		cli = clingon.NewConsole(cliRenderer, i)
		cli.SetPrompt("gospeccy> ")
	}
}

// A Go routine for processing SDL events.
func sdlEventLoop(evtLoop *spectrum.EventLoop, speccy *spectrum.Spectrum48k, verboseKeyboard bool) {
	var toUpper bool

	app := evtLoop.App()

	for {

		select {
		case <-evtLoop.Pause:
			evtLoop.Pause <- 0

		case <-evtLoop.Terminate:
			// Terminate this Go routine
			if app.Verbose {
				app.PrintfMsg("SDL event loop: exit")
			}
			evtLoop.Terminate <- 0
			return

		case event := <-sdl.Events:
			switch e := event.(type) {
			case sdl.QuitEvent:
				if app.Verbose {
					app.PrintfMsg("SDL quit -> request[exit the application]")
				}
				app.RequestExit()

			case sdl.KeyboardEvent:
				keyName := sdl.GetKeyName(sdl.Key(e.Keysym.Sym))

				if verboseKeyboard {
					app.PrintfMsg("\n")
					app.PrintfMsg("%v: %v", e.Keysym.Sym, ": ", keyName)

					app.PrintfMsg("%04x ", e.Type)

					for i := 0; i < len(e.Pad0); i++ {
						app.PrintfMsg("%02x ", e.Pad0[i])
					}
					app.PrintfMsg("\n")

					app.PrintfMsg("Type: %02x Which: %02x State: %02x Pad: %02x\n", e.Type, e.Which, e.State, e.Pad0[0])
					app.PrintfMsg("Scancode: %02x Sym: %08x Mod: %04x Unicode: %04x\n", e.Keysym.Scancode, e.Keysym.Sym, e.Keysym.Mod, e.Keysym.Unicode)
				}

				if (keyName == "escape") && (e.Type == sdl.KEYDOWN) {
					if app.Verbose {
						app.PrintfMsg("escape key -> request[exit the application]")
					}
					app.RequestExit()

				} else if (keyName == "f10") && (e.Type == sdl.KEYDOWN) {
					if app.Verbose {
						app.PrintfMsg("f10 key -> toggle console")
					}
					toggleCLI()
				} else {
					if enableCLI {
						if (keyName == "left shift") && (e.Type == sdl.KEYDOWN) {
							toUpper = true
						} else if (keyName == "up") && (e.Type == sdl.KEYDOWN) {
							cli.HistoryCh() <- clingon.HISTORY_PREV
						} else if (keyName == "down") && (e.Type == sdl.KEYDOWN) {
							cli.HistoryCh() <- clingon.HISTORY_NEXT
						} else if (keyName == "left") && (e.Type == sdl.KEYDOWN) {
							cli.CursorCh() <- clingon.CURSOR_LEFT
						} else if (keyName == "right") && (e.Type == sdl.KEYDOWN) {
							cli.CursorCh() <- clingon.CURSOR_RIGHT
						} else {
							unicode := e.Keysym.Unicode
							if unicode > 0 {
								if toUpper {
									cli.CharCh() <- uint16([]int(strings.ToUpper(string(unicode)))[0])
								} else {
									cli.CharCh() <- unicode
								}
							}
						}

					} else {
						sequence, haveMapping := spectrum.SDL_KeyMap[keyName]

						if haveMapping {
							switch e.Type {
							case sdl.KEYDOWN:
								// Normal order
								for i := 0; i < len(sequence); i++ {
									speccy.Keyboard.KeyDown(sequence[i])
								}
							case sdl.KEYUP:
								// Reverse order
								for i := len(sequence) - 1; i >= 0; i-- {
									speccy.Keyboard.KeyUp(sequence[i])
								}
							}
						}
					}
				}
			}

		default: render()

		}

	}
}

type handler_SIGTERM struct {
	app *spectrum.Application
}

func (h *handler_SIGTERM) HandleSignal(s signal.Signal) {
	switch ss := s.(type) {
	case signal.UnixSignal:
		switch ss {
		case signal.SIGTERM, signal.SIGINT:
			if h.app.Verbose {
				h.app.PrintfMsg("%v", ss)
			}

			h.app.RequestExit()
		}
	}
}

func main() {
	// Use at least two OS threads. This helps to prevent sound
	// buffer underflows in case SDL rendering is consuming too
	// much CPU.
	if runtime.GOMAXPROCS(-1) < 2 {
		runtime.GOMAXPROCS(2)
	}

	help := flag.Bool("help", false, "Show usage")
	scale2x := flag.Bool("2x", false, "2x display scaler")
	fullscreen := flag.Bool("fullscreen", false, "Fullscreen (enable 2x scaler by default)")
	fps := flag.Float("fps", spectrum.DefaultFPS, "Frames per second")
	sound := flag.Bool("sound", true, "Enable or disable sound")
	acceleratedLoad := flag.Bool("accelerated-load", false, "Enable or disable accelerated tapes loading")
	verbose := flag.Bool("verbose", false, "Enable debugging messages")
	verboseKeyboard := flag.Bool("verbose-keyboard", false, "Enable debugging messages (keyboard events)")

	{
		flag.Usage = func() {
			fmt.Fprintf(os.Stderr, "GoSpeccy - A ZX Spectrum 48k Emulator written in GO\n\n")
			fmt.Fprintf(os.Stderr, "Usage:\n\n")
			fmt.Fprintf(os.Stderr, "\tgospeccy [options] [image.sna]\n\n")
			fmt.Fprintf(os.Stderr, "Options are:\n\n")
			flag.PrintDefaults()
		}

		flag.Parse()

		if *help == true {
			flag.Usage()
			return
		}
	}

	app := spectrum.NewApplication()
	app.Verbose = *verbose

	// Install SIGTERM handler
	{
		handler := handler_SIGTERM{app}
		spectrum.InstallSignalHandler(&handler)
	}

	// Create new emulator core
	speccy, err := spectrum.NewSpectrum48k(app, spectrum.SystemRomPath("48.rom"))
	if err != nil {
		app.PrintfMsg("%s", err)
		app.RequestExit()
		goto quit
	}

	if *acceleratedLoad {
		speccy.TapeDrive().AcceleratedLoad = true
	}

	if sdl.Init(sdl.INIT_VIDEO|sdl.INIT_AUDIO) != 0 {
		app.PrintfMsg("%s", sdl.GetError())
		app.RequestExit()
		goto quit
	}

	if ttf.Init() != 0 {
		panic(sdl.GetError())
	}

	font = ttf.OpenFont("font/VeraMono.ttf", 12)

	if font == nil {
		panic(sdl.GetError())
	}

	sdl.EnableUNICODE(1)

	appSurface = sdl.SetVideoMode(640, 480, 32, 0)

	sdl.WM_SetCaption("GoSpeccy - ZX Spectrum Emulator", "")

	// Run startup scripts. The startup scripts may create a display/audio receiver.
	{
		i = new(Interpreter)
		InitConsole(app, speccy)

		if app.TerminationInProgress() || closed(app.HasTerminated) {
			goto quit
		}
	}

	{
		n := make(chan uint)

		// Setup the display
		speccy.CommandChannel <- spectrum.Cmd_GetNumDisplayReceivers{n}
		if <-n == 0 {
			if *fullscreen {
				*scale2x = true
			}

			if *scale2x {
				sdlScreen = spectrum.NewSDLScreen2x(app, *fullscreen)
				speccy.CommandChannel <- spectrum.Cmd_AddDisplay{sdlScreen}
			} else {
				speccy.CommandChannel <- spectrum.Cmd_AddDisplay{spectrum.NewSDLScreen(app)}
			}
		}

		// Setup the audio
		speccy.CommandChannel <- spectrum.Cmd_GetNumAudioReceivers{n}
		if *sound && (<-n == 0) {
			audio, err := spectrum.NewSDLAudio(app)
			if err == nil {
				speccy.CommandChannel <- spectrum.Cmd_AddAudioReceiver{audio}
			} else {
				app.PrintfMsg("%s", err)
			}
		}

		close(n)
	}

	go sdlEventLoop(app.NewEventLoop(), speccy, *verboseKeyboard)

	// Begin speccy emulation
	go speccy.EmulatorLoop()

	speccy.CommandChannel <- spectrum.Cmd_SetFPS{*fps}

	// Start the console goroutine.
//	go console.Run(true)

	// Process command line argument. Load the given program (if any)
	if flag.Arg(0) != "" {
		file := flag.Arg(0)

		path := spectrum.ProgramPath(file)

		program, err := formats.ReadProgram(path)
		if err != nil {
			app.PrintfMsg("%s", err)
			app.RequestExit()
			goto quit
		}

		fmt.Printf("%T\n", program)
		if _, isTAP := program.(*formats.TAP); isTAP {
			romLoaded := make(chan bool, 1)
			speccy.CommandChannel <- spectrum.Cmd_Reset{romLoaded}
			<-romLoaded
		}

		errChan := make(chan os.Error)
		speccy.CommandChannel <- spectrum.Cmd_Load{file, program, errChan}
		err = <-errChan
		if err != nil {
			app.PrintfMsg("%s", err)
			app.RequestExit()
			goto quit
		}
	}

	sdlInitComplete = true

	// Drain systemROMLoaded channel
	<-speccy.ROMLoaded()

quit:
	<-app.HasTerminated
	sdl.Quit()
}
