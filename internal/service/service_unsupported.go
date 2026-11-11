//go:build !windows && !darwin && !linux

package service

import "errors"

const managerName = "unsupported"

func ManagerName() string {
	return managerName
}

func Install(Spec, bool) ([]string, error) {
	return nil, errors.New("automatic startup is not supported on this platform")
}

func Uninstall(bool) ([]string, error) {
	return nil, errors.New("automatic startup is not supported on this platform")
}

func Start(bool) ([]string, error) {
	return nil, errors.New("automatic startup is not supported on this platform")
}

func Stop(bool) ([]string, error) {
	return nil, errors.New("automatic startup is not supported on this platform")
}

func CurrentStatus() (Status, error) {
	return Status{Manager: managerName}, nil
}
