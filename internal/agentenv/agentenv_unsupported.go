//go:build !windows && !darwin && !linux

package agentenv

import "errors"

const managerName = "unsupported"

func ManagerName() string {
	return managerName
}

func Install(string, bool) ([]string, error) {
	return nil, errors.New("environment installation is not supported on this platform")
}

func Uninstall(bool) ([]string, error) {
	return nil, errors.New("environment installation is not supported on this platform")
}

func CurrentStatus() (Status, error) {
	return Status{Manager: managerName}, nil
}
