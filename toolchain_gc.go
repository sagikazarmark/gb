// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"runtime"
	"strings"
)

type gcToolchain struct{}

func (g gcToolchain) Gc(ctx Context, exec Executor, a Action, archive string, importcfg string, symabis string, asmhdr bool, gofiles []string) (ofile string, err error) {
	p := a.Package

	objdir := a.Objdir

	if archive != "" {
		ofile = archive
	} else {
		out := "_go_.o"
		ofile = objdir + out
	}

	pkgpath := pkgPath(a)
	gcargs := []string{"-p", pkgpath}

	// If we're giving the compiler the entire package (no C etc files), tell it that,
	// so that it can give good error messages about forward declarations.
	// Exceptions: a few standard packages have forward declarations for
	// pieces supplied behind-the-scenes by package runtime.
	extFiles := len(p.CgoFiles) + len(p.CFiles) + len(p.CXXFiles) + len(p.MFiles) + len(p.FFiles) + len(p.SFiles) + len(p.SysoFiles) + len(p.SwigFiles) + len(p.SwigCXXFiles)
	if extFiles == 0 {
		gcargs = append(gcargs, "-complete")
	}

	if ctx.GOOS() == "plan9" || ctx.GOARCH() == "wasm" {
		gcargs = append(gcargs, "-dwarf=false")
	}

	if runtimeVersion := runtime.Version(); strings.HasPrefix(runtimeVersion, "go1") {
		gcargs = append(gcargs, "-goversion", runtimeVersion)
	}

	if symabis != "" {
		gcargs = append(gcargs, "-symabis", symabis)
	}

	args := []interface{}{ctx.GoTool(), "tool", "compile", "-o", ofile, "-trimpath", a.trimpath(), gcargs}

	if importcfg != "" {
		args = append(args, "-importcfg", importcfg)
	}

	if ofile == archive {
		args = append(args, "-pack")
	}

	if asmhdr {
		args = append(args, "-asmhdr", objdir+"go_asm.h")
	}

	for _, f := range gofiles {
		args = append(args, mkAbs(p.Dir, f))
	}

	wd, err := os.Getwd()
	if err != nil {
		return ofile, err
	}

	err = exec.Run(a, wd, nil, args...)

	return ofile, err
}

func (g gcToolchain) Cc(ctx Context, exec Executor, a Action, ofile string, cfile string) error {
	panic("implement me")
}

func (g gcToolchain) Asm(ctx Context, exec Executor, a Action, sfiles []string) ([]string, error) {
	panic("implement me")
}

func (g gcToolchain) Symabis(ctx Context, exec Executor, a Action, sfiles []string) (string, error) {
	panic("implement me")
}

func (g gcToolchain) Pack(ctx Context, exec Executor, a Action, afile string, ofiles []string) error {
	panic("implement me")
}

func (g gcToolchain) Ld(ctx Context, exec Executor, a Action, out string, importcfg string, mainpkg string) error {
	panic("implement me")
}
