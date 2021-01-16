// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// The 'path' used for GOROOT_FINAL when -trimpath is specified
const trimPathGoRootFinal = "go"

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

	if ctx.GOOS == "plan9" || ctx.GOARCH == "wasm" {
		gcargs = append(gcargs, "-dwarf=false")
	}

	if runtimeVersion := runtime.Version(); strings.HasPrefix(runtimeVersion, "go1") {
		gcargs = append(gcargs, "-goversion", runtimeVersion)
	}

	if symabis != "" {
		gcargs = append(gcargs, "-symabis", symabis)
	}

	args := []interface{}{ctx.GoTool, "tool", "compile", "-o", ofile, "-trimpath", a.trimpath(), gcargs}

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

	err = exec.Run(a, nil, args...)

	return ofile, err
}

func (g gcToolchain) Cc(ctx Context, exec Executor, a Action, ofile string, cfile string) error {
	return fmt.Errorf("%s: C source files not supported without cgo", mkAbs(a.Package.Dir, cfile))
}

func asmArgs(ctx Context, a Action) []interface{} {
	// Add -I pkg/GOOS_GOARCH so #include "textflag.h" works in .s files.
	inc := filepath.Join(ctx.GOROOT, "pkg", "include")

	pkgpath := pkgPath(a)

	args := []interface{}{ctx.GoTool, "tool", "asm", "-p", pkgpath, "-trimpath", a.trimpath(), "-I", a.Objdir, "-I", inc, "-D", "GOOS_" + ctx.GOOS, "-D", "GOARCH_" + ctx.GOARCH}

	// GOMIPS

	return args
}

func (g gcToolchain) Asm(ctx Context, exec Executor, a Action, sfiles []string) ([]string, error) {
	args := asmArgs(ctx, a)

	var ofiles []string

	for _, sfile := range sfiles {
		ofile := a.Objdir + sfile[:len(sfile)-len(".s")] + ".o"
		ofiles = append(ofiles, ofile)
		args1 := append(args, "-o", ofile, sfile)
		if err := exec.Run(a, nil, args1...); err != nil {
			return nil, err
		}
	}

	return ofiles, nil
}

func (g gcToolchain) Symabis(ctx Context, exec Executor, a Action, sfiles []string) (string, error) {
	mkSymabis := func(p Package, sfiles []string, path string) error {
		args := asmArgs(ctx, a)
		args = append(args, "-gensymabis", "-o", path)
		for _, sfile := range sfiles {
			args = append(args, mkAbs(p.Dir, sfile))
		}

		// Supply an empty go_asm.h as if the compiler had been run.
		// -gensymabis parsing is lax enough that we don't need the
		// actual definitions that would appear in go_asm.h.
		if err := exec.WriteFile(a.Objdir+"go_asm.h", nil); err != nil {
			return err
		}

		return exec.Run(a, nil, args...)
	}

	var symabis string // Only set if we actually create the file
	p := a.Package
	if len(sfiles) != 0 {
		symabis = a.Objdir + "symabis"
		if err := mkSymabis(p, sfiles, symabis); err != nil {
			return "", err
		}
	}

	return symabis, nil
}

func (g gcToolchain) Pack(ctx Context, exec Executor, a Action, afile string, ofiles []string) error {
	absAfile := mkAbs(a.Objdir, afile)

	args := []interface{}{ctx.GoTool, "tool", "pack", "r", absAfile}

	for _, f := range ofiles {
		args = append(args, mkAbs(a.Objdir, f))
	}

	return exec.Run(a, nil, args...)
}

func (g gcToolchain) Ld(ctx Context, exec Executor, a Action, out string, importcfg string, mainpkg string) error {
	var ldflags []string

	ldflags = append(ldflags, a.Package.CgoLDFLAGS...) // TODO: only if cgo?

	env := []string{}
	if true { // TODO: TRIMPATH
		env = append(env, "GOROOT_FINAL="+trimPathGoRootFinal)
	}
	return exec.Run(a, env, ctx.GoTool, "tool", "link", "-o", out, "-importcfg", importcfg, ldflags, mainpkg)
}
