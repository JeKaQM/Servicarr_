package resources

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultNUTPort = "3493"

// NUTClient fetches UPS details from a Network UPS Tools upsd server.
type NUTClient struct {
	Address string
	Dialer  *net.Dialer
	Timeout time.Duration
}

// NewNUTClient creates a NUT client for host[:port]. Port defaults to 3493.
func NewNUTClient(address string) *NUTClient {
	return &NUTClient{
		Address: NormalizeNUTAddress(address),
		Dialer:  &net.Dialer{Timeout: 5 * time.Second},
		Timeout: 5 * time.Second,
	}
}

// NormalizeNUTAddress returns an address suitable for net.Dial.
func NormalizeNUTAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if strings.Contains(address, "://") {
		parsed, err := url.Parse(address)
		if err != nil || parsed.Host == "" {
			return ""
		}
		address = parsed.Host
	}
	address = strings.TrimRight(address, "/")
	if strings.ContainsAny(address, "/?#") {
		return ""
	}

	if _, _, err := net.SplitHostPort(address); err == nil {
		return address
	}
	if strings.HasPrefix(address, "[") && strings.HasSuffix(address, "]") {
		return net.JoinHostPort(strings.Trim(address, "[]"), defaultNUTPort)
	}
	if strings.Count(address, ":") > 1 {
		return net.JoinHostPort(address, defaultNUTPort)
	}
	return net.JoinHostPort(address, defaultNUTPort)
}

// FetchUPS returns a normalized UPS snapshot for the configured NUT server.
func (c *NUTClient) FetchUPS(ctx context.Context, upsName string) (*UPSInfo, error) {
	upsName = strings.TrimSpace(upsName)
	if err := validateUPSName(upsName); err != nil {
		return nil, err
	}
	if c == nil || c.Address == "" {
		return nil, fmt.Errorf("nut address is not configured")
	}

	dialer := c.Dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 5 * time.Second}
	}

	conn, err := dialer.DialContext(ctx, "tcp", c.Address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stopCancel := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
	})
	defer stopCancel()

	deadline := time.Now().Add(c.Timeout)
	if c.Timeout <= 0 {
		deadline = time.Now().Add(5 * time.Second)
	}
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)

	if _, err := fmt.Fprintf(conn, "LIST VAR %s\n", upsName); err != nil {
		return nil, err
	}

	vars, err := readNUTVarList(conn, upsName)
	if err != nil {
		return nil, err
	}
	return UPSInfoFromNUTVars(upsName, vars), nil
}

// FetchUPSNames returns UPS names advertised by the NUT server.
func (c *NUTClient) FetchUPSNames(ctx context.Context) ([]string, error) {
	if c == nil || c.Address == "" {
		return nil, fmt.Errorf("nut address is not configured")
	}

	dialer := c.Dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 5 * time.Second}
	}

	conn, err := dialer.DialContext(ctx, "tcp", c.Address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stopCancel := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
	})
	defer stopCancel()

	deadline := time.Now().Add(c.Timeout)
	if c.Timeout <= 0 {
		deadline = time.Now().Add(5 * time.Second)
	}
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)

	if _, err := fmt.Fprint(conn, "LIST UPS\n"); err != nil {
		return nil, err
	}

	return readNUTUPSList(conn)
}

func validateUPSName(name string) error {
	if name == "" {
		return fmt.Errorf("ups name is required")
	}
	if strings.ContainsAny(name, " \t\r\n") {
		return fmt.Errorf("ups name must not contain whitespace")
	}
	return nil
}

