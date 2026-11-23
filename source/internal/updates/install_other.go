//go:build !windows && !android

package updates

import "errors"

func RunInstaller() bool { return false }
func Install(path, state string) error {
	return errors.New("Установите обновление вручную")
}
