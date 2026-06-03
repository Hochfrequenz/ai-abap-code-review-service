package agent

import (
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

// ObjectURI maps a TransportObject to its ADT URI path.
// Returns "" for unsupported types — the agent prompt instructs Claude to skip those.
//
// All paths were verified via ADT MCP against the live HF S/4HANA system.
// Types without a working /source/main endpoint are excluded; see adtler issues
// for follow-up work on MSAG, BDEF, SRVD, SRVB, ENHO.
//
// Excluded with a reason:
//   - FUGR: adtler has no include-discovery API for function groups
//   - DTEL, DOMA, VIEW, TTYP: no /source/main ADT endpoint (metadata-only in ADT)
//   - BDEF, SRVD, SRVB, ENHO: /source/main not available on this system; correct path unknown
//   - MSAG: needs a dedicated GetMessageClass call, not /source/main
func ObjectURI(obj adt.TransportObject) string {
	// ADT object names in URI paths are case-insensitive on SAP NetWeaver;
	// lowercase is used as the canonical form.
	name := strings.ToLower(obj.Name)
	switch obj.Type {
	// ABAP source objects
	case "PROG":
		return "/sap/bc/adt/programs/programs/" + name
	case "CLAS":
		return "/sap/bc/adt/oo/classes/" + name
	case "INTF":
		return "/sap/bc/adt/oo/interfaces/" + name
	// Function groups — main source lists INCLUDE statements; individual includes are
	// at /sap/bc/adt/programs/includes/<name> and can be fetched via fetch_source.
	case "FUGR":
		return "/sap/bc/adt/functions/groups/" + name
	// DDIC objects — return DDL source (define table / define structure syntax)
	case "TABL":
		return "/sap/bc/adt/ddic/tables/" + name
	// CDS objects — DDL sources endpoint handles both view definitions and extensions
	case "DDLS", "DDLX":
		return "/sap/bc/adt/ddic/ddl/sources/" + name
	// CDS access control
	case "DCLS":
		return "/sap/bc/adt/acm/dcl/sources/" + name
	default:
		return ""
	}
}
