package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/types"
	"golang.org/x/tools/go/loader" //nolint:staticcheck
	"os"
	"path/filepath"
	"sync"
)

// GetMostInnerAstIdent takes most inner identifier used for
// function call. For a.b.foo(), `b` will be the most inner identifier.
func GetMostInnerAstIdent(inSel *ast.SelectorExpr) *ast.Ident {
	var l []*ast.Ident
	var e ast.Expr
	e = inSel
	for e != nil {
		if _, ok := e.(*ast.Ident); ok {
			l = append(l, e.(*ast.Ident))
			break
		} else if _, ok := e.(*ast.SelectorExpr); ok {
			l = append(l, e.(*ast.SelectorExpr).Sel)
			e = e.(*ast.SelectorExpr).X
		} else if _, ok := e.(*ast.CallExpr); ok {
			e = e.(*ast.CallExpr).Fun
		} else if _, ok := e.(*ast.IndexExpr); ok {
			e = e.(*ast.IndexExpr).X
		} else if _, ok := e.(*ast.UnaryExpr); ok {
			e = e.(*ast.UnaryExpr).X
		} else if _, ok := e.(*ast.ParenExpr); ok {
			e = e.(*ast.ParenExpr).X
		} else if _, ok := e.(*ast.SliceExpr); ok {
			e = e.(*ast.SliceExpr).X
		} else if _, ok := e.(*ast.IndexListExpr); ok {
			e = e.(*ast.IndexListExpr).X
		} else if _, ok := e.(*ast.StarExpr); ok {
			e = e.(*ast.StarExpr).X
		} else if _, ok := e.(*ast.TypeAssertExpr); ok {
			e = e.(*ast.TypeAssertExpr).X
		} else if _, ok := e.(*ast.CompositeLit); ok {
			// TODO dummy implementation
			if len(e.(*ast.CompositeLit).Elts) == 0 {
				e = e.(*ast.CompositeLit).Type
			} else {
				e = e.(*ast.CompositeLit).Elts[0]
			}
		} else if _, ok := e.(*ast.KeyValueExpr); ok {
			e = e.(*ast.KeyValueExpr).Value
		} else {
			// TODO this is uncaught expression
			//panic("uncaught expression")
			return nil
		}
	}
	if len(l) < 2 {
		panic("selector list should have at least 2 elems")
	}
	// caller or receiver is always
	// at position 1, function is at 0
	return l[0]
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	conf := loader.Config{ParserMode: parser.ParseComments}
	conf.Build = &build.Default
	conf.Build.CgoEnabled = false
	projectPath := os.Args[1]
	conf.Build.Dir = filepath.Join(cwd, projectPath)
	conf.Import(projectPath)
	ginfo := &types.Info{
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	var mutex = &sync.RWMutex{}
	conf.AfterTypeCheck = func(info *loader.PackageInfo, files []*ast.File) {
		for k, v := range info.Defs {
			mutex.Lock()
			ginfo.Defs[k] = v
			mutex.Unlock()
		}
		for k, v := range info.Uses {
			mutex.Lock()
			ginfo.Uses[k] = v
			mutex.Unlock()
		}
		for k, v := range info.Selections {
			mutex.Lock()
			ginfo.Selections[k] = v
			mutex.Unlock()
		}
	}
	prog, err := conf.Load()
	if err != nil {
		fmt.Println(err)
	}

	for _, pkg := range prog.AllPackages {

		fmt.Printf("Package path %q\n", pkg.Pkg.Path())
		for _, file := range pkg.Files {
			_ = file
			fmt.Println(prog.Fset.Position(file.Name.Pos()).String())
			ast.Inspect(file, func(n ast.Node) bool {
				if funDeclNode, ok := n.(*ast.FuncDecl); ok {
					ftype := ginfo.Defs[funDeclNode.Name].Type()
					signature := ftype.(*types.Signature)
					recv := signature.Recv()

					var recvStr string
					if recv != nil {
						recvStr = recv.Type().String()
					}

					fmt.Println("FuncDecl:" + file.Name.Name + "." + recvStr + "." + funDeclNode.Name.String() + "." + ftype.String())

				}
				if callExpr, ok := n.(*ast.CallExpr); ok {
					if id, ok := callExpr.Fun.(*ast.Ident); ok {
						ftype := ginfo.Uses[id].Type()
						if ftype != nil {
							fmt.Println("FuncCall:" + file.Name.Name + "." + id.Name + ":" + ginfo.Uses[id].Type().String())
						}
					}
					if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {

						obj := ginfo.Selections[sel]
						id := GetMostInnerAstIdent(sel)
						if obj != nil {
							recv := obj.Recv()
							var ftypeStr string
							ftype := ginfo.Uses[id]

							if ftype != nil {
								ftypeStr = ftype.Type().String()
							}
							fmt.Println("FuncCall:" + file.Name.Name + "." + recv.String() + "." + obj.Obj().Name() + "." + ftypeStr)
						}
					}
				}
				return true
			})
		}
	}

}
