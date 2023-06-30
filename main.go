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

func Contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}

func getInterfaces(defs map[*ast.Ident]types.Object) map[string]types.Object {
	interfaces := make(map[string]types.Object)
	for id, obj := range defs {
		if obj == nil || obj.Type() == nil {
			continue
		}
		if _, ok := obj.(*types.TypeName); !ok {
			continue
		}
		if types.IsInterface(obj.Type()) {
			interfaces[id.Name] = obj
		}
	}
	return interfaces
}

func isAny(obj types.Object) bool {
	return obj.Type().String() == "any" || obj.Type().Underlying().String() == "any"
}

func getInterfaceNameForReceiver(interfaces map[string]types.Object, recv *types.Var) string {
	var recvInterface string
	for _, obj := range interfaces {
		if t, ok := obj.Type().Underlying().(*types.Interface); ok {
			if types.Implements(recv.Type(), t) && !isAny(obj) {
				recvInterface = "." + obj.Type().String()
			}
		}
	}
	return recvInterface
}

func findFuncDecls(file *ast.File, ginfo *types.Info, interfaces map[string]types.Object, funcDecls map[string]bool) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			ftype := ginfo.Defs[node.Name].Type()
			signature := ftype.(*types.Signature)
			recv := signature.Recv()

			var recvStr string
			var recvInterface string
			if recv != nil {
				recvStr = "." + recv.Type().String()
				recvInterface = getInterfaceNameForReceiver(interfaces, recv)
			}
			if recvInterface != "" {
				funcDecl := file.Name.Name + recvInterface + "." + node.Name.String() + "." + ftype.String()
				funcDecls[funcDecl] = true
			}
			funcDecl := file.Name.Name + recvStr + "." + node.Name.String() + "." + ftype.String()
			funcDecls[funcDecl] = true
		}
		return true
	})
}

func dumpFuncDecls(funcDecls map[string]bool) {
	fmt.Println("FuncDecls")
	for fun, _ := range funcDecls {
		fmt.Println(fun)
	}
}

func buildCallGraph(file *ast.File, ginfo *types.Info, interfaces map[string]types.Object, funcDecls map[string]bool, backwardCallGraph map[string][]string) {
	currentFun := ""
	_ = currentFun
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			ftype := ginfo.Defs[node.Name].Type()
			signature := ftype.(*types.Signature)
			recv := signature.Recv()

			var recvStr string
			var recvInterface string
			if recv != nil {
				recvStr = "." + recv.Type().String()
				recvInterface = getInterfaceNameForReceiver(interfaces, recv)
			}
			fmt.Println("FuncDecl:" + file.Name.Name + recvStr + recvInterface + "." + node.Name.String() + "." + ftype.String())
			currentFun = file.Name.Name + recvStr + recvInterface + "." + node.Name.String() + "." + ftype.String()
		case *ast.CallExpr:
			switch node := node.Fun.(type) {
			case *ast.Ident:
				ftype := ginfo.Uses[node].Type()
				if ftype != nil {
					fmt.Println("FuncCall:" + file.Name.Name + "." + node.Name + ":" + ftype.String())
				}
			case *ast.SelectorExpr:
				obj := ginfo.Selections[node]
				if obj != nil {
					recv := obj.Recv()
					var ftypeStr string
					// sel.Sel is function ident
					ftype := ginfo.Uses[node.Sel]

					if ftype != nil {
						ftypeStr = ftype.Type().String()
					}
					var recvStr string
					if len(recv.String()) > 0 {
						recvStr = "." + recv.String()
					}
					fmt.Println("FuncCall:" + file.Name.Name + recvStr + "." + obj.Obj().Name() + "." + ftypeStr)
				}

			}
		}
		return true

	})

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
	funcDecls := make(map[string]bool)
	backwardCallGraph := make(map[string][]string)
	_ = backwardCallGraph
	interfaces := getInterfaces(ginfo.Defs)
	/*
	           for _, pkg := range prog.AllPackages {

	                   fmt.Printf("Package path %q\n", pkg.Pkg.Path())
	                   for _, file := range pkg.Files {
	                           _ = file
	                           fmt.Println(prog.Fset.Position(file.Name.Pos()).String())
	                           findFuncDecls(file, ginfo, interfaces, funcDecls)
	                   }
	           }

	           dumpFuncDecls(funcDecls)
	*/
	for _, pkg := range prog.AllPackages {

		fmt.Printf("Package path %q\n", pkg.Pkg.Path())
		for _, file := range pkg.Files {
			_ = file
			fmt.Println(prog.Fset.Position(file.Name.Pos()).String())
			buildCallGraph(file, ginfo, interfaces, funcDecls, backwardCallGraph)
		}
	}

}
