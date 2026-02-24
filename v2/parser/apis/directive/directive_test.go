package directive

import (
	"context"
	"go/ast"
	"go/token"
	"regexp"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"

	"encr.dev/v2/internals/perr"
)

func TestParseDirective(t *testing.T) {
	testcases := []struct {
		desc     string
		line     string
		expected Directive
		wantErr  string
	}{
		{
			desc: "api public endpoint",
			line: "api public",
			expected: Directive{
				Name:    "api",
				Options: []Field{{Value: "public"}},
			},
		},
		{
			desc: "custom method",
			line: "api public method=FOO",
			expected: Directive{
				Name:    "api",
				Options: []Field{{Value: "public"}},
				Fields:  []Field{{Key: "method", Value: "FOO"}},
			},
		},
		{
			desc: "multiple methods",
			line: "api public raw method=GET,POST",
			expected: Directive{
				Name:    "api",
				Options: []Field{{Value: "public"}, {Value: "raw"}},
				Fields:  []Field{{Key: "method", Value: "GET,POST"}},
			},
		},
		{
			desc: "api with tags",
			line: "api public tag:foo method=FOO raw tag:bar",
			expected: Directive{
				Name:    "api",
				Options: []Field{{Value: "public"}, {Value: "raw"}},
				Fields:  []Field{{Key: "method", Value: "FOO"}},
				Tags:    []Field{{Value: "tag:foo"}, {Value: "tag:bar"}},
			},
		},
		{
			desc:    "api with duplicate tag",
			line:    "api public tag:foo tag:foo",
			wantErr: `(?m)The tag "tag:foo" is already defined on this declaration\.`,
		},
		{
			desc: "middleware",
			line: "middleware target=tag:foo,tag:bar",
			expected: Directive{
				Name:   "middleware",
				Fields: []Field{{Key: "target", Value: "tag:foo,tag:bar"}},
			},
		},
		{
			desc:    "middleware empty target",
			line:    "middleware target=",
			wantErr: `(?m)Directive fields must have a value\.`,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			c := qt.New(t)
			fs := token.NewFileSet()
			errs := perr.NewList(context.Background(), fs)

			// Split the line into name and args
			parts := strings.Fields(tc.line)
			if len(parts) == 0 {
				t.Fatalf("empty test case line")
			}
			name := parts[0]
			args := strings.TrimPrefix(tc.line, name)
			args = strings.TrimSpace(args)

			dir, ok := parseOne(errs, name, 0, args)
			if tc.wantErr != "" {
				re := regexp.MustCompile(tc.wantErr)
				if errStr := errs.FormatErrors(); !re.MatchString(errStr) {
					c.Fatalf("error did not match regexp %s: %s", tc.wantErr, errStr)
				}
			} else {
				c.Assert(ok, qt.IsTrue)

				cmp := qt.CmpEquals(
					cmpopts.EquateEmpty(),
					cmpopts.IgnoreUnexported(Field{}),
					cmpopts.IgnoreUnexported(Directive{}),
				)
				c.Assert(dir, cmp, tc.expected)
			}
		})
	}
}

