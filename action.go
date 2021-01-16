// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package main

import (
	"go/build"
	"path/filepath"
	"strings"
)

type Package struct {
	*build.Package

	ModulePath    string
	ModuleVersion string
}

type Action struct {
	Package   Package
	Objdir    string
	Importcfg string
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
