//go:build darwin

package detect

import "os/exec"

func platformSystemProxyTargets() []Target {
	output, err := exec.Command("scutil", "--proxy").Output()
	if err != nil {
		return nil
	}
	return targetsForSource("macOS system proxy", parseScutilProxy(string(output)))
}
