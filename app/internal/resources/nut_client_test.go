package resources

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestNormalizeNUTAddress_DefaultPort(t *testing.T) {
	got := NormalizeNUTAddress("server.local")
	if got != "server.local:3493" {
		t.Fatalf("NormalizeNUTAddress = %q, want server.local:3493", got)
	}
}

func TestNormalizeNUTAddress_URL(t *testing.T) {
	got := NormalizeNUTAddress("http://server.local:3493/")
	if got != "server.local:3493" {
		t.Fatalf("NormalizeNUTAddress = %q, want server.local:3493", got)
	}
}

func TestParseNUTVarLine_UnquotesValue(t *testing.T) {
	key, value, ok := parseNUTVarLine(`VAR apc device.model "Back-UPS \"BX750MI\""`, "apc")
	if !ok {
		t.Fatal("expected line to parse")
	}
	if key != "device.model" {
		t.Fatalf("key = %q", key)
	}
	if value != `Back-UPS "BX750MI"` {
		t.Fatalf("value = %q", value)
	}
}

func TestParseNUTUPSLine(t *testing.T) {
	name, ok := parseNUTUPSLine(`UPS apc "Back-UPS BX750MI"`)
	if !ok {
		t.Fatal("expected line to parse")
	}
	if name != "apc" {
		t.Fatalf("name = %q", name)
	}
}

func TestNUTProtocolError_UnknownUPS(t *testing.T) {
	err := nutProtocolError("UNKNOWN-UPS", "apc")
	if err == nil || err.Error() != `NUT does not know UPS "apc"` {
		t.Fatalf("error = %v", err)
	}
	if !IsUnknownUPSError(err) {
		t.Fatal("expected typed unknown UPS error")
	}
}

func TestNUTClientFetchUPSNames(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		line, err := reader.ReadString('\n')
		if err != nil {
			done <- err
			return
		}
		if strings.TrimSpace(line) != "LIST UPS" {
			done <- fmt.Errorf("command = %q", line)
			return
		}

		_, err = fmt.Fprint(conn, strings.Join([]string{
			`BEGIN LIST UPS`,
			`UPS apc "Back-UPS BX750MI"`,
			`END LIST UPS`,
			"",
		}, "\n"))
		done <- err
	}()

	client := NewNUTClient(listener.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	names, err := client.FetchUPSNames(ctx)
	if err != nil {
		t.Fatalf("FetchUPSNames: %v", err)
	}
	if len(names) != 1 || names[0] != "apc" {
		t.Fatalf("names = %#v, want [apc]", names)
	}

	if err := <-done; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func TestUPSInfoFromNUTVars(t *testing.T) {
	info := UPSInfoFromNUTVars("apc", map[string]string{
		"battery.charge":        "100",
		"battery.runtime":       "1446",
		"battery.runtime.low":   "300",
		"battery.voltage":       "13.7",
		"device.mfr":            "Example Power Systems",
		"device.model":          "Back-UPS BX750MI",
		"device.serial":         "TEST-SERIAL-0001",
		"input.voltage":         "241.0",
		"ups.load":              "21",
		"ups.realpower.nominal": "410",
		"ups.status":            "OL CHRG",
		"ups.test.result":       "Done and passed",
	})

	if info.Name != "apc" {
		t.Fatalf("name = %q", info.Name)
	}
	if info.Manufacturer != "Example Power Systems" {
		t.Fatalf("manufacturer = %q", info.Manufacturer)
	}
	if info.BatteryChargePercent == nil || *info.BatteryChargePercent != 100 {
		t.Fatalf("charge = %v, want 100", info.BatteryChargePercent)
	}
	if info.BatteryRuntimeSeconds == nil || *info.BatteryRuntimeSeconds != 1446 {
		t.Fatalf("runtime = %v, want 1446", info.BatteryRuntimeSeconds)
	}
	if info.OutputPowerWatt == nil || *info.OutputPowerWatt != 86.1 {
		t.Fatalf("output watts = %v, want 86.1", info.OutputPowerWatt)
	}
	if !info.OutputPowerEstimated {
		t.Fatal("derived output watts should be marked as estimated")
	}
	if info.PowerPresent == nil || !*info.PowerPresent {
		t.Fatalf("power_present = %v, want true", info.PowerPresent)
	}
	if info.StatusText != "Online, Charging" {
		t.Fatalf("status_text = %q", info.StatusText)
	}
}

func TestUPSInfoFromNUTVars_OnBattery(t *testing.T) {
	info := UPSInfoFromNUTVars("apc", map[string]string{"ups.status": "OB DISCHRG LB"})
	if info.PowerPresent == nil || *info.PowerPresent {
		t.Fatalf("power_present = %v, want false", info.PowerPresent)
	}
	if info.StatusText != "On battery, Discharging, Low battery" {
		t.Fatalf("status_text = %q", info.StatusText)
	}
}

func TestUPSInfoFromNUTVars_UsesDirectRealpower(t *testing.T) {
	info := UPSInfoFromNUTVars("apc", map[string]string{
		"ups.load":              "21",
		"ups.realpower":         "88",
		"ups.realpower.nominal": "410",
	})
	if info.OutputPowerWatt == nil || *info.OutputPowerWatt != 88 {
		t.Fatalf("output watts = %v, want direct realpower 88", info.OutputPowerWatt)
	}
	if info.OutputPowerEstimated {
		t.Fatal("direct realpower should not be marked as estimated")
	}
}

func TestNUTClientFetchUPS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		line, err := reader.ReadString('\n')
		if err != nil {
			done <- err
			return
		}
		if strings.TrimSpace(line) != "LIST VAR apc" {
			done <- fmt.Errorf("command = %q", line)
			return
		}

		_, err = fmt.Fprint(conn, strings.Join([]string{
			`BEGIN LIST VAR apc`,
			`VAR apc battery.charge "87"`,
			`VAR apc battery.runtime "900"`,
			`VAR apc device.model "Back-UPS BX750MI"`,
			`VAR apc input.voltage "241.0"`,
			`VAR apc ups.load "21"`,
			`VAR apc ups.status "OL"`,
			`END LIST VAR apc`,
			"",
		}, "\n"))
		done <- err
	}()

	client := NewNUTClient(listener.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	info, err := client.FetchUPS(ctx, "apc")
	if err != nil {
		t.Fatalf("FetchUPS: %v", err)
	}
	if info.Model != "Back-UPS BX750MI" {
		t.Fatalf("model = %q", info.Model)
	}
	if info.BatteryChargePercent == nil || *info.BatteryChargePercent != 87 {
		t.Fatalf("charge = %v, want 87", info.BatteryChargePercent)
	}

	if err := <-done; err != nil {
		t.Fatalf("server: %v", err)
	}
}
