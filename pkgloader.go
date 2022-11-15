package main

import (
	"go/ast"
	"go/token"
	"golang.org/x/tools/go/ssa/ssautil"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

type loaderOpts struct {
	ignore    []string
	include   []string
	limit     []string
	tests     bool
	noDeps    bool
	depsLevel int
}

func (l *loaderOpts) inIncludes(p *packages.Package) bool {
	for _, ic := range l.include {
		if strings.HasPrefix(p.PkgPath, ic) {
			return true
		}
	}
	return false
}

func (l *loaderOpts) inLimits(p *packages.Package) bool {
	for _, lm := range l.limit {
		if strings.HasPrefix(p.PkgPath, lm) {
			return true
		}
	}
	return false
}

func (l *loaderOpts) inIgnores(p *packages.Package) bool {
	for _, ig := range l.ignore {
		if strings.HasPrefix(p.PkgPath, ig) {
			return true
		}
	}
	return false
}

func LoadPackages(initial []*packages.Package, opts *loaderOpts) (*ssa.Program, []*ssa.Package) {
	if opts.noDeps {
		return ssautil.Packages(initial, 0)
	}
	return doPackages(initial, opts)
}

func doPackages(initial []*packages.Package, opts *loaderOpts) (*ssa.Program, []*ssa.Package) {
	var fset *token.FileSet
	if len(initial) > 0 {
		fset = initial[0].Fset
	}

	prog := ssa.NewProgram(fset, 0)

	isInitial := make(map[*packages.Package]bool, len(initial))
	for _, p := range initial {
		isInitial[p] = true
	}

	ssamap := make(map[*packages.Package]*ssa.Package)
	Visit(initial, nil, func(p *packages.Package, lv int) {
		if p.Types != nil && !p.IllTyped {
			var files []*ast.File
			if opts.depsLevel == -1 || opts.depsLevel >= lv || isInitial[p] {
				// 跟原先 render 逻辑一致, limit 代表限制的路径, ignore 代表在路径限制内的忽略, include 代表无条件包含
				if opts.inIncludes(p) || opts.inLimits(p) && !opts.inIgnores(p) {
					logf("Load Package %s", p.PkgPath)
					files = p.Syntax
				}
			}
			ssamap[p] = prog.CreatePackage(p.Types, files, p.TypesInfo, true)
		}
	})

	var ssapkgs []*ssa.Package
	for _, p := range initial {
		ssapkgs = append(ssapkgs, ssamap[p]) // may be nil
	}
	return prog, ssapkgs
}

func Visit(pkgs []*packages.Package,
	pre func(pkgs *packages.Package, lv int) bool,
	post func(pkgs *packages.Package, lv int),
) {
	seen := make(map[*packages.Package]bool)
	var visit func(*packages.Package, int)
	visit = func(pkg *packages.Package, lv int) {
		if !seen[pkg] {
			seen[pkg] = true

			if pre == nil || pre(pkg, lv) {
				paths := make([]string, 0, len(pkg.Imports))
				for path := range pkg.Imports {
					paths = append(paths, path)
				}
				sort.Strings(paths) // Imports is a map, this makes visit stable
				for _, path := range paths {
					visit(pkg.Imports[path], lv+1)
				}
			}

			if post != nil {
				post(pkg, lv)
			}
		}
	}
	for _, pkg := range pkgs {
		visit(pkg, 0)
	}
}
