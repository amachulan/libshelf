package nativedialog

import "errors"

var (
	ErrCanceled    = errors.New("canceled")
	ErrUnsupported = errors.New("native dialogs are not supported on this platform")
)
