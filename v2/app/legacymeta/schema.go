package legacymeta

import (
	"fmt"
	"go/ast"
	"go/token"

	"encr.dev/pkg/fns"
	"encr.dev/pkg/idents"
	"encr.dev/pkg/paths"
	schema "encr.dev/proto/afterpiece/parser/schema/v1"
	"encr.dev/v2/internals/pkginfo"
	schemav2 "encr.dev/v2/internals/schema"
	"encr.dev/v2/internals/schema/schemautil"
	"encr.dev/v2/parser"
	"encr.dev/v2/parser/apis/constant"
	"github.com/fatih/structtag"
)

func (b *builder) builtinType(typ schemav2.BuiltinType) schema.Builtin {
	switch typ.Kind {
	case schemav2.Bool:
		return schema.Builtin_BOOL
	case schemav2.Int:
		return schema.Builtin_INT
	case schemav2.Int8:
		return schema.Builtin_INT8
	case schemav2.Int16:
		return schema.Builtin_INT16
	case schemav2.Int32:
		return schema.Builtin_INT32
	case schemav2.Int64:
		return schema.Builtin_INT64
	case schemav2.Uint:
		return schema.Builtin_UINT
	case schemav2.Uint8:
		return schema.Builtin_UINT8
	case schemav2.Uint16:
		return schema.Builtin_UINT16
	case schemav2.Uint32:
		return schema.Builtin_UINT32
	case schemav2.Uint64:
		return schema.Builtin_UINT64

	case schemav2.Float32:
		return schema.Builtin_FLOAT32
	case schemav2.Float64:
		return schema.Builtin_FLOAT64
	case schemav2.String:
		return schema.Builtin_STRING
	case schemav2.Bytes:
		return schema.Builtin_BYTES

	case schemav2.Time:
		return schema.Builtin_TIME
	case schemav2.UUID:
		return schema.Builtin_UUID
	case schemav2.JSON:
		return schema.Builtin_JSON
	case schemav2.UserID:
		return schema.Builtin_USER_ID

	default:
		panic(fmt.Sprintf("unknown builtin type %v", typ.Kind))
	}
}

func (b *builder) schemaType(typ schemav2.Type) *schema.Type {
	switch typ := typ.(type) {
	case nil:
		return nil

	case schemav2.BuiltinType:
		return &schema.Type{Typ: &schema.Type_Builtin{
			Builtin: b.builtinType(typ),
		}}

	case schemav2.NamedType:
		if typ.DeclInfo.File.Pkg.ImportPath == "encore.dev/config" {
			return b.configValue(typ)
		}
		// Track this type as used in an exported schema
		qn := pkginfo.Q(typ.DeclInfo.File.Pkg.ImportPath, typ.DeclInfo.Name)
		b.usedNamedTypes[qn] = true

		return &schema.Type{Typ: &schema.Type_Named{
			Named: &schema.Named{
				Id:            b.decl(typ.Decl()),
				TypeArguments: b.schemaTypes(typ.TypeArgs...),
			},
		}}

	case schemav2.StructType:
		var fields []*schema.Field
		for _, f := range typ.Fields {
			if f.IsAnonymous() {
				continue // not supported by meta
			}
			field := b.structField(f)
			if f.IsExported() { // to match legacy meta behavior
				fields = append(fields, field)
			}
		}

		return &schema.Type{Typ: &schema.Type_Struct{
			Struct: &schema.Struct{
				Fields: fields,
			},
		}}

	case schemav2.MapType:
		return &schema.Type{Typ: &schema.Type_Map{
			Map: &schema.Map{
				Key:   b.schemaType(typ.Key),
				Value: b.schemaType(typ.Value),
			},
		}}

	case schemav2.ListType:
		// An array of bytes (like [16]byte for a UUID) is not represented
		// as the builtin BYTES in the schemav2 parser, but the legacy metadata does.
		if typ.Len >= 0 && schemautil.IsBuiltinKind(typ.Elem, schemav2.Uint8) {
			return &schema.Type{Typ: &schema.Type_Builtin{
				Builtin: schema.Builtin_BYTES,
			}}
		}

		return &schema.Type{Typ: &schema.Type_List{
			List: &schema.List{
				Elem: b.schemaType(typ.Elem),
			},
		}}

	case schemav2.PointerType:
		return &schema.Type{Typ: &schema.Type_Pointer{
			Pointer: &schema.Pointer{
				Base: b.schemaType(typ.Elem),
			},
		}}

	case schemav2.OptionType:
		return &schema.Type{Typ: &schema.Type_Option{
			Option: &schema.Option{
				Value: b.schemaType(typ.Value),
			},
		}}

	case schemav2.TypeParamRefType:
		return &schema.Type{Typ: &schema.Type_TypeParameter{
			TypeParameter: &schema.TypeParameterRef{
				DeclId:   b.decl(typ.Decl),
				ParamIdx: uint32(typ.Index),
			},
		}}

	default:
		b.errs.Addf(typ.ASTExpr().Pos(), "unsupported schema type %T", typ)
	}

	return nil
}

