package agent_test

import (
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/agent"
)

func TestObjectURI_KnownTypes(t *testing.T) {
	tests := []struct {
		obj  adt.TransportObject
		want string
	}{
		{adt.TransportObject{Type: "PROG", Name: "ZREPORT"}, "/sap/bc/adt/programs/programs/zreport"},
		{adt.TransportObject{Type: "CLAS", Name: "ZCL_EXAMPLE"}, "/sap/bc/adt/oo/classes/zcl_example"},
		{adt.TransportObject{Type: "INTF", Name: "ZIF_EXAMPLE"}, "/sap/bc/adt/oo/interfaces/zif_example"},
	}
	for _, tt := range tests {
		got := agent.ObjectURI(tt.obj)
		if got != tt.want {
			t.Errorf("ObjectURI(%q) = %q, want %q", tt.obj.Type, got, tt.want)
		}
	}
}

func TestObjectURI_UnknownType_ReturnsEmpty(t *testing.T) {
	got := agent.ObjectURI(adt.TransportObject{Type: "FUGR", Name: "ZFUGR"})
	if got != "" {
		t.Errorf("expected empty for FUGR, got %q", got)
	}
}
