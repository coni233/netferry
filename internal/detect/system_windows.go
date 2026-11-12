//go:build windows

package detect

import (
	"encoding/binary"
	"strings"
	"syscall"
)

const internetSettingsKeyPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

func platformSystemProxyTargets() []Target {
	enabled, valueType, err := readInternetSettingsValue("ProxyEnable")
	if err != nil || valueType != syscall.REG_DWORD || len(enabled) < 4 {
		return nil
	}
	if binary.LittleEndian.Uint32(enabled[:4]) == 0 {
		return nil
	}

	server, _, err := readInternetSettingsValue("ProxyServer")
	if err != nil {
		return nil
	}
	return targetsForSource("Windows system proxy", ParseProxyServer(registryString(server)))
}

func readInternetSettingsValue(name string) ([]byte, uint32, error) {
	var key syscall.Handle
	err := syscall.RegOpenKeyEx(
		syscall.HKEY_CURRENT_USER,
		syscall.StringToUTF16Ptr(internetSettingsKeyPath),
		0,
		syscall.KEY_READ,
		&key,
	)
	if err != nil {
		return nil, 0, err
	}
	defer syscall.RegCloseKey(key)

	valueName := syscall.StringToUTF16Ptr(name)

	var valueType uint32
	var size uint32
	err = syscall.RegQueryValueEx(key, valueName, nil, &valueType, nil, &size)
	if err != nil && err != syscall.ERROR_MORE_DATA {
		return nil, 0, err
	}
	if size == 0 {
		return nil, valueType, nil
	}

	value := make([]byte, size)
	if err := syscall.RegQueryValueEx(key, valueName, nil, &valueType, &value[0], &size); err != nil {
		return nil, 0, err
	}
	return value[:size], valueType, nil
}

func registryString(value []byte) string {
	units := make([]uint16, 0, len(value)/2)
	for index := 0; index+1 < len(value); index += 2 {
		units = append(units, binary.LittleEndian.Uint16(value[index:index+2]))
	}
	return strings.TrimRight(syscall.UTF16ToString(units), "\x00")
}
