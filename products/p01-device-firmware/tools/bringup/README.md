# Board bring-up tools

Scripts used to bring up the NU-54V-DK board: check the serial console, BLE, buttons and
debug registers. They run on a computer connected to the board over USB.

| Script | What it checks | Needs firmware | Needs |
|---|---|---|---|
| `serial_cli.py` | Sends CLI commands over the virtual COM port (VCOM) and prints replies | reference example with the CLI (`button`, `ble_nus`) | Python only |
| `ble_scan.py` | Finds the board's BLE advertisements | reference `ble_nus` | [bleak](https://github.com/hbldh/bleak) |
| `ble_nus_cli.py` | Connects over BLE and runs CLI commands through the Nordic UART Service | reference `ble_nus` | bleak |
| `ble_adv_rate.py` | Measures how often advertisements arrive | reference `ble_nus` | bleak |
| `button_sim.py` | Simulates button presses from the debugger and prints the firmware's button log | product firmware or reference `button` | [pyOCD](https://pyocd.io) |
| `button_watch.py` | Compares real button pin levels with what the firmware reports | reference `button` | pyOCD |
| `debug_regs.py` | Reads core state (DHCSR) and GPIO/GPIOTE registers without halting | any | pyOCD |

`board.py` holds the shared parts: VCOM access, the pyOCD session, register addresses and
button pins.

The reference examples are in the board vendor's repository
([`chcbaram/nu54v-dk`](https://github.com/chcbaram/nu54v-dk), `firmware/projects/`). Build and flash
one with the same toolchain as the product firmware.

## Usage

The scripts that need bleak or pyOCD declare it inline, so [uv](https://docs.astral.sh/uv/)
installs it on first run:

```bash
cd products/p01-device-firmware/tools/bringup
python3 serial_cli.py "ble info"
uv run ble_scan.py
uv run ble_nus_cli.py "ble info"
uv run button_sim.py --button BTN1 --hold 0.3 --query
uv run debug_regs.py core
```

The VCOM device is picked automatically when exactly one `/dev/cu.usbmodem*04` exists.
Otherwise pass `--port` or set `NU54_VCOM`. The device name changes with the USB port.

## Notes

- The reference CLI runs a command only when it receives CR (`\r`). Terminals and apps that
  send only LF echo the text without running it.
- pyOCD attaches without halting or resetting the target. It prints
  "NRF54L15 is not in a secure state" because the board's debug port is not locked; this is
  expected on a development board.
- The BLE scripts use the host's Bluetooth stack (CoreBluetooth on macOS). They do not run in CI.
- `button_sim.py` changes a pin's pull resistor for the length of the press and then restores
  the original pull setting. If the script is interrupted, reset the board.
- `button_sim.py` attaches the debugger only while it changes a pin. With a debug session held
  open, the core's sleep and kernel timers stalled in testing, so firmware that polls a held
  button (the product firmware's click/long-press logic) missed releases. The product firmware
  needs no CLI: it logs `SWn click` and `SWn long press` on VCOM by itself.

## Factory firmware backup

Before the first flash, the whole RRAM of the board (`0x0`–`0x17D000`) was read with
`pyocd cmd -t nrf54l -c 'savemem 0x0 0x17D000 nu54dk-factory-rram.bin'`. The image is the
vendor's firmware, so it is kept outside this repository. Everything else is built from
this repository when needed. Check the copy against this hash before you restore it:

| File | Size | SHA-256 |
|---|---|---|
| `nu54dk-factory-rram.bin` | 1,560,576 bytes | `cbd6664d3e847a473916446a3c2eaa3004c4a23bdd5039c5be1743c1db445707` |
