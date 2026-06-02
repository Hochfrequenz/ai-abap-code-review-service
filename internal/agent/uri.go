package agent

import (
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

// ObjectURI maps a TransportObject to its ADT URI path.
// Returns "" for unsupported types (FUGR, DTEL, etc.) — the agent
// prompt instructs Claude to skip objects with empty URIs.
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
