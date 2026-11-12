package driver

import (
	"bytes"
	"testing"
)

func TestINFOnlyBindsTheSerialInterface(t *testing.T) {
	if !bytes.Contains(INF, []byte(`USB\VID_303A&PID_1001&MI_00`)) || bytes.Contains(INF, []byte("MI_02")) {
		t.Fatal("the INF must match interface 0 (CDC) only")
	}
	if !bytes.Contains(INF, []byte("usbser.sys")) || !bytes.Contains(INF, []byte("NTamd64")) || !bytes.Contains(INF, []byte("NTx86")) {
		t.Fatal("the INF must use usbser.sys on x86 and x64")
	}
	if bytes.Count(INF, []byte("\n")) != bytes.Count(INF, []byte("\r\n")) {
		t.Fatal("the INF must use CRLF line endings")
	}
}
