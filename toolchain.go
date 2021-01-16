package main

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
