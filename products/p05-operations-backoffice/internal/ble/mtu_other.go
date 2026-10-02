//go:build !darwin

package ble

// attMTU: BlueZ reports the ATT_MTU itself.
func attMTU(reported uint16) int { return int(reported) }
