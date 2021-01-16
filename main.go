package main

import (
	"go/build"
	"os"
)

func main() {
	ctx := Context{
		GOROOT: build.Default.GOROOT,
		GOOS:   build.Default.GOOS,
		GOARCH: build.Default.GOARCH,
		GoTool: "",
	}

	pkg, err := build.Default.Import(os.Args[1], ".", 0)
	if err != nil {
		panic(err)
	}

	action := Action{
		Package: Package{
			Package:       pkg,
			ModulePath:    "",
			ModuleVersion: "",
		},
		Objdir:    "",
		Importcfg: "",
	}

	toolchain := gcToolchain{}

	err = Build(ctx, nil, toolchain, action)
	if err != nil {
		panic(err)
	}
}
