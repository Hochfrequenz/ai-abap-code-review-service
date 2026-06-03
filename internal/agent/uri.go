package agent

import (
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

// ObjectURI maps a TransportObject to its ADT URI path.
// Returns "" for unsupported types (FUGR, DTEL, TABL, etc.) — the agent
// prompt instructs Claude to skip objects with empty URIs.
//
// Supported types (PROG, CLAS, INTF) are the repository object types that
// expose source code via the ADT /source/main endpoint. FUGR is excluded
// because adtler has no include-discovery API for function groups (no way
// to enumerate L/E/F-includes without knowing their names). The type codes
// match the "Type" field returned by adtler's GetTransportObjects.
func ObjectURI(obj adt.TransportObject) string {
	// ADT object names in URI paths are case-insensitive on SAP NetWeaver;
	// lowercase is used as the canonical form. adtler's encodeNamespacePath
	// also lowercases namespace segments, consistent with this choice.
	name := strings.ToLower(obj.Name)
	// obj.Type codes from adtler's GetTransportObjects are always uppercase
	// (PROG, CLAS, INTF, FUGR, …) as returned by the SAP XML response.
	switch obj.Type {
	case "PROG":
		return "/sap/bc/adt/programs/programs/" + name
	case "CLAS":
		return "/sap/bc/adt/oo/classes/" + name
	case "INTF":
		return "/sap/bc/adt/oo/interfaces/" + name
	default:
		return ""
	}
}
