//go:build !android

package mobile

func Call(method, arg string) (string, error) { return "", nil }
