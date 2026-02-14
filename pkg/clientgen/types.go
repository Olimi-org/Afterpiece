package clientgen

import (
	"fmt"
	"reflect"
	"sort"

	"encr.dev/pkg/clientgen/clientgentypes"
	meta "encr.dev/proto/afterpiece/parser/meta/v1"
	schema "encr.dev/proto/afterpiece/parser/schema/v1"
)

func getNamedTypes(md *meta.Data, set clientgentypes.ServiceSet) *typeRegistry {
	r := &typeRegistry{
		md:         md,
		namespaces: make(map[string][]*schema.Decl),
		seenDecls:  make(map[uint32]bool),
		declRefs:   make(map[uint32]map[uint32]bool),
		constants:  make(map[string][]*schema.ConstantDecl),
		enums:      make(map[string][]*schema.EnumDecl),
	}

	for _, svc := range md.Svcs {
		if !set.Has(svc.Name) {
			continue
		}
		for _, rpc := range svc.Rpcs {
			if rpc.AccessType != meta.RPC_PRIVATE {
				r.Visit(rpc.HandshakeSchema)
				r.Visit(rpc.RequestSchema)
				r.Visit(rpc.ResponseSchema)
			}
		}
	}

	if md.AuthHandler != nil && md.AuthHandler.Params != nil {
		r.Visit(md.AuthHandler.Params)
	}

	for _, c := range md.Constants {
		if c.PkgName == "" {
			continue
		}

		r.constants[c.PkgName] = append(r.constants[c.PkgName], c)
	}

	for _, enum := range md.Enums {
		if enum.PkgName == "" {
			continue
		}

		r.enums[enum.PkgName] = append(r.enums[enum.PkgName], enum)
	}

	// Filter out any named types that are also enums to avoid duplicate definitions
	for pkgName, enums := range r.enums {
		enumNames := make(map[string]bool, len(enums))
		for _, e := range enums {
			enumNames[e.Name] = true
		}

		if decls, ok := r.namespaces[pkgName]; ok {
			// Filter in place
			n := 0
			for _, d := range decls {
				if !enumNames[d.Name] {
					decls[n] = d
					n++
				}
			}
			r.namespaces[pkgName] = decls[:n]
		}
	}

	return r
}

// typeRegistry computes the visible set of type declarations
// and how to group them into namespaces.
type typeRegistry struct {
	md         *meta.Data
	namespaces map[string][]*schema.Decl
	seenDecls  map[uint32]bool
	declRefs   map[uint32]map[uint32]bool // tracks which decls reference which other decls
	currDecl   *schema.Decl               // may be nil
	constants  map[string][]*schema.ConstantDecl
	enums      map[string][]*schema.EnumDecl
}

func (v *typeRegistry) Decls(name string) []*schema.Decl {
	return v.namespaces[name]
}

func (v *typeRegistry) Namespaces() []string {
	nss := make([]string, 0, len(v.namespaces))
	for ns := range v.namespaces {
		nss = append(nss, ns)
	}
	sort.Strings(nss)
	return nss
}

func (v *typeRegistry) Constants(pkgName string) []*schema.ConstantDecl {
	return v.constants[pkgName]
}

func (v *typeRegistry) Enums(pkgName string) []*schema.EnumDecl {
	return v.enums[pkgName]
}

func (v *typeRegistry) Visit(typ *schema.Type) {
	if typ == nil {
		return
	}
	switch t := typ.Typ.(type) {
	case *schema.Type_Named:
		v.visitNamed(t.Named)
	case *schema.Type_List:
		v.Visit(t.List.Elem)
	case *schema.Type_Map:
		v.Visit(t.Map.Key)
		v.Visit(t.Map.Value)
	case *schema.Type_Struct:
		for _, f := range t.Struct.Fields {
			v.Visit(f.Typ)
		}
	case *schema.Type_Builtin, *schema.Type_TypeParameter, *schema.Type_Literal:
	// do nothing

	case *schema.Type_Pointer:
		v.Visit(t.Pointer.Base)

	case *schema.Type_Option:
		v.Visit(t.Option.Value)

	case *schema.Type_Config:
		v.Visit(t.Config.Elem)

	case *schema.Type_Union:
		for _, tt := range t.Union.Types {
			v.Visit(tt)
		}

	default:
		panic(fmt.Sprintf("unhandled type: %+v", reflect.TypeOf(typ.Typ)))
	}
}

func (v *typeRegistry) visitDecl(decl *schema.Decl) {
	if decl == nil {
		return
	}

	if !v.seenDecls[decl.Id] {
		v.seenDecls[decl.Id] = true
		ns := decl.Loc.PkgName
		v.namespaces[ns] = append(v.namespaces[ns], decl)

		// Set currDecl when processing this and then reset it
		prev := v.currDecl
		v.currDecl = decl
		v.Visit(decl.Type)
		v.currDecl = prev
	}
}

func (v *typeRegistry) visitNamed(n *schema.Named) {
	to := n.Id
	curr := v.currDecl
	if curr != nil {
		from := curr.Id
		if _, ok := v.declRefs[from]; !ok {
			v.declRefs[from] = make(map[uint32]bool)
		}
		v.declRefs[from][to] = true
	}

	decl := v.md.Decls[to]
	v.visitDecl(decl)

	// Add transitive refs
	if curr != nil {
		from := curr.Id
		for to2 := range v.declRefs[to] {
			v.declRefs[from][to2] = true
		}
	}

	for _, typeArg := range n.TypeArguments {
		v.Visit(typeArg)
	}
}

func (v *typeRegistry) IsRecursiveRef(from, to uint32) bool {
	return v.declRefs[from][to] && v.declRefs[to][from]
}
