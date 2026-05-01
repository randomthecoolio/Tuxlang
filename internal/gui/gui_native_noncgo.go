//go:build (darwin || linux) && !cgo

package gui

func showNativeDesktop(w *Window) (*Result, error) {
	return showBrowserBackend(w)
}
