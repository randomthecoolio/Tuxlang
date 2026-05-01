//go:build !darwin && !linux

package gui

func showNativeDesktop(w *Window) (*Result, error) {
	return showBrowserBackend(w)
}
