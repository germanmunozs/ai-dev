//go:build !((darwin && !ios) || (linux && !android))

package configstore

func newPlatformStore(string) Store { return unsupportedStore{} }
