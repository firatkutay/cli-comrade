// Copyright 2013 @atotto. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build freebsd linux netbsd openbsd solaris dragonfly

package clipboard

import (
	"errors"
	"os"
	"os/exec"
	"sync"
)

const (
	xsel               = "xsel"
	xclip              = "xclip"
	powershellExe      = "powershell.exe"
	clipExe            = "clip.exe"
	wlcopy             = "wl-copy"
	wlpaste            = "wl-paste"
	termuxClipboardGet = "termux-clipboard-get"
	termuxClipboardSet = "termux-clipboard-set"
)

var (
	Primary bool
	trimDos bool

	pasteCmdArgs []string
	copyCmdArgs  []string

	xselPasteArgs = []string{xsel, "--output", "--clipboard"}
	xselCopyArgs  = []string{xsel, "--input", "--clipboard"}

	xclipPasteArgs = []string{xclip, "-out", "-selection", "clipboard"}
	xclipCopyArgs  = []string{xclip, "-in", "-selection", "clipboard"}

	powershellExePasteArgs = []string{powershellExe, "Get-Clipboard"}
	clipExeCopyArgs        = []string{clipExe}

	wlpasteArgs = []string{wlpaste, "--no-newline"}
	wlcopyArgs  = []string{wlcopy}

	termuxPasteArgs = []string{termuxClipboardGet}
	termuxCopyArgs  = []string{termuxClipboardSet}

	missingCommands = errors.New("No clipboard utilities available. Please install xsel, xclip, wl-clipboard or Termux:API add-on for termux-clipboard-get/set.")
)

// probeOnce guards detectCommands so the (up to five, PATH-scanning)
// exec.LookPath probes below run at most once per process, and — this is
// the actual fix — lazily, on first real clipboard use, instead of
// unconditionally in a package-level init() that used to run for every
// process that merely imported this package, whether or not it ever
// touched the clipboard.
//
// On a host whose PATH has many entries (observed: a WSL2 process whose
// PATH is the union of its Linux PATH and the full Windows PATH via
// interop, some 100+ entries, most 9p/DrvFs-mounted and slow to stat),
// the five sequential LookPath scans this package used to run at package
// init could add several hundred milliseconds to EVERY invocation of a
// binary that imports this package — e.g. cli-comrade's `comrade
// --version`, which never touches the clipboard at all. Deferring the
// scan to first actual use (readAll/writeAll, reached only through
// clipboard.ReadAll/clipboard.WriteAll and — transitively — an actual
// Ctrl+V/Ctrl+Y keystroke in a textinput/textarea) removes that cost from
// every command that never pastes or copies, without changing this
// package's public API or behavior for the one that does.
//
// This file is a locally vendored, behavior-preserving patch of upstream
// github.com/atotto/clipboard v0.1.4's clipboard_unix.go (see
// cli-comrade's go.mod replace directive and
// docs/history/phases/FAZ-11.md/KNOWN_LIMITATIONS for the rationale) — every line
// below this comment is upstream's own detectCommands logic, moved out of
// func init() verbatim, not reimplemented.
var probeOnce sync.Once

func detectCommands() {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		pasteCmdArgs = wlpasteArgs
		copyCmdArgs = wlcopyArgs

		if _, err := exec.LookPath(wlcopy); err == nil {
			if _, err := exec.LookPath(wlpaste); err == nil {
				return
			}
		}
	}

	pasteCmdArgs = xclipPasteArgs
	copyCmdArgs = xclipCopyArgs

	if _, err := exec.LookPath(xclip); err == nil {
		return
	}

	pasteCmdArgs = xselPasteArgs
	copyCmdArgs = xselCopyArgs

	if _, err := exec.LookPath(xsel); err == nil {
		return
	}

	pasteCmdArgs = termuxPasteArgs
	copyCmdArgs = termuxCopyArgs

	if _, err := exec.LookPath(termuxClipboardSet); err == nil {
		if _, err := exec.LookPath(termuxClipboardGet); err == nil {
			return
		}
	}

	pasteCmdArgs = powershellExePasteArgs
	copyCmdArgs = clipExeCopyArgs
	trimDos = true

	if _, err := exec.LookPath(clipExe); err == nil {
		if _, err := exec.LookPath(powershellExe); err == nil {
			return
		}
	}

	Unsupported = true
}

func getPasteCommand() *exec.Cmd {
	probeOnce.Do(detectCommands)
	if Primary {
		pasteCmdArgs = pasteCmdArgs[:1]
	}
	return exec.Command(pasteCmdArgs[0], pasteCmdArgs[1:]...)
}

func getCopyCommand() *exec.Cmd {
	probeOnce.Do(detectCommands)
	if Primary {
		copyCmdArgs = copyCmdArgs[:1]
	}
	return exec.Command(copyCmdArgs[0], copyCmdArgs[1:]...)
}

func readAll() (string, error) {
	probeOnce.Do(detectCommands)
	if Unsupported {
		return "", missingCommands
	}
	pasteCmd := getPasteCommand()
	out, err := pasteCmd.Output()
	if err != nil {
		return "", err
	}
	result := string(out)
	if trimDos && len(result) > 1 {
		result = result[:len(result)-2]
	}
	return result, nil
}

func writeAll(text string) error {
	probeOnce.Do(detectCommands)
	if Unsupported {
		return missingCommands
	}
	copyCmd := getCopyCommand()
	in, err := copyCmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := copyCmd.Start(); err != nil {
		return err
	}
	if _, err := in.Write([]byte(text)); err != nil {
		return err
	}
	if err := in.Close(); err != nil {
		return err
	}
	return copyCmd.Wait()
}
