//go:build !windows && !darwin && !linux

package detect

func platformSystemProxyTargets() []Target {
	return nil
}
