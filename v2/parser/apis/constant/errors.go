package constant

import "encr.dev/pkg/errors"

var (
	errRange = errors.Range(
		"constant",
		`hint: valid usage is:
//encore:export
const (
		Foo = 1
		Bar = 2
)

For more information on exporting constants, see https://encore.dev/docs/primitives/constants`,
		errors.WithRangeSize(50),
	)

	errInvalidConstant = errRange.New(
		"Invalid Constant",
		"encore:export can only be used on constant declarations",
	)

	errUnexportedConstant = errRange.New(
		"Invalid Constant",
		"encore:export can only be used on exported constants",
	)

	errInvalidIotaValue = errRange.New(
		"Invalid Iota",
		"iota constants must have compatible types",
	)
)
