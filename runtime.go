// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"strings"
)

type FunctionCache struct {
	*compiler
	functions map[string]llvm.Value
}

func NewFunctionCache(c *compiler) *FunctionCache {
	return &FunctionCache{c, make(map[string]llvm.Value)}
}

func (c *FunctionCache) NamedFunction(name string, signature string) llvm.Value {
	f, _ := c.functions[name+":"+signature]
	if !f.IsNil() {
		return f
	}

	if strings.HasPrefix(name, c.module.Name+".") {
		obj := c.pkg.Scope.Lookup(name[len(c.module.Name)+1:])
		if obj == nil {
			panic("Missing function: " + name)
		}
		value := c.Resolve(obj)
		f = llvm.ConstExtractValue(value.LLVMValue(), []uint32{0})
	} else {
		fset := token.NewFileSet()
		code := `package runtime;import("unsafe");` + signature + `{panic("")}`
		file, err := parser.ParseFile(fset, "", code, 0)
		if err != nil {
			panic(err)
		}

		// Parse the runtime package, since we may need to refer to
		// its types. Can't be cached, because type-checking can't
		// be done twice on the AST.
		buildpkg, err := build.Import("github.com/axw/llgo/pkg/runtime", "", 0)
		if err != nil {
			panic(err)
		}

		// All types visible to the compiler are in "types.go".
		runtimefiles := []string{path.Join(buildpkg.Dir, "types.go")}

		files, err := parseFiles(fset, runtimefiles)
		if err != nil {
			panic(err)
		}
		files["<src>"] = file

		archinfo := c.ArchInfo()
		ctx := &types.Context{
			IntSize: archinfo.IntSize,
			PtrSize: archinfo.PtrSize,
		}
		pkg, err := ctx.Check(fset, files)
		if err != nil {
			panic(err)
		}
		for obj, pkg := range createPackageMap(pkg, "runtime") {
			c.pkgmap[obj] = pkg
		}

		fdecl := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
		ftype := fdecl.Name.Obj.Type.(*types.Signature)
		llvmfntyp := c.types.ToLLVM(ftype).StructElementTypes()[0].ElementType()
		f = llvm.AddFunction(c.module.Module, name, llvmfntyp)
	}
	c.functions[name+":"+signature] = f
	return f
}

func parseFile(fset *token.FileSet, name string) (*ast.File, error) {
	return parser.ParseFile(fset, name, nil, parser.DeclarationErrors)
}

func parseFiles(fset *token.FileSet, filenames []string) (files map[string]*ast.File, err error) {
	files = make(map[string]*ast.File)
	for _, filename := range filenames {
		var file *ast.File
		file, err = parseFile(fset, filename)
		if err != nil {
			return
		} else if file != nil {
			if files[filename] != nil {
				err = fmt.Errorf("%q: duplicate file", filename)
				return
			}
			files[filename] = file
		}
	}
	return
}

// parseReflect parses the reflect package and type-checks its AST.
// This is used to generate runtime type structures.
func (c *compiler) parseReflect() (*ast.Package, error) {
	buildpkg, err := build.Import("reflect", "", 0)
	if err != nil {
		return nil, err
	}

	filenames := make([]string, len(buildpkg.GoFiles))
	for i, f := range buildpkg.GoFiles {
		filenames[i] = path.Join(buildpkg.Dir, f)
	}
	fset := token.NewFileSet()
	files, err := parseFiles(fset, filenames)
	if err != nil {
		return nil, err
	}

	archinfo := c.ArchInfo()
	ctx := &types.Context{
		IntSize: archinfo.IntSize,
		PtrSize: archinfo.PtrSize,
	}
	pkg, err := ctx.Check(fset, files)
	if err != nil {
		return nil, err
	}
	for obj, pkg := range createPackageMap(pkg, "reflect") {
		c.pkgmap[obj] = pkg
	}
	return pkg, nil
}

func (c *compiler) createMalloc(size llvm.Value) llvm.Value {
	malloc := c.NamedFunction("runtime.malloc", "func f(uintptr) unsafe.Pointer")
	switch n := size.Type().IntTypeWidth() - c.target.IntPtrType().IntTypeWidth(); {
	case n < 0:
		size = c.builder.CreateZExt(size, c.target.IntPtrType(), "")
	case n > 0:
		size = c.builder.CreateTrunc(size, c.target.IntPtrType(), "")
	}
	return c.builder.CreateCall(malloc, []llvm.Value{size}, "")
}

func (c *compiler) createTypeMalloc(t llvm.Type) llvm.Value {
	ptr := c.createMalloc(llvm.SizeOf(t))
	return c.builder.CreateIntToPtr(ptr, llvm.PointerType(t, 0), "")
}