// schemaTypeUnwrapPointer returns the schema type for the given type,
// but unwraps the initial pointer if it is one.
// This is used for backwards compatibility with the legacy metadata,
// where certain types where returned without the leading pointer
// (most usages of *est.Param).
func (b *builder) schemaTypeUnwrapPointer(typ schemav2.Type) *schema.Type {
	if ptr, ok := typ.(schemav2.PointerType); ok {
		return b.schemaType(ptr.Elem)
	}
	return b.schemaType(typ)
}

func (b *builder) structField(f schemav2.StructField) *schema.Field {
	field := &schema.Field{
		Typ:             b.schemaType(f.Type),
		Name:            f.Name.MustGet(),
		Doc:             f.Doc,
		JsonName:        "",
		Optional:        false,
		QueryStringName: "",
		RawTag:          f.Tag.String(),
		Tags:            nil,
	}

	for _, tag := range f.Tag.Tags() {
		field.Tags = append(field.Tags, &schema.Tag{
			Key:     tag.Key,
			Name:    tag.Name,
			Options: tag.Options,
		})
	}

	// Process encore tags
	if enc, _ := f.Tag.Get("encore"); enc != nil {
		ops := append([]string{enc.Name}, enc.Options...)
		for _, o := range ops {
			switch o {
			case "optional":
				field.Optional = true
			case "httpstatus":
				// Set WireSpec for HttpStatus fields
				field.Wire = &schema.WireSpec{
					Location: &schema.WireSpec_HttpStatus_{
						HttpStatus: &schema.WireSpec_HttpStatus{},
					},
				}
			}
		}
	}

	// Treat option types as optional.
	if schemautil.IsOption(f.Type) {
		field.Optional = true
	}

	// Set WireSpec for header fields
	if header, _ := f.Tag.Get("header"); header != nil {
		headerSpec := &schema.WireSpec_Header{}
		if header.Name != "" {
			headerSpec.Name = &header.Name
		}
		field.Wire = &schema.WireSpec{
			Location: &schema.WireSpec_Header_{
				Header: headerSpec,
			},
		}
	}

	getQueryTag := func() *structtag.Tag {
		if q, _ := f.Tag.Get("query"); q != nil {
			return q
		}
		if q, _ := f.Tag.Get("qs"); q != nil {
			return q
		}
		return nil
	}

	// Set WireSpec for query string fields
	if query := getQueryTag(); query != nil {
		querySpec := &schema.WireSpec_Query{}
		if query.Name != "" {
			querySpec.Name = &query.Name
		}
		field.Wire = &schema.WireSpec{
			Location: &schema.WireSpec_Query_{
				Query: querySpec,
			},
		}
	}

	// Set WireSpec for cookie fields
	if cookie, _ := f.Tag.Get("cookie"); cookie != nil {
		cookieSpec := &schema.WireSpec_Cookie{}
		if cookie.Name != "" {
			cookieSpec.Name = &cookie.Name
		}
		field.Wire = &schema.WireSpec{
			Location: &schema.WireSpec_Cookie_{
				Cookie: cookieSpec,
			},
		}
	}

	if js, _ := f.Tag.Get("json"); js != nil {
		if v := js.Name; v != "" {
			field.JsonName = v
		}
	}

	if qs := getQueryTag(); qs != nil {
		if v := qs.Name; v != "" {
			field.QueryStringName = v
		}
	}
	if field.QueryStringName == "" {
		field.QueryStringName = idents.Convert(field.Name, idents.SnakeCase)
	}

	return field
}

