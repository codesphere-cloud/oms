package util

func IgnoreError(fn func() error) {
	_ = fn()
}
