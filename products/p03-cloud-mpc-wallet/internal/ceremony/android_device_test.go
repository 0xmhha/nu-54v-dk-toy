package ceremony_test

import (
	"bytes"
	"net/url"
	"os"
	"os/exec"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
)

func androidParticipant(t *testing.T, serial string, participants map[string]ceremony.Endpoint, conf string, files map[string]string) func() {
	t.Helper()
	adb := func(payload []byte, args ...string) {
		t.Helper()
		cmd := exec.Command("adb", append([]string{"-s", serial}, args...)...)
		cmd.Stdin = bytes.NewReader(payload)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Android fixture setup command failed: %v", err)
		} else if len(out) > 0 {
			t.Logf("Android fixture command completed")
		}
	}
	for id, e := range participants {
		target, err := url.Parse(e.URL)
		if err != nil {
			t.Fatal(err)
		}
		port := "tcp:" + target.Port()
		if id == "a1" {
			adb(nil, "forward", "--no-rebind", port, port)
			t.Cleanup(func() { _ = exec.Command("adb", "-s", serial, "forward", "--remove", port).Run() })
		} else {
			adb(nil, "reverse", "--no-rebind", port, port)
			t.Cleanup(func() { _ = exec.Command("adb", "-s", serial, "reverse", "--remove", port).Run() })
		}
	}
	adb(nil, "shell", "run-as", "com.nu54renter", "mkdir", "-p", "no_backup/cloud-p03-participant")
	files["participant.json"] = conf
	for dest, src := range files {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		adb(data, "shell", "-T", "run-as com.nu54renter sh -c 'umask 077 && cat > no_backup/cloud-p03-participant/"+dest+"'")
		clear(data)
	}
	var cmd *exec.Cmd
	stop := func() {
		if cmd == nil {
			return
		}
		_ = exec.Command("adb", "-s", serial, "shell", "am", "force-stop", "com.nu54renter").Run()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		cmd = nil
	}
	start := func() {
		stop()
		cmd = exec.Command("adb", "-s", serial, "shell", "am", "instrument", "-w",
			"com.nu54renter.test/com.nu54renter.ble.CloudWalletParticipantLab")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	start()
	t.Cleanup(stop)
	return func() {
		adb(nil, "shell", "-T", "run-as com.nu54renter sh -c 'test -s no_backup/cloud-p03-participant/vault-key.sealed && ! test -e no_backup/cloud-p03-participant/vault.key'")
		start()
	}
}