func (b *builder) configValue(typ schemav2.NamedType) *schema.Type {
	switch typ.DeclInfo.Name {
	case "Value", "Values":
		isList := typ.DeclInfo.Name == "Values"
		elem := b.schemaType(typ.TypeArgs[0])

		if isList {
			elem = &schema.Type{Typ: &schema.Type_List{
				List: &schema.List{
					Elem: elem,
				},
			}}
		}

		return &schema.Type{Typ: &schema.Type_Config{
			Config: &schema.ConfigValue{
				Elem:         elem,
				IsValuesList: isList,
			},
		}}

	default:
		// Should be a named config type, like "type String = Value[string]".
		if named, ok := typ.Decl().Type.(schemav2.NamedType); ok {
			return b.configValue(named)
		} else {
			b.errs.Addf(typ.ASTExpr().Pos(), "unsupported config type %q", typ.DeclInfo.Name)
			return nil
		}
	}
}

func (b *builder) schemaTypes(typs ...schemav2.Type) []*schema.Type {
	return fns.Map(typs, b.schemaType)
}

func (b *builder) decl(decl schemav2.Decl) uint32 {
	typeDecl, ok := decl.(*schemav2.TypeDecl)
	if !ok {
		b.errs.Fatalf(decl.ASTNode().Pos(), "cannot add declaration %q of type %T", decl.String(), decl)
		return 0 // unreachable
	}

	pkgName, ok := typeDecl.PkgName().Get()
	if !ok {
		b.errs.Fatalf(decl.ASTNode().Pos(), "cannot add declaration %q that's not at package level", decl.String())
		return 0 // unreachable
	}

	// Do we already have this declaration added?
	file := decl.DeclaredIn()
	pkg := file.Pkg
	k := declKey{pkgPath: pkg.ImportPath, pkgName: pkgName}
	if n, ok := b.decls[k]; ok {
		return n
	}

	// Otherwise add it.
	declIdx := uint32(len(b.md.Decls))
	b.decls[k] = declIdx

	typeParams := fns.Map(typeDecl.TypeParams, func(p schemav2.DeclTypeParam) *schema.TypeParameter {
		return &schema.TypeParameter{Name: p.Name}
	})

	// Allocate the object and add it to the list
	// without the underlying type. We'll add the
	// underlying type afterwards to properly handle
	// recursive and mutually recursive types.
	d := &schema.Decl{
		Id:         declIdx,
		Name:       pkgName,
		Type:       nil, // computed below
		TypeParams: typeParams,
		Doc:        typeDecl.Info.Doc,
		Loc:        b.schemaLoc(file, decl.ASTNode()),
	}
	b.md.Decls = append(b.md.Decls, d)

	d.Type = b.schemaType(typeDecl.Type)

	return declIdx
}

func (b *builder) schemaLoc(f *pkginfo.File, node ast.Node) *schema.Loc {
	tokenFile := f.Token()
	sPos, ePos := tokenFile.Position(node.Pos()), tokenFile.Position(node.Pos())
	return &schema.Loc{
		PkgName:      f.Pkg.Name,
		PkgPath:      string(f.Pkg.ImportPath),
		Filename:     f.Name,
		StartPos:     int32(tokenFile.Offset(node.Pos())),
		EndPos:       int32(tokenFile.Offset(node.End())),
		SrcLineStart: int32(sPos.Line),
		SrcLineEnd:   int32(ePos.Line),
		SrcColStart:  int32(sPos.Column),
		SrcColEnd:    int32(ePos.Column),
	}
}

type declKey struct {
	pkgPath paths.Pkg
	pkgName string
}

func (b *builder) declKey(pkgPath paths.Pkg, pkgName string) uint32 {
	k := declKey{pkgPath: pkgPath, pkgName: pkgName}
	if n, ok := b.decls[k]; ok {
		return n
	}

	n := uint32(len(b.decls))
	b.decls[k] = n
	return n
}

func (b *builder) typeDeclRef(typ *schemav2.TypeDeclRef) *schema.Type {
	return b.schemaType(typ.ToType())
}

func (b *builder) typeDeclRefUnwrapPointer(typ *schemav2.TypeDeclRef) *schema.Type {
	return b.schemaTypeUnwrapPointer(typ.ToType())
}

