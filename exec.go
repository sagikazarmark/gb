package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
)

// Executor interacts with the environment and executes build steps.
type Executor interface {
	Run(a Action, env []string, cmdargs ...interface{}) error

	WriteFile(path string, content []byte) error
}

type Context struct {
	GOROOT string
	GOOS   string
	GOARCH string

	GoTool string
}

// Build is the action for building a single package.
// Note that any new influence on this logic must be reported in b.buildActionID above as well.
func Build(ctx Context, exec Executor, t Toolchain, a Action) (err error) {
	objdir := a.Objdir

	gofiles := a.Package.GoFiles
	cgofiles := a.Package.CgoFiles
	cfiles := a.Package.CFiles
	sfiles := a.Package.SFiles
	cxxfiles := a.Package.CXXFiles
	var objects, cgoObjects, pcCFLAGS, pcLDFLAGS []string

	// Run cgo.
	if len(a.Package.CgoFiles) > 0 {
		// In a package using cgo, cgo compiles the C, C++ and assembly files with gcc.
		// There is one exception: runtime/cgo's job is to bridge the
		// cgo and non-cgo worlds, so it necessarily has files in both.
		// In that case gcc only gets the gcc_* files.
		var gccfiles []string
		gccfiles = append(gccfiles, cfiles...)
		cfiles = nil

		for _, sfile := range sfiles {
			data, err := ioutil.ReadFile(filepath.Join(a.Package.Dir, sfile))
			if err == nil {
				if bytes.HasPrefix(data, []byte("TEXT")) || bytes.Contains(data, []byte("\nTEXT")) ||
					bytes.HasPrefix(data, []byte("DATA")) || bytes.Contains(data, []byte("\nDATA")) ||
					bytes.HasPrefix(data, []byte("GLOBL")) || bytes.Contains(data, []byte("\nGLOBL")) {
					return fmt.Errorf("package using cgo has Go assembly file %s", sfile)
				}
			}
		}
		gccfiles = append(gccfiles, sfiles...)
		sfiles = nil

		outGo, outObj, err := cgo(a, base.Tool("cgo"), objdir, pcCFLAGS, pcLDFLAGS, mkAbsFiles(a.Package.Dir, cgofiles), gccfiles, cxxfiles, a.Package.MFiles, a.Package.FFiles)
		if err != nil {
			return err
		}
		if "" == "gccgo" {
			cgoObjects = append(cgoObjects, a.Objdir+"_cgo_flags")
		}
		cgoObjects = append(cgoObjects, outObj...)
		gofiles = append(gofiles, outGo...)
	}

	var srcfiles []string // .go and non-.go
	srcfiles = append(srcfiles, gofiles...)
	srcfiles = append(srcfiles, sfiles...)
	srcfiles = append(srcfiles, cfiles...)
	srcfiles = append(srcfiles, cxxfiles...)

	// Collect symbol ABI requirements from assembly.
	symabis, err := t.Symabis(ctx, exec, a, sfiles)
	if err != nil {
		return err
	}

	// Compile Go.
	objpkg := objdir + "_pkg_.a"
	ofile, err := t.Gc(ctx, exec, a, a.Importcfg, objpkg, symabis, len(sfiles) > 0, gofiles)
	if err != nil {
		return err
	}
	if ofile != objpkg {
		objects = append(objects, ofile)
	}

	for _, file := range cfiles {
		out := file[:len(file)-len(".c")] + ".o"
		if err := t.Cc(ctx, exec, a, objdir+out, file); err != nil {
			return err
		}
		objects = append(objects, out)
	}

	// Assemble .s files.
	if len(sfiles) > 0 {
		ofiles, err := t.Asm(ctx, exec, a, sfiles)
		if err != nil {
			return err
		}
		objects = append(objects, ofiles...)
	}

	// NOTE(rsc): On Windows, it is critically important that the
	// gcc-compiled objects (cgoObjects) be listed after the ordinary
	// objects in the archive. I do not know why this is.
	// https://golang.org/issue/2601
	objects = append(objects, cgoObjects...)

	// Add system object files.
	for _, syso := range a.Package.SysoFiles {
		objects = append(objects, filepath.Join(a.Package.Dir, syso))
	}

	// Pack into archive in objdir directory.
	// If the Go compiler wrote an archive, we only need to add the
	// object files for non-Go sources to the archive.
	// If the Go compiler wrote an archive and the package is entirely
	// Go sources, there is no pack to execute at all.
	if len(objects) > 0 {
		if err := t.Pack(ctx, exec, a, objpkg, objects); err != nil {
			return err
		}
	}

	return nil
}

