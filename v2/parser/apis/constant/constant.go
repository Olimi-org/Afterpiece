package constant

import (
	"go/ast"
	"go/token"
	"strconv"

	v1schema "encr.dev/proto/afterpiece/parser/schema/v1"
	"encr.dev/v2/internals/perr"
	"encr.dev/v2/internals/pkginfo"
	"encr.dev/v2/internals/schema"
	"encr.dev/v2/parser/apis/directive"
	"encr.dev/v2/parser/resource"
)

type ConstantKind int

const (
	ConstantString ConstantKind = iota
	ConstantInt
	ConstantBool
	ConstantFloat
)

type ConstantValue struct {
	StrValue   string
	IntValue   int64
	BoolValue  bool
	FloatValue float64
	Kind       ConstantKind
}

func (cv ConstantValue) String() string {
	switch cv.Kind {
	case ConstantString:
		return cv.StrValue
	case ConstantInt:
		return strconv.FormatInt(cv.IntValue, 10)
	case ConstantBool:
		return strconv.FormatBool(cv.BoolValue)
	case ConstantFloat:
		return strconv.FormatFloat(cv.FloatValue, 'f', -1, 64)
	}
	return ""
}

type Constant struct {
	errs *perr.List

	Name    string
	Type    schema.Type
	Value   ConstantValue
	Doc     string
	Loc     v1schema.Loc
	PkgName string
	PkgPath string
}

func (c *Constant) GoString() string {
	if c == nil {
		return "(*constant.Constant)(nil)"
	}
	return "constant." + c.Name
}

func (c *Constant) Kind() resource.Kind       { return resource.Constant }
func (c *Constant) Package() *pkginfo.Package { return nil }
func (c *Constant) Pos() token.Pos            { return token.Pos(c.Loc.StartPos) }
func (c *Constant) End() token.Pos            { return token.Pos(c.Loc.EndPos) }
func (c *Constant) SortKey() string           { return c.PkgPath + "." + c.Name }

type Enum struct {
	errs *perr.List

	Name           string
	UnderlyingType schema.Type
	Members        []*Constant
	Doc            string
	Loc            v1schema.Loc
	PkgName        string
	PkgPath        string
}

func (e *Enum) GoString() string {
	if e == nil {
		return "(*constant.Enum)(nil)"
	}
	return "constant.Enum(" + e.Name + ")"
}

func (e *Enum) Kind() resource.Kind       { return resource.Enum }
func (e *Enum) Package() *pkginfo.Package { return nil }
func (e *Enum) Pos() token.Pos            { return token.Pos(e.Loc.StartPos) }
func (e *Enum) End() token.Pos            { return token.Pos(e.Loc.EndPos) }
func (e *Enum) SortKey() string           { return e.PkgPath + "." + e.Name }

type ParseData struct {
	Errs   *perr.List
	Schema *schema.Parser
	File   *pkginfo.File
	Decl   *ast.GenDecl
	Dir    *directive.Directive
	Doc    string
}