// TestParseCommentGroup tests the Parse function with ast.CommentGroup inputs.
// These tests verify the behavior of directive detection using ast.ParseDirective.
func TestParseCommentGroup(t *testing.T) {
	testcases := []struct {
		desc       string
		comments   []string // Each string is a comment line (with // prefix)
		expectDir  bool     // Whether a directive should be detected
		expectName string   // Expected directive name (if detected)
		expectArgs string   // Expected args string (rough check)
		expectDoc  string   // Expected remaining doc text
		wantErr    string   // Expected error pattern (if any)
	}{
		{
			desc:       "standard syntax encore:api",
			comments:   []string{"//encore:api public"},
			expectDir:  true,
			expectName: "api",
			expectArgs: "public",
			expectDoc:  "",
		},
		{
			desc:       "standard syntax with path",
			comments:   []string{"//encore:api public path=/foo/:id"},
			expectDir:  true,
			expectName: "api",
			expectArgs: "public path=/foo/:id",
			expectDoc:  "",
		},
		{
			desc:       "standard syntax with doc comment",
			comments:   []string{"// This is a doc comment", "//encore:api public"},
			expectDir:  true,
			expectName: "api",
			expectArgs: "public",
			expectDoc:  "This is a doc comment\n",
		},
		{
			desc:       "legacy syntax with space - NOT recognized after refactor",
			comments:   []string{"// encore:api public"},
			expectDir:  false,
			expectName: "",
			expectDoc:  "encore:api public\n",
		},
		{
			desc:       "regular comment - no directive",
			comments:   []string{"// This is just a comment"},
			expectDir:  false,
			expectName: "",
			expectDoc:  "This is just a comment\n",
		},
		{
			desc:       "non-encore directive",
			comments:   []string{"//go:generate mockgen"},
			expectDir:  false,
			expectName: "",
			expectDoc:  "",
		},
		{
			desc:       "empty comment group",
			comments:   []string{},
			expectDir:  false,
			expectName: "",
			expectDoc:  "",
		},
		{
			desc:      "multiple directives - error",
			comments:  []string{"//encore:api public", "//encore:middleware"},
			expectDir: false,
			wantErr:   "Multiple directives are not allowed",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			c := qt.New(t)
			fs := token.NewFileSet()
			errs := perr.NewList(context.Background(), fs)

			// Create a CommentGroup from the test comments
			var cg *ast.CommentGroup
			if len(tc.comments) > 0 {
				cg = &ast.CommentGroup{}
				for _, comment := range tc.comments {
					cg.List = append(cg.List, &ast.Comment{
						Slash: token.NoPos, // Position not critical for this test
						Text:  comment,
					})
				}
			}

			dir, doc, ok := Parse(errs, cg)

			if tc.wantErr != "" {
				re := regexp.MustCompile(tc.wantErr)
				if errStr := errs.FormatErrors(); !re.MatchString(errStr) {
					c.Fatalf("error did not match regexp %s: %s", tc.wantErr, errStr)
				}
				c.Assert(ok, qt.IsFalse)
				return
			}

			if tc.expectDir {
				c.Assert(ok, qt.IsTrue, qt.Commentf("expected directive to be detected"))
				c.Assert(dir, qt.IsNotNil)
				c.Assert(dir.Name, qt.Equals, tc.expectName)
				// Check that args are present in the directive string representation
				if tc.expectArgs != "" {
					dirStr := dir.String()
					c.Assert(dirStr, qt.Contains, tc.expectArgs, qt.Commentf("directive string should contain args"))
				}
				c.Assert(doc, qt.Equals, tc.expectDoc)
			} else {
				// No directive expected
				if dir != nil {
					c.Fatalf("expected no directive, got: %+v", dir)
				}
				c.Assert(ok, qt.IsTrue)
				c.Assert(doc, qt.Equals, tc.expectDoc)
			}
		})
	}
}

// TestParseDirectivePositions tests that position information is correctly tracked.
func TestParseDirectivePositions(t *testing.T) {
	c := qt.New(t)
	fs := token.NewFileSet()
	errs := perr.NewList(context.Background(), fs)

	// Create a comment at a known position
	pos := token.Pos(100)
	cg := &ast.CommentGroup{
		List: []*ast.Comment{
			{
				Slash: pos,
				Text:  "//encore:api public raw",
			},
		},
	}

	dir, _, ok := Parse(errs, cg)
	c.Assert(ok, qt.IsTrue)
	c.Assert(dir, qt.IsNotNil)

	// The directive should have position information
	c.Assert(dir.Pos(), qt.Not(qt.Equals), token.NoPos, qt.Commentf("directive should have a valid position"))
	c.Assert(dir.Name, qt.Equals, "api")
	c.Assert(len(dir.Options), qt.Equals, 2)
	c.Assert(dir.Options[0].Value, qt.Equals, "public")
	c.Assert(dir.Options[1].Value, qt.Equals, "raw")
}
