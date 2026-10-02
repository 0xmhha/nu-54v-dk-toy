package ble

// attMTU converts what CoreBluetooth reports (the largest write value, ATT_MTU - 3) to the ATT_MTU.
func attMTU(reported uint16) int { return int(reported) + 3 }