// addEnumToMeta converts a constant.Enum to proto format and adds to metadata
func (b *builder) addEnumToMeta(e *constant.Enum) {
	enumDecl := &schema.EnumDecl{
		Name:    e.Name,
		Doc:     e.Doc,
		PkgName: e.PkgName,
	}

	// Convert underlying type
	if e.UnderlyingType != nil {
		enumDecl.UnderlyingType = b.schemaType(e.UnderlyingType)
	}

	// Convert members
	for _, m := range e.Members {
		memberDecl := &schema.ConstantDecl{
			Name: m.Name,
			Doc:  m.Doc,
		}

		switch m.Value.Kind {
		case constant.ConstantString:
			memberDecl.Value = &schema.ConstantDecl_StrValue{StrValue: m.Value.StrValue}
		case constant.ConstantInt:
			memberDecl.Value = &schema.ConstantDecl_IntValue{IntValue: m.Value.IntValue}
		case constant.ConstantBool:
			memberDecl.Value = &schema.ConstantDecl_BoolValue{BoolValue: m.Value.BoolValue}
		}

		if memberDecl.Value != nil {
			enumDecl.Members = append(enumDecl.Members, memberDecl)
		}
	}

	if len(enumDecl.Members) > 0 {
		b.md.Enums = append(b.md.Enums, enumDecl)
	}
}

// populateEnums handles auto-export of enums that are referenced in exported types.
// It parses all const blocks to find enums and exports those that are either:
// 1. Explicitly marked with //encore:export (already handled in resource loop), or
// 2. Used as dependencies in exported types (usedNamedTypes)
func (b *builder) populateEnums() {
	// First, collect explicitly exported enums
	explicitEnums := make(map[pkginfo.QualifiedName]bool)
	for _, enum := range parser.Resources[*constant.Enum](b.app.Parse) {
		qn := pkginfo.Q(paths.Pkg(enum.PkgPath), enum.Name)
		explicitEnums[qn] = true
	}

	// Now find and parse enum candidates that are used as dependencies
	for _, pkg := range b.app.Parse.AppPackages() {
		for _, file := range pkg.Files {
			for _, decl := range file.AST().Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok || genDecl.Tok != token.CONST {
					continue
				}

				// Parse this const block without requiring //encore:export
				resources := constant.ParseWithoutDirective(constant.ParseData{
					Schema: b.app.SchemaParser,
					Errs:   b.errs,
					File:   file,
					Decl:   genDecl,
					Doc:    "", // Doc will be extracted from type declaration
				})

				for _, res := range resources {
					if enum, ok := res.(*constant.Enum); ok {
						qn := pkginfo.Q(paths.Pkg(enum.PkgPath), enum.Name)

						// Skip if already exported explicitly or already processed
						if explicitEnums[qn] || b.exportedEnums[qn] {
							continue
						}

						// Only export if used as a dependency
						if b.usedNamedTypes[qn] {
							b.exportedEnums[qn] = true
							b.addEnumToMeta(enum)
						}
					}
				}
			}
		}
	}
}

// populateConstants converts explicitly exported constants to proto format
func (b *builder) populateConstants() {
	explicitConstants := parser.Resources[*constant.Constant](b.app.Parse)

	for _, c := range explicitConstants {
		constDecl := &schema.ConstantDecl{
			Name:    c.Name,
			Doc:     c.Doc,
			PkgName: c.PkgName,
		}

		// Convert type (or infer from value if not specified)
		if c.Type != nil {
			constDecl.Type = b.schemaType(c.Type)
		} else {
			// Infer type from value
			switch c.Value.Kind {
			case constant.ConstantString:
				constDecl.Type = &schema.Type{Typ: &schema.Type_Builtin{Builtin: schema.Builtin_STRING}}
			case constant.ConstantInt:
				constDecl.Type = &schema.Type{Typ: &schema.Type_Builtin{Builtin: schema.Builtin_INT}}
			case constant.ConstantBool:
				constDecl.Type = &schema.Type{Typ: &schema.Type_Builtin{Builtin: schema.Builtin_BOOL}}
			case constant.ConstantFloat:
				constDecl.Type = &schema.Type{Typ: &schema.Type_Builtin{Builtin: schema.Builtin_FLOAT64}}
			}
		}

		// Convert value
		switch c.Value.Kind {
		case constant.ConstantString:
			constDecl.Value = &schema.ConstantDecl_StrValue{StrValue: c.Value.StrValue}
		case constant.ConstantInt:
			constDecl.Value = &schema.ConstantDecl_IntValue{IntValue: c.Value.IntValue}
		case constant.ConstantBool:
			constDecl.Value = &schema.ConstantDecl_BoolValue{BoolValue: c.Value.BoolValue}
		case constant.ConstantFloat:
			constDecl.Value = &schema.ConstantDecl_IntValue{IntValue: int64(c.Value.FloatValue)}
		}

		if constDecl.Type != nil && constDecl.Value != nil {
			b.md.Constants = append(b.md.Constants, constDecl)
		}
	}
}