func readNUTVarList(conn net.Conn, upsName string) (map[string]string, error) {
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	begin := "BEGIN LIST VAR " + upsName
	end := "END LIST VAR " + upsName
	seenBegin := false
	vars := map[string]string{}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		switch {
		case strings.HasPrefix(line, "ERR "):
			return nil, nutProtocolError(strings.TrimPrefix(line, "ERR "), upsName)
		case line == begin:
			seenBegin = true
		case line == end:
			return vars, nil
		case seenBegin:
			key, value, ok := parseNUTVarLine(line, upsName)
			if ok {
				vars[key] = value
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("nut incomplete LIST VAR response")
}

func readNUTUPSList(conn net.Conn) ([]string, error) {
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	seenBegin := false
	names := make([]string, 0)

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		switch {
		case strings.HasPrefix(line, "ERR "):
			return nil, nutProtocolError(strings.TrimPrefix(line, "ERR "), "")
		case line == "BEGIN LIST UPS":
			seenBegin = true
		case line == "END LIST UPS":
			return names, nil
		case seenBegin:
			if name, ok := parseNUTUPSLine(line); ok {
				names = append(names, name)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("nut incomplete LIST UPS response")
}

// NUTProtocolError represents an ERR response from upsd.
type NUTProtocolError struct {
	Code    string
	UPSName string
}

func (e *NUTProtocolError) Error() string {
	switch e.Code {
	case "UNKNOWN-UPS":
		return fmt.Sprintf("NUT does not know UPS %q", e.UPSName)
	case "ACCESS-DENIED":
		return fmt.Sprintf("NUT denied access to UPS %q", e.UPSName)
	case "VAR-NOT-SUPPORTED":
		return fmt.Sprintf("NUT does not support variable listing for UPS %q", e.UPSName)
	default:
		return fmt.Sprintf("NUT error %s", strings.ToLower(e.Code))
	}
}

// IsUnknownUPSError reports whether upsd rejected an unknown UPS name.
func IsUnknownUPSError(err error) bool {
	var protocolErr *NUTProtocolError
	return errors.As(err, &protocolErr) && protocolErr.Code == "UNKNOWN-UPS"
}

func nutProtocolError(code, upsName string) error {
	return &NUTProtocolError{
		Code:    strings.ToUpper(strings.TrimSpace(code)),
		UPSName: upsName,
	}
}

func parseNUTVarLine(line, upsName string) (string, string, bool) {
	prefix := "VAR " + upsName + " "
	if !strings.HasPrefix(line, prefix) {
		return "", "", false
	}

	rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	parts := strings.SplitN(rest, " ", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", false
	}

	value := strings.TrimSpace(parts[1])
	if strings.HasPrefix(value, "\"") {
		unquoted, err := strconv.Unquote(value)
		if err == nil {
			value = unquoted
		}
	}
	return parts[0], value, true
}

func parseNUTUPSLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "UPS ") {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "UPS "))
	parts := strings.SplitN(rest, " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", false
	}
	return parts[0], true
}

// UPSInfoFromNUTVars converts raw upsd variables into the API shape.
func UPSInfoFromNUTVars(name string, vars map[string]string) *UPSInfo {
	status := strings.TrimSpace(vars["ups.status"])
	outputPower, outputPowerEstimated := nutOutputPowerWatt(vars)

	return &UPSInfo{
		Name: name,

		Manufacturer: firstNUTValue(vars, "device.mfr", "ups.mfr"),
		Model:        firstNUTValue(vars, "device.model", "ups.model"),
		Serial:       firstNUTValue(vars, "device.serial", "ups.serial"),

		Status:       status,
		StatusText:   nutStatusText(status),
		PowerPresent: nutPowerPresent(status),

		BatteryChargePercent:     nutFloatPtr(vars, "battery.charge"),
		BatteryRuntimeSeconds:    nutFloatPtr(vars, "battery.runtime"),
		BatteryRuntimeLowSeconds: nutFloatPtr(vars, "battery.runtime.low"),
		BatteryVoltage:           nutFloatPtr(vars, "battery.voltage"),
		BatteryVoltageNominal:    nutFloatPtr(vars, "battery.voltage.nominal"),
		BatteryType:              vars["battery.type"],

		LoadPercent:          nutFloatPtr(vars, "ups.load"),
		OutputPowerWatt:      outputPower,
		OutputPowerEstimated: outputPowerEstimated,
		InputVoltage:         nutFloatPtr(vars, "input.voltage"),
		InputVoltageNominal:  nutFloatPtr(vars, "input.voltage.nominal"),
		RealPowerNominalWatt: nutFloatPtr(vars, "ups.realpower.nominal"),

		TestResult: vars["ups.test.result"],
	}
}

func firstNUTValue(vars map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(vars[key]); value != "" {
			return value
		}
	}
	return ""
}

func nutFloatPtr(vars map[string]string, key string) *float64 {
	value := strings.TrimSpace(vars[key])
	if value == "" {
		return nil
	}
	return asFloatPtr(value)
}

func nutOutputPowerWatt(vars map[string]string) (*float64, bool) {
	if watts := nutFloatPtr(vars, "ups.realpower"); watts != nil {
		return watts, false
	}
	load := nutFloatPtr(vars, "ups.load")
	nominal := nutFloatPtr(vars, "ups.realpower.nominal")
	if load == nil || nominal == nil {
		return nil, false
	}
	watts := (*nominal * *load) / 100
	return &watts, true
}

func nutPowerPresent(status string) *bool {
	tokens := strings.Fields(status)
	for _, token := range tokens {
		if token == "OB" {
			v := false
			return &v
		}
	}
	for _, token := range tokens {
		if token == "OL" {
			v := true
			return &v
		}
	}
	return nil
}

func nutStatusText(status string) string {
	if status == "" {
		return ""
	}

	labels := make([]string, 0)
	for _, token := range strings.Fields(status) {
		switch token {
		case "OL":
			labels = append(labels, "Online")
		case "OB":
			labels = append(labels, "On battery")
		case "LB":
			labels = append(labels, "Low battery")
		case "CHRG":
			labels = append(labels, "Charging")
		case "DISCHRG":
			labels = append(labels, "Discharging")
		case "BYPASS":
			labels = append(labels, "Bypass")
		case "CAL":
			labels = append(labels, "Calibration")
		case "OFF":
			labels = append(labels, "Off")
		case "OVER":
			labels = append(labels, "Overload")
		case "RB":
			labels = append(labels, "Replace battery")
		case "TRIM":
			labels = append(labels, "Trimming input")
		case "BOOST":
			labels = append(labels, "Boosting input")
		case "FSD":
			labels = append(labels, "Forced shutdown")
		case "ALARM":
			labels = append(labels, "Alarm")
		case "WAIT":
			labels = append(labels, "Waiting")
		default:
			labels = append(labels, token)
		}
	}

	return strings.Join(labels, ", ")
}
