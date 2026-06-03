package agent_test

import (
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/hochfrequenz/ai-abap-code-review-service/internal/agent"
)

func TestObjectURI_KnownTypes(t *testing.T) {
	tests := []struct {
		obj  adt.TransportObject
		want string
	}{
		{adt.TransportObject{Type: "PROG", Name: "ZREPORT"}, "/sap/bc/adt/programs/programs/zreport"},
		{adt.TransportObject{Type: "CLAS", Name: "ZCL_EXAMPLE"}, "/sap/bc/adt/oo/classes/zcl_example"},
		{adt.TransportObject{Type: "INTF", Name: "ZIF_EXAMPLE"}, "/sap/bc/adt/oo/interfaces/zif_example"},
		{adt.TransportObject{Type: "FUGR", Name: "ZFUGR"}, "/sap/bc/adt/functions/groups/zfugr"},
		{adt.TransportObject{Type: "TABL", Name: "ZTABLE"}, "/sap/bc/adt/ddic/tables/ztable"},
		{adt.TransportObject{Type: "TABL", Name: "/HFQ/ZTABLE"}, "/sap/bc/adt/ddic/tables//hfq/ztable"},
		{adt.TransportObject{Type: "DDLS", Name: "ZV_EXAMPLE"}, "/sap/bc/adt/ddic/ddl/sources/zv_example"},
		{adt.TransportObject{Type: "DDLX", Name: "ZVX_EXAMPLE"}, "/sap/bc/adt/ddic/ddl/sources/zvx_example"},
		{adt.TransportObject{Type: "DCLS", Name: "ZAC_EXAMPLE"}, "/sap/bc/adt/acm/dcl/sources/zac_example"},
		{adt.TransportObject{Type: "DDLS", Name: "/HFQ/C_BPEM_OBJ"}, "/sap/bc/adt/ddic/ddl/sources//hfq/c_bpem_obj"},
	}
	for _, tt := range tests {
		got := agent.ObjectURI(tt.obj)
		if got != tt.want {
			t.Errorf("ObjectURI(%q) = %q, want %q", tt.obj.Type, got, tt.want)
		}
	}
}

func TestObjectURI_UnknownType_ReturnsEmpty(t *testing.T) {
	got := agent.ObjectURI(adt.TransportObject{Type: "DTEL", Name: "ZDTEL"})
	if got != "" {
		t.Errorf("expected empty for DTEL, got %q", got)
	}
}