// Parse processes const declarations with //encore:export directive
func Parse(d ParseData) (results []any) {
	if d.Decl.Tok != token.CONST {
		d.Errs.Add(errInvalidConstant.AtGoPos(d.Decl.Pos(), d.Decl.End()))
		return nil
	}

	schemaType := func(expr ast.Expr) schema.Type {
		if d.Schema != nil {
			return d.Schema.ParseType(d.File, expr)
		}
		return nil
	}

	var constants []*Constant
	var enums []*Enum

	// Track lastType and lastTypeExpr for type inheritance across specs
	var lastType schema.Type
	var lastTypeExpr ast.Expr

	for specIndex, spec := range d.Decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		// Update inherited type if this spec has an explicit type
		if valueSpec.Type != nil {
			lastTypeExpr = valueSpec.Type
			lastType = schemaType(valueSpec.Type)
		}

		for i, name := range valueSpec.Names {
			if !name.IsExported() {
				d.Errs.Add(errUnexportedConstant.AtGoNode(name))
				continue
			}

			// Use inherited type
			typ := lastType

			// Evaluate value with specIndex for correct iota handling
			value := evaluateConstantWithSpecIndex(d.File, valueSpec.Values, i, specIndex, lastTypeExpr)

			tokenFile := d.File.Token()
			startPos := tokenFile.Position(name.Pos())
			endPos := tokenFile.Position(name.End())

			loc := v1schema.Loc{
				PkgPath:      d.File.Pkg.ImportPath.String(),
				PkgName:      d.File.Pkg.Name,
				Filename:     d.File.Name,
				StartPos:     int32(name.Pos()),
				EndPos:       int32(name.End()),
				SrcLineStart: int32(startPos.Line),
				SrcLineEnd:   int32(endPos.Line),
				SrcColStart:  int32(startPos.Column),
				SrcColEnd:    int32(endPos.Column),
			}

			c := &Constant{
				Name:    name.Name,
				Type:    typ,
				Value:   value,
				Doc:     d.Doc,
				Loc:     loc,
				PkgName: d.File.Pkg.Name,
				PkgPath: d.File.Pkg.ImportPath.String(),
			}
			constants = append(constants, c)
		}
	}

	if len(constants) == 0 {
		return nil
	}

	if e := tryGroupAsEnum(constants, d.File.Pkg.ImportPath.String(), d.File.Pkg.Name, d.Doc); e != nil {
		enums = append(enums, e)
	} else {
		for _, c := range constants {
			results = append(results, c)
		}
	}

	for _, e := range enums {
		results = append(results, e)
	}

	return results
}

// ParseWithoutDirective processes const declarations without requiring //encore:export directive.
// This is used for auto-exporting enums that are dependencies of exported types.
func ParseWithoutDirective(d ParseData) (results []any) {
	if d.Decl.Tok != token.CONST {
		return nil
	}

	schemaType := func(expr ast.Expr) schema.Type {
		if d.Schema != nil {
			return d.Schema.ParseType(d.File, expr)
		}
		return nil
	}

	var constants []*Constant
	var enums []*Enum

	// Track lastType and lastTypeExpr for type inheritance across specs
	var lastType schema.Type
	var lastTypeExpr ast.Expr

	for specIndex, spec := range d.Decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		// Update inherited type if this spec has an explicit type
		if valueSpec.Type != nil {
			lastTypeExpr = valueSpec.Type
			lastType = schemaType(valueSpec.Type)
		}

		for i, name := range valueSpec.Names {
			if !name.IsExported() {
				continue // Skip unexported, don't error
			}

			// Use inherited type
			typ := lastType

			// Evaluate value with specIndex for correct iota handling
			value := evaluateConstantWithSpecIndex(d.File, valueSpec.Values, i, specIndex, lastTypeExpr)

			tokenFile := d.File.Token()
			startPos := tokenFile.Position(name.Pos())
			endPos := tokenFile.Position(name.End())

			loc := v1schema.Loc{
				PkgPath:      d.File.Pkg.ImportPath.String(),
				PkgName:      d.File.Pkg.Name,
				Filename:     d.File.Name,
				StartPos:     int32(name.Pos()),
				EndPos:       int32(name.End()),
				SrcLineStart: int32(startPos.Line),
				SrcLineEnd:   int32(endPos.Line),
				SrcColStart:  int32(startPos.Column),
				SrcColEnd:    int32(endPos.Column),
			}

			c := &Constant{
				Name:    name.Name,
				Type:    typ,
				Value:   value,
				Doc:     d.Doc,
				Loc:     loc,
				PkgName: d.File.Pkg.Name,
				PkgPath: d.File.Pkg.ImportPath.String(),
			}
			constants = append(constants, c)
		}
	}

	if len(constants) == 0 {
		return nil
	}

	if e := tryGroupAsEnum(constants, d.File.Pkg.ImportPath.String(), d.File.Pkg.Name, d.Doc); e != nil {
		enums = append(enums, e)
	} else {
		for _, c := range constants {
			results = append(results, c)
		}
	}

	for _, e := range enums {
		results = append(results, e)
	}

	return results
}

