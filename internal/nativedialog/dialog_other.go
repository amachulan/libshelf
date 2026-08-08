//go:build !windows

package nativedialog

func Available() bool { return false }

func Folder(string) (string, error) { return "", ErrUnsupported }

func File(string, string) (string, error) { return "", ErrUnsupported }
