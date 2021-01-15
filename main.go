package main

import (
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"strings"
)

// Executor interacts with the environment and executes build steps.
type Executor interface {
	Run(a Action, dir string, env []string, cmdargs ...interface{}) error
}

type Context interface {
	GOROOT() string
	GOOS() string
	GOARCH() string

	GoTool() string
	// OS
	// Arch
	// Cross-compile?
	// Goroot
	// Go tool
}

type Package struct {
	*build.Package

	ModulePath    string
	ModuleVersion string
}

type Action struct {
	Package Package
	Objdir  string
}

// trimpath returns the -trimpath argument to use
// when compiling the action.
func (a *Action) trimpath() string {
	// Keep in sync with Builder.ccompile
	// The trimmed paths are a little different, but we need to trim in the
	// same situations.

	// Strip the object directory entirely.
	objdir := a.Objdir
	if len(objdir) > 1 && objdir[len(objdir)-1] == filepath.Separator {
		objdir = objdir[:len(objdir)-1]
	}
	rewrite := ""

	rewriteDir := a.Package.Dir
	if true { // TODO: trimpath? // TODO: I didn't have the patience to sort this shit out
		if m := a.Package.ModulePath; m != "" {
			if v := a.Package.ModuleVersion; v != "" {
				rewriteDir = m + "@" + v + strings.TrimPrefix(a.Package.ImportPath, m)
			} else {
				rewriteDir = m + strings.TrimPrefix(a.Package.ImportPath, m)
			}
		} else {
			rewriteDir = a.Package.ImportPath
		}

		rewrite += a.Package.Dir + "=>" + rewriteDir + ";"
	}

	rewrite += objdir + "=>"

	return rewrite
}

type Toolchain interface {
	// Gc runs the compiler in a specific directory on a set of files
	// and returns the name of the generated output file.
	Gc(ctx Context, exec Executor, a Action, archive string, importcfg string, symabis string, asmhdr bool, gofiles []string) (ofile string, err error)

	// Cc runs the toolchain's C compiler in a directory on a C file
	// to produce an output file.
	Cc(ctx Context, exec Executor, a Action, ofile string, cfile string) error

	// Asm runs the assembler in a specific directory on specific files
	// and returns a list of named output files.
	Asm(ctx Context, exec Executor, a Action, sfiles []string) ([]string, error)

	// Symabis scans the symbol ABIs from sfiles and returns the
	// path to the output symbol ABIs file, or "" if none.
	Symabis(ctx Context, exec Executor, a Action, sfiles []string) (string, error)

	// Pack runs the archive packer in a specific directory to create
	// an archive from a set of object files.
	// typically it is run in the object directory.
	Pack(ctx Context, exec Executor, a Action, afile string, ofiles []string) error

	// Ld runs the linker to create an executable starting at mainpkg.
	Ld(ctx Context, exec Executor, a Action, out string, importcfg string, mainpkg string) error
}

func main() {
	pkg, err := build.Default.Import(os.Args[1], ".", 0)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%#v\n", pkg)
}