// evaluateConstantWithSpecIndex evaluates a constant value using specIndex for iota.
// It also handles cases where no value is provided (inherited from previous spec).
func evaluateConstantWithSpecIndex(file *pkginfo.File, values []ast.Expr, valueIndex int, specIndex int, lastTypeExpr ast.Expr) ConstantValue {
	var expr ast.Expr
	if len(values) > valueIndex {
		expr = values[valueIndex]
	}

	if expr == nil {
		// No explicit value - for iota-based enums, return specIndex
		// This handles cases like:
		//   const (
		//       A Status = iota  // specIndex=0
		//       B                // specIndex=1, no value expr, should be 1
		//   )
		return ConstantValue{IntValue: int64(specIndex), Kind: ConstantInt}
	}

	return evaluateExprWithSpecIndex(file, expr, specIndex)
}

// evaluateExprWithSpecIndex recursively evaluates an expression using specIndex for iota
func evaluateExprWithSpecIndex(file *pkginfo.File, expr ast.Expr, specIndex int) ConstantValue {
	switch lit := expr.(type) {
	case *ast.BasicLit:
		if lit.Kind == token.STRING {
			val, err := strconv.Unquote(lit.Value)
			if err == nil {
				return ConstantValue{StrValue: val, Kind: ConstantString}
			}
			return ConstantValue{StrValue: lit.Value[1 : len(lit.Value)-1], Kind: ConstantString}
		} else if lit.Kind == token.INT {
			val, err := strconv.ParseInt(lit.Value, 10, 64)
			if err == nil {
				return ConstantValue{IntValue: val, Kind: ConstantInt}
			}
		} else if lit.Kind == token.FLOAT {
			val, err := strconv.ParseFloat(lit.Value, 64)
			if err == nil {
				return ConstantValue{FloatValue: val, Kind: ConstantFloat}
			}
		} else if lit.Kind == token.CHAR {
			return ConstantValue{StrValue: lit.Value, Kind: ConstantString}
		}

	case *ast.Ident:
		if lit.Name == "iota" {
			return ConstantValue{IntValue: int64(specIndex), Kind: ConstantInt}
		}
		// Could be "true", "false", or another constant reference
		if lit.Name == "true" {
			return ConstantValue{BoolValue: true, Kind: ConstantBool}
		}
		if lit.Name == "false" {
			return ConstantValue{BoolValue: false, Kind: ConstantBool}
		}

	case *ast.UnaryExpr:
		if lit.Op == token.SUB {
			val := evaluateExprWithSpecIndex(file, lit.X, specIndex)
			if val.Kind == ConstantInt {
				return ConstantValue{IntValue: -val.IntValue, Kind: ConstantInt}
			} else if val.Kind == ConstantFloat {
				return ConstantValue{FloatValue: -val.FloatValue, Kind: ConstantFloat}
			}
		}

	case *ast.BinaryExpr:
		left := evaluateExprWithSpecIndex(file, lit.X, specIndex)
		right := evaluateExprWithSpecIndex(file, lit.Y, specIndex)

		if left.Kind == ConstantInt && right.Kind == ConstantInt {
			switch lit.Op {
			case token.ADD:
				return ConstantValue{IntValue: left.IntValue + right.IntValue, Kind: ConstantInt}
			case token.SUB:
				return ConstantValue{IntValue: left.IntValue - right.IntValue, Kind: ConstantInt}
			case token.MUL:
				return ConstantValue{IntValue: left.IntValue * right.IntValue, Kind: ConstantInt}
			case token.QUO:
				if right.IntValue != 0 {
					return ConstantValue{IntValue: left.IntValue / right.IntValue, Kind: ConstantInt}
				}
			case token.REM:
				if right.IntValue != 0 {
					return ConstantValue{IntValue: left.IntValue % right.IntValue, Kind: ConstantInt}
				}
			case token.AND:
				return ConstantValue{IntValue: left.IntValue & right.IntValue, Kind: ConstantInt}
			case token.OR:
				return ConstantValue{IntValue: left.IntValue | right.IntValue, Kind: ConstantInt}
			case token.XOR:
				return ConstantValue{IntValue: left.IntValue ^ right.IntValue, Kind: ConstantInt}
			case token.SHL:
				return ConstantValue{IntValue: left.IntValue << uint(right.IntValue), Kind: ConstantInt}
			case token.SHR:
				return ConstantValue{IntValue: left.IntValue >> uint(right.IntValue), Kind: ConstantInt}
			}
		}

		if left.Kind == ConstantFloat && right.Kind == ConstantFloat {
			switch lit.Op {
			case token.ADD:
				return ConstantValue{FloatValue: left.FloatValue + right.FloatValue, Kind: ConstantFloat}
			case token.SUB:
				return ConstantValue{FloatValue: left.FloatValue - right.FloatValue, Kind: ConstantFloat}
			case token.MUL:
				return ConstantValue{FloatValue: left.FloatValue * right.FloatValue, Kind: ConstantFloat}
			case token.QUO:
				return ConstantValue{FloatValue: left.FloatValue / right.FloatValue, Kind: ConstantFloat}
			}
		}

	case *ast.CallExpr:
		if fun, ok := lit.Fun.(*ast.Ident); ok && fun.Name == "len" {
			return ConstantValue{IntValue: 0, Kind: ConstantInt}
		}
	}

	return ConstantValue{}
}

