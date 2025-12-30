package builderimpl

import (
	"encore.dev/appruntime/exported/experiments"
	"encr.dev/pkg/appfile"
	"encr.dev/pkg/builder"
	"encr.dev/v2/v2builder"
)

func Resolve(lang appfile.Lang, expSet *experiments.Set) builder.Impl {
	return v2builder.BuilderImpl{}
}