func cgo(a *Action, cgoExe, objdir string, pcCFLAGS, pcLDFLAGS, cgofiles, gccfiles, gxxfiles, mfiles, ffiles []string) (outGo, outObj []string, err error) {
	p := a.Package
	cgoCPPFLAGS, cgoCFLAGS, cgoCXXFLAGS, cgoFFLAGS, cgoLDFLAGS, err := b.CFlags(p)
	if err != nil {
		return nil, nil, err
	}

	cgoCPPFLAGS = append(cgoCPPFLAGS, pcCFLAGS...)
	cgoLDFLAGS = append(cgoLDFLAGS, pcLDFLAGS...)
	// If we are compiling Objective-C code, then we need to link against libobjc
	if len(mfiles) > 0 {
		cgoLDFLAGS = append(cgoLDFLAGS, "-lobjc")
	}

	// Likewise for Fortran, except there are many Fortran compilers.
	// Support gfortran out of the box and let others pass the correct link options
	// via CGO_LDFLAGS
	if len(ffiles) > 0 {
		fc := cfg.Getenv("FC")
		if fc == "" {
			fc = "gfortran"
		}
		if strings.Contains(fc, "gfortran") {
			cgoLDFLAGS = append(cgoLDFLAGS, "-lgfortran")
		}
	}

	if cfg.BuildMSan {
		cgoCFLAGS = append([]string{"-fsanitize=memory"}, cgoCFLAGS...)
		cgoLDFLAGS = append([]string{"-fsanitize=memory"}, cgoLDFLAGS...)
	}

	// Allows including _cgo_export.h, as well as the user's .h files,
	// from .[ch] files in the package.
	cgoCPPFLAGS = append(cgoCPPFLAGS, "-I", objdir)

	// cgo
	// TODO: CGO_FLAGS?
	gofiles := []string{objdir + "_cgo_gotypes.go"}
	cfiles := []string{"_cgo_export.c"}
	for _, fn := range cgofiles {
		f := strings.TrimSuffix(filepath.Base(fn), ".go")
		gofiles = append(gofiles, objdir+f+".cgo1.go")
		cfiles = append(cfiles, f+".cgo2.c")
	}

	// TODO: make cgo not depend on $GOARCH?

	cgoflags := []string{}
	if p.Standard && p.ImportPath == "runtime/cgo" {
		cgoflags = append(cgoflags, "-import_runtime_cgo=false")
	}
	if p.Standard && (p.ImportPath == "runtime/race" || p.ImportPath == "runtime/msan" || p.ImportPath == "runtime/cgo") {
		cgoflags = append(cgoflags, "-import_syscall=false")
	}

	// Update $CGO_LDFLAGS with p.CgoLDFLAGS.
	// These flags are recorded in the generated _cgo_gotypes.go file
	// using //go:cgo_ldflag directives, the compiler records them in the
	// object file for the package, and then the Go linker passes them
	// along to the host linker. At this point in the code, cgoLDFLAGS
	// consists of the original $CGO_LDFLAGS (unchecked) and all the
	// flags put together from source code (checked).
	cgoenv := b.cCompilerEnv()
	if len(cgoLDFLAGS) > 0 {
		flags := make([]string, len(cgoLDFLAGS))
		for i, f := range cgoLDFLAGS {
			flags[i] = strconv.Quote(f)
		}
		cgoenv = []string{"CGO_LDFLAGS=" + strings.Join(flags, " ")}
	}

	if cfg.BuildToolchainName == "gccgo" {
		if b.gccSupportsFlag([]string{BuildToolchain.compiler()}, "-fsplit-stack") {
			cgoCFLAGS = append(cgoCFLAGS, "-fsplit-stack")
		}
		cgoflags = append(cgoflags, "-gccgo")
		if pkgpath := gccgoPkgpath(p); pkgpath != "" {
			cgoflags = append(cgoflags, "-gccgopkgpath="+pkgpath)
		}
	}

	switch cfg.BuildBuildmode {
	case "c-archive", "c-shared":
		// Tell cgo that if there are any exported functions
		// it should generate a header file that C code can
		// #include.
		cgoflags = append(cgoflags, "-exportheader="+objdir+"_cgo_install.h")
	}

	execdir := p.Dir

	// Rewrite overlaid paths in cgo files.
	// cgo adds //line and #line pragmas in generated files with these paths.
	var trimpath []string
	for i := range cgofiles {
		path := mkAbs(p.Dir, cgofiles[i])
		if opath, ok := fsys.OverlayPath(path); ok {
			cgofiles[i] = opath
			trimpath = append(trimpath, opath+"=>"+path)
		}
	}
	if len(trimpath) > 0 {
		cgoflags = append(cgoflags, "-trimpath", strings.Join(trimpath, ";"))
	}

	if err := b.run(a, execdir, p.ImportPath, cgoenv, cfg.BuildToolexec, cgoExe, "-objdir", objdir, "-importpath", p.ImportPath, cgoflags, "--", cgoCPPFLAGS, cgoCFLAGS, cgofiles); err != nil {
		return nil, nil, err
	}
	outGo = append(outGo, gofiles...)

	// Use sequential object file names to keep them distinct
	// and short enough to fit in the .a header file name slots.
	// We no longer collect them all into _all.o, and we'd like
	// tools to see both the .o suffix and unique names, so
	// we need to make them short enough not to be truncated
	// in the final archive.
	oseq := 0
	nextOfile := func() string {
		oseq++
		return objdir + fmt.Sprintf("_x%03d.o", oseq)
	}

	// gcc
	cflags := str.StringList(cgoCPPFLAGS, cgoCFLAGS)
	for _, cfile := range cfiles {
		ofile := nextOfile()
		if err := b.gcc(a, p, a.Objdir, ofile, cflags, objdir+cfile); err != nil {
			return nil, nil, err
		}
		outObj = append(outObj, ofile)
	}

	for _, file := range gccfiles {
		ofile := nextOfile()
		if err := b.gcc(a, p, a.Objdir, ofile, cflags, file); err != nil {
			return nil, nil, err
		}
		outObj = append(outObj, ofile)
	}

	cxxflags := str.StringList(cgoCPPFLAGS, cgoCXXFLAGS)
	for _, file := range gxxfiles {
		ofile := nextOfile()
		if err := b.gxx(a, p, a.Objdir, ofile, cxxflags, file); err != nil {
			return nil, nil, err
		}
		outObj = append(outObj, ofile)
	}

	for _, file := range mfiles {
		ofile := nextOfile()
		if err := b.gcc(a, p, a.Objdir, ofile, cflags, file); err != nil {
			return nil, nil, err
		}
		outObj = append(outObj, ofile)
	}

	fflags := str.StringList(cgoCPPFLAGS, cgoFFLAGS)
	for _, file := range ffiles {
		ofile := nextOfile()
		if err := b.gfortran(a, p, a.Objdir, ofile, fflags, file); err != nil {
			return nil, nil, err
		}
		outObj = append(outObj, ofile)
	}

	switch cfg.BuildToolchainName {
	case "gc":
		importGo := objdir + "_cgo_import.go"
		if err := b.dynimport(a, p, objdir, importGo, cgoExe, cflags, cgoLDFLAGS, outObj); err != nil {
			return nil, nil, err
		}
		outGo = append(outGo, importGo)

	case "gccgo":
		defunC := objdir + "_cgo_defun.c"
		defunObj := objdir + "_cgo_defun.o"
		if err := BuildToolchain.cc(b, a, defunObj, defunC); err != nil {
			return nil, nil, err
		}
		outObj = append(outObj, defunObj)

	default:
		noCompiler()
	}

	// Double check the //go:cgo_ldflag comments in the generated files.
	// The compiler only permits such comments in files whose base name
	// starts with "_cgo_". Make sure that the comments in those files
	// are safe. This is a backstop against people somehow smuggling
	// such a comment into a file generated by cgo.
	if cfg.BuildToolchainName == "gc" && !cfg.BuildN {
		var flags []string
		for _, f := range outGo {
			if !strings.HasPrefix(filepath.Base(f), "_cgo_") {
				continue
			}

			src, err := os.ReadFile(f)
			if err != nil {
				return nil, nil, err
			}

			const cgoLdflag = "//go:cgo_ldflag"
			idx := bytes.Index(src, []byte(cgoLdflag))
			for idx >= 0 {
				// We are looking at //go:cgo_ldflag.
				// Find start of line.
				start := bytes.LastIndex(src[:idx], []byte("\n"))
				if start == -1 {
					start = 0
				}

				// Find end of line.
				end := bytes.Index(src[idx:], []byte("\n"))
				if end == -1 {
					end = len(src)
				} else {
					end += idx
				}

				// Check for first line comment in line.
				// We don't worry about /* */ comments,
				// which normally won't appear in files
				// generated by cgo.
				commentStart := bytes.Index(src[start:], []byte("//"))
				commentStart += start
				// If that line comment is //go:cgo_ldflag,
				// it's a match.
				if bytes.HasPrefix(src[commentStart:], []byte(cgoLdflag)) {
					// Pull out the flag, and unquote it.
					// This is what the compiler does.
					flag := string(src[idx+len(cgoLdflag) : end])
					flag = strings.TrimSpace(flag)
					flag = strings.Trim(flag, `"`)
					flags = append(flags, flag)
				}
				src = src[end:]
				idx = bytes.Index(src, []byte(cgoLdflag))
			}
		}

		// We expect to find the contents of cgoLDFLAGS in flags.
		if len(cgoLDFLAGS) > 0 {
		outer:
			for i := range flags {
				for j, f := range cgoLDFLAGS {
					if f != flags[i+j] {
						continue outer
					}
				}
				flags = append(flags[:i], flags[i+len(cgoLDFLAGS):]...)
				break
			}
		}

		if err := checkLinkerFlags("LDFLAGS", "go:cgo_ldflag", flags); err != nil {
			return nil, nil, err
		}
	}

	return outGo, outObj, nil
}
