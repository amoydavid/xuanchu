package config

import (
	"errors"
	"fmt"
	"os"
)

// RemoteTokenPermissionWarning 返回可直接打印到命令行的 warning 文本，但不阻断执行。
func RemoteTokenPermissionWarning(configDir string) (string, error) {
	if configDir == "" {
		return "", nil
	}
	path, values, err := loadTomlConfigWithPath(configDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	if values["remote.token"] == "" {
		return "", nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Mode().Perm()&0o077 == 0 {
		return "", nil
	}
	return fmt.Sprintf(
		"warning: %s contains remote.token but file permissions are %04o; restrict it to 0600 or stricter",
		path,
		info.Mode().Perm(),
	), nil
}
