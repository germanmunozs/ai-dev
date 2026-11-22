//go:build !unix

package app

func brokenPipeAsError() (release func()) { return func() {} }