// tryGroupAsEnum attempts to group constants as an enum
// Returns an Enum if constants form an enum, nil otherwise
func tryGroupAsEnum(constants []*Constant, pkgPath, pkgName, doc string) *Enum {
	if len(constants) < 2 {
		return nil
	}

	firstType := constants[0].Type
	if firstType == nil {
		return nil
	}

	for _, c := range constants[1:] {
		if c.Type == nil || !typesEqual(firstType, c.Type) {
			return nil
		}
	}

	// Extract enum name and doc from the type (for NamedType, use the type's name and doc)
	enumName := ""
	enumDoc := doc // Default to const block's doc
	if namedType, ok := firstType.(schema.NamedType); ok {
		// Get the type name and doc from the NamedType's declaration
		if namedType.DeclInfo != nil {
			enumName = namedType.DeclInfo.Name
			if namedType.DeclInfo.Doc != "" {
				enumDoc = namedType.DeclInfo.Doc
			}
		}
	}

	// Fallback: derive from constant name prefix
	if enumName == "" {
		enumName = constants[0].Name
		if idx := len(constants[0].Name) - 1; idx > 0 && constants[0].Name[idx] >= '0' && constants[0].Name[idx] <= '9' {
			prefix := constants[0].Name[:idx]
			allSamePrefix := true
			for _, c := range constants[1:] {
				if len(c.Name) < idx || c.Name[:idx] != prefix {
					allSamePrefix = false
					break
				}
			}
			if allSamePrefix {
				enumName = prefix
			}
		}
	}

	loc := constants[0].Loc
	loc.EndPos = constants[len(constants)-1].Loc.EndPos

	return &Enum{
		Name:           enumName,
		UnderlyingType: firstType,
		Members:        constants,
		Doc:            enumDoc,
		Loc:            loc,
		PkgName:        pkgName,
		PkgPath:        pkgPath,
	}
}

// typesEqual checks if two schema types are equal
func typesEqual(t1, t2 schema.Type) bool {
	if t1 == nil || t2 == nil {
		return t1 == t2
	}

	if _, ok := t1.(schema.BuiltinType); ok {
		if _, ok2 := t2.(schema.BuiltinType); ok2 {
			return true
		}
	}

	if _, ok := t1.(schema.NamedType); ok {
		if _, ok2 := t2.(schema.NamedType); ok2 {
			return true
		}
	}

	return false
}
